package plugin

import (
	"fmt"
	"strings"
	"time"
)

// ── Arena Tier Menu ─────────────────────────────────────────────────────────

func renderArenaStreakEntry(c *DnDCharacter, stats *ArenaPersonalStats, tier *ArenaTier, firstMonster *ArenaMonster) string {
	var b strings.Builder
	b.WriteString("⚔️ **THE ARENA**\n\n")
	b.WriteString("The Arena is a streak. You start at Tier 1 and fight your way down.\n")
	b.WriteString("After each tier, you have 30 seconds to bail or you auto-advance.\n")
	b.WriteString("Death forfeits all accumulated rewards.\n\n")

	b.WriteString(fmt.Sprintf("Level: %d\n\n", c.Level))

	for i := range arenaTiers {
		t := &arenaTiers[i]
		eligible := c.Level >= t.MinLevel
		icon := "🔒"
		if eligible {
			icon = "⬚"
		}
		if stats != nil && stats.HighestTier >= t.Number {
			icon = "✅"
		}
		mult := arenaStreakEuroMultiplier[t.Number]
		b.WriteString(fmt.Sprintf("%s **Tier %d — %s** (Lv.%d+) — %.1f× euros\n", icon, t.Number, t.Name, t.MinLevel, mult))
	}

	if stats != nil && stats.TotalRuns > 0 {
		b.WriteString(fmt.Sprintf("\nRuns: %d | Deaths: %d | Earned: €%d\n",
			stats.TotalRuns, stats.TotalDeaths, stats.TotalEarnings))
	}

	b.WriteString("\n⛑️ _Today's helmets are provided by Brim & Battle — celebrated makers of baseball hats — and proud sponsors of the Arena. ")
	b.WriteString("Their new VeriFort line of field-evaluated combat headgear represents an exciting expansion beyond their area of expertise. ")
	b.WriteString("Winners may receive a complimentary sample. Brim & Battle thanks you for your participation in their ongoing verification process._\n\n")

	b.WriteString(fmt.Sprintf("**Round 1 opponent: %s**\n", firstMonster.Name))
	b.WriteString(fmt.Sprintf("_%s_\n\n", firstMonster.Flavor))
	b.WriteString("`!arena fight` — Enter and fight Round 1\n")
	b.WriteString("`!arena cancel` — Back out")
	return b.String()
}

// ── Round Start (Monster Reveal) ────────────────────────────────────────────

func renderArenaRoundStart(tier *ArenaTier, round int, monster *ArenaMonster, run *ArenaRun) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("⚔️ **Tier %d — %s | Round %d/4**\n\n", tier.Number, tier.Name, round))
	b.WriteString(fmt.Sprintf("**%s**\n", monster.Name))
	b.WriteString(fmt.Sprintf("_%s_\n\n", monster.Flavor))

	sessionTotal := run.Earnings + run.TierEarnings
	if sessionTotal > 0 {
		b.WriteString(fmt.Sprintf("Session earnings: €%d (at risk)\n\n", sessionTotal))
	}

	b.WriteString("`!arena fight` — Face this opponent\n")
	return b.String()
}

// renderArenaSurvival removed — survival text is built by renderArenaCombatLog + inline formatting.

// ── Tier Complete (Transition Prompt) ───────────────────────────────────────

// renderArenaTierComplete is no longer used — tier complete text is built inline
// in resolveArenaSurvival and the countdown DM handles the opt-out flow.

// renderArenaTier5Complete removed — T5 completion is part of arenaCompleteSession payout summary.

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

// renderArenaCashout and renderArenaAutoCashout removed — payout handled by arenaCompleteSession.

// ── Status ──────────────────────────────────────────────────────────────────

func renderArenaStatus(run *ArenaRun) string {
	tier := arenaGetTier(run.Tier)
	if tier == nil {
		return "No active arena run."
	}

	var b strings.Builder
	b.WriteString("⚔️ **Arena Streak Status**\n\n")
	b.WriteString(fmt.Sprintf("Tier: %d — %s\n", tier.Number, tier.Name))

	sessionTotal := run.Earnings + run.TierEarnings

	switch run.Status {
	case "active":
		monster := arenaGetMonster(run.Tier, run.Round)
		b.WriteString(fmt.Sprintf("Round: %d/4\n", run.Round))
		if monster != nil {
			b.WriteString(fmt.Sprintf("Opponent: %s\n", monster.Name))
		}
		b.WriteString(fmt.Sprintf("Session total: €%d (at risk)\n", sessionTotal))
		b.WriteString(fmt.Sprintf("Rounds survived: %d\n", run.RoundsSurvived))
		b.WriteString("\n`!arena fight` to continue")
	case "awaiting":
		b.WriteString(fmt.Sprintf("Tier %d cleared — advancing shortly\n", run.Tier))
		b.WriteString(fmt.Sprintf("Session total: €%d (at risk)\n", sessionTotal))
		b.WriteString(fmt.Sprintf("Rounds survived: %d\n", run.RoundsSurvived))
		b.WriteString("\n`!bail` to collect your rewards")
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

func renderArenaLeaderboard(season string, entries []ArenaLeaderboardEntry) string {
	if len(entries) == 0 {
		return fmt.Sprintf("⚔️ **Arena Leaderboard — Season %s**\n\nNobody has entered the arena this season. Be the first.", season)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("⚔️ **Arena Leaderboard — Season %s**\n\n", season))

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
	b.WriteString("\n_Season standings. `!arena stats` for your lifetime record._")

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

// renderArenaPersonalStats renders the !arena stats DM. Wins/losses come
// from player_meta post-Phase L2 step 5 (the caller is responsible for
// loading them); displayName is sourced from the AdvCharacter.
func renderArenaPersonalStats(displayName string, wins, losses int, stats *ArenaPersonalStats) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("⚔️ **%s's Arena Stats**\n\n", displayName))

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

	b.WriteString(fmt.Sprintf("\nArena W/L: %d/%d", wins, losses))
	return b.String()
}

// ── Level Gate Message ──────────────────────────────────────────────────────

func renderArenaLevelGate(tier *ArenaTier, playerLevel int) string {
	return fmt.Sprintf(
		"⚔️ Tier %d — %s requires Combat Level %d. You are Level %d. "+
			"The Arena does not negotiate.",
		tier.Number, tier.Name, tier.MinLevel, playerLevel)
}

// ── Helmet Drop ────────────────────────────────────────────────────────────

func renderArenaHelmetDrop(gear *ArenaGearSet) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("⛑️ **Equipment Drop: %s**\n\n", gear.HelmetName))
	b.WriteString(fmt.Sprintf("_%s_\n\n", gear.Description))
	b.WriteString(fmt.Sprintf("Tier %d Arena gear — 1.5x effectiveness. ", gear.Tier))

	switch gear.SetKey {
	case "bloodied":
		b.WriteString("Set bonus: **Survivor's Instinct** — +3% to all activity success rates, +3% critical hit rate in combat.")
	case "ironclad":
		b.WriteString("Set bonus: **Battle-Hardened** — +5% XP gain from all activities.")
	case "tempered":
		b.WriteString("Set bonus: **Seasoned** — Equipment condition degrades 25% slower across all slots.")
	case "champions":
		b.WriteString("Set bonus: **Commanding Presence** — +10% to equipment score in all probability calculations.")
	case "sovereign":
		b.WriteString("Set bonus: **Death's Reprieve** — Once per 7 days, survive a lethal outcome instead of dying.")
	}

	b.WriteString("\n\nEquipped automatically.")
	return b.String()
}

// ── Death's Reprieve Announcement ──────────────────────────────────────────

func renderArenaDeathReprieve(playerName, location string, nextWindow time.Time) string {
	return fmt.Sprintf("⚔️ **%s**'s Sovereign gear activated **Death's Reprieve**. "+
		"They should be dead. They are not. %s will have to try harder. Next window: %s.",
		playerName, location, nextWindow.Format("2006-01-02 15:04 UTC"))
}

// ── Already In Run Message ──────────────────────────────────────────────────

func renderArenaAlreadyInRun(run *ArenaRun) string {
	switch run.Status {
	case "awaiting":
		return fmt.Sprintf(
			"Tier %d cleared, €%d accumulated. Waiting for next tier or `!bail` to collect.\n",
			run.Tier, run.Earnings)
	default:
		return fmt.Sprintf(
			"You're in an arena streak. Tier %d, Round %d.\n\n"+
				"`!arena fight` to continue.",
			run.Tier, run.Round)
	}
}
