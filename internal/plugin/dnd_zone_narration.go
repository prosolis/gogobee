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
	case ZoneManorBlackspire:
		return append(append([]string{}, flavor.RoomEntryHauntedManor...), flavor.RoomEntryGeneric...)
	case ZoneUnderdark:
		return append(append([]string{}, flavor.RoomEntryUnderdark...), flavor.RoomEntryGeneric...)
	case ZoneDragonsLair:
		return append(append([]string{}, flavor.RoomEntryDragonsLair...), flavor.RoomEntryGeneric...)
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
		return flavor.LoreLines
	}
	return nil
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
