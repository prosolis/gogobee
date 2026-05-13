package plugin

import (
	"log/slog"

	"gogobee/internal/db"
)

// bootstrapRestoreExpeditionStreakDecay reverses the one-time streak halving
// applied by midnightReset to players who were on an active expedition. The
// idle reaper used to ignore expedition activity (it only inspected the
// legacy CombatActionsUsed/HarvestActionsUsed counters), so expeditioners
// were shamed and had their streak divided by 2.
//
// Restoration: CurrentStreak doubles (recovering the integer-divide), capped
// at BestStreak. The odd-half is recovered by adding 1 when BestStreak
// allows — pre-decay value was either CurrentStreak*2 or CurrentStreak*2+1,
// and BestStreak is a monotonic ceiling that bounds the original.
//
// Idempotent: gated on a fixed job key so it runs at most once per DB.
func bootstrapRestoreExpeditionStreakDecay() {
	const jobName = "idle_reaper_expedition_streak_restore_v1"
	if db.JobCompleted(jobName, "once") {
		return
	}

	chars, err := loadAllAdvCharacters()
	if err != nil {
		slog.Error("bootstrap: streak restore — failed to load characters", "err", err)
		return
	}

	restored := 0
	for _, char := range chars {
		if !char.StreakDecayed || char.CurrentStreak == 0 {
			continue
		}
		exp, err := getActiveExpedition(char.UserID)
		if err != nil {
			slog.Warn("bootstrap: streak restore — expedition lookup failed", "user", char.UserID, "err", err)
			continue
		}
		if exp == nil {
			continue
		}

		old := char.CurrentStreak
		candidate := old*2 + 1
		if char.BestStreak > 0 && candidate > char.BestStreak {
			candidate = char.BestStreak
		}
		if candidate < old*2 {
			candidate = old * 2
		}
		char.CurrentStreak = candidate
		char.StreakDecayed = false
		if err := saveAdvCharacter(&char); err != nil {
			slog.Error("bootstrap: streak restore — save failed", "user", char.UserID, "err", err)
			continue
		}
		slog.Info("bootstrap: streak restored", "user", char.UserID, "old", old, "new", candidate, "best", char.BestStreak)
		restored++
	}

	db.MarkJobCompleted(jobName, "once")
	if restored > 0 {
		slog.Warn("bootstrap: expedition streak decay restored", "count", restored)
	}
}
