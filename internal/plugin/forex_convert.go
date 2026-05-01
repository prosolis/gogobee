package plugin

import (
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
)

// fxParseAmount parses a numeric token, allowing thousands separators
// ("1,500", "1500", "1500.50", "1.5k"). Returns an error on a non-numeric
// token. Negative amounts are rejected.
func fxParseAmount(tok string) (float64, error) {
	t := strings.TrimSpace(tok)
	t = strings.ReplaceAll(t, ",", "")
	t = strings.ReplaceAll(t, "_", "")
	multiplier := 1.0
	if len(t) > 0 {
		switch strings.ToLower(t[len(t)-1:]) {
		case "k":
			multiplier = 1_000
			t = t[:len(t)-1]
		case "m":
			multiplier = 1_000_000
			t = t[:len(t)-1]
		}
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, err
	}
	if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("non-positive amount")
	}
	return v * multiplier, nil
}

// fxParseConvert parses a conversion request from free-form args.
// Accepts: `1500 USD EUR`, `1500 USD to EUR`, `1500 USD/EUR`, `USD/EUR 1500`,
// `1500USD EUR` is NOT supported (keep parsing simple — require whitespace).
// "to" / "into" / "in" are filtered as noise tokens.
func fxParseConvert(args []string) (amount float64, base, quote string, err error) {
	if len(args) == 0 {
		return 0, "", "", fmt.Errorf("no arguments")
	}
	tokens := make([]string, 0, len(args))
	for _, a := range args {
		switch strings.ToLower(a) {
		case "to", "into", "in", "=":
			continue
		}
		tokens = append(tokens, a)
	}

	var amountSet bool
	var currencies []string
	for _, tok := range tokens {
		// Pair form: USD/EUR
		if strings.ContainsAny(tok, "/|-") {
			pair := fxParsePair([]string{tok})
			if pair != nil {
				if base != "" {
					return 0, "", "", fmt.Errorf("multiple currency pairs")
				}
				base, quote = pair.Base, pair.Quote
				continue
			}
		}
		// Number?
		if v, perr := fxParseAmount(tok); perr == nil {
			if amountSet {
				return 0, "", "", fmt.Errorf("multiple amounts")
			}
			amount = v
			amountSet = true
			continue
		}
		// Currency code?
		up := strings.ToUpper(tok)
		if fxIsSupported(up) {
			currencies = append(currencies, up)
			continue
		}
		return 0, "", "", fmt.Errorf("unrecognized token: %s", tok)
	}

	if !amountSet {
		return 0, "", "", fmt.Errorf("missing amount")
	}
	if base == "" {
		if len(currencies) != 2 {
			return 0, "", "", fmt.Errorf("expected two currencies, got %d", len(currencies))
		}
		base, quote = currencies[0], currencies[1]
	} else if len(currencies) != 0 {
		return 0, "", "", fmt.Errorf("currency pair plus extra currency")
	}
	if base == quote {
		return 0, "", "", fmt.Errorf("base and quote are the same")
	}
	return amount, base, quote, nil
}

func (p *ForexPlugin) cmdConvert(ctx MessageContext, args []string) error {
	amount, base, quote, err := fxParseConvert(args)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			"Usage: `!fx 1500 USD to EUR` (or `!fx convert 1500 USD EUR`, `!fx convert USD/EUR 1500`)")
	}

	pair := &fxPair{Base: base, Quote: quote}
	rate, err := p.fxLivePairRate(pair)
	if err != nil {
		slog.Error("forex: fetch rates failed", "err", err)
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not fetch rates. Try again shortly.")
	}

	converted := amount * rate
	baseEmoji := fxEmoji(base)
	quoteEmoji := fxEmoji(quote)

	msg := fmt.Sprintf("%s **%s %s** = %s **%s %s**\n_1 %s = %s %s_",
		baseEmoji, fxFormatAmount(base, amount), base,
		quoteEmoji, fxFormatAmount(quote, converted), quote,
		base, fxFormatRate(quote, rate), quote)
	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

// fxEmoji returns the flag emoji for a supported currency, falling back
// to USD's flag.
func fxEmoji(cur string) string {
	if m, ok := fxMeta[cur]; ok && m.Emoji != "" {
		return m.Emoji
	}
	if cur == "USD" {
		return "🇺🇸"
	}
	return ""
}

// fxFormatAmount formats a converted amount with thousands separators.
// JPY uses 0 decimal places (yen are not subdivided in practice for display),
// other currencies use 2.
func fxFormatAmount(currency string, amount float64) string {
	decimals := 2
	if currency == "JPY" {
		decimals = 0
	}
	return fxAddCommas(amount, decimals)
}

// fxAddCommas formats a float with N decimal places and thousands separators.
func fxAddCommas(v float64, decimals int) string {
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	intPart := s
	fracPart := ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart = s[:i]
		fracPart = s[i:]
	}
	neg := false
	if strings.HasPrefix(intPart, "-") {
		neg = true
		intPart = intPart[1:]
	}
	n := len(intPart)
	if n <= 3 {
		if neg {
			return "-" + intPart + fracPart
		}
		return intPart + fracPart
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	pre := n % 3
	if pre > 0 {
		b.WriteString(intPart[:pre])
		if n > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < n; i += 3 {
		b.WriteString(intPart[i : i+3])
		if i+3 < n {
			b.WriteByte(',')
		}
	}
	b.WriteString(fracPart)
	return b.String()
}
