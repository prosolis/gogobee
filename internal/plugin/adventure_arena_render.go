package plugin

import (
	"fmt"
	"strings"
)

// ── Arena Tier Menu ─────────────────────────────────────────────────────────

func renderArenaTierMenu(char *AdventureCharacter, stats *ArenaPersonalStats) string {
	var b strings.Builder
	b.WriteString("⚔️ **THE ARENA**\n\n")
	b.WriteString(fmt.Sprintf("Combat Level: %d\n\n", char.CombatLevel))

	for i := range arenaTiers {
		t := &arenaTiers[i]
		eligible := char.CombatLevel >= t.MinLevel
		icon := "🔒"
		if eligible {
			icon = "⬚" // eligible but not cleared
		}
		if stats != nil && stats.HighestTier >= t.Number {
			icon = "✅" // cleared
		}
		b.WriteString(fmt.Sprintf("%s **Tier %d — %s** (Lv.%d+)\n", icon, t.Number, t.Name, t.MinLevel))
	}

	if stats != nil && stats.TotalRuns > 0 {
		b.WriteString(fmt.Sprintf("\nRuns: %d | Deaths: %d | Earned: €%d\n",
			stats.TotalRuns, stats.TotalDeaths, stats.TotalEarnings))
	}

	b.WriteString("\n`!arena tier <1-5>` — Enter a tier\n")
	b.WriteString("`!arena stats` — Your arena stats\n")
	b.WriteString("`!arena leaderboard` — Top arena players\n")
	return b.String()
}

// ── Round Start (Monster Reveal) ────────────────────────────────────────────

func renderArenaRoundStart(tier *ArenaTier, round int, monster *ArenaMonster, run *ArenaRun) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("⚔️ **Tier %d — %s | Round %d/4**\n\n", tier.Number, tier.Name, round))
	b.WriteString(fmt.Sprintf("**%s**\n", monster.Name))
	b.WriteString(fmt.Sprintf("_%s_\n\n", monster.Flavor))

	if run.Earnings > 0 {
		b.WriteString(fmt.Sprintf("Run earnings: €%d (at risk)\n\n", run.Earnings))
	}

	b.WriteString("`!arena fight` — Face this opponent\n")
	return b.String()
}

// ── Survival ────────────────────────────────────────────────────────────────

func renderArenaSurvival(tier *ArenaTier, round int, monster *ArenaMonster, reward int64, xp int, totalEarnings int64) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("✅ **%s defeated.**\n\n", monster.Name))
	b.WriteString(fmt.Sprintf("Round reward: €%d\n", reward))
	b.WriteString(fmt.Sprintf("Battle XP: +%d\n", xp))
	b.WriteString(fmt.Sprintf("Run total: €%d\n", totalEarnings))
	return b.String()
}

// ── Tier Complete (Transition Prompt) ───────────────────────────────────────

func renderArenaTierComplete(tier *ArenaTier, completionBonus int64, totalEarnings int64) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🏆 **Tier %d — %s cleared!**\n\n", tier.Number, tier.Name))
	b.WriteString(fmt.Sprintf("Completion bonus: €%d\n", completionBonus))
	b.WriteString(fmt.Sprintf("Run total: €%d\n\n", totalEarnings))

	if tier.Number < 5 {
		nextTier := arenaGetTier(tier.Number + 1)
		b.WriteString(fmt.Sprintf("Descend to Tier %d — %s? Your earnings are at risk.\n\n", nextTier.Number, nextTier.Name))
		b.WriteString(fmt.Sprintf("`!arena descend` — Enter Tier %d (earnings carry over, still at risk)\n", nextTier.Number))
		b.WriteString(fmt.Sprintf("`!arena cashout` — Take your €%d and leave\n\n", totalEarnings))
		b.WriteString("_You have 10 minutes to decide. After that, GogoBee collects on your behalf._")
	}

	return b.String()
}

// ── Tier 5 Complete ─────────────────────────────────────────────────────────

func renderArenaTier5Complete(totalEarnings int64, startTier int) string {
	var b strings.Builder
	b.WriteString("🏆🏆🏆 **THE ARENA HAS BEEN CONQUERED.**\n\n")
	b.WriteString("That Which Has Always Been has fallen. The machine has logged a loss.\n")
	b.WriteString("The crowd is silent. Not out of respect — out of disbelief.\n\n")
	b.WriteString(fmt.Sprintf("**Total earnings: €%d**\n\n", totalEarnings))
	b.WriteString("Your euros have been credited. Your name has been etched. The Arena remembers.")

	if startTier == 1 {
		b.WriteString("\n\n_All the way down. From Tier 1 to Tier 5 in a single run. Statistically impossible. Empirically: you._")
	}

	return b.String()
}

// ── Death ────────────────────────────────────────────────────────────────────

func renderArenaDeath(tier *ArenaTier, round int, monster *ArenaMonster, lostEarnings int64, deathMsg string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("💀 **DEAD** — Tier %d, Round %d\n\n", tier.Number, round))
	b.WriteString(fmt.Sprintf("_%s_\n\n", deathMsg))

	if lostEarnings > 0 {
		b.WriteString(fmt.Sprintf("Forfeited earnings: €%d\n", lostEarnings))
	}
	b.WriteString("You are dead until midnight UTC. Both arena and daily adventure are blocked until you respawn.")
	return b.String()
}

// ── Cashout ─────────────────────────────────────────────────────────────────

func renderArenaCashout(totalEarnings int64, lastTier int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("💰 **Cashed out: €%d**\n\n", totalEarnings))
	b.WriteString(fmt.Sprintf("You cleared through Tier %d and lived to spend it. ", lastTier))
	b.WriteString("Wisdom or cowardice — the euros don't care which.")
	return b.String()
}

// ── Auto-Cashout ────────────────────────────────────────────────────────────

func renderArenaAutoCashout(totalEarnings int64) string {
	return fmt.Sprintf(
		"⏰ **Auto-cashout: €%d**\n\n"+
			"You took too long to decide. GogoBee collected your winnings on your behalf "+
			"and is annoyed about it. The money is in your account. You're welcome.",
		totalEarnings)
}

// ── Status ──────────────────────────────────────────────────────────────────

func renderArenaStatus(run *ArenaRun, char *AdventureCharacter) string {
	tier := arenaGetTier(run.Tier)
	if tier == nil {
		return "No active arena run."
	}

	var b strings.Builder
	b.WriteString("⚔️ **Arena Run Status**\n\n")
	b.WriteString(fmt.Sprintf("Tier: %d — %s\n", tier.Number, tier.Name))

	switch run.Status {
	case "active":
		monster := arenaGetMonster(run.Tier, run.Round)
		b.WriteString(fmt.Sprintf("Round: %d/4\n", run.Round))
		if monster != nil {
			b.WriteString(fmt.Sprintf("Opponent: %s\n", monster.Name))
		}
		b.WriteString(fmt.Sprintf("Earnings: €%d (at risk)\n", run.Earnings))
		b.WriteString(fmt.Sprintf("Rounds survived: %d\n", run.RoundsSurvived))
		b.WriteString("\n`!arena fight` to continue")
	case "awaiting":
		b.WriteString(fmt.Sprintf("Tier %d cleared — awaiting decision\n", run.Tier))
		b.WriteString(fmt.Sprintf("Earnings: €%d (at risk)\n", run.Earnings))
		b.WriteString(fmt.Sprintf("Rounds survived: %d\n", run.RoundsSurvived))
		b.WriteString("\n`!arena descend` or `!arena cashout`")
	}

	return b.String()
}

// ── Leaderboard ─────────────────────────────────────────────────────────────

type ArenaLeaderboardEntry struct {
	DisplayName     string
	TotalEarnings   int64
	HighestTier     int
	Tier5Completions int
	TotalRuns       int
	TotalDeaths     int
}

func renderArenaLeaderboard(entries []ArenaLeaderboardEntry) string {
	if len(entries) == 0 {
		return "⚔️ **Arena Leaderboard**\n\nNo arena runs recorded yet. Be the first."
	}

	var b strings.Builder
	b.WriteString("⚔️ **Arena Leaderboard**\n\n")

	medals := []string{"🥇", "🥈", "🥉"}
	for i, e := range entries {
		prefix := fmt.Sprintf("%d.", i+1)
		if i < 3 {
			prefix = medals[i]
		}

		tierLabel := fmt.Sprintf("T%d", e.HighestTier)
		if e.Tier5Completions > 0 {
			tierLabel = fmt.Sprintf("T5×%d", e.Tier5Completions)
		}

		b.WriteString(fmt.Sprintf("%s **%s** — €%d earned | %s | %d runs | %d deaths\n",
			prefix, e.DisplayName, e.TotalEarnings, tierLabel, e.TotalRuns, e.TotalDeaths))
	}

	return b.String()
}

// ── Tier Entry Confirmation ──────────────────────────────────────────────────

func renderArenaTierConfirm(tier *ArenaTier, firstMonster *ArenaMonster) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("⚔️ **Tier %d — %s**\n\n", tier.Number, tier.Name))
	b.WriteString(fmt.Sprintf("4 rounds. You cannot leave mid-tier. Death forfeits all earnings.\n\n"))
	b.WriteString(fmt.Sprintf("Round 1 opponent: **%s**\n", firstMonster.Name))
	b.WriteString(fmt.Sprintf("_%s_\n\n", firstMonster.Flavor))
	b.WriteString("`!arena fight` — Enter and fight Round 1\n")
	b.WriteString("`!arena cancel` — Back out")
	return b.String()
}

// ── Personal Stats ──────────────────────────────────────────────────────────

type ArenaPersonalStats struct {
	TotalRuns        int
	TotalEarnings    int64
	TotalDeaths      int
	HighestTier      int
	Tier5Completions int
}

func renderArenaPersonalStats(char *AdventureCharacter, stats *ArenaPersonalStats) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("⚔️ **%s's Arena Stats**\n\n", char.DisplayName))

	if stats == nil || stats.TotalRuns == 0 {
		b.WriteString("No arena runs yet. Type `!arena` to begin.")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("Total runs: %d\n", stats.TotalRuns))
	b.WriteString(fmt.Sprintf("Total earnings: €%d\n", stats.TotalEarnings))
	b.WriteString(fmt.Sprintf("Total deaths: %d\n", stats.TotalDeaths))
	b.WriteString(fmt.Sprintf("Highest tier cleared: %d\n", stats.HighestTier))

	if stats.TotalRuns > 0 {
		survivalRate := float64(stats.TotalRuns-stats.TotalDeaths) / float64(stats.TotalRuns) * 100
		b.WriteString(fmt.Sprintf("Survival rate: %.0f%%\n", survivalRate))
	}

	if stats.Tier5Completions > 0 {
		b.WriteString(fmt.Sprintf("Tier 5 completions: %d\n", stats.Tier5Completions))
	}

	b.WriteString(fmt.Sprintf("\nArena W/L: %d/%d", char.ArenaWins, char.ArenaLosses))
	return b.String()
}

// ── Level Gate Message ──────────────────────────────────────────────────────

func renderArenaLevelGate(tier *ArenaTier, playerLevel int) string {
	return fmt.Sprintf(
		"⚔️ Tier %d — %s requires Combat Level %d. You are Level %d. "+
			"The Arena does not negotiate.",
		tier.Number, tier.Name, tier.MinLevel, playerLevel)
}

// ── Already In Run Message ──────────────────────────────────────────────────

func renderArenaAlreadyInRun(run *ArenaRun) string {
	switch run.Status {
	case "awaiting":
		return fmt.Sprintf(
			"You have a pending arena decision. Tier %d cleared, €%d at risk.\n\n"+
				"`!arena descend` or `!arena cashout`",
			run.Tier, run.Earnings)
	default:
		return fmt.Sprintf(
			"You're already in an arena run. Tier %d, Round %d.\n\n"+
				"`!arena fight` to continue.",
			run.Tier, run.Round)
	}
}
