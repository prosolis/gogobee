package plugin

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

const (
	vibeBufferSize   = 50
	vibeCooldownMins = 5
	vibeMinMessages  = 10
)

type bufferedMessage struct {
	Sender string
	Body   string
	Time   time.Time
}

// VibePlugin uses LLM to describe room energy or summarize conversation.
type VibePlugin struct {
	Base
	mu        sync.Mutex
	buffers   map[id.RoomID][]bufferedMessage
	cooldowns map[id.RoomID]time.Time
	startedAt time.Time
}

// NewVibePlugin creates a new vibe plugin.
func NewVibePlugin(client *mautrix.Client) *VibePlugin {
	return &VibePlugin{
		Base:      NewBase(client),
		buffers:   make(map[id.RoomID][]bufferedMessage),
		cooldowns: make(map[id.RoomID]time.Time),
		startedAt: time.Now().UTC(),
	}
}

func (p *VibePlugin) Name() string { return "vibe" }

func (p *VibePlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "vibe", Description: "LLM describes the current room energy", Usage: "!vibe", Category: "LLM & Sentiment"},
		{Name: "tldr", Description: "LLM summarizes recent conversation", Usage: "!tldr", Category: "LLM & Sentiment"},
	}
}

func (p *VibePlugin) Init() error { return nil }

func (p *VibePlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *VibePlugin) OnMessage(ctx MessageContext) error {
	// Always buffer messages (except commands)
	if !ctx.IsCommand {
		p.addToBuffer(ctx)
	}

	if p.IsCommand(ctx.Body, "vibe") {
		return p.handleVibe(ctx)
	}
	if p.IsCommand(ctx.Body, "tldr") {
		return p.handleTLDR(ctx)
	}

	return nil
}

func (p *VibePlugin) addToBuffer(ctx MessageContext) {
	p.mu.Lock()
	defer p.mu.Unlock()

	buf := p.buffers[ctx.RoomID]
	buf = append(buf, bufferedMessage{
		Sender: string(ctx.Sender),
		Body:   ctx.Body,
		Time:   time.Now().UTC(),
	})

	// Keep only the last N messages
	if len(buf) > vibeBufferSize {
		buf = buf[len(buf)-vibeBufferSize:]
	}

	p.buffers[ctx.RoomID] = buf
}

func (p *VibePlugin) getBuffer(roomID id.RoomID) []bufferedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	buf := p.buffers[roomID]
	// Return a copy
	result := make([]bufferedMessage, len(buf))
	copy(result, buf)
	return result
}

func (p *VibePlugin) checkCooldown(roomID id.RoomID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	last, ok := p.cooldowns[roomID]
	if ok && time.Since(last) < vibeCooldownMins*time.Minute {
		return false
	}
	p.cooldowns[roomID] = time.Now()
	return true
}

func (p *VibePlugin) resetCooldown(roomID id.RoomID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.cooldowns, roomID)
}

func (p *VibePlugin) handleVibe(ctx MessageContext) error {
	ollamaHost := os.Getenv("OLLAMA_HOST")
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaHost == "" || ollamaModel == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "LLM is not configured.")
	}

	buf := p.getBuffer(ctx.RoomID)
	if len(buf) < vibeMinMessages {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Need at least %d messages to read the vibe. Currently have %d. (uptime: %s)", vibeMinMessages, len(buf), p.uptime()))
	}

	if !p.checkCooldown(ctx.RoomID) {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Vibe check is on cooldown. Try again in %d minutes.", vibeCooldownMins))
	}

	botName := os.Getenv("BOT_DISPLAY_NAME")
	if botName == "" {
		botName = "GogoBee"
	}
	transcript := formatTranscript(buf)
	prompt := fmt.Sprintf(
		`You are %s, a fun community bot. Based on the following recent chat messages, describe the current "vibe" or energy of the room in 2-3 sentences. Be creative, playful, and use colorful language. Reference specific topics or dynamics you notice.

Recent messages:
%s

Describe the room's current vibe:`, botName, transcript)

	if err := p.SendReply(ctx.RoomID, ctx.EventID, "Reading the room..."); err != nil {
		slog.Error("vibe: send thinking", "err", err)
	}

	response, err := callOllama(ollamaHost, ollamaModel, prompt)
	if err != nil {
		slog.Error("vibe: ollama call", "err", err)
		p.resetCooldown(ctx.RoomID) // Don't consume cooldown on failure
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to read the vibe. LLM might be offline.")
	}

	return p.SendMessage(ctx.RoomID, response)
}

func (p *VibePlugin) handleTLDR(ctx MessageContext) error {
	ollamaHost := os.Getenv("OLLAMA_HOST")
	ollamaModel := os.Getenv("OLLAMA_MODEL")
	if ollamaHost == "" || ollamaModel == "" {
		return p.SendReply(ctx.RoomID, ctx.EventID, "LLM is not configured.")
	}

	buf := p.getBuffer(ctx.RoomID)
	if len(buf) < vibeMinMessages {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("Need at least %d messages for a summary. Currently have %d. (uptime: %s)", vibeMinMessages, len(buf), p.uptime()))
	}

	if !p.checkCooldown(ctx.RoomID) {
		return p.SendReply(ctx.RoomID, ctx.EventID,
			fmt.Sprintf("TLDR is on cooldown. Try again in %d minutes.", vibeCooldownMins))
	}

	tldrBotName := os.Getenv("BOT_DISPLAY_NAME")
	if tldrBotName == "" {
		tldrBotName = "GogoBee"
	}
	transcript := formatTranscript(buf)
	prompt := fmt.Sprintf(
		`You are %s, a community bot. Summarize the following recent chat conversation in 3-5 concise bullet points. Focus on the main topics discussed, any decisions made, and key moments. Be brief and informative.

Recent messages:
%s

Summary:`, tldrBotName, transcript)

	if err := p.SendReply(ctx.RoomID, ctx.EventID, "Summarizing the conversation..."); err != nil {
		slog.Error("vibe: send thinking", "err", err)
	}

	response, err := callOllama(ollamaHost, ollamaModel, prompt)
	if err != nil {
		slog.Error("vibe: ollama call", "err", err)
		p.resetCooldown(ctx.RoomID) // Don't consume cooldown on failure
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to summarize. LLM might be offline.")
	}

	return p.SendMessage(ctx.RoomID, response)
}

func (p *VibePlugin) uptime() string {
	d := time.Since(p.startedAt)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h >= 24 {
		days := h / 24
		h = h % 24
		return fmt.Sprintf("%dd %dh", days, h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

func formatTranscript(messages []bufferedMessage) string {
	var sb strings.Builder
	for _, m := range messages {
		// Truncate long messages
		body := m.Body
		if len(body) > 300 {
			body = body[:300] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.Time.Format("15:04"), m.Sender, body))
	}
	return sb.String()
}
