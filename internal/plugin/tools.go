package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/skip2/go-qrcode"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// ToolsPlugin provides calculator and QR code generation.
type ToolsPlugin struct {
	Base
}

// NewToolsPlugin creates a new ToolsPlugin.
func NewToolsPlugin(client *mautrix.Client) *ToolsPlugin {
	return &ToolsPlugin{
		Base: NewBase(client),
	}
}

func (p *ToolsPlugin) Name() string { return "tools" }

func (p *ToolsPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "calc", Description: "Math calculator", Usage: "!calc <expression>", Category: "Lookup & Reference"},
		{Name: "qr", Description: "Generate a QR code", Usage: "!qr <text>", Category: "Lookup & Reference"},
	}
}

func (p *ToolsPlugin) Init() error { return nil }

func (p *ToolsPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *ToolsPlugin) OnMessage(ctx MessageContext) error {
	switch {
	case p.IsCommand(ctx.Body, "calc"):
		return p.handleCalc(ctx)
	case p.IsCommand(ctx.Body, "qr"):
		return p.handleQR(ctx)
	}
	return nil
}

func (p *ToolsPlugin) handleCalc(ctx MessageContext) error {
	expression := strings.TrimSpace(p.GetArgs(ctx.Body, "calc"))
	if expression == "" {
		return p.SendMessage(ctx.RoomID, "Usage: !calc <expression>\nExamples: !calc 2+2, !calc sqrt(144), !calc 5 times 3")
	}

	// Normalize natural language
	normalized := normalizeExpression(expression)

	// Evaluate using expr
	program, err := expr.Compile(normalized)
	if err != nil {
		slog.Debug("calc: compile error", "expr", normalized, "err", err)
		return p.SendMessage(ctx.RoomID, fmt.Sprintf("Could not parse expression: %s", expression))
	}

	output, err := expr.Run(program, nil)
	if err != nil {
		slog.Debug("calc: eval error", "expr", normalized, "err", err)
		return p.SendMessage(ctx.RoomID, fmt.Sprintf("Error evaluating: %s", expression))
	}

	return p.SendMessage(ctx.RoomID, fmt.Sprintf("🧮 %s = **%s**", expression, formatCalcResult(output)))
}

// commaNumberRe matches numbers with commas like 40,000 or 1,234,567.89
var commaNumberRe = regexp.MustCompile(`\d{1,3}(,\d{3})+(\.\d+)?`)

// percentOfRe matches patterns like "8% of 40000" or "15.5% of 200"
var percentOfRe = regexp.MustCompile(`(?i)([\d.]+)\s*%\s*of\s+([\d,.]+)`)

// normalizeExpression converts natural language math to operators.
func normalizeExpression(s string) string {
	// Remove common prefixes
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	for _, prefix := range []string{"what is ", "what's ", "calculate ", "compute ", "evaluate "} {
		if strings.HasPrefix(lower, prefix) {
			s = s[len(prefix):]
			lower = strings.ToLower(s)
			break
		}
	}

	// Strip commas from numbers (e.g. 40,000 -> 40000)
	s = commaNumberRe.ReplaceAllStringFunc(s, func(m string) string {
		return strings.ReplaceAll(m, ",", "")
	})

	// Handle "X% of Y" -> (X / 100) * Y
	s = percentOfRe.ReplaceAllString(s, "($1 / 100) * $2")

	// Replace natural language operators (order matters: longer phrases first)
	replacements := []struct{ from, to string }{
		{"divided by", "/"},
		{"to the power of", "**"},
		{"raised to", "**"},
		{"times", "*"},
		{"multiplied by", "*"},
		{"plus", "+"},
		{"minus", "-"},
		{"mod", "%"},
	}

	result := s
	for _, r := range replacements {
		result = strings.ReplaceAll(strings.ToLower(result), r.from, r.to)
	}

	return strings.TrimSpace(result)
}

// formatCalcResult formats a calculation result, adding commas for large numbers.
func formatCalcResult(v interface{}) string {
	switch n := v.(type) {
	case int:
		return formatNumber(n)
	case int64:
		return formatNumberInt64(n)
	case float64:
		if n == math.Trunc(n) && !math.IsInf(n, 0) && !math.IsNaN(n) && math.Abs(n) < 1e15 {
			// Whole number - format without decimals
			return formatNumberInt64(int64(n))
		}
		// Format with decimals, then add commas to the integer part
		formatted := fmt.Sprintf("%g", n)
		parts := strings.SplitN(formatted, ".", 2)
		// Try to parse the integer part for comma formatting
		if len(parts[0]) > 3 {
			negative := strings.HasPrefix(parts[0], "-")
			digits := parts[0]
			if negative {
				digits = digits[1:]
			}
			var result strings.Builder
			remainder := len(digits) % 3
			if remainder > 0 {
				result.WriteString(digits[:remainder])
			}
			for i := remainder; i < len(digits); i += 3 {
				if result.Len() > 0 {
					result.WriteByte(',')
				}
				result.WriteString(digits[i : i+3])
			}
			prefix := ""
			if negative {
				prefix = "-"
			}
			if len(parts) == 2 {
				return prefix + result.String() + "." + parts[1]
			}
			return prefix + result.String()
		}
		return formatted
	default:
		return fmt.Sprintf("%v", v)
	}
}

// formatNumberInt64 adds commas to an int64 for display.
func formatNumberInt64(n int64) string {
	if n < 0 {
		return "-" + formatNumberInt64(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if result.Len() > 0 {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}

func (p *ToolsPlugin) handleQR(ctx MessageContext) error {
	text := strings.TrimSpace(p.GetArgs(ctx.Body, "qr"))
	if text == "" {
		return p.SendMessage(ctx.RoomID, "Usage: !qr <text or URL>")
	}

	if len(text) > 2048 {
		return p.SendMessage(ctx.RoomID, "Text is too long for a QR code (max 2048 characters).")
	}

	// Generate QR code PNG
	pngData, err := qrcode.Encode(text, qrcode.Medium, 256)
	if err != nil {
		slog.Error("qr: generate failed", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to generate QR code.")
	}

	// Upload to Matrix
	mxcURI, err := p.UploadContent(pngData, "image/png", "qrcode.png")
	if err != nil {
		slog.Error("qr: upload failed", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to upload QR code image.")
	}

	// Send as image message
	content := &event.MessageEventContent{
		MsgType: event.MsgImage,
		Body:    "qrcode.png",
		URL:     id.ContentURIString(mxcURI.String()),
		Info: &event.FileInfo{
			MimeType: "image/png",
			Size:     len(pngData),
		},
	}

	_, err = p.Client.SendMessageEvent(context.Background(), ctx.RoomID, event.EventMessage, content)
	if err != nil {
		slog.Error("qr: send image failed", "err", err)
		return p.SendMessage(ctx.RoomID, "Failed to send QR code image.")
	}

	return nil
}
