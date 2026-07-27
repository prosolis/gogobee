package plugin

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// Phase 12 E7 — Ambient between-day events. The expedition autopilot's
// "background ticking" pass: while an active expedition is sitting
// between briefings and recaps, the dungeon does its own small things
// every few hours. Most fires are pure flavor (TwinBee narrating a
// dripping sound, a draft, a polite possum); a few apply tiny
// mechanical nudges (±SU, +threat, small coin find, +1 HP if camped).
//
// Design constraints:
//   • At most one event per expedition per ambientCooldown window.
//   • Idempotency via compare-and-set on dnd_expedition.last_ambient_at
//     — survives bot restarts, double-fires, clock skew.
//   • Skip if a turn-based combat session is open: the per-round combat
//     DMs are the player's feed right now; ambient noise would talk
//     over them.
//   • Skip if we're inside the politeness window around the scheduled
//     06:00 briefing or 21:00 recap so we don't pile DMs on top of the
//     big daily message.
//   • Effects are deliberately small — ambient is texture, not balance.

const (
	// ambientCooldown — minimum time between ambient events for one
	// expedition. Tuned so a player offline for a workday sees ~1–2 hits,
	// not a flood, but a multi-day expedition still feels alive.
	ambientCooldown = 6 * time.Hour

	// ambientNearScheduleWindow — skip if we're within this many minutes
	// of a scheduled briefing (06:00 UTC) or recap (21:00 UTC).
	ambientNearScheduleWindow = 60 * time.Minute

	// ambientTickInterval — how often the ticker wakes up to check.
	// The cooldown gate is what actually paces events; the tick just
	// has to be frequent enough that the cooldown is enforced cleanly.
	ambientTickInterval = 5 * time.Minute
)

// expeditionAmbientTicker — fires ambient events every ambientTickInterval.
func (p *AdventurePlugin) expeditionAmbientTicker() {
	ticker := time.NewTicker(ambientTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.fireExpeditionAmbients(time.Now().UTC())
		}
	}
}

// fireExpeditionAmbients walks active expeditions whose last_ambient_at
// is older than ambientCooldown and fires one ambient event per.
func (p *AdventurePlugin) fireExpeditionAmbients(now time.Time) {
	if inAmbientQuietWindow(now) {
		return
	}
	due, err := loadExpeditionsForAmbient(now.Add(-ambientCooldown))
	if err != nil {
		slog.Error("expedition: load ambient candidates", "err", err)
		return
	}
	for _, expID := range due {
		// Re-load each row fresh — the candidate list was a cheap
		// id-only scan, and per-expedition state may have moved.
		e, err := getExpedition(expID)
		if err != nil || e == nil {
			continue
		}
		if e.Status != ExpeditionStatusActive {
			continue
		}
		// Safety net: if the wrapping zone run is abandoned or completed,
		// the player has functionally left the dungeon even though the
		// expedition row hasn't been flipped. Skip silently rather than
		// DM about a place they're no longer in.
		if e.RunID != "" {
			if run, _ := getZoneRun(e.RunID); run != nil && !run.IsActive() {
				continue
			}
		}
		uid := id.UserID(e.UserID)
		if hasActiveCombatSession(uid) {
			continue
		}
		if err := p.deliverAmbient(e, now); err != nil {
			slog.Warn("expedition: ambient deliver", "expedition", e.ID, "err", err)
		}
	}
}

// inAmbientQuietWindow reports whether `now` is inside the politeness
// window around 06:00 UTC (briefing) or 21:00 UTC (recap). Centered on
// the scheduled minute: ±ambientNearScheduleWindow.
func inAmbientQuietWindow(now time.Time) bool {
	briefing := time.Date(now.Year(), now.Month(), now.Day(),
		expeditionBriefingHour, 0, 0, 0, time.UTC)
	recap := time.Date(now.Year(), now.Month(), now.Day(),
		expeditionRecapHour, 0, 0, 0, time.UTC)
	for _, t := range []time.Time{briefing, recap} {
		d := now.Sub(t)
		if d < 0 {
			d = -d
		}
		if d < ambientNearScheduleWindow {
			return true
		}
	}
	return false
}

// loadExpeditionsForAmbient — active expedition IDs whose last_ambient_at
// is NULL or older than the cutoff. Also requires start_date older than
// the cutoff so a freshly-started expedition gets a real beat of silence
// before the dungeon starts chiming in.
func loadExpeditionsForAmbient(cutoff time.Time) ([]string, error) {
	rows, err := db.Get().Query(`
		SELECT expedition_id
		  FROM dnd_expedition
		 WHERE status = 'active'
		   AND start_date < ?
		   AND (last_ambient_at IS NULL OR last_ambient_at < ?)`,
		cutoff, cutoff)
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

// deliverAmbient claims the slot, picks an event, applies its effect,
// DMs the player, and appends a log entry. Idempotent: the CAS update
// short-circuits on rowsAffected == 0 so a double-fire is a no-op.
func (p *AdventurePlugin) deliverAmbient(e *Expedition, now time.Time) error {
	// Pick before claiming the slot so the CAS can persist the new Kind
	// alongside last_ambient_at — that way back-to-back fires for the
	// same expedition (different ticker tick, different rng) read the
	// updated LastAmbientKind on the next load. Picking is pure and
	// cheap; if the CAS loses, we simply discard the picked event.
	ev := pickAmbientEvent(e, defaultAmbientRNG(), e.LastAmbientKind)
	line := flavor.Pick(ev.Pool)
	if line == "" {
		// Pool was empty — degrade to a generic monologue rather than
		// firing an event with no narration.
		ev = ambientEventMonologue()
		line = flavor.Pick(ev.Pool)
	}

	cutoff := now.Add(-ambientCooldown)
	res, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET last_ambient_at = ?,
		       last_ambient_kind = ?,
		       last_activity = ?
		 WHERE expedition_id = ?
		   AND status = 'active'
		   AND (last_ambient_at IS NULL OR last_ambient_at < ?)`,
		now, ev.Kind, now, e.ID, cutoff)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // someone else got there first
	}

	footer := p.applyAmbientEffect(e, ev)

	// Once-a-day cadence: the effect above still lands on schedule, but the
	// DM does not. The log entry below is what the next morning's briefing
	// digest reads back, and the site renders it as it happens.
	summary := fmt.Sprintf("ambient: %s", ev.Kind)
	if footer != "" {
		summary += " — " + footer
	}
	if err := appendExpeditionLog(e.ID, e.CurrentDay, "ambient",
		summary, line); err != nil {
		return err
	}
	return nil
}

// ── Event table ──────────────────────────────────────────────────────────────

// ambientEvent — one entry in the ambient pool. Weight is relative; the
// resolver normalizes over eligible events at pick time.
type ambientEvent struct {
	Kind     string
	Pool     []string
	Weight   int
	Eligible func(*Expedition) bool
}

// ambientEventMonologue — the always-eligible flavor fallback. Pulled
// out so deliverAmbient can degrade to it if a chosen pool is empty.
func ambientEventMonologue() ambientEvent {
	return ambientEvent{
		Kind:     "monologue",
		Pool:     flavor.AmbientMonologue,
		Weight:   35,
		Eligible: func(*Expedition) bool { return true },
	}
}

// ambientEvents returns the full event table. Constructed per-call so
// it's cheap to test in isolation and free of package-level state.
func ambientEvents() []ambientEvent {
	always := func(*Expedition) bool { return true }
	events := []ambientEvent{
		ambientEventMonologue(),
		{
			Kind:     "small_find",
			Pool:     flavor.AmbientSmallFind,
			Weight:   10,
			Eligible: always,
		},
		{
			Kind:     "noise",
			Pool:     flavor.AmbientNoise,
			Weight:   15,
			Eligible: always,
		},
		{
			Kind:   "pack_rat",
			Pool:   flavor.AmbientPackRat,
			Weight: 12,
			// Pack rats need something to nibble. If supplies are
			// already at the floor, skip — no point in the player
			// getting a "you lost 0.0 SU" DM.
			Eligible: func(e *Expedition) bool {
				return e.Supplies.Current >= 1
			},
		},
		{
			Kind:     "lucky_find",
			Pool:     flavor.AmbientLuckyFind,
			Weight:   10,
			Eligible: always,
		},
		{
			Kind:   "camp_visitor",
			Pool:   flavor.AmbientCampVisitor,
			Weight: 8,
			Eligible: func(e *Expedition) bool {
				return e.Camp != nil && e.Camp.Active
			},
		},
		{
			Kind:   "faction_whisper",
			Pool:   flavor.AmbientFactionWhisper,
			Weight: 10,
			// Whispers from a faction that doesn't know you exist
			// land flat. Gate behind a real threat level.
			Eligible: func(e *Expedition) bool {
				return e.ThreatLevel >= 30 && !e.SiegeMode
			},
		},
	}
	// N7/E4 — a live season adds a themed road visitor. Non-combat: it leaves a
	// keepsake and a coin gift, resolved in applyAmbientEffect. The pool and
	// reward come from activeSeason(); the ambient seam is production-only, so
	// this never reaches the balance sim.
	if s, ok := activeSeason(); ok && len(s.Visitor.FlavorPool) > 0 {
		events = append(events, ambientEvent{
			Kind:     "season_visitor",
			Pool:     s.Visitor.FlavorPool,
			Weight:   s.Visitor.Weight,
			Eligible: func(*Expedition) bool { return true },
		})
	}
	return events
}

// pickAmbientEvent runs a weighted pick over eligible events. avoidKind
// is the Kind of the previous fire (or "" if none); when a pick lands on
// that same Kind, we re-roll once. We only re-roll once so the bias is a
// soft nudge — if the user is in a state where most weight sits on one
// Kind (e.g. starving cuts pack_rat → camped + low threat narrows the
// table further) we still let it repeat rather than loop forever.
// Exposed for tests; the rng parameter lets them seed deterministically.
func pickAmbientEvent(e *Expedition, rng *rand.Rand, avoidKind string) ambientEvent {
	first := weightedPickAmbient(e, rng)
	if avoidKind == "" || first.Kind != avoidKind {
		return first
	}
	return weightedPickAmbient(e, rng)
}

// weightedPickAmbient is the underlying weighted-pick over eligible events.
func weightedPickAmbient(e *Expedition, rng *rand.Rand) ambientEvent {
	events := ambientEvents()
	total := 0
	for _, ev := range events {
		if ev.Eligible(e) {
			total += ev.Weight
		}
	}
	if total == 0 {
		return ambientEventMonologue()
	}
	r := rng.IntN(total)
	for _, ev := range events {
		if !ev.Eligible(e) {
			continue
		}
		if r < ev.Weight {
			return ev
		}
		r -= ev.Weight
	}
	return ambientEventMonologue() // unreachable barring float drift
}

func defaultAmbientRNG() *rand.Rand {
	return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
}

// ── Effects ──────────────────────────────────────────────────────────────────

// applyAmbientEffect mutates expedition state per event Kind and returns a
// terse footer string for the DM ("Supplies −0.3", "+3 coins", "Threat +1").
// Pure-flavor kinds return "".
func (p *AdventurePlugin) applyAmbientEffect(e *Expedition, ev ambientEvent) string {
	rng := defaultAmbientRNG()
	switch ev.Kind {
	case "monologue", "small_find":
		return ""
	case "noise":
		if err := applyThreatDelta(e.ID, 1, "ambient: distant noise"); err != nil {
			slog.Warn("expedition: ambient threat delta", "expedition", e.ID, "err", err)
		}
		// Verb-form footer — "Threat +N" exposed the hidden meter.
		return "The dungeon listens a little harder."
	case "faction_whisper":
		if err := applyThreatDelta(e.ID, 2, "ambient: faction whisper"); err != nil {
			slog.Warn("expedition: ambient threat delta", "expedition", e.ID, "err", err)
		}
		return "Something out there is paying attention now."
	case "pack_rat":
		drain := float32(0.2) + float32(rng.IntN(2))*float32(0.1) // 0.2 or 0.3
		if _, err := p.withExpeditionSupplies(e.ID, func(fresh *Expedition) (ExpeditionSupplies, error) {
			ns := fresh.Supplies
			ns.Current -= drain
			if ns.Current < 0 {
				ns.Current = 0
			}
			return ns, nil
		}); err != nil {
			slog.Warn("expedition: ambient supply drain", "expedition", e.ID, "err", err)
			return ""
		}
		// Don't surface the raw SU number — it's a hidden meter and
		// "SU" doesn't appear in player vocab anywhere else.
		_ = drain
		return "A little less in the pack than there was."
	case "lucky_find":
		coins := 1 + rng.IntN(4) // 1d4
		if p.euro != nil {
			p.euro.Credit(id.UserID(e.UserID), float64(coins), "expedition ambient: lucky find")
		}
		// Mirror into coins_earned so the post-expedition tally is honest.
		if _, err := db.Get().Exec(`
			UPDATE dnd_expedition
			   SET coins_earned = coins_earned + ?,
			       last_activity = CURRENT_TIMESTAMP
			 WHERE expedition_id = ?`, coins, e.ID); err != nil {
			slog.Warn("expedition: ambient coin tally", "expedition", e.ID, "err", err)
		}
		return fmt.Sprintf("+%d coins", coins)
	case "camp_visitor":
		// Tiny morale-style HP +1, clipped at max. Only fires when
		// camped (gated in Eligible). No-op if already full.
		uid := id.UserID(e.UserID)
		c, err := LoadDnDCharacter(uid)
		if err != nil || c == nil {
			return ""
		}
		if c.HPCurrent >= c.HPMax {
			return ""
		}
		c.HPCurrent++
		if err := SaveDnDCharacter(c); err != nil {
			slog.Warn("expedition: ambient HP nudge", "user", uid, "err", err)
			return ""
		}
		return "+1 HP"
	case "season_visitor":
		// N7/E4 — the visitor leaves a sellable keepsake and a coin gift. Re-read
		// the season so the reward matches the current window even if it turned
		// over between pick and apply; bail quietly if it just ended.
		s, ok := activeSeason()
		if !ok {
			return ""
		}
		v := s.Visitor
		uid := id.UserID(e.UserID)
		if err := addAdvInventoryItem(uid, AdvItem{
			Name:  v.Keepsake,
			Type:  "trophy",
			Tier:  1,
			Value: v.KeepsakeValue,
		}); err != nil {
			slog.Warn("expedition: ambient season keepsake", "user", uid, "err", err)
			return ""
		}
		if v.Coins > 0 {
			if p.euro != nil {
				p.euro.Credit(uid, float64(v.Coins), "expedition ambient: "+s.Key+" visitor")
			}
			if _, err := db.Get().Exec(`
				UPDATE dnd_expedition
				   SET coins_earned = coins_earned + ?,
				       last_activity = CURRENT_TIMESTAMP
				 WHERE expedition_id = ?`, v.Coins, e.ID); err != nil {
				slog.Warn("expedition: ambient season coin tally", "expedition", e.ID, "err", err)
			}
			return fmt.Sprintf("You keep the %s. +%d coins", v.Keepsake, v.Coins)
		}
		return fmt.Sprintf("You keep the %s.", v.Keepsake)
	}
	return ""
}

// renderAmbientDM builds the DM body. Short header so it doesn't look
// like a briefing, then the flavor line, then a one-line footer when
// there was a mechanical effect.
func renderAmbientDM(e *Expedition, ev ambientEvent, line, footer string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🌫 *Day %d — between briefings*\n\n", e.CurrentDay))
	b.WriteString(line)
	if footer != "" {
		b.WriteString("\n\n_")
		b.WriteString(footer)
		b.WriteString("_")
	}
	return b.String()
}

// ── Test seam — sql.NullTime helper kept here so tests can compose ──────────

// expeditionAmbientStamp returns the last_ambient_at value for a row, or
// (zero, false) if NULL. Used in tests to assert idempotency.
func expeditionAmbientStamp(expID string) (time.Time, bool, error) {
	var raw sql.NullTime
	err := db.Get().QueryRow(`
		SELECT last_ambient_at FROM dnd_expedition WHERE expedition_id = ?`,
		expID).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	if !raw.Valid {
		return time.Time{}, false, nil
	}
	return raw.Time, true, nil
}
