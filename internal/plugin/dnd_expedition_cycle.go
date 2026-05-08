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
)

// expeditionBriefingTicker — 06:00 UTC daily briefing.
func (p *AdventurePlugin) expeditionBriefingTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now().UTC()
		if now.Hour() != expeditionBriefingHour || now.Minute() != 0 {
			continue
		}
		p.fireExpeditionBriefings(now)
	}
}

// expeditionRecapTicker — 21:00 UTC daily recap.
func (p *AdventurePlugin) expeditionRecapTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now().UTC()
		if now.Hour() != expeditionRecapHour || now.Minute() != 0 {
			continue
		}
		p.fireExpeditionRecaps(now)
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
		       last_briefing_at, last_recap_at, last_activity, completed_at
		  FROM dnd_expedition
		 WHERE status IN ('active', 'extracting')
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
		       last_briefing_at, last_recap_at, last_activity, completed_at
		  FROM dnd_expedition
		 WHERE status IN ('active', 'extracting')
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

// deliverBriefing rolls a day forward, applies supply burn, posts the
// morning briefing DM, appends a log entry, and stamps last_briefing_at.
func (p *AdventurePlugin) deliverBriefing(e *Expedition, now time.Time) error {
	// Advance day + supply burn happen together at the morning rollover.
	harsh := e.ThreatLevel > 60 || e.TemporalStack > 0
	newSupplies, burn := applyDailyBurn(e.Supplies, harsh, e.SiegeMode)
	if err := updateSupplies(e.ID, newSupplies); err != nil {
		return err
	}
	if err := advanceExpeditionDay(e.ID); err != nil {
		return err
	}
	e.Supplies = newSupplies
	e.CurrentDay++

	// E2a: daily threat drift (+3 base, GM-mood modifier). No-op after
	// boss kill. May cross a threshold and append a flavor-bearing log.
	if _, _, err := applyDailyThreatDrift(e); err != nil {
		slog.Warn("expedition: threat drift", "expedition", e.ID, "err", err)
	}

	line := pickMorningBriefing(e.CurrentDay)
	body := renderMorningBriefing(e, line, burn)

	if uid := id.UserID(e.UserID); uid != "" {
		if err := p.SendDM(uid, body); err != nil {
			slog.Warn("expedition: send briefing DM", "user", uid, "err", err)
		}
	}
	if err := appendExpeditionLog(e.ID, e.CurrentDay, "briefing",
		fmt.Sprintf("morning briefing — %.1f SU consumed overnight", burn), line); err != nil {
		return err
	}
	_, err := db.Get().Exec(`
		UPDATE dnd_expedition
		   SET last_briefing_at = ?,
		       last_activity = ?
		 WHERE expedition_id = ?`, now, now, e.ID)
	return err
}

// deliverRecap posts the evening recap DM, appends a log entry, and stamps
// last_recap_at. No supply burn here — that's the briefing's job.
func (p *AdventurePlugin) deliverRecap(e *Expedition, now time.Time) error {
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
	_, err = db.Get().Exec(`
		UPDATE dnd_expedition
		   SET last_recap_at = ?,
		       last_activity = ?
		 WHERE expedition_id = ?`, now, now, e.ID)
	return err
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
