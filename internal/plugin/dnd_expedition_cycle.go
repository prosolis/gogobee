package plugin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/flavor"
	"maunium.net/go/mautrix/id"
)

// Phase 12 E1d — real-time day cycle engine.
// Spec: gogobee_expedition_system.md §3, §12.
//
// Two tickers run at 1-minute granularity: briefings fire once per UTC day
// at 06:00, recaps once per UTC day at 21:00. Each tick walks all active
// expeditions and fires for any whose last_briefing_at / last_recap_at is
// stale relative to today's threshold. Per-expedition gating (rather than
// global JobCompleted) lets a player who started mid-day still receive a
// briefing the next morning even if some other expedition already triggered
// the global ticker.
//
// Day rollover: the briefing also bumps current_day by one and applies one
// day's supply burn. Day 1's briefing fires the first morning *after* the
// expedition begins (so an expedition started at 23:00 doesn't immediately
// flip to Day 2 at midnight; the spec treats Day 1 as the calendar day
// of departure).

const (
	expeditionBriefingHour = 6
	expeditionRecapHour    = 21

	// nightSafetyNet — if an event-anchored expedition's last rollover is
	// older than this, the 06:00 UTC briefing ticker force-fires
	// processNightCamp itself. Without this, an expedition whose autopilot
	// stalled (long combat, missed tick, manual halt) would drift forever
	// without burning supplies or advancing days. 28h gives the autopilot
	// 4h of slop past a normal day before we step in.
	nightSafetyNet = 28 * time.Hour
)

// eventAnchoredCutoff — expeditions started at or after this timestamp
// use the D2-b event-anchored day-rollover model: day++/burn/threat-drift
// fire when the autopilot (or a player !camp) pitches a night camp, and
// the 06:00 UTC briefing becomes a re-engagement DM with a safety-net
// force. Expeditions started before this stay on the legacy UTC-anchored
// briefing rollover until they end.
var eventAnchoredCutoff = time.Date(2026, 5, 27, 18, 0, 0, 0, time.UTC)

// isEventAnchored — true when this expedition uses the D2-b model.
func isEventAnchored(e *Expedition) bool {
	if e == nil {
		return false
	}
	return !e.StartDate.Before(eventAnchoredCutoff)
}

// nightRolloverResult — the side effects processNightCamp produced, so
// callers (autopilot pitch, manual !camp, safety-net briefing) can render
// the same numbers into their own DM block.
type nightRolloverResult struct {
	Burn           float32
	TemporalLines  []string
	MilestoneLines []string
	Starved        bool
}

// nightRolloverBurn — stage 1 of the day rollover: zone temporal pre-burn
// + daily supply burn + current_day++. Returns the burn applied. Callers
// follow this with applyCampRest (if pitching) and then nightRolloverDrift
// to finish the rollover; legacy deliverBriefing interleaves processOvernightCamp
// between the two so a fortified camp's −5 lands before drift's +3.
func (p *AdventurePlugin) nightRolloverBurn(e *Expedition) (float32, error) {
	burnOverride := applyZoneTemporalPreBurn(e, e.CurrentDay+1)
	var newSupplies ExpeditionSupplies
	var burn float32
	if burnOverride.Multiplier > 0 {
		burn = e.Supplies.DailyBurn * burnOverride.Multiplier * float32(phase5BDailyBurnRatePct) / 100
		newSupplies = e.Supplies
		newSupplies.Current -= burn
		if newSupplies.Current < 0 {
			newSupplies.Current = 0
		}
		newSupplies.ForagedToday = false
	} else {
		harsh := e.ThreatLevel > 60 || zoneTemporalHarsh(e)
		newSupplies, burn = applyDailyBurn(e.Supplies, harsh, e.SiegeMode)
	}
	if err := updateSupplies(e.ID, newSupplies); err != nil {
		return 0, err
	}
	if err := advanceExpeditionDay(e.ID); err != nil {
		return 0, err
	}
	e.Supplies = newSupplies
	e.CurrentDay++
	return burn, nil
}

// nightRolloverDrift — stage 2: daily threat drift, zone temporal post-
// rollover, starvation check, max-threat record, milestones. Stamps
// last_briefing_at = now so the UTC briefing ticker treats today as
// already-rolled. `now` is the wallclock to stamp; callers that already
// did the stamp via a CAS (deliverBriefing) pass time.Time{} to skip.
func (p *AdventurePlugin) nightRolloverDrift(e *Expedition, now time.Time) nightRolloverResult {
	var out nightRolloverResult
	if _, _, err := applyDailyThreatDrift(e); err != nil {
		slog.Warn("expedition: threat drift", "expedition", e.ID, "err", err)
	}
	out.TemporalLines = applyZoneTemporalPostRollover(e)

	if supplyDepletion(e.Supplies) == SupplyStarvation && e.Status == ExpeditionStatusActive {
		_, _, _ = forcedExtractExpedition(e.ID, "starvation")
		e.Status = ExpeditionStatusAbandoned
		line := flavor.Pick(flavor.ExtractionForced)
		_ = appendExpeditionLog(e.ID, e.CurrentDay, "narrative",
			"forced extraction: starvation", line)
		out.Starved = true
	}
	if e.Status == ExpeditionStatusAbandoned && p.euro != nil {
		tax := int(float64(e.CoinsEarned) * forcedExtractCoinTaxFrac)
		if tax > 0 {
			p.euro.Debit(id.UserID(e.UserID), float64(tax),
				"forced extraction tax")
		}
	}
	_ = recordMaxThreat(e)
	out.MilestoneLines = p.checkDailyMilestones(e)

	if !now.IsZero() {
		if _, err := db.Get().Exec(`
			UPDATE dnd_expedition
			   SET last_briefing_at = ?
			 WHERE expedition_id = ?`, now, e.ID); err == nil {
			e.LastBriefingAt = &now
		}
	}
	return out
}

// processNightCamp — burn + drift in one go, no rest in between. Used by
// callers (autopilot pitch, manual !camp, event-anchored safety net) that
// either apply their own rest separately or don't apply one at all.
func (p *AdventurePlugin) processNightCamp(e *Expedition) (nightRolloverResult, error) {
	burn, err := p.nightRolloverBurn(e)
	if err != nil {
		return nightRolloverResult{}, err
	}
	out := p.nightRolloverDrift(e, time.Now().UTC())
	out.Burn = burn
	return out, nil
}

// expeditionBriefingTicker — 06:00 UTC daily briefing.
func (p *AdventurePlugin) expeditionBriefingTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			now := time.Now().UTC()
			if now.Hour() != expeditionBriefingHour || now.Minute() != 0 {
				continue
			}
			p.fireExpeditionBriefings(now)
		}
	}
}

// expeditionRecapTicker — 21:00 UTC daily recap.
func (p *AdventurePlugin) expeditionRecapTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			now := time.Now().UTC()
			if now.Hour() != expeditionRecapHour || now.Minute() != 0 {
				continue
			}
			p.fireExpeditionRecaps(now)
		}
	}
}

// fireExpeditionBriefings finds every active expedition without a briefing
// for today's UTC date and fires one per. Public-on-package for testing.
func (p *AdventurePlugin) fireExpeditionBriefings(now time.Time) {
	threshold := time.Date(now.Year(), now.Month(), now.Day(),
		expeditionBriefingHour, 0, 0, 0, time.UTC)
	exps, err := loadExpeditionsNeedingBriefing(threshold)
	if err != nil {
		slog.Error("expedition: load briefings", "err", err)
		return
	}
	for _, e := range exps {
		// Don't roll the expedition day forward into a live turn-based fight:
		// an active combat session locks the run, and the briefing burns
		// supply / advances the day / processes overnight camp. last_briefing_at
		// stays stale, so the rollover simply lands on the next 06:00 tick —
		// the expedition holds on its current day for one extra real day, which
		// is mild and player-favorable versus mutating a locked run.
		if hasActiveCombatSession(id.UserID(e.UserID)) {
			slog.Info("expedition: briefing deferred — player in combat session", "expedition", e.ID, "user", e.UserID)
			continue
		}
		if err := p.deliverBriefing(e, now); err != nil {
			slog.Error("expedition: briefing", "expedition", e.ID, "err", err)
		}
	}
}

// fireExpeditionRecaps finds every active expedition without a recap for
// today's UTC date and fires one per.
func (p *AdventurePlugin) fireExpeditionRecaps(now time.Time) {
	threshold := time.Date(now.Year(), now.Month(), now.Day(),
		expeditionRecapHour, 0, 0, 0, time.UTC)
	exps, err := loadExpeditionsNeedingRecap(threshold)
	if err != nil {
		slog.Error("expedition: load recaps", "err", err)
		return
	}
	for _, e := range exps {
		// Same guard as briefings: the recap runs the night wandering check,
		// which can bump threat. Don't mutate a run locked by a live fight —
		// last_recap_at stays stale and the recap lands on the next tick.
		if hasActiveCombatSession(id.UserID(e.UserID)) {
			slog.Info("expedition: recap deferred — player in combat session", "expedition", e.ID, "user", e.UserID)
			continue
		}
		if err := p.deliverRecap(e, now); err != nil {
			slog.Error("expedition: recap", "expedition", e.ID, "err", err)
		}
	}
}

// loadExpeditionsNeedingBriefing — active expeditions whose last_briefing_at
// is NULL or strictly before today's 06:00 UTC.
func loadExpeditionsNeedingBriefing(threshold time.Time) ([]*Expedition, error) {
	rows, err := db.Get().Query(`
		SELECT expedition_id, user_id, zone_id, run_id, status,
		       start_date, current_day, current_region, boss_defeated,
		       supplies_json, camp_json, threat_level, threat_siege,
		       threat_events, temporal_stack, region_state,
		       xp_earned, coins_earned, gm_mood,
		       last_briefing_at, last_recap_at, last_ambient_kind,
		       last_activity, completed_at
		  FROM dnd_expedition
		 WHERE status = 'active'
		   AND start_date < ?
		   AND (last_briefing_at IS NULL OR last_briefing_at < ?)`,
		threshold, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExpeditionRows(rows)
}

// loadExpeditionsNeedingRecap — same but for the 21:00 threshold.
func loadExpeditionsNeedingRecap(threshold time.Time) ([]*Expedition, error) {
	rows, err := db.Get().Query(`
		SELECT expedition_id, user_id, zone_id, run_id, status,
		       start_date, current_day, current_region, boss_defeated,
		       supplies_json, camp_json, threat_level, threat_siege,
		       threat_events, temporal_stack, region_state,
		       xp_earned, coins_earned, gm_mood,
		       last_briefing_at, last_recap_at, last_ambient_kind,
		       last_activity, completed_at
		  FROM dnd_expedition
		 WHERE status = 'active'
		   AND (last_recap_at IS NULL OR last_recap_at < ?)`, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExpeditionRows(rows)
}

func scanExpeditionRows(rows *sql.Rows) ([]*Expedition, error) {
	var out []*Expedition
	for rows.Next() {
		e, err := scanExpedition(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// deliverBriefing posts the morning briefing DM. For legacy UTC-anchored
// expeditions it also drives the day rollover (supply burn, day++, threat
// drift) via processNightCamp. For event-anchored expeditions (D2-b) the
// rollover is owned by the autopilot's night-camp pitch; the briefing
// ticker only re-engages the player and force-fires the rollover after a
// safety-net window.
//
// Idempotency: atomic compare-and-set on last_briefing_at gates the body.
// A double-fire on the same expedition is a no-op.
func (p *AdventurePlugin) deliverBriefing(e *Expedition, now time.Time) error {
	priorBriefing := e.LastBriefingAt
	threshold := time.Date(now.Year(), now.Month(), now.Day(),
		expeditionBriefingHour, 0, 0, 0, time.UTC)
	res, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET last_briefing_at = ?,
		       last_activity = ?
		 WHERE expedition_id = ?
		   AND status = 'active'
		   AND (last_briefing_at IS NULL OR last_briefing_at < ?)`,
		now, now, e.ID, threshold)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // already delivered for this day
	}
	e.LastBriefingAt = &now

	// D2-b: event-anchored expeditions own the day rollover via the
	// autopilot night camp. The 06:00 ticker either posts a re-engagement
	// DM (rollover happened recently) or force-fires processNightCamp
	// itself (safety net for stalled autopilots).
	if isEventAnchored(e) {
		return p.deliverBriefingEventAnchored(e, priorBriefing)
	}

	burn, err := p.nightRolloverBurn(e)
	if err != nil {
		return err
	}

	// E2d: overnight camp rest. Runs after burn/day++ but before drift so
	// a fortified camp's −5 lands first and a same-day +3 drift can't push
	// back over a threshold the rest just dropped.
	restSummary := processOvernightCamp(e)
	if restSummary != "" {
		if fresh, err := getExpedition(e.ID); err == nil && fresh != nil {
			e.ThreatLevel = fresh.ThreatLevel
			e.SiegeMode = fresh.SiegeMode
			e.Camp = fresh.Camp
		}
	}

	// Pass time.Time{} — the CAS at the top of deliverBriefing already
	// stamped last_briefing_at with the synthetic now; don't overwrite it.
	roll := p.nightRolloverDrift(e, time.Time{})
	roll.Burn = burn

	line := pickMorningBriefing(e.CurrentDay)
	body := renderMorningBriefing(e, line, burn)
	if restSummary != "" {
		body += "\n💤 _" + restSummary + "_\n"
	}
	for _, tl := range roll.TemporalLines {
		body += "\n🌀 " + tl + "\n"
	}
	for _, ml := range roll.MilestoneLines {
		body += "\n" + ml
	}

	if uid := id.UserID(e.UserID); uid != "" {
		// The legacy overworld morning DM is skipped while underground, so
		// its 25% morning pet event fires here instead, granting the one-day
		// defense buff (reset at midnight via resetAllPetMorningDefense).
		// Pet *arrival* is handled separately on the emergence seam below —
		// not queued here — so we only roll the morning event.
		if pet, perr := loadPetState(uid); perr == nil {
			if petEvent := petMorningEvent(pet); petEvent != "" {
				if char, cerr := loadAdvCharacter(uid); cerr == nil {
					char.PetMorningDefense = true
					if serr := saveAdvCharacter(char); serr != nil {
						slog.Warn("expedition: save pet morning defense", "user", uid, "err", serr)
					}
				}
				body = fmt.Sprintf("🐾 *%s*\n\n%s", petEvent, body)
			}
		}
		if err := p.SendDM(uid, body); err != nil {
			slog.Warn("expedition: send briefing DM", "user", uid, "err", err)
		}
		// Emergence seam: a briefing-time forced extraction (starvation /
		// abyss collapse) surfaces the player alive — roll pet arrival.
		// Combat/patrol deaths never reach deliverBriefing (the row is
		// already abandoned), so an abandoned status here means a survived
		// emergence; those death paths roll on respawn instead.
		if e.Status == ExpeditionStatusAbandoned {
			p.maybeRollPetArrivalOnEmerge(uid)
		}
	}
	if err := appendExpeditionLog(e.ID, e.CurrentDay, "briefing",
		fmt.Sprintf("morning briefing — %.1f SU consumed overnight", burn), line); err != nil {
		return err
	}
	return nil
}

// deliverBriefingEventAnchored — D2-b 06:00 UTC ticker for event-anchored
// expeditions. The autopilot's night-camp pitch owns day++/burn/threat-
// drift; the ticker just re-engages the player. If the autopilot has
// stalled past nightSafetyNet we force-fire processNightCamp ourselves so
// the expedition doesn't sit frozen.
//
// priorBriefing is the last_briefing_at value as of entry into deliverBriefing
// (before the CAS clobbered it). nil means day-1 or genuinely never rolled.
func (p *AdventurePlugin) deliverBriefingEventAnchored(e *Expedition, priorBriefing *time.Time) error {
	now := time.Now().UTC()
	var since time.Duration
	if priorBriefing != nil {
		since = now.Sub(*priorBriefing)
	} else {
		since = now.Sub(e.StartDate)
	}
	forced := since > nightSafetyNet

	var (
		burn          float32
		temporalLines []string
		mileLines     []string
	)
	if forced {
		roll, err := p.processNightCamp(e)
		if err != nil {
			return err
		}
		burn = roll.Burn
		temporalLines = roll.TemporalLines
		mileLines = roll.MilestoneLines
	}

	line := pickMorningBriefing(e.CurrentDay)
	body := renderMorningBriefing(e, line, burn)
	if forced {
		body += "\n_The autopilot stalled overnight; the day rolled over without rest._\n"
	}
	for _, tl := range temporalLines {
		body += "\n🌀 " + tl + "\n"
	}
	for _, ml := range mileLines {
		body += "\n" + ml
	}

	if uid := id.UserID(e.UserID); uid != "" {
		if pet, perr := loadPetState(uid); perr == nil {
			if petEvent := petMorningEvent(pet); petEvent != "" {
				if char, cerr := loadAdvCharacter(uid); cerr == nil {
					char.PetMorningDefense = true
					if serr := saveAdvCharacter(char); serr != nil {
						slog.Warn("expedition: save pet morning defense", "user", uid, "err", serr)
					}
				}
				body = fmt.Sprintf("🐾 *%s*\n\n%s", petEvent, body)
			}
		}
		if err := p.SendDM(uid, body); err != nil {
			slog.Warn("expedition: send briefing DM", "user", uid, "err", err)
		}
		if forced && e.Status == ExpeditionStatusAbandoned {
			p.maybeRollPetArrivalOnEmerge(uid)
		}
	}
	summary := "morning re-engagement"
	if forced {
		summary = fmt.Sprintf("safety-net rollover — %.1f SU consumed overnight", burn)
	}
	if err := appendExpeditionLog(e.ID, e.CurrentDay, "briefing", summary, line); err != nil {
		return err
	}
	return nil
}

// deliverRecap posts the evening recap DM, appends a log entry, and stamps
// last_recap_at. No supply burn here — that's the briefing's job.
// Idempotency: same pattern as deliverBriefing — claim last_recap_at first.
func (p *AdventurePlugin) deliverRecap(e *Expedition, now time.Time) error {
	threshold := time.Date(now.Year(), now.Month(), now.Day(),
		expeditionRecapHour, 0, 0, 0, time.UTC)
	res, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET last_recap_at = ?,
		       last_activity = ?
		 WHERE expedition_id = ?
		   AND status = 'active'
		   AND (last_recap_at IS NULL OR last_recap_at < ?)`,
		now, now, e.ID, threshold)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}

	// E2b: night phase wandering check fires before the recap so its
	// outcome is part of today's log when the recap renders.
	var night *NightCheck
	if e.Camp != nil && e.Camp.Active {
		c, _ := LoadDnDCharacter(id.UserID(e.UserID))
		var charClass DnDClass
		if c != nil {
			charClass = c.Class
		}
		nc := resolveWanderingCheck(e, charClass, nil)
		if err := processNightCheck(e, nc); err != nil {
			slog.Warn("expedition: night check", "expedition", e.ID, "err", err)
		}
		night = &nc
		// §7.4: Feywild double-day fires an extra wandering check.
		if e.ZoneID == ZoneFeywildCrossing {
			if today, _ := e.RegionState["feywild_today"].(string); today == string(FeywildDistortionDouble) {
				extra := resolveWanderingCheck(e, charClass, nil)
				if err := processNightCheck(e, extra); err != nil {
					slog.Warn("expedition: feywild extra night check", "expedition", e.ID, "err", err)
				}
			}
		}
		// Refresh in-memory threat after possible signs-of-passage bump.
		if fresh, err := getExpedition(e.ID); err == nil && fresh != nil {
			e.ThreatLevel = fresh.ThreatLevel
			e.SiegeMode = fresh.SiegeMode
		}
	}

	dayEntries, err := dayLogEntries(e.ID, e.CurrentDay)
	if err != nil {
		return err
	}
	line := pickEveningRecap(e, dayEntries)
	body := renderEveningRecap(e, line, dayEntries)
	if night != nil {
		body += "\n" + renderNightCheck(*night)
	}

	if uid := id.UserID(e.UserID); uid != "" {
		if err := p.SendDM(uid, body); err != nil {
			slog.Warn("expedition: send recap DM", "user", uid, "err", err)
		}
	}
	if err := appendExpeditionLog(e.ID, e.CurrentDay, "recap",
		fmt.Sprintf("evening recap — %d log entries today", len(dayEntries)), line); err != nil {
		return err
	}
	return nil
}

// pickMorningBriefing returns a flavor line based on day-band: Day 1, 3, 7,
// 14, 21 each have their own pool; everything else uses the generic pool.
func pickMorningBriefing(day int) string {
	var pool []string
	switch day {
	case 1:
		pool = flavor.MorningBriefingDay1
	case 3:
		pool = flavor.MorningBriefingDay3
	case 7:
		pool = flavor.MorningBriefingDay7
	case 14:
		pool = flavor.MorningBriefingDay14
	case 21:
		pool = flavor.MorningBriefingDay21
	default:
		pool = flavor.MorningBriefingGeneric
	}
	if line := flavor.Pick(pool); line != "" {
		return line
	}
	return flavor.Pick(flavor.MorningBriefingGeneric)
}

// pickEveningRecap chooses a recap line based on what happened today.
// Boss kill > nothing-happened > generic. Close-call detection isn't wired
// in E1d (no HP-delta tracking yet); deferred to E2 alongside the threat
// clock combat hooks.
func pickEveningRecap(e *Expedition, today []ExpeditionEntry) string {
	bossKilled := false
	combatCount := 0
	for _, en := range today {
		if en.Type == "combat" {
			combatCount++
		}
		if en.Type == "boss" || strings.Contains(strings.ToLower(en.Summary), "boss defeated") {
			bossKilled = true
		}
	}
	if bossKilled {
		if l := flavor.Pick(flavor.EveningRecapBossKilled); l != "" {
			return l
		}
	}
	// "Nothing happened" = no combat, no narrative-action entries today.
	if combatCount == 0 && len(today) <= 1 {
		if l := flavor.Pick(flavor.EveningRecapNothingHappened); l != "" {
			return l
		}
	}
	return flavor.Pick(flavor.EveningRecapGeneric)
}

// dayLogEntries returns log entries for a specific expedition-day, oldest first.
func dayLogEntries(expID string, day int) ([]ExpeditionEntry, error) {
	rows, err := db.Get().Query(`
		SELECT entry_id, expedition_id, day, timestamp, entry_type, summary, flavor
		  FROM dnd_expedition_log
		 WHERE expedition_id = ? AND day = ?
		 ORDER BY timestamp ASC, entry_id ASC`, expID, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpeditionEntry
	for rows.Next() {
		var e ExpeditionEntry
		if err := rows.Scan(
			&e.EntryID, &e.ExpeditionID, &e.Day, &e.Timestamp,
			&e.Type, &e.Summary, &e.Flavor,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// renderMorningBriefing builds the §12.1 block.
func renderMorningBriefing(e *Expedition, line string, burnConsumed float32) string {
	zone, _ := getZone(e.ZoneID)
	burn := currentBurn(e)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🎭 **TwinBee — Morning Briefing, Day %d**\n\n", e.CurrentDay))
	b.WriteString(fmt.Sprintf("📍 **Zone:** %s _(T%d)_\n", zone.Display, int(zone.Tier)))
	b.WriteString(fmt.Sprintf("🎒 **Supplies:** %.1f / %.1f SU  _(burned %.1f overnight, ~%d days left)_\n",
		e.Supplies.Current, e.Supplies.Max, burnConsumed, estimateDays(e.Supplies.Current, burn)))
	b.WriteString(fmt.Sprintf("⏰ **Threat:** %d / 100 — %s\n",
		e.ThreatLevel, threatThresholdLabel(e.ThreatLevel, e.SiegeMode)))
	if e.TemporalStack != 0 {
		b.WriteString(fmt.Sprintf("🌡 **Zone stack:** %d\n", e.TemporalStack))
	}
	if state := supplyDepletion(e.Supplies); state != SupplyNormal {
		b.WriteString(fmt.Sprintf("⚠ **%s** (roll %d)\n",
			depletionLabel(state), supplyRollModifier(state)))
	}
	if line != "" {
		b.WriteString("\n")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// renderEveningRecap builds the §12.2 block.
func renderEveningRecap(e *Expedition, line string, today []ExpeditionEntry) string {
	zone, _ := getZone(e.ZoneID)
	combatCount := 0
	for _, en := range today {
		if en.Type == "combat" {
			combatCount++
		}
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🎭 **TwinBee — Evening Recap, Day %d**\n\n", e.CurrentDay))
	b.WriteString(fmt.Sprintf("📍 **Zone:** %s\n", zone.Display))
	b.WriteString(fmt.Sprintf("🎒 **Supplies:** %.1f / %.1f SU\n", e.Supplies.Current, e.Supplies.Max))
	b.WriteString(fmt.Sprintf("⚔ **Combat encounters today:** %d\n", combatCount))
	b.WriteString(fmt.Sprintf("⏰ **Threat:** %d / 100 — %s\n",
		e.ThreatLevel, threatThresholdLabel(e.ThreatLevel, e.SiegeMode)))
	if e.Camp == nil || !e.Camp.Active {
		b.WriteString("\n_You haven't camped tonight. The dungeon does not._\n")
	}
	if line != "" {
		b.WriteString("\n")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
