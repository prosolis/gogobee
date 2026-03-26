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
		{Name: "fx", Description: "Forex rates and analysis", Usage: "!fx rate [EUR|JPY] · !fx report [EUR|JPY] · !fx setalert <cur> <rate> · !fx alerts · !fx delalert <cur> <rate>", Category: "Entertainment"},
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
			"Usage: `!fx rate [EUR|JPY]` · `!fx report [EUR|JPY]` · `!fx setalert <currency> <rate>` · `!fx alerts` · `!fx delalert <currency> <rate>`")
	}

	parts := strings.Fields(args)
	sub := strings.ToLower(parts[0])

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
		go func() {
			var err error
			if sub == "rate" {
				err = p.cmdRate(ctx, parts[1:])
			} else {
				err = p.cmdReport(ctx, parts[1:])
			}
			if err != nil {
				slog.Error("forex: handler error", "err", err)
			}
		}()
		return nil
	default:
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Unknown subcommand `%s`. Try `!fx help`.", parts[0]))
	}
}

const fxHelpText = "**Forex Commands**\n\n" +
	"`!fx rate [EUR|JPY]` — current rate + quick signal\n" +
	"`!fx report [EUR|JPY]` — full analysis (averages, 52w range, buy score)\n" +
	"`!fx setalert <currency> <rate>` — alert when USD/currency reaches threshold\n" +
	"`!fx alerts` — list active alerts in this room\n" +
	"`!fx delalert <currency> <rate>` — remove an alert\n" +
	"`!fx help` — this message"

// ── Command Handlers ────────────────────────────────────────────────────────

func (p *ForexPlugin) cmdRate(ctx MessageContext, args []string) error {
	currencies := fxParseCurrencies(args)
	rates, err := p.fxLiveRates(currencies)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Could not fetch rates: %v", err))
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

func (p *ForexPlugin) cmdReport(ctx MessageContext, args []string) error {
	currencies := fxParseCurrencies(args)
	rates, err := p.fxLiveRates(currencies)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Could not fetch rates: %v", err))
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
