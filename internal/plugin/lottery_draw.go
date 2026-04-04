package plugin

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// ── Prize Tiers (fixed payouts) ─────────────────────────────────────────────

var lotteryFixedPrizes = map[int]int{
	4: 1000,
	3: 100,
	2: 10,
	1: 2,
}

// ── Draw Ticker ─────────────────────────────────────────────────────────────

func (p *LotteryPlugin) drawTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		if now.Weekday() != time.Friday || now.Hour() != 23 || now.Minute() != 59 {
			continue
		}

		weekKey := lotteryCurrentWeekStart()
		jobName := "lottery_draw"
		if db.JobCompleted(jobName, weekKey) {
			continue
		}

		slog.Info("lottery: executing weekly draw")
		p.executeDraw(weekKey)
		db.MarkJobCompleted(jobName, weekKey)
	}
}

// ── Reminder Ticker ─────────────────────────────────────────────────────────

func (p *LotteryPlugin) reminderTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		if now.Weekday() != time.Thursday || now.Hour() != 20 || now.Minute() != 0 {
			continue
		}

		weekKey := lotteryCurrentWeekStart()
		jobName := "lottery_reminder"
		if db.JobCompleted(jobName, weekKey) {
			continue
		}

		gr := gamesRoom()
		if gr != "" {
			pot := communityPotBalance()
			p.SendMessage(gr, fmt.Sprintf(
				"🎟️ Lottery draw tomorrow. 23:59 UTC. Current pot: €%d. "+
					"Tickets €1 each. Max 100 per player. `!lottery buy [N]` to enter.",
				pot))
		}

		db.MarkJobCompleted(jobName, weekKey)
	}
}

// ── Draw Execution ──────────────────────────────────────────────────────────

func (p *LotteryPlugin) executeDraw(weekStart string) {
	winning := generateLotteryNumbers()

	tickets, err := lotteryLoadAllWeekTickets(weekStart)
	if err != nil {
		slog.Error("lottery: failed to load tickets for draw", "err", err)
		return
	}

	if len(tickets) == 0 {
		gr := gamesRoom()
		if gr != "" {
			p.SendMessage(gr, "🎟️ **LOTTERY DRAW** — No tickets were sold this week. The pot rolls over.")
		}
		return
	}

	// Score all tickets.
	matchBuckets := make(map[int][]lotteryTicket) // matchCount -> tickets
	for i := range tickets {
		mc := countMatches(tickets[i].Numbers, winning)
		tickets[i].MatchCount = &mc

		prize := 0
		if mc >= 1 && mc <= 4 {
			prize = lotteryFixedPrizes[mc]
		}
		tickets[i].Prize = &prize

		lotteryUpdateTicketResult(tickets[i].ID, mc, prize)
		matchBuckets[mc] = append(matchBuckets[mc], tickets[i])
	}

	pot := communityPotBalance()
	initialPot := pot

	// Calculate fixed tier payouts.
	fixedTotal := 0
	for tier := 4; tier >= 1; tier-- {
		count := len(matchBuckets[tier])
		fixedTotal += count * lotteryFixedPrizes[tier]
	}

	// If pot can't cover all fixed payouts, prorate top-down.
	actualFixed := fixedTotal
	if actualFixed > pot {
		actualFixed = pot
	}

	// Debit fixed payouts from pot.
	if actualFixed > 0 {
		if !communityPotDebit(actualFixed) {
			slog.Error("lottery: failed to debit fixed payouts from pot", "amount", actualFixed)
			actualFixed = 0
		}
		pot -= actualFixed
	}

	// Credit fixed tier winners.
	prorateRatio := 1.0
	if fixedTotal > 0 && actualFixed < fixedTotal {
		prorateRatio = float64(actualFixed) / float64(fixedTotal)
	}

	if actualFixed > 0 {
		for tier := 4; tier >= 1; tier-- {
			for _, t := range matchBuckets[tier] {
				amount := int(float64(lotteryFixedPrizes[tier]) * prorateRatio)
				if amount > 0 {
					p.euro.Credit(t.UserID, float64(amount), fmt.Sprintf("lottery_%dmatch", tier))
				}
			}
		}
	}

	// Jackpot.
	jackpotWinners := matchBuckets[5]
	jackpotAmount := 0
	rolledOver := 0

	if len(jackpotWinners) > 0 && pot >= 500 {
		// Split remaining pot among jackpot winners.
		perWinner := pot / len(jackpotWinners)
		remainder := pot - (perWinner * len(jackpotWinners))
		jackpotAmount = perWinner

		totalJackpotDebit := perWinner * len(jackpotWinners)
		if !communityPotDebit(totalJackpotDebit) {
			slog.Error("lottery: failed to debit jackpot from pot", "amount", totalJackpotDebit)
			// Jackpot rolls over — don't credit winners.
			jackpotAmount = 0
			rolledOver = pot
		} else {
			for _, t := range jackpotWinners {
				p.euro.Credit(t.UserID, float64(perWinner), "lottery_jackpot")
				lotteryUpdateTicketResult(t.ID, 5, perWinner)
			}
			rolledOver = remainder
		}
	} else {
		// Jackpot rolls over.
		rolledOver = pot
	}

	// Insert history.
	h := &lotteryHistoryRow{
		DrawDate:       time.Now().UTC().Format("2006-01-02"),
		WinningNumbers: winning,
		JackpotWinners: len(jackpotWinners),
		JackpotAmount:  jackpotAmount,
		Match4Winners:  len(matchBuckets[4]),
		Match3Winners:  len(matchBuckets[3]),
		Match2Winners:  len(matchBuckets[2]),
		Match1Winners:  len(matchBuckets[1]),
		PotTotal:       initialPot,
		RolledOver:     rolledOver,
	}
	lotteryInsertHistory(h)

	// Room announcement.
	gr := gamesRoom()
	if gr != "" {
		announcement := p.buildDrawAnnouncement(winning, h, jackpotWinners)
		p.SendMessage(gr, announcement)
	}

	// Cleanup old tickets.
	lotteryCleanupOldTickets()

	slog.Info("lottery: draw completed",
		"winning", winning,
		"tickets", len(tickets),
		"pot", initialPot,
		"jackpot_winners", len(jackpotWinners))
}

// ── Announcement Builder ────────────────────────────────────────────────────

func (p *LotteryPlugin) buildDrawAnnouncement(winning []int, h *lotteryHistoryRow, jackpotWinners []lotteryTicket) string {
	var sb strings.Builder

	sb.WriteString("🎟️ **LOTTERY DRAW** — Friday 23:59 UTC\n\n")
	sb.WriteString(fmt.Sprintf("This week's numbers: **%s**\n\n", formatLotteryNumbers(winning)))

	// Jackpot line.
	if len(jackpotWinners) > 0 && h.JackpotAmount > 0 {
		names := p.resolveWinnerNames(jackpotWinners)
		if len(jackpotWinners) == 1 {
			sb.WriteString(fmt.Sprintf("Jackpot (5 match): **%s** — €%d 🎉\n", names[0], h.JackpotAmount))
		} else {
			sb.WriteString(fmt.Sprintf("Jackpot (5 match): Split between %s. €%d each. 🎉\n",
				joinNames(names), h.JackpotAmount))
		}
	} else if len(jackpotWinners) > 0 && h.JackpotAmount == 0 {
		sb.WriteString("Jackpot withheld — pot insufficient. Rolls to next week. Fixed tiers paid as normal.\n")
	} else {
		sb.WriteString("Jackpot (5 match): No winner this week. Pot rolls over.\n")
	}

	// Fixed tiers.
	sb.WriteString(fmt.Sprintf("4 match: %d winner(s) — €1,000 each\n", h.Match4Winners))
	sb.WriteString(fmt.Sprintf("3 match: %d winner(s) — €100 each\n", h.Match3Winners))
	sb.WriteString(fmt.Sprintf("2 match: %d winner(s) — €10 each\n", h.Match2Winners))
	sb.WriteString(fmt.Sprintf("1 match: %d winner(s) — €2 each\n", h.Match1Winners))

	distributed := h.PotTotal - h.RolledOver
	sb.WriteString(fmt.Sprintf("\nPot distributed: €%d. Next draw: Friday. Tickets on sale now.", distributed))

	return sb.String()
}

func (p *LotteryPlugin) resolveWinnerNames(winners []lotteryTicket) []string {
	// Deduplicate by user ID (multiple tickets from same player).
	seen := make(map[id.UserID]bool)
	var names []string
	for _, t := range winners {
		if seen[t.UserID] {
			continue
		}
		seen[t.UserID] = true
		// Try to get display name from DM room.
		name := p.DisplayName(t.UserID)
		names = append(names, name)
	}
	return names
}

func joinNames(names []string) string {
	if len(names) <= 2 {
		return strings.Join(names, " and ")
	}
	return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
}
