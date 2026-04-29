package plugin

import (
	"database/sql"
	"fmt"
	"strings"

	"gogobee/internal/db"
)

// ── Stats aggregation ──────────────────────────────────────────────────────

type coopTierStats struct {
	Tier              int
	Open              int
	Active            int
	Complete          int
	Wiped             int
	Cancelled         int
	AvgPartySize      float64
	TotalReward       int64 // Σ reward_total of completed runs
	TotalForfeited    int64 // Σ gold_pool of wiped runs (lost to the dungeon)
}

type coopBetStats struct {
	BetsPlaced int
	TotalStake int64
	TotalPaid  int64 // sum of non-NULL payouts (winners)
	BettorsLT  int   // distinct bettors lifetime
}

type coopGiftStats struct {
	BasketSent     int
	MimicSent      int
	BasketOpened   int
	BasketLeft     int
	MimicOpened    int
	MimicLeft      int
}

func loadCoopTierStats() ([]coopTierStats, error) {
	d := db.Get()
	stats := map[int]*coopTierStats{}
	for t := 1; t <= 5; t++ {
		stats[t] = &coopTierStats{Tier: t}
	}

	// Run outcomes by tier
	rows, err := d.Query(`SELECT tier, status, COUNT(*) FROM coop_dungeon_runs
		GROUP BY tier, status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tier int
		var status string
		var n int
		if err := rows.Scan(&tier, &status, &n); err != nil {
			continue
		}
		s, ok := stats[tier]
		if !ok {
			continue
		}
		switch status {
		case "open":
			s.Open = n
		case "active":
			s.Active = n
		case "complete":
			s.Complete = n
		case "wiped":
			s.Wiped = n
		case "cancelled":
			s.Cancelled = n
		}
	}

	// Avg party size by tier (over locked runs only — open invites with one
	// member would bias the number).
	rows2, err := d.Query(`SELECT r.tier, AVG(m.cnt) FROM coop_dungeon_runs r
		JOIN (SELECT run_id, COUNT(*) cnt FROM coop_dungeon_members GROUP BY run_id) m
		ON m.run_id = r.id
		WHERE r.status IN ('active','complete','wiped')
		GROUP BY r.tier`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var tier int
			var avg sql.NullFloat64
			if err := rows2.Scan(&tier, &avg); err == nil && avg.Valid {
				if s, ok := stats[tier]; ok {
					s.AvgPartySize = avg.Float64
				}
			}
		}
	}

	// Reward / forfeit totals per tier
	rows3, err := d.Query(`SELECT tier, status, SUM(reward_total), SUM(gold_pool)
		FROM coop_dungeon_runs WHERE status IN ('complete','wiped')
		GROUP BY tier, status`)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var tier int
			var status string
			var reward, pool sql.NullInt64
			if err := rows3.Scan(&tier, &status, &reward, &pool); err != nil {
				continue
			}
			s, ok := stats[tier]
			if !ok {
				continue
			}
			if status == "complete" && reward.Valid {
				s.TotalReward += reward.Int64
			}
			if status == "wiped" && pool.Valid {
				s.TotalForfeited += pool.Int64
			}
		}
	}

	out := make([]coopTierStats, 0, 5)
	for t := 1; t <= 5; t++ {
		out = append(out, *stats[t])
	}
	return out, nil
}

func loadCoopBetStats() (coopBetStats, error) {
	d := db.Get()
	var s coopBetStats
	var stake, paid sql.NullInt64
	err := d.QueryRow(`SELECT COUNT(*), COALESCE(SUM(amount),0),
		COALESCE(SUM(CASE WHEN payout > 0 THEN payout ELSE 0 END), 0)
		FROM coop_dungeon_bets`).Scan(&s.BetsPlaced, &stake, &paid)
	if err != nil {
		return s, err
	}
	s.TotalStake = stake.Int64
	s.TotalPaid = paid.Int64
	_ = d.QueryRow(`SELECT COUNT(DISTINCT player_id) FROM coop_dungeon_bets`).Scan(&s.BettorsLT)
	return s, nil
}

func loadCoopGiftStats() (coopGiftStats, error) {
	d := db.Get()
	var s coopGiftStats
	rows, err := d.Query(`SELECT gift_type, COALESCE(vote_result, 'pending'), COUNT(*)
		FROM coop_dungeon_gifts GROUP BY gift_type, vote_result`)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var giftType, voteResult string
		var n int
		if err := rows.Scan(&giftType, &voteResult, &n); err != nil {
			continue
		}
		switch giftType {
		case coopGiftBasket:
			s.BasketSent += n
			if voteResult == "opened" {
				s.BasketOpened = n
			} else if voteResult == "left" {
				s.BasketLeft = n
			}
		case coopGiftMimic:
			s.MimicSent += n
			if voteResult == "opened" {
				s.MimicOpened = n
			} else if voteResult == "left" {
				s.MimicLeft = n
			}
		}
	}
	return s, nil
}

// ── Render ──────────────────────────────────────────────────────────────────

func renderCoopStats() string {
	tiers, _ := loadCoopTierStats()
	bets, _ := loadCoopBetStats()
	gifts, _ := loadCoopGiftStats()
	help30 := coopTwinBeeHelpfulness(30)
	helpAll := coopTwinBeeHelpfulness(10000) // effectively lifetime

	var sb strings.Builder
	sb.WriteString("⚔️ **Co-op Dungeon Stats**\n\n")

	sb.WriteString("**Runs by tier**\n")
	sb.WriteString("```\n")
	sb.WriteString(fmt.Sprintf("%-6s %-6s %-6s %-6s %-6s %-6s %-7s %-12s\n",
		"tier", "open", "active", "won", "wiped", "canc", "win%", "avg party"))
	for _, t := range tiers {
		finished := t.Complete + t.Wiped
		winPct := 0.0
		if finished > 0 {
			winPct = float64(t.Complete) / float64(finished) * 100
		}
		sb.WriteString(fmt.Sprintf("T%-5d %-6d %-6d %-6d %-6d %-6d %-6.1f%% %-12.2f\n",
			t.Tier, t.Open, t.Active, t.Complete, t.Wiped, t.Cancelled, winPct, t.AvgPartySize))
	}
	sb.WriteString("```\n\n")

	sb.WriteString("**Gold flow**\n")
	var totalReward, totalForfeit int64
	for _, t := range tiers {
		totalReward += t.TotalReward
		totalForfeit += t.TotalForfeited
	}
	sb.WriteString(fmt.Sprintf("  Rewards distributed: €%d\n", totalReward))
	sb.WriteString(fmt.Sprintf("  Funding forfeited (wipes): €%d\n\n", totalForfeit))

	sb.WriteString("**Spectator betting**\n")
	rake := int64(float64(bets.TotalStake) * coopBetRake)
	sb.WriteString(fmt.Sprintf("  Bets placed: %d (by %d distinct bettors)\n", bets.BetsPlaced, bets.BettorsLT))
	sb.WriteString(fmt.Sprintf("  Total wagered: €%d\n", bets.TotalStake))
	sb.WriteString(fmt.Sprintf("  Paid to winners: €%d\n", bets.TotalPaid))
	sb.WriteString(fmt.Sprintf("  House cut (TwinBee): ~€%d\n\n", rake))

	sb.WriteString("**Gifts**\n")
	openRate := func(opened, total int) float64 {
		if total == 0 {
			return 0
		}
		return float64(opened) / float64(total) * 100
	}
	sb.WriteString(fmt.Sprintf("  Baskets sent: %d (opened %d, left %d, %.0f%% open rate)\n",
		gifts.BasketSent, gifts.BasketOpened, gifts.BasketLeft,
		openRate(gifts.BasketOpened, gifts.BasketOpened+gifts.BasketLeft)))
	sb.WriteString(fmt.Sprintf("  Mimics sent: %d (opened %d, left %d, %.0f%% open rate)\n\n",
		gifts.MimicSent, gifts.MimicOpened, gifts.MimicLeft,
		openRate(gifts.MimicOpened, gifts.MimicOpened+gifts.MimicLeft)))

	sb.WriteString("**TwinBee helpfulness**\n")
	sb.WriteString(fmt.Sprintf("  Last 30 events: %.0f%%\n", help30*100))
	sb.WriteString(fmt.Sprintf("  Lifetime:       %.0f%%\n", helpAll*100))
	return sb.String()
}
