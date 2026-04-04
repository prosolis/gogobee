package plugin

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"maunium.net/go/mautrix"
)

// ── Plugin ──────────────────────────────────────────────────────────────────

type LotteryPlugin struct {
	Base
	euro *EuroPlugin
}

func NewLotteryPlugin(client *mautrix.Client, euro *EuroPlugin) *LotteryPlugin {
	return &LotteryPlugin{
		Base: NewBase(client),
		euro: euro,
	}
}

func (p *LotteryPlugin) Name() string { return "lottery" }

func (p *LotteryPlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "lottery", Description: "Community lottery — buy tickets, win big", Usage: "!lottery buy [N]", Category: "Games"},
	}
}

func (p *LotteryPlugin) Init() error {
	go p.drawTicker()
	go p.reminderTicker()
	return nil
}

func (p *LotteryPlugin) OnReaction(_ ReactionContext) error { return nil }

func (p *LotteryPlugin) OnMessage(ctx MessageContext) error {
	if !p.IsCommand(ctx.Body, "lottery") {
		return nil
	}

	args := strings.TrimSpace(p.GetArgs(ctx.Body, "lottery"))
	lower := strings.ToLower(args)

	switch {
	case lower == "" || lower == "help":
		return p.handleLotteryHelp(ctx)
	case lower == "buy" || strings.HasPrefix(lower, "buy "):
		return p.handleLotteryBuy(ctx, strings.TrimSpace(strings.TrimPrefix(lower, "buy")))
	case lower == "tickets":
		return p.handleLotteryTickets(ctx)
	case lower == "pot":
		return p.handleLotteryPot(ctx)
	case lower == "odds":
		return p.handleLotteryOdds(ctx)
	case lower == "history":
		return p.handleLotteryHistory(ctx)
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, "Unknown lottery command. Try `!lottery help`.")
}

// ── Commands ────────────────────────────────────────────────────────────────

func (p *LotteryPlugin) handleLotteryHelp(ctx MessageContext) error {
	text := `🎟️ **Community Lottery**

` + "`!lottery buy [N]`" + ` — Purchase N tickets (default 1, max 100/week). €1 each.
` + "`!lottery tickets`" + ` — View your tickets for this week's draw
` + "`!lottery pot`" + ` — Current pot balance and draw countdown
` + "`!lottery odds`" + ` — Prize tier table and odds
` + "`!lottery history`" + ` — Last 5 draw results

Draw: Every Friday at 23:59 UTC`
	return p.SendReply(ctx.RoomID, ctx.EventID, text)
}

func (p *LotteryPlugin) handleLotteryBuy(ctx MessageContext, args string) error {
	n := 1
	if args != "" {
		parsed, err := strconv.Atoi(args)
		if err != nil || parsed < 1 {
			return p.SendReply(ctx.RoomID, ctx.EventID, "Usage: `!lottery buy [N]` where N is 1-100.")
		}
		n = parsed
	}

	weekStart := lotteryCurrentWeekStart()
	existing := lotteryTicketCount(ctx.Sender, weekStart)
	remaining := 100 - existing

	if remaining <= 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "You've reached the 100 ticket limit for this week's draw. Come back Friday.")
	}

	adjusted := false
	if n > remaining {
		n = remaining
		adjusted = true
	}

	cost := float64(n)
	if !p.euro.Debit(ctx.Sender, cost, "lottery_tickets") {
		balance := p.euro.GetBalance(ctx.Sender)
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("Tickets cost €1 each. You need €%d but have €%.0f.", n, balance))
	}

	// Generate tickets.
	tickets := make([][]int, n)
	for i := range tickets {
		tickets[i] = generateLotteryNumbers()
	}

	lotteryInsertTickets(ctx.Sender, weekStart, tickets)

	// Build confirmation.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎟️ **%d ticket(s) purchased** — €%d\n\n", n, n))

	displayLimit := n
	if displayLimit > 10 {
		displayLimit = 10
	}
	for i := 0; i < displayLimit; i++ {
		sb.WriteString(fmt.Sprintf("  #%d  —  %s\n", existing+i+1, formatLotteryNumbers(tickets[i])))
	}
	if n > 10 {
		sb.WriteString(fmt.Sprintf("  ... and %d more\n", n-10))
	}

	if adjusted {
		sb.WriteString(fmt.Sprintf("\nAdjusted to %d (weekly cap reached).", n))
	}

	sb.WriteString(fmt.Sprintf("\nTotal tickets this week: %d/100", existing+n))
	sb.WriteString(fmt.Sprintf("\nDraw: Friday 23:59 UTC (%s)", lotteryDrawCountdown()))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *LotteryPlugin) handleLotteryTickets(ctx MessageContext) error {
	weekStart := lotteryCurrentWeekStart()
	tickets, err := lotteryLoadUserTickets(ctx.Sender, weekStart)
	if err != nil || len(tickets) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "🎟️ You have no tickets for this week's draw. `!lottery buy` to get started.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎟️ **Your tickets this week** (%d):\n\n", len(tickets)))

	displayLimit := len(tickets)
	if displayLimit > 20 {
		displayLimit = 20
	}
	for i := 0; i < displayLimit; i++ {
		sb.WriteString(fmt.Sprintf("  #%d  —  %s\n", i+1, formatLotteryNumbers(tickets[i].Numbers)))
	}
	if len(tickets) > 20 {
		sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(tickets)-20))
	}

	pot := communityPotBalance()
	sb.WriteString(fmt.Sprintf("\nDraw: Friday 23:59 UTC (%s)  |  Pot: €%d", lotteryDrawCountdown(), pot))

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

func (p *LotteryPlugin) handleLotteryPot(ctx MessageContext) error {
	weekStart := lotteryCurrentWeekStart()
	pot := communityPotBalance()
	totalTickets := lotteryTotalTicketCount(weekStart)
	countdown := lotteryDrawCountdown()

	text := fmt.Sprintf("🎟️ **Lottery Pot**\n\n"+
		"Current pot: **€%d**\n"+
		"Tickets sold this week: %d\n"+
		"Next draw: Friday 23:59 UTC (%s)\n\n"+
		"Pot is funded by rival duel shares and ticket sales.",
		pot, totalTickets, countdown)

	return p.SendReply(ctx.RoomID, ctx.EventID, text)
}

func (p *LotteryPlugin) handleLotteryOdds(ctx MessageContext) error {
	text := `🎟️ **Lottery Prize Tiers**

| Match | Prize | Odds (approx.) |
|-------|-------|----------------|
| 5 of 5 | Jackpot (split among winners) | 1 in 142,506 |
| 4 of 5 | €1,000 (fixed) | 1 in 3,062 |
| 3 of 5 | €100 (fixed) | 1 in 141 |
| 2 of 5 | €10 (fixed) | 1 in 16 |
| 1 of 5 | €2 (fixed) | 1 in 4 |
| 0 of 5 | Nothing | — |

Tickets: €1 each. Max 100 per week. 5 numbers from 1–30.
Minimum €500 pot required for jackpot payout.`

	return p.SendReply(ctx.RoomID, ctx.EventID, text)
}

func (p *LotteryPlugin) handleLotteryHistory(ctx MessageContext) error {
	history, err := lotteryLoadHistory(5)
	if err != nil || len(history) == 0 {
		return p.SendReply(ctx.RoomID, ctx.EventID, "🎟️ No draw history yet.")
	}

	var sb strings.Builder
	sb.WriteString("🎟️ **Recent Draws**\n\n")

	for _, h := range history {
		sb.WriteString(fmt.Sprintf("**%s** — %s\n", h.DrawDate, formatLotteryNumbers(h.WinningNumbers)))
		sb.WriteString(fmt.Sprintf("  Pot: €%d", h.PotTotal))
		if h.JackpotWinners > 0 {
			sb.WriteString(fmt.Sprintf("  |  Jackpot: %d winner(s), €%d each", h.JackpotWinners, h.JackpotAmount))
		}
		if h.RolledOver > 0 {
			sb.WriteString(fmt.Sprintf("  |  Rolled over: €%d", h.RolledOver))
		}
		sb.WriteString(fmt.Sprintf("\n  4-match: %d  |  3-match: %d  |  2-match: %d  |  1-match: %d\n\n",
			h.Match4Winners, h.Match3Winners, h.Match2Winners, h.Match1Winners))
	}

	return p.SendReply(ctx.RoomID, ctx.EventID, sb.String())
}

// ── Number Generation ───────────────────────────────────────────────────────

func generateLotteryNumbers() []int {
	// Partial Fisher-Yates on [1..30], take first 5, sort.
	pool := make([]int, 30)
	for i := range pool {
		pool[i] = i + 1
	}
	for i := 0; i < 5; i++ {
		j := i + rand.IntN(30-i)
		pool[i], pool[j] = pool[j], pool[i]
	}
	nums := make([]int, 5)
	copy(nums, pool[:5])
	sort.Ints(nums)
	return nums
}

func countMatches(ticket, winning []int) int {
	winSet := make(map[int]bool, len(winning))
	for _, n := range winning {
		winSet[n] = true
	}
	count := 0
	for _, n := range ticket {
		if winSet[n] {
			count++
		}
	}
	return count
}

// ── Formatting Helpers ──────────────────────────────────────────────────────

func formatLotteryNumbers(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, " \u00b7 ")
}

func lotteryDrawCountdown() string {
	now := time.Now().UTC()
	// Find next Friday 23:59.
	daysUntilFriday := (int(time.Friday) - int(now.Weekday()) + 7) % 7
	if daysUntilFriday == 0 && (now.Hour() > 23 || (now.Hour() == 23 && now.Minute() >= 59)) {
		daysUntilFriday = 7
	}
	nextDraw := time.Date(now.Year(), now.Month(), now.Day()+daysUntilFriday, 23, 59, 0, 0, time.UTC)
	d := nextDraw.Sub(now)

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}
