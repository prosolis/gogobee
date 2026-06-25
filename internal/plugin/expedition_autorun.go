package plugin

import (
	"fmt"
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
//   • DM-suppression rules (D4-a — long expedition bundling):
//     - Surface ONLY for: fork (player decision), death, run-complete,
//       boss-safety camp pitch (explicit hold), or a Night=true camp
//       pitch (end-of-day digest).
//     - Everything else — uneventful walks, preflight pauses, harvest
//       interrupts, mid-day rest camps — goes silent. The accumulated
//       day reads as one EoD digest DM when the autopilot night-camps.
//     - Each successful background walk logs a `walk` entry so the EoD
//       digest can count rooms walked without persisting raw narration.
//   • Idempotency: CAS-claim last_autorun_at before doing any work. A
//     double-fire on the same expedition is a no-op.

const (
	// autoRunTickInterval — how often the ticker wakes. The per-expedition
	// cooldown is what actually paces; the tick just has to be frequent
	// enough that the cooldown is enforced cleanly.
	autoRunTickInterval = 15 * time.Minute

	// autoRunCooldown — minimum gap between background walks for one
	// expedition. 2h is the slow cadence: the dungeon still moves on its
	// own while the player is away, but the auto-walk DM stays a
	// once-in-a-while ping instead of a steady drip.
	autoRunCooldown = 2 * time.Hour

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
		// Honor the rest lockout — short/long rest set RestingUntil and
		// foreground !expedition run checks it; background must too, or
		// the auto-walk ticker fires through a long rest.
		if c, cerr := LoadDnDCharacter(uid); cerr == nil && c != nil {
			if restingLockoutRemaining(c) > 0 {
				continue
			}
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

// autoRunCombatSegmentCap bounds the walk→fight→walk ping-pong inside a
// single background tick. With autoRunRoomCap == 3 rooms/tick the loop
// can realistically only hit a couple doorways; the cap is a backstop
// against a pathological state where a fight wins but the next walk
// re-presents the same doorway.
const autoRunCombatSegmentCap = 8

// runAutopilotWalkDriven runs the compact background walk and, when it
// halts at a boss/elite doorway, drives that fight through the real turn
// engine (manual `!fight` parity — long-expedition D8-f) before resuming
// the walk. It loops walk→fight→walk so one tick still covers up to
// maxRooms rooms, exactly as the old inline-boss path did, but bosses now
// face the player's full kit against the enemy's full multiattack profile
// instead of the rosier inline SimulateCombat path.
//
// Combat narration is suppressed via a silent ctx — the day digest
// summarizes the outcome, matching the rest of the compact autopilot
// surface. The returned result carries the cumulative room count and the
// reason of whichever non-combat stop ended the loop.
func (p *AdventurePlugin) runAutopilotWalkDriven(ctx MessageContext, maxRooms int) autopilotWalkResult {
	silent := ctx
	silent.Silent = true
	total := 0
	for seg := 0; seg < autoRunCombatSegmentCap; seg++ {
		budget := maxRooms - total
		if budget <= 0 {
			return autopilotWalkResult{rooms: total, reason: stopOK, finalMsg: autopilotFooter(stopOK, total)}
		}
		r := p.runAutopilotWalk(ctx, budget, true, false)
		if r.initErr != "" {
			return r
		}
		total += r.rooms
		r.rooms = total
		if r.reason != stopBoss && r.reason != stopElite {
			return r
		}
		// Standing at an elite/boss doorway — drive the fight on the turn
		// engine. handleFightCmd opens the session at the current doorway;
		// autoDriveCombat loops until it resolves.
		won, err := p.autoDriveCombat(silent)
		if err != nil {
			slog.Warn("expedition: autopilot turn-engine combat", "user", ctx.Sender, "err", err)
			// Leave the doorway stop in place; the next tick retries the
			// engagement after the cooldown.
			return r
		}
		if won {
			// The won session is recorded; the next walk advances the now-
			// cleared room (a boss win surfaces as stopComplete, an elite as
			// a normal continue). Loop.
			continue
		}
		// Lost: the turn engine never voluntarily flees, so a non-win means
		// the party fell. finishCombatSession (CombatStatusLost) already
		// abandoned the run and force-extracted the expedition; surface it
		// as a death so the digest + pet-emergence seam fire.
		if c, _ := LoadDnDCharacter(ctx.Sender); c == nil || c.HPCurrent <= 0 {
			r.reason = stopEnded
			r.finalMsg = fmt.Sprintf("💀 The party fell in battle after %s. The expedition is over.", roomsWalkedPhrase(total))
			return r
		}
		// Alive but the session didn't open / resolve to a win (rare —
		// bestiary miss or stall). Leave the doorway stop; retry next tick.
		return r
	}
	// Segment cap hit — stop cleanly rather than spin.
	return autopilotWalkResult{rooms: total, reason: stopOK, finalMsg: autopilotFooter(stopOK, total)}
}

// roomsWalkedPhrase renders "N room(s)" for autopilot footers.
func roomsWalkedPhrase(rooms int) string {
	if rooms == 1 {
		return "1 room"
	}
	return fmt.Sprintf("%d rooms", rooms)
}

// tryAutoRun claims the slot for this expedition, runs one background
// walk, and DMs the result if the suppression rules say to. The CAS-
// update is the only persistent side effect on the autorun column —
// supplies/threat/run-graph state are mutated by the walk itself, just
// as they would be in a foreground !expedition run.
func (p *AdventurePlugin) tryAutoRun(e *Expedition, now time.Time) error {
	// Long-expedition D2-a — camp dwell gate. A camp (player or auto)
	// still inside its dwell window means the party is resting; skip
	// the walk entirely. An auto-pitched camp past dwell gets broken
	// here so this tick can proceed.
	if shouldSkipAutoRunForCamp(e, now) {
		return nil
	}
	autoCampBroken := breakAutoCampIfDue(e, now)

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
	// D8-f — boss/elite encounters route through the real turn engine for
	// manual `!fight` parity (enemy multiattack included). Trash mobs still
	// auto-resolve on the fast inline path inside the walk.
	r := p.runAutopilotWalkDriven(MessageContext{Sender: uid}, autoRunRoomCap)
	if r.initErr != "" {
		// "no expedition" / "no run" — race with abandon/extract. Silent.
		return nil
	}

	// D4-a — drop a `walk` log entry per successful background walk so
	// the EoD digest can count rooms walked from structured logs without
	// persisting the raw stream narration we used to DM.
	if r.rooms > 0 {
		_ = appendExpeditionLog(e.ID, e.CurrentDay, "walk",
			fmt.Sprintf("auto-walk: %d room(s)", r.rooms), "")
	}

	// Long-expedition D2-a — post-walk camp scheduler. After the walk
	// settles, see if the autopilot should pitch a rest camp (HP low)
	// or a base-camp waypoint (region boss just cleared). The walk's
	// own preflight handles low-SU pauses; the scheduler stays out of
	// fork/combat/death/complete branches by checking r.reason.
	campBlock := ""
	var campDecision autoCampDecision
	campPitched := false
	if r.reason == stopBossSafety {
		// D3 — the boss-engage gate tripped. Force-pitch a rest camp
		// regardless of decideAutopilotCamp's normal HP threshold and
		// its RoomBoss block. Next tick past dwell retries the boss.
		if fresh, ferr := getExpedition(e.ID); ferr == nil && fresh != nil &&
			fresh.Status == ExpeditionStatusActive {
			campBlock, campDecision, campPitched = p.pitchBossSafetyCamp(fresh, now)
		}
	} else if r.reason != stopEnded && r.reason != stopComplete &&
		r.reason != stopBlocked && r.reason != stopFork {
		if fresh, ferr := getExpedition(e.ID); ferr == nil && fresh != nil &&
			fresh.Status == ExpeditionStatusActive {
			campBlock, campDecision, campPitched = p.maybeAutoCamp(fresh, now)
		}
	}
	_ = autoCampBroken // hint reserved for downstream digest tweaks
	_ = campPitched    // surfaced via campBlock != ""; kept for readability

	// D4-a DM dispatch. The old per-tick auto-walk DM is retired in compact
	// mode: a Night-camp pitch flushes the accumulated day as a digest;
	// every other quiet path stays silent until something interactive fires.
	if body, ok := buildAutoRunDM(e.ID, r, campBlock, campDecision); ok {
		if err := p.SendDM(uid, body); err != nil {
			slog.Warn("expedition: autorun DM", "user", uid, "err", err)
		}
	}

	// Emergence seam: a run-complete reached by the background ticker is
	// still a live emergence — roll pet arrival. See maybeRollPetArrivalOnEmerge.
	// Deferred until after the run-summary DM below so the "animal in your
	// house" prompt lands after the summary, not ahead of it.
	if r.reason == stopComplete {
		p.maybeRollPetArrivalOnEmerge(uid)
	}
	return nil
}

// buildAutoRunDM applies D4-a's surface rules and returns the DM body to
// send. ok=false means the tick is silent. Inputs:
//   - expID: the expedition row id (for the EoD digest log fetch).
//   - r:     walk result, including reason + accumulated stream.
//   - camp:  rendered camp block from maybeAutoCamp / pitchBossSafetyCamp,
//           or "" when no camp was pitched this tick.
//   - dec:   the camp decision; dec.Night is the trigger for the EoD
//           digest variant. Zero-value when no pitch happened.
//
// Surface rules:
//   - stopFork / stopEnded / stopComplete  → render the walk DM. These
//     are the interactive / climax beats and stay their own messages.
//   - Night camp pitched                  → render the EoD digest +
//     camp block. Walk stream is dropped (the digest summarizes the day).
//   - Boss-safety camp pitched            → short hold notice + camp
//     block; walk stream dropped (compact bail was deliberate).
//   - Anything else                       → silent.
func buildAutoRunDM(expID string, r autopilotWalkResult, camp string, dec autoCampDecision) (string, bool) {
	switch r.reason {
	case stopFork, stopEnded, stopComplete:
		body := renderAutoRunWalkDM(r)
		if camp != "" {
			body += camp
		}
		return body, true
	}
	if camp == "" {
		return "", false
	}
	if dec.Night {
		// EoD digest. The camp pitch already bumped current_day in
		// nightRolloverBurn, so the day-that-just-ended is CurrentDay-1.
		// digest is the day rollup, then the camp block lays out the rest.
		fresh, ferr := getExpedition(expID)
		prevDay := 0
		if ferr == nil && fresh != nil {
			prevDay = fresh.CurrentDay - 1
		}
		digest := ""
		if prevDay > 0 {
			digest = renderEndOfDayDigest(expID, prevDay)
		}
		if digest == "" {
			// No structured day yet — fall back to a thin header so the
			// camp block isn't dropped on the player without context.
			digest = "🌙 *The day winds down.*\n\n"
		}
		return digest + camp, true
	}
	if dec.Reason == "boss-safety hold — resting before re-engaging" {
		return "⏸ *Holding before the boss — pitching a rest camp.*\n" + camp, true
	}
	// Non-night auto-camp (mid-day rest / base camp waypoint). Surface a
	// short notice so the player can see the dungeon's decision; full
	// digest waits for the night pitch.
	return camp, true
}

// renderAutoRunWalkDM is the legacy concat-the-stream renderer, kept for
// the surfaces D4-a still DMs (fork / death / run-complete).
func renderAutoRunWalkDM(r autopilotWalkResult) string {
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
