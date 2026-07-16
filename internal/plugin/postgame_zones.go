package plugin

// Tier 6 "Mythic" post-game zones — shared plumbing (Phase P1).
//
// Post-game means earned, not leveled. zonesForLevel can't gate these
// (playerTier = (L-1)/3+1 plus the +2 lookahead would surface a T6 zone far
// too early — and at L18 the lookahead reaches T6 outright), so entry is
// gated by postgameUnlocked: both Tier 5 zone bosses beaten AND level ≥ 18.
// Locked zones still render in `!zone list` under a "Beyond the Map" divider
// with a hint line — visible aspiration, gated entry.
//
// See gogobee_postgame_zones_plan.md.

import (
	"fmt"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// postgameLevelFloor — the minimum character level for T6 entry. L18 lets
// near-cap players scout and die educationally before hitting L20.
const postgameLevelFloor = 18

// postgameZones returns every registered Tier 6 zone in declared order.
func postgameZones() []ZoneDefinition {
	var out []ZoneDefinition
	for _, z := range allZones() {
		if z.Tier >= ZoneTierMythic {
			out = append(out, z)
		}
	}
	return out
}

// isPostgameZone reports whether a zone id is a Tier 6 post-game zone.
func isPostgameZone(id ZoneID) bool {
	z, ok := getZone(id)
	return ok && z.Tier >= ZoneTierMythic
}

// postgameUnlocked reports whether a player may enter Tier 6 zones, and if
// not, a one-line player-facing reason. The two gates:
//   - both Tier 5 zone bosses (dragons_lair AND abyss_portal) beaten, and
//   - character level ≥ postgameLevelFloor.
//
// The T5-clear check reuses clearedZoneIDs (boss_defeated DISTINCT query),
// so an extraction that merely ended in 'complete' does not count.
func postgameUnlocked(userID id.UserID, level int) (bool, string) {
	if level < postgameLevelFloor {
		return false, fmt.Sprintf(
			"The way past the map's edge opens at level %d. You're level %d.",
			postgameLevelFloor, level)
	}
	cleared := clearedZoneIDs(db.Get(), userID)
	if !cleared[ZoneDragonsLair] || !cleared[ZoneAbyssPortal] {
		return false, "The way past the map's edge stays shut until both " +
			"Infernax and Belaxath lie dead — the dragon and the balor, " +
			"the two ends of the known world."
	}
	return true, ""
}

// availableZonesFor returns the zones a player may enter for command
// resolution: the normal level-gated set, plus the Tier 6 post-game zones
// when postgameUnlocked. Command sites use this in place of a bare
// zonesForLevel so an unlocked player can resolve a T6 zone id/index.
func availableZonesFor(userID id.UserID, level int) []ZoneDefinition {
	out := zonesForLevel(level)
	if ok, _ := postgameUnlocked(userID, level); ok {
		out = append(out, postgameZones()...)
	}
	return out
}

// postgameLockReason returns a player-facing lock reason when input names a
// Tier 6 zone the player has NOT unlocked, otherwise "". Lets command sites
// answer "you're not ready" instead of a generic "unknown zone" when a
// player types a locked post-game zone by name/id.
func postgameLockReason(input string, userID id.UserID, level int) string {
	if _, ok := resolveZoneInput(input, postgameZones()); !ok {
		return ""
	}
	if unlocked, reason := postgameUnlocked(userID, level); !unlocked {
		return reason
	}
	return ""
}
