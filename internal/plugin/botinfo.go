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
	}
}

func (p *BotInfoPlugin) Init() error { return nil }

func (p *BotInfoPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *BotInfoPlugin) OnMessage(ctx MessageContext) error {
	if !p.IsCommand(ctx.Body, "botinfo") {
		// Passively count messages
		IncrementMessageCount()
		return nil
	}

	if !p.IsAdmin(ctx.Sender) {
		return p.SendReply(ctx.RoomID, ctx.EventID, "This command is admin-only.")
	}

	return p.handleBotInfo(ctx)
}

func (p *BotInfoPlugin) handleBotInfo(ctx MessageContext) error {
	var sb strings.Builder
	sb.WriteString("Bot Diagnostics\n\n")

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
