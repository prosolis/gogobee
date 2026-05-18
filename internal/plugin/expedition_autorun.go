package plugin

import (
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase 4 of expedition autopilot — background auto-run ticker. Walks
// active expeditions on real time so the player only engages when a real
// decision is needed (fork, elite, boss, low HP/SU, rare node, death).
//
// Design (settled 2026-05-16):
//   • Tick every autoRunTickInterval; per-expedition rate-limited by
//     autoRunCooldown via a last_autorun_at CAS.
//   • Skip when the player is in active combat, inside the briefing/recap
//     quiet window, or on a very-fresh expedition (give them a beat).
//   • Walk up to autoRunRoomCap rooms per tick. Smaller than foreground's
//     cap so background DMs stay digestible.
//   • DM-suppression rules:
//     - 0 rooms walked → silent (still stuck at a fork / blocked / etc.;
//       the player already knows, no point spamming).
//     - rooms > 0 with stopOK (hit room cap) → silent; we'll keep walking
//       on the next tick. Cadence handles pacing; no "stretch complete"
//       footer churn.
//     - Any other reason with rooms > 0 → DM the bundled walk.
//   • Idempotency: CAS-claim last_autorun_at before doing any work. A
//     double-fire on the same expedition is a no-op.

const (
	// autoRunTickInterval — how often the ticker wakes. The per-expedition
	// cooldown is what actually paces; the tick just has to be frequent
	// enough that the cooldown is enforced cleanly.
	autoRunTickInterval = 5 * time.Minute

	// autoRunCooldown — minimum gap between background walks for one
	// expedition. 15 min keeps the dungeon moving while leaving room for
	// the player to step in and steer if they want.
	autoRunCooldown = 15 * time.Minute

	// autoRunMinExpeditionAge — don't auto-walk a brand-new expedition;
	// let the player walk the first beat manually so the opening reads
	// as deliberate.
	autoRunMinExpeditionAge = 30 * time.Minute

	// autoRunRoomCap — rooms per background tick. Smaller than the
	// foreground cap so a single background DM doesn't dump an
	// overwhelming wall of narration.
	autoRunRoomCap = 3
)

// expeditionAutoRunTicker — fires the auto-run pass every tick interval.
func (p *AdventurePlugin) expeditionAutoRunTicker() {
	ticker := time.NewTicker(autoRunTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.fireExpeditionAutoRuns(time.Now().UTC())
		}
	}
}

// fireExpeditionAutoRuns walks active expeditions due for a background
// step and dispatches one walk per. Cheap per-expedition gating happens
// in tryAutoRun; this layer just selects candidates.
func (p *AdventurePlugin) fireExpeditionAutoRuns(now time.Time) {
	// Reuse the ambient quiet window — same intent: don't talk over the
	// scheduled 06:00 briefing or 21:00 recap.
	if inAmbientQuietWindow(now) {
		return
	}
	due, err := loadExpeditionsForAutoRun(
		now.Add(-autoRunCooldown), now.Add(-autoRunMinExpeditionAge))
	if err != nil {
		slog.Error("expedition: load autorun candidates", "err", err)
		return
	}
	for _, expID := range due {
		e, err := getExpedition(expID)
		if err != nil || e == nil {
			continue
		}
		if e.Status != ExpeditionStatusActive {
			continue
		}
		uid := id.UserID(e.UserID)
		if hasActiveCombatSession(uid) {
			continue
		}
		if err := p.tryAutoRun(e, now); err != nil {
			slog.Warn("expedition: autorun step", "expedition", e.ID, "err", err)
		}
	}
}

// loadExpeditionsForAutoRun returns active expeditions whose
// last_autorun_at is NULL or older than runCutoff, AND whose start_date
// is older than ageCutoff so a freshly-started expedition isn't yanked
// out from under the player on tick 1.
func loadExpeditionsForAutoRun(runCutoff, ageCutoff time.Time) ([]string, error) {
	rows, err := db.Get().Query(`
		SELECT expedition_id
		  FROM dnd_expedition
		 WHERE status = 'active'
		   AND start_date < ?
		   AND (last_autorun_at IS NULL OR last_autorun_at < ?)`,
		ageCutoff, runCutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		ids = append(ids, s)
	}
	return ids, rows.Err()
}

// tryAutoRun claims the slot for this expedition, runs one background
// walk, and DMs the result if the suppression rules say to. The CAS-
// update is the only persistent side effect on the autorun column —
// supplies/threat/run-graph state are mutated by the walk itself, just
// as they would be in a foreground !expedition run.
func (p *AdventurePlugin) tryAutoRun(e *Expedition, now time.Time) error {
	cutoff := now.Add(-autoRunCooldown)
	res, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET last_autorun_at = ?,
		       last_activity = ?
		 WHERE expedition_id = ?
		   AND status = 'active'
		   AND (last_autorun_at IS NULL OR last_autorun_at < ?)`,
		now, now, e.ID, cutoff)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // someone else got there first
	}

	uid := id.UserID(e.UserID)
	r := p.runAutopilotWalk(MessageContext{Sender: uid}, autoRunRoomCap, true)
	if r.initErr != "" {
		// "no expedition" / "no run" — race with abandon/extract. Silent.
		return nil
	}
	if !shouldDMAutoRun(r) {
		return nil
	}

	body := renderAutoRunDM(r)
	if err := p.SendDM(uid, body); err != nil {
		slog.Warn("expedition: autorun DM", "user", uid, "err", err)
	}
	return nil
}

// shouldDMAutoRun applies the background suppression rules. See the
// file-top design block for the rationale.
func shouldDMAutoRun(r autopilotWalkResult) bool {
	if r.rooms == 0 {
		return false
	}
	if r.reason == stopOK {
		// Hit the per-tick room cap. Next tick will continue; no need to
		// post a "stretch complete" filler DM.
		return false
	}
	return true
}

// renderAutoRunDM bundles the staged walk narration into a single DM.
// Background can't pace via streamFlow, so we concatenate phases with a
// blank line between each beat and tack on the final message.
func renderAutoRunDM(r autopilotWalkResult) string {
	var b strings.Builder
	b.WriteString("🚶 *Auto-walk*\n\n")
	for _, s := range r.stream {
		if s == "" {
			continue
		}
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	b.WriteString(r.finalMsg)
	return b.String()
}
