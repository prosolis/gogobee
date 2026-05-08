package plugin

import (
	"fmt"
	"hash/fnv"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/flavor"
)

// Phase 11 D1d — TwinBee narration + GM mood triggers. Implements
// `gogobee_dungeon_zones.md` §3 (TwinBee GM System) on top of the D1b
// state machine.
//
// Flavor lines come from the canonical pools in internal/flavor
// (twinbee_gm_flavor.go) — this file does not author new prose.
// Per-zone overrides are routed through zoneRoomEntryPool / bossEntryPool.
//
// Mood-banded *flavor* selection is a D6 polish item. D1d wires the
// score, the event triggers, and the passive decay; mood currently
// surfaces as the human label in `!zone status` and gates downstream
// generosity (loot rolls, hint drops) once those land.

// GMNarrationType — see design doc §7.
type GMNarrationType string

const (
	GMRoomEntry    GMNarrationType = "room_entry"
	GMCombatStart  GMNarrationType = "combat_start"
	GMCombatEnd    GMNarrationType = "combat_end"
	GMNat20        GMNarrationType = "nat_20"
	GMNat1         GMNarrationType = "nat_1"
	GMBossEntry    GMNarrationType = "boss_entry"
	GMBossDeath    GMNarrationType = "boss_death"
	GMPlayerDeath  GMNarrationType = "player_death"
	GMZoneComplete GMNarrationType = "zone_complete"
	GMTrapDetected GMNarrationType = "trap_detected"
	GMTrapTripped  GMNarrationType = "trap_tripped"
	GMLore         GMNarrationType = "lore"
)

// MoodBand — five-state band derived from the 0–100 score.
type MoodBand int

const (
	MoodBandHostile MoodBand = iota
	MoodBandGrumpy
	MoodBandNeutral
	MoodBandFriendly
	MoodBandEffusive
)

// moodBand maps a raw score to its band per design doc §3.2.
func moodBand(score int) MoodBand {
	switch {
	case score >= 80:
		return MoodBandEffusive
	case score >= 60:
		return MoodBandFriendly
	case score >= 40:
		return MoodBandNeutral
	case score >= 20:
		return MoodBandGrumpy
	default:
		return MoodBandHostile
	}
}

// zoneRoomEntryPool returns the zone-specific RoomEntry pool when one
// exists, else the generic pool. Pools live in internal/flavor.
func zoneRoomEntryPool(zoneID ZoneID) []string {
	switch zoneID {
	case ZoneGoblinWarrens:
		return append(append([]string{}, flavor.RoomEntryGoblinWarrens...), flavor.RoomEntryGeneric...)
	case ZoneCryptValdris:
		return append(append([]string{}, flavor.RoomEntryCryptValdris...), flavor.RoomEntryGeneric...)
	case ZoneForestShadows:
		return append(append([]string{}, flavor.RoomEntryForestShadows...), flavor.RoomEntryGeneric...)
	case ZoneSunkenTemple:
		return append(append([]string{}, flavor.RoomEntrySunkenTemple...), flavor.RoomEntryGeneric...)
	case ZoneManorBlackspire:
		return append(append([]string{}, flavor.RoomEntryHauntedManor...), flavor.RoomEntryGeneric...)
	case ZoneUnderforge:
		return append(append([]string{}, flavor.RoomEntryUnderforge...), flavor.RoomEntryGeneric...)
	case ZoneUnderdark:
		return append(append([]string{}, flavor.RoomEntryUnderdark...), flavor.RoomEntryGeneric...)
	case ZoneFeywildCrossing:
		return append(append([]string{}, flavor.RoomEntryFeywildCrossing...), flavor.RoomEntryGeneric...)
	case ZoneDragonsLair:
		return append(append([]string{}, flavor.RoomEntryDragonsLair...), flavor.RoomEntryGeneric...)
	case ZoneAbyssPortal:
		return append(append([]string{}, flavor.RoomEntryAbyssPortal...), flavor.RoomEntryGeneric...)
	}
	return flavor.RoomEntryGeneric
}

// bossEntryPool returns the boss-specific pool when one exists, else
// the generic boss-entry pool.
func bossEntryPool(zoneID ZoneID) []string {
	switch zoneID {
	case ZoneGoblinWarrens:
		return flavor.BossEntryGrol
	case ZoneCryptValdris:
		return flavor.BossEntryValdris
	case ZoneForestShadows:
		return flavor.BossEntryHollowKing
	case ZoneSunkenTemple:
		return flavor.BossEntryDreamingAboleth
	case ZoneManorBlackspire:
		return flavor.BossEntryAldricBlackspire
	case ZoneUnderforge:
		return flavor.BossEntryEmberlordThyrak
	case ZoneUnderdark:
		return flavor.BossEntryIlvaras
	case ZoneFeywildCrossing:
		return flavor.BossEntryThornmother
	case ZoneDragonsLair:
		return flavor.BossEntryInfernax
	case ZoneAbyssPortal:
		return flavor.BossEntryBelaxath
	}
	return flavor.BossEntryGeneric
}

// pickPool routes a narration type to the right []string pool from
// internal/flavor. Returns nil for kinds that have no pool yet (caller
// is expected to skip rendering or fall back to a static line).
func pickPool(zoneID ZoneID, kind GMNarrationType) []string {
	switch kind {
	case GMRoomEntry:
		return zoneRoomEntryPool(zoneID)
	case GMBossEntry:
		return bossEntryPool(zoneID)
	case GMCombatStart:
		return flavor.CombatStart
	case GMCombatEnd:
		return flavor.CombatVictory
	case GMNat20:
		return flavor.Nat20
	case GMNat1:
		return flavor.Nat1
	case GMBossDeath:
		return flavor.BossDeath
	case GMPlayerDeath:
		return flavor.PlayerDeath
	case GMZoneComplete:
		return flavor.ZoneComplete
	case GMTrapDetected:
		return flavor.TrapDetected
	case GMTrapTripped:
		return flavor.TrapTriggered
	case GMLore:
		return zoneLorePool(zoneID)
	}
	return nil
}

// zoneLorePool returns the zone-specific lore pool prepended to the
// generic LoreLines pool. Tier 1 zones ship dedicated lore (D2b); other
// tiers fall back to the generic pool until their flavor files land.
func zoneLorePool(zoneID ZoneID) []string {
	switch zoneID {
	case ZoneGoblinWarrens:
		return append(append([]string{}, flavor.LoreLinesWarrens...), flavor.LoreLines...)
	case ZoneCryptValdris:
		return append(append([]string{}, flavor.LoreLinesCrypt...), flavor.LoreLines...)
	case ZoneForestShadows:
		return append(append([]string{}, flavor.LoreLinesForestShadows...), flavor.LoreLines...)
	case ZoneSunkenTemple:
		return append(append([]string{}, flavor.LoreLinesSunkenTemple...), flavor.LoreLines...)
	case ZoneManorBlackspire:
		return append(append([]string{}, flavor.LoreLinesManorBlackspire...), flavor.LoreLines...)
	case ZoneUnderforge:
		return append(append([]string{}, flavor.LoreLinesUnderforge...), flavor.LoreLines...)
	case ZoneUnderdark:
		return append(append([]string{}, flavor.LoreLinesUnderdark...), flavor.LoreLines...)
	case ZoneFeywildCrossing:
		return append(append([]string{}, flavor.LoreLinesFeywildCrossing...), flavor.LoreLines...)
	case ZoneDragonsLair:
		return append(append([]string{}, flavor.LoreLinesDragonsLair...), flavor.LoreLines...)
	case ZoneAbyssPortal:
		return append(append([]string{}, flavor.LoreLinesAbyssPortal...), flavor.LoreLines...)
	}
	return flavor.LoreLines
}

// bossSignaturePool returns the zone-boss ability-callout pool (one-line
// cinematic suffix sampled at boss-entry render). Returns nil when the
// zone has no signature pool yet — caller skips the suffix.
func bossSignaturePool(zoneID ZoneID) []string {
	switch zoneID {
	case ZoneGoblinWarrens:
		return flavor.GrolSignatureCallouts
	case ZoneCryptValdris:
		return flavor.ValdrisSignatureCallouts
	case ZoneForestShadows:
		return flavor.HollowKingSignatureCallouts
	case ZoneSunkenTemple:
		return flavor.AbolethSignatureCallouts
	case ZoneManorBlackspire:
		return flavor.AldricSignatureCallouts
	case ZoneUnderforge:
		return flavor.ThyrakSignatureCallouts
	case ZoneUnderdark:
		return flavor.IlvarasSignatureCallouts
	case ZoneFeywildCrossing:
		return flavor.ThornmotherSignatureCallouts
	case ZoneDragonsLair:
		return flavor.InfernaxSignatureCallouts
	case ZoneAbyssPortal:
		return flavor.BelaxathSignatureCallouts
	}
	return nil
}

// bossPhaseTwoPool returns the zone-boss phase-two callout pool — the
// line surfaced when the boss crosses its PhaseTwoAt HP threshold mid-fight.
// Returns nil for bosses with no phase-two transition (Grol, Aboleth).
func bossPhaseTwoPool(zoneID ZoneID) []string {
	switch zoneID {
	case ZoneCryptValdris:
		return flavor.ValdrisPhaseTwoLines
	case ZoneForestShadows:
		return flavor.HollowKingPhaseTwoLines
	case ZoneManorBlackspire:
		return flavor.AldricPhaseTwoLines
	case ZoneUnderforge:
		return flavor.ThyrakPhaseTwoLines
	case ZoneUnderdark:
		return flavor.IlvarasPhaseTwoLines
	case ZoneFeywildCrossing:
		return flavor.ThornmotherPhaseTwoLines
	case ZoneDragonsLair:
		return flavor.InfernaxPhaseTwoLines
	case ZoneAbyssPortal:
		return flavor.BelaxathPhaseTwoLines
	}
	return nil
}

// bossPhaseTwoLine renders the zone-specific phase-two transition line.
// Empty string when the zone has no phase-two pool.
func bossPhaseTwoLine(zoneID ZoneID, runID string, roomIdx int) string {
	pool := bossPhaseTwoPool(zoneID)
	if len(pool) == 0 {
		return ""
	}
	line := pickLineDeterministic(pool, runID, roomIdx^0x71F4A2D3)
	if line == "" {
		return ""
	}
	return "🎭 **TwinBee:** " + line
}

// phaseTwoCrossedInEvents reports whether enemy HP crossed the
// PhaseTwoAt fraction of startHP at any point in the events log. Returns
// false when threshold <= 0 (no phase 2) or startHP <= 0.
func phaseTwoCrossedInEvents(events []CombatEvent, startHP int, threshold float64) bool {
	if threshold <= 0 || startHP <= 0 {
		return false
	}
	cutoff := int(float64(startHP) * threshold)
	for _, ev := range events {
		if ev.EnemyHP <= cutoff {
			return true
		}
	}
	return false
}

// eliteRoomEntryPool returns the zone-specific elite-room atmospheric
// pool. Returns nil when the zone has no dedicated elite prose.
func eliteRoomEntryPool(zoneID ZoneID) []string {
	switch zoneID {
	case ZoneGoblinWarrens:
		return flavor.EliteRoomEntryWarrens
	case ZoneCryptValdris:
		return flavor.EliteRoomEntryCrypt
	case ZoneForestShadows:
		return flavor.EliteRoomEntryForestShadows
	case ZoneSunkenTemple:
		return flavor.EliteRoomEntrySunkenTemple
	case ZoneManorBlackspire:
		return flavor.EliteRoomEntryManorBlackspire
	case ZoneUnderforge:
		return flavor.EliteRoomEntryUnderforge
	case ZoneUnderdark:
		return flavor.EliteRoomEntryUnderdark
	case ZoneFeywildCrossing:
		return flavor.EliteRoomEntryFeywildCrossing
	case ZoneDragonsLair:
		return flavor.EliteRoomEntryDragonsLair
	case ZoneAbyssPortal:
		return flavor.EliteRoomEntryAbyssPortal
	}
	return nil
}

// composeBossEntry renders the boss-entry narration plus, when the zone
// has a signature pool, a single ability callout on the line below.
// Falls back to the bare boss-entry line when no signature pool exists.
func composeBossEntry(zoneID ZoneID, runID string, roomIdx int) string {
	entry := twinBeeLine(zoneID, GMBossEntry, runID, roomIdx)
	pool := bossSignaturePool(zoneID)
	if entry == "" || len(pool) == 0 {
		return entry
	}
	// Salt the suffix selector with a constant so the entry line and the
	// callout line vary independently across rooms.
	callout := pickLineDeterministic(pool, runID, roomIdx^0x5BD17EF1)
	if callout == "" {
		return entry
	}
	return entry + "\n🎭 **TwinBee:** " + callout
}

// eliteRoomEntryLine renders the zone-specific elite atmospheric line
// for an elite room. Empty string when the zone has no elite pool yet.
func eliteRoomEntryLine(zoneID ZoneID, runID string, roomIdx int) string {
	pool := eliteRoomEntryPool(zoneID)
	if len(pool) == 0 {
		return ""
	}
	line := pickLineDeterministic(pool, runID, roomIdx^0x2C9E1A37)
	if line == "" {
		return ""
	}
	return "🎭 **TwinBee:** " + line
}

// pickLineDeterministic — stable selection across (runID, salt). Same
// inputs always yield the same line, so a player who re-reads status
// sees the same prose; different rooms / different runs vary.
func pickLineDeterministic(lines []string, runID string, salt int) string {
	if len(lines) == 0 {
		return ""
	}
	h := fnv.New64a()
	h.Write([]byte(runID))
	var sb [8]byte
	for i := 0; i < 8; i++ {
		sb[i] = byte(salt >> (8 * i))
	}
	h.Write(sb[:])
	return lines[int(h.Sum64()%uint64(len(lines)))]
}

// twinBeeLine renders the TwinBee narration block for a given event,
// drawing from the canonical internal/flavor pools.
//
// roomIdx is folded into the deterministic selector so the same room
// in the same run always renders the same prose (idempotent reads).
func twinBeeLine(zoneID ZoneID, kind GMNarrationType, runID string, roomIdx int) string {
	pool := pickPool(zoneID, kind)
	line := pickLineDeterministic(pool, runID, roomIdx)
	if line == "" {
		return ""
	}
	return "🎭 **TwinBee:** " + line
}

// tauntResponseLine renders a deterministic TwinBee reply to !zone taunt
// from the prewritten flavor.TauntResponses pool. roomIdx is folded so
// repeated taunts in the same room are stable but cross-room taunts vary.
func tauntResponseLine(runID string, roomIdx int) string {
	line := pickLineDeterministic(flavor.TauntResponses, runID, roomIdx^0x4A8D9F25)
	if line == "" {
		return ""
	}
	return "🎭 **TwinBee:** " + line
}

// complimentResponseLine renders a deterministic TwinBee reply to
// !zone compliment from the prewritten flavor.ComplimentResponses pool.
func complimentResponseLine(runID string, roomIdx int) string {
	line := pickLineDeterministic(flavor.ComplimentResponses, runID, roomIdx^0x3E172B6C)
	if line == "" {
		return ""
	}
	return "🎭 **TwinBee:** " + line
}

// moodAsidePool returns the mood-banded aside pool when the mood sits at
// an extreme band (hostile or effusive), else nil. Mid-bands (grumpy /
// neutral / friendly) intentionally surface no aside — flavor stays
// strictly extra, never replacing the core narration.
func moodAsidePool(mood int) []string {
	switch moodBand(mood) {
	case MoodBandHostile:
		return flavor.MoodAsidesHostile
	case MoodBandEffusive:
		return flavor.MoodAsidesEffusive
	}
	return nil
}

// moodAsideLine renders a deterministic TwinBee aside reflecting the
// run's mood at the extreme bands. roomIdx is folded so the same room
// in the same run is stable; cross-room asides vary. Returns "" for
// mid-band moods.
func moodAsideLine(mood int, runID string, roomIdx int) string {
	pool := moodAsidePool(mood)
	if len(pool) == 0 {
		return ""
	}
	line := pickLineDeterministic(pool, runID, roomIdx^0x6F2D8B14)
	if line == "" {
		return ""
	}
	return "🎭 **TwinBee:** " + line
}

// ── Mood event table & application ───────────────────────────────────────────

// GMMoodEvent enumerates the mood-modifier triggers from design doc §3.2.
type GMMoodEvent string

const (
	MoodEventZoneComplete       GMMoodEvent = "zone_complete"
	MoodEventPlayerDeath        GMMoodEvent = "player_death"
	MoodEventNat20              GMMoodEvent = "nat_20"
	MoodEventNat1               GMMoodEvent = "nat_1"
	MoodEventCommunityMilestone GMMoodEvent = "community_milestone"
	MoodEventDowntime           GMMoodEvent = "downtime"
	MoodEventTaunt              GMMoodEvent = "taunt"
	MoodEventCompliment         GMMoodEvent = "compliment"
)

// moodEventDelta — design doc §3.2 mood-modifier table.
var moodEventDelta = map[GMMoodEvent]int{
	MoodEventZoneComplete:       +10,
	MoodEventPlayerDeath:        -5,
	MoodEventNat20:              +3,
	MoodEventNat1:               -2,
	MoodEventCommunityMilestone: +15,
	MoodEventDowntime:           -20,
	MoodEventTaunt:              -10,
	MoodEventCompliment:         +5,
}

// MoodEventDelta exposes the static delta for an event (test use).
func MoodEventDelta(ev GMMoodEvent) int { return moodEventDelta[ev] }

// applyMoodEvent updates the run's mood by the event's delta. Returns
// the new mood score after clamping.
func applyMoodEvent(runID string, ev GMMoodEvent) (int, error) {
	delta, ok := moodEventDelta[ev]
	if !ok {
		return 0, fmt.Errorf("unknown mood event: %s", ev)
	}
	if err := adjustGMMood(runID, delta); err != nil {
		return 0, err
	}
	r, err := getZoneRun(runID)
	if err != nil || r == nil {
		return 0, err
	}
	return r.GMMood, nil
}

// passiveDecayMood drifts the mood toward 50 at ±2/hour, scaled by
// hours elapsed since lastTouched. Pure function; caller persists.
//
// Design doc §3.2: "Mood decays toward 50 at a rate of ±2 per hour
// (passive rebalancing)." Long-idle runs reset toward neutral.
func passiveDecayMood(score int, lastTouched, now time.Time) int {
	hours := int(now.Sub(lastTouched).Hours())
	if hours <= 0 {
		return clampMood(score)
	}
	const ratePerHour = 2
	drift := hours * ratePerHour
	switch {
	case score > 50:
		score -= drift
		if score < 50 {
			score = 50
		}
	case score < 50:
		score += drift
		if score > 50 {
			score = 50
		}
	}
	return clampMood(score)
}

func clampMood(s int) int {
	if s < 0 {
		return 0
	}
	if s > 100 {
		return 100
	}
	return s
}

// applyMoodDecayIfStale persists a passive-decay correction when the
// run has been idle long enough to drift. Cheap no-op on fresh runs.
func applyMoodDecayIfStale(r *DungeonRun) error {
	if r == nil {
		return nil
	}
	newMood := passiveDecayMood(r.GMMood, r.LastActionAt, time.Now().UTC())
	if newMood == r.GMMood {
		return nil
	}
	if _, err := db.Get().Exec(`
		UPDATE dnd_zone_run SET gm_mood = ? WHERE run_id = ?`,
		newMood, r.RunID); err != nil {
		return err
	}
	r.GMMood = newMood
	return nil
}
