package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/version"

	"maunium.net/go/mautrix"
)

var (
	botStartTime  = time.Now().UTC()
	botMsgCounter atomic.Int64
)

// IncrementMessageCount increments the global session message counter.
func IncrementMessageCount() {
	botMsgCounter.Add(1)
}

// BotInfoPlugin provides admin-only bot diagnostics.
type BotInfoPlugin struct {
	Base
}

// NewBotInfoPlugin creates a new botinfo plugin.
func NewBotInfoPlugin(client *mautrix.Client) *BotInfoPlugin {
	return &BotInfoPlugin{
		Base: NewBase(client),
	}
}

func (p *BotInfoPlugin) Name() string { return "botinfo" }

func (p *BotInfoPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "botinfo", Description: "Show bot diagnostics (admin only)", Usage: "!botinfo", Category: "Admin", AdminOnly: true},
		{Name: "version", Description: "Show bot version", Usage: "!version", Category: "Info"},
	}
}

func (p *BotInfoPlugin) Init() error { return nil }

func (p *BotInfoPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *BotInfoPlugin) OnMessage(ctx MessageContext) error {
	if p.IsCommand(ctx.Body, "version") {
		return p.handleVersion(ctx)
	}
	if !p.IsCommand(ctx.Body, "botinfo") {
		IncrementMessageCount()
		return nil
	}

	if !p.IsAdmin(ctx.Sender) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "This command is admin-only.")
	}

	return p.handleBotInfo(ctx)
}

func (p *BotInfoPlugin) handleVersion(ctx MessageContext) error {
	uptime := time.Since(botStartTime)
	days := int(uptime.Hours() / 24)
	hours := int(uptime.Hours()) % 24

	msg := fmt.Sprintf("**%s**\nUptime: %dd %dh", version.Full(), days, hours)

	crashes := db.CrashCount(version.Short())
	if crashes > 0 {
		msg += fmt.Sprintf(" · %d crash(es) this version", crashes)
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, msg)
}

func (p *BotInfoPlugin) handleBotInfo(ctx MessageContext) error {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Bot Diagnostics** — %s\n\n", version.Short()))

	// Uptime
	uptime := time.Since(botStartTime)
	days := int(uptime.Hours() / 24)
	hours := int(uptime.Hours()) % 24
	minutes := int(uptime.Minutes()) % 60
	sb.WriteString(fmt.Sprintf("Uptime: %dd %dh %dm\n", days, hours, minutes))

	// Session message count
	sessionMsgs := botMsgCounter.Load()
	sb.WriteString(fmt.Sprintf("Messages this session: %d\n", sessionMsgs))

	// Total room messages from DB
	d := db.Get()
	var totalMsgs int
	err := d.QueryRow(`SELECT COALESCE(SUM(total_messages), 0) FROM user_stats`).Scan(&totalMsgs)
	if err != nil {
		slog.Error("botinfo: total messages", "err", err)
	}
	sb.WriteString(fmt.Sprintf("Total room messages (DB): %s\n", formatNumber(totalMsgs)))

	// DB size
	var pageCount, pageSize int
	_ = d.QueryRow(`PRAGMA page_count`).Scan(&pageCount)
	_ = d.QueryRow(`PRAGMA page_size`).Scan(&pageSize)
	dbSizeBytes := int64(pageCount) * int64(pageSize)
	dbSizeMB := float64(dbSizeBytes) / (1024 * 1024)
	sb.WriteString(fmt.Sprintf("Database size: %.2f MB\n", dbSizeMB))

	// Active reminders
	var activeReminders int
	_ = d.QueryRow(`SELECT COUNT(*) FROM reminders WHERE fired = 0`).Scan(&activeReminders)
	sb.WriteString(fmt.Sprintf("Active reminders: %d\n", activeReminders))

	// LLM status
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if ollamaHost != "" {
		llmStatus := p.checkLLMStatus(ollamaHost)
		sb.WriteString(fmt.Sprintf("LLM status: %s\n", llmStatus))
	} else {
		sb.WriteString("LLM status: not configured\n")
	}

	// Crash stats
	crashesThisVersion := db.CrashCount(version.Short())
	crashesTotal := db.CrashCountAll()
	sb.WriteString(fmt.Sprintf("\nCrashes: %d this version / %d total\n", crashesThisVersion, crashesTotal))

	// Recent crashes (last 5)
	d2 := db.Get()
	rows, err := d2.Query(`SELECT plugin, message, created_at FROM crash_log ORDER BY id DESC LIMIT 5`)
	if err == nil {
		defer rows.Close()
		var hasCrashes bool
		for rows.Next() {
			var plug, msg, ts string
			if rows.Scan(&plug, &msg, &ts) != nil {
				continue
			}
			if !hasCrashes {
				sb.WriteString("Recent crashes:\n")
				hasCrashes = true
			}
			if len(msg) > 80 {
				msg = msg[:80] + "..."
			}
			sb.WriteString(fmt.Sprintf("  %s [%s] %s\n", ts, plug, msg))
		}
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *BotInfoPlugin) checkLLMStatus(ollamaHost string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	apiURL := strings.TrimRight(ollamaHost, "/") + "/api/tags"

	resp, err := client.Get(apiURL)
	if err != nil {
		return fmt.Sprintf("offline (%s)", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Sprintf("error (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "online (could not read response)"
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "online (could not parse response)"
	}

	modelNames := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		modelNames = append(modelNames, m.Name)
	}

	if len(modelNames) == 0 {
		return "online (no models loaded)"
	}

	return fmt.Sprintf("online (%d models: %s)", len(modelNames), strings.Join(modelNames, ", "))
}
