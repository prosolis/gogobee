package plugin

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// ForexPlugin tracks USD exchange rates against EUR and JPY using the
// Frankfurter API (ECB-sourced, no API key required). Stores daily history,
// computes buy signals, and provides rate alerts.
type ForexPlugin struct {
	Base
	httpClient *http.Client
}

func NewForexPlugin(client *mautrix.Client) *ForexPlugin {
	return &ForexPlugin{
		Base: NewBase(client),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *ForexPlugin) Name() string { return "forex" }

func (p *ForexPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "fx", Description: "Forex rates, conversion, and analysis", Usage: "!fx rate [EUR|JPY] · !fx 1500 USD to EUR · !fx report [EUR|JPY] · !fx setalert <cur> <rate> · !fx alerts · !fx delalert <cur> <rate>", Category: "Entertainment"},
	}
}

func (p *ForexPlugin) Init() error {
	go p.backfill()
	return nil
}

func (p *ForexPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *ForexPlugin) OnMessage(ctx MessageContext) error {
	if !p.IsCommand(ctx.Body, "fx") {
		return nil
	}

	args := strings.TrimSpace(p.GetArgs(ctx.Body, "fx"))
	if args == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: `!fx rate [EUR|JPY]` · `!fx 1500 USD to EUR` · `!fx report [EUR|JPY]` · `!fx setalert <currency> <rate>` · `!fx alerts` · `!fx delalert <currency> <rate>`")
	}

	parts := strings.Fields(args)
	sub := strings.ToLower(parts[0])

	// Auto-detect conversion: if the first token parses as a number,
	// treat the whole arg list as a convert request (e.g. `!fx 1500 USD EUR`).
	if _, err := fxParseAmount(parts[0]); err == nil {
		safeGo("forex-handler", func() {
			if err := p.cmdConvert(ctx, parts); err != nil {
				slog.Error("forex: handler error", "err", err)
			}
		})
		return nil
	}

	// DB-only and instant subcommands
	switch sub {
	case "setalert":
		return p.cmdSetAlert(ctx, parts[1:])
	case "alerts":
		return p.cmdListAlerts(ctx)
	case "delalert":
		return p.cmdDelAlert(ctx, parts[1:])
	case "help":
		return p.SendReply(ctx.RoomID, ctx.EventID, fxHelpText)
	}

	// API-calling subcommands run async
	switch sub {
	case "rate", "report":
		safeGo("forex-handler", func() {
			var err error
			if sub == "rate" {
				err = p.cmdRate(ctx, parts[1:])
			} else {
				err = p.cmdReport(ctx, parts[1:])
			}
			if err != nil {
				slog.Error("forex: handler error", "err", err)
			}
		})
		return nil
	case "convert":
		safeGo("forex-handler", func() {
			if err := p.cmdConvert(ctx, parts[1:]); err != nil {
				slog.Error("forex: handler error", "err", err)
			}
		})
		return nil
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Unknown subcommand `%s`. Try `!fx help`.", parts[0]))
	}
}

const fxHelpText = "**Forex Commands**\n\n" +
	"`!fx rate [EUR|JPY|CAD]` — current rate + quick signal\n" +
	"`!fx rate EUR/USD` — cross-pair rate from first currency's perspective\n" +
	"`!fx report [EUR|JPY|CAD]` — full analysis (averages, 52w range, buy score)\n" +
	"`!fx 1500 USD to EUR` — convert an amount (also: `!fx convert 1500 USD EUR`)\n" +
	"`!fx 100 USD to BTC` — convert to/from crypto (BTC, ETH, SOL, XRP, DOGE, ADA, LTC)\n" +
	"`!fx setalert <currency> <rate>` — alert when USD/currency reaches threshold\n" +
	"`!fx alerts` — list active alerts in this room\n" +
	"`!fx delalert <currency> <rate>` — remove an alert\n" +
	"`!fx help` — this message"

// ── Command Handlers ────────────────────────────────────────────────────────

func (p *ForexPlugin) cmdRate(ctx MessageContext, args []string) error {
	// Check for cross-pair syntax like EUR/USD or USD/JPY
	if pair := fxParsePair(args, false); pair != nil {
		return p.cmdPairRateOrReport(ctx, pair, false)
	}

	currencies := fxParseCurrencies(args)
	rates, err := p.fxLiveRates(currencies)
	if err != nil {
		slog.Error("forex: fetch rates failed", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not fetch rates. Try again shortly.")
	}

	var lines []string
	for _, cur := range currencies {
		rate, ok := rates[cur]
		if !ok {
			continue
		}
		sig, err := p.fxComputeSignal(cur, rate)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s %s: %s (insufficient data for analysis)", fxMeta[cur].Emoji, cur, fxFormatRate(cur, rate)))
			continue
		}
		lines = append(lines, sig.FormatQuick())
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, strings.Join(lines, "\n"))
}

// fxPair represents a currency cross-pair like EUR/USD.
type fxPair struct {
	Base  string // first currency (perspective)
	Quote string // second currency
}

// fxParsePair detects "XXX/YYY" syntax in args. Both sides must be USD or a
// tracked currency; when allowCrypto is set, supported crypto assets are
// accepted too (used by the conversion path, not by analysis). Returns nil if
// no pair is found.
func fxParsePair(args []string, allowCrypto bool) *fxPair {
	if len(args) == 0 {
		return nil
	}
	raw := strings.ToUpper(args[0])
	// Accept common pair separators: EUR/USD, EUR|USD, EUR-USD
	var parts []string
	for _, sep := range []string{"/", "|", "-"} {
		if strings.Contains(raw, sep) {
			parts = strings.SplitN(raw, sep, 2)
			break
		}
	}
	if len(parts) != 2 {
		return nil
	}
	base, quote := parts[0], parts[1]
	ok := fxIsSupported
	if allowCrypto {
		ok = fxIsConvertible
	}
	if !ok(base) || !ok(quote) {
		return nil
	}
	if base == quote {
		return nil
	}
	return &fxPair{Base: base, Quote: quote}
}

// fxIsSupported returns true if the currency is USD or a tracked currency.
func fxIsSupported(cur string) bool {
	return cur == "USD" || fxIsTracked(cur)
}

// cmdPairRateOrReport handles cross-pair display for both rate and report.
func (p *ForexPlugin) cmdPairRateOrReport(ctx MessageContext, pair *fxPair, full bool) error {
	currentRate, err := p.fxLivePairRate(pair)
	if err != nil {
		slog.Error("forex: fetch rates failed", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not fetch rates. Try again shortly.")
	}

	sig, err := fxComputePairSignal(pair, currentRate)
	if err != nil {
		// Fall back to just the rate if insufficient history
		meta := fxMeta[pair.Base]
		emoji := meta.Emoji
		if emoji == "" {
			emoji = "🇺🇸"
		}
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("%s **%s/%s** 1 %s = **%s** %s (insufficient data for analysis)",
				emoji, pair.Base, pair.Quote, pair.Base, fxFormatRate(pair.Quote, currentRate), pair.Quote))
	}

	if full {
		return p.SendReply(ctx.RoomID, ctx.EventID, sig.FormatReport())
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, sig.FormatQuick())
}

// fxLivePairRate computes the live cross-pair rate (how many Quote per 1 Base).
// Each side is resolved to a "units per USD" figure, then crossed; this handles
// USD, fiat (Frankfurter), and crypto (CoinGecko) sides uniformly.
func (p *ForexPlugin) fxLivePairRate(pair *fxPair) (float64, error) {
	// Batch the fiat sides into a single Frankfurter call.
	var fiat []string
	for _, c := range []string{pair.Base, pair.Quote} {
		if c != "USD" && !fxIsCrypto(c) {
			fiat = append(fiat, c)
		}
	}
	var fiatRates map[string]float64
	if len(fiat) > 0 {
		var err error
		if fiatRates, err = p.fxLiveRates(fiat); err != nil {
			return 0, err
		}
	}

	basePer, err := p.fxPerUSD(pair.Base, fiatRates)
	if err != nil {
		return 0, err
	}
	quotePer, err := p.fxPerUSD(pair.Quote, fiatRates)
	if err != nil {
		return 0, err
	}
	if basePer == 0 {
		return 0, fmt.Errorf("missing rate data for cross-pair")
	}
	return quotePer / basePer, nil
}

// fxPerUSD returns how many units of cur equal 1 USD. Fiat rates are taken from
// the pre-fetched fiatRates map; crypto is fetched live from CoinGecko.
func (p *ForexPlugin) fxPerUSD(cur string, fiatRates map[string]float64) (float64, error) {
	switch {
	case cur == "USD":
		return 1.0, nil
	case fxIsCrypto(cur):
		price, err := p.cgSpotUSD(cur)
		if err != nil {
			return 0, err
		}
		return 1.0 / price, nil
	default:
		r, ok := fiatRates[cur]
		if !ok || r == 0 {
			return 0, fmt.Errorf("no rate data for %s", cur)
		}
		return r, nil
	}
}

func (p *ForexPlugin) cmdReport(ctx MessageContext, args []string) error {
	if pair := fxParsePair(args, false); pair != nil {
		return p.cmdPairRateOrReport(ctx, pair, true)
	}
	currencies := fxParseCurrencies(args)
	rates, err := p.fxLiveRates(currencies)
	if err != nil {
		slog.Error("forex: fetch rates failed", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not fetch rates. Try again shortly.")
	}

	var sections []string
	for _, cur := range currencies {
		rate, ok := rates[cur]
		if !ok {
			continue
		}
		sig, err := p.fxComputeSignal(cur, rate)
		if err != nil {
			sections = append(sections, fmt.Sprintf("%s **%s** — %s\n_Insufficient history for full analysis._", fxMeta[cur].Emoji, cur, fxFormatRate(cur, rate)))
			continue
		}
		sections = append(sections, sig.FormatReport())
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, strings.Join(sections, "\n\n---\n\n"))
}

func (p *ForexPlugin) cmdSetAlert(ctx MessageContext, args []string) error {
	if len(args) < 2 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!fx setalert <currency> <rate>` — e.g. `!fx setalert JPY 155`")
	}
	cur := strings.ToUpper(args[0])
	if !fxIsTracked(cur) {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Unknown currency `%s`. Tracked: %s", cur, strings.Join(fxTrackedCurrencies, ", ")))
	}
	threshold, err := strconv.ParseFloat(args[1], 64)
	if err != nil || threshold <= 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Invalid rate `%s`.", args[1]))
	}
	if err := fxSaveAlert(string(ctx.Sender), cur, threshold); err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Could not save alert: %v", err))
	}
	meta := fxMeta[cur]
	return p.SendReply(ctx.RoomID, ctx.EventID,
		fmt.Sprintf("Alert set: I'll notify when 1 USD >= **%s %s** %s", fxFormatRate(cur, threshold), cur, meta.Emoji))
}

func (p *ForexPlugin) cmdListAlerts(ctx MessageContext) error {
	alerts, err := fxAlertsForUser(string(ctx.Sender))
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Error: %v", err))
	}
	if len(alerts) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No active alerts. Set one with `!fx setalert <currency> <rate>`.")
	}
	var sb strings.Builder
	sb.WriteString("**Your FX alerts:**\n")
	for _, a := range alerts {
		meta := fxMeta[a.Currency]
		status := ""
		if a.FiredAt != 0 {
			status = fmt.Sprintf(" _(fired %s)_", time.Unix(a.FiredAt, 0).UTC().Format("Jan 2 15:04 UTC"))
		}
		sb.WriteString(fmt.Sprintf("  %s `%s %s`%s\n",
			meta.Emoji, fxFormatRate(a.Currency, a.Threshold), a.Currency, status))
	}
	sb.WriteString("\nAlerts are delivered via DM.")
	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *ForexPlugin) cmdDelAlert(ctx MessageContext, args []string) error {
	if len(args) < 2 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!fx delalert <currency> <rate>` — e.g. `!fx delalert JPY 155`")
	}
	cur := strings.ToUpper(args[0])
	threshold, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Invalid rate `%s`.", args[1]))
	}
	if err := fxDeleteAlert(string(ctx.Sender), cur, threshold); err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Error: %v", err))
	}
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Alert for `%s %s` removed.", fxFormatRate(cur, threshold), cur))
}

// ── Daily Poll (called by cron) ─────────────────────────────────────────────

func (p *ForexPlugin) DailyPoll() {
	rates, err := p.fxFetchCurrent(fxTrackedCurrencies)
	if err != nil {
		slog.Error("forex: daily poll fetch failed", "err", err)
		return
	}

	today := time.Now().UTC().Format("2006-01-02")
	for cur, rate := range rates {
		fxSaveRate(cur, today, rate)
	}

	fxResetExpiredAlerts()
	p.checkAlerts(rates)
}

func (p *ForexPlugin) checkAlerts(rates map[string]float64) {
	alerts, err := fxAllAlerts()
	if err != nil {
		slog.Error("forex: alert check error", "err", err)
		return
	}
	for _, a := range alerts {
		if a.FiredAt != 0 {
			continue
		}
		rate, ok := rates[a.Currency]
		if !ok || rate < a.Threshold {
			continue
		}
		meta := fxMeta[a.Currency]
		msg := fmt.Sprintf("**FX Alert** %s — 1 USD = **%s %s** has reached your threshold of **%s**.",
			meta.Emoji, fxFormatRate(a.Currency, rate), a.Currency,
			fxFormatRate(a.Currency, a.Threshold))
		p.SendDM(id.UserID(a.UserID), msg)
		fxMarkAlertFired(a)
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func fxParseCurrencies(args []string) []string {
	var out []string
	for _, a := range args {
		c := strings.ToUpper(a)
		if fxIsTracked(c) {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return fxTrackedCurrencies
	}
	return out
}
