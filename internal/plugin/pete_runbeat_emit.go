package plugin

import (
	"sort"

	"gogobee/internal/peteclient"

	"maunium.net/go/mautrix/id"
)

// The emit half of the run liveblog: the handful of places in the walk that
// know something worth telling, and the shape they tell it in.
//
// Every function here is a leaf. They read state, they append a row, they return
// nothing. None of them is allowed to change what the engine does or how long it
// takes to do it — if a beat can't be recorded, the run carries on exactly as it
// did before this file existed.
//
// The nouns are the payload and the numbers are the payload. No sentence
// assembled here ever reaches a reader: Pete writes the words, the same contract
// emitFact has always had.

// beatRunStart opens a run's story: who, where, and how far it goes. The token
// is the public board token, so Pete can hang the liveblog off the adventurer
// page the roster already links to.
//
// This is the only beat carrying identity. Every beat after it is keyed on the
// run id alone, which means a run whose start beat was dropped is anonymous
// rather than misattributed.
func beatRunStart(userID id.UserID, run *DungeonRun, zone ZoneDefinition) {
	if run == nil {
		return
	}
	b := peteclient.RunBeat{
		Kind:       "start",
		Token:      eventToken(userID, "roster"),
		Zone:       zone.Display,
		TotalRooms: run.TotalRooms,
		Room:       1,
		RoomKind:   string(RoomEntry),
	}
	if name := charName(userID); name != "" {
		b.Name = name
	}
	if c, err := LoadDnDCharacter(userID); err == nil && c != nil && !c.PendingSetup {
		b.Level = c.Level
	}
	recordRunBeat(run.RunID, b)
}

// beatRoom records an arrival. outcome distinguishes walking on from doubling
// back — the map on the who page already shows *where* the party is, and the
// difference between those two is most of what the log adds to it.
func beatRoom(run *DungeonRun, node string, idx int, outcome string) {
	if run == nil {
		return
	}
	b := peteclient.RunBeat{
		Kind:       "room",
		Room:       idx + 1,
		TotalRooms: run.TotalRooms,
		Outcome:    outcome,
	}
	if g, ok := loadZoneGraph(run.ZoneID); ok {
		if n, exists := g.Nodes[node]; exists {
			b.RoomKind = string(nodeKindToRoomType(n.Kind))
		}
	}
	recordRunBeat(run.RunID, b)
}

// beatCombat records one resolved fight: what it was, how it went, and what it
// cost. Amount is damage taken by the party's leader — the HP pair is the state
// after, so a reader can see the run getting thinner room by room, which is the
// tension the Matrix DM has and the web has never had.
func beatCombat(run *DungeonRun, monster string, elite, boss, won, timedOut bool,
	preHP, postHP, maxHP, crits, fumbles int) {
	if run == nil {
		return
	}
	outcome := "won"
	switch {
	case won:
	case timedOut:
		outcome = "retreat" // outlasted, not killed: mechanically a withdrawal
	default:
		outcome = "down"
	}
	kind := string(RoomExploration)
	switch {
	case boss:
		kind = string(RoomBoss)
	case elite:
		kind = string(RoomElite)
	}
	dmg := preHP - postHP
	if dmg < 0 {
		dmg = 0 // healed through the fight; "negative damage" is not a fact
	}
	recordRunBeat(run.RunID, peteclient.RunBeat{
		Kind:       "combat",
		Room:       run.CurrentRoom + 1,
		TotalRooms: run.TotalRooms,
		RoomKind:   kind,
		Target:     monster,
		Outcome:    outcome,
		Amount:     dmg,
		HP:         postHP,
		HPMax:      maxHP,
		Crits:      crits,
		Fumbles:    fumbles,
	})
}

// beatTrap records a sprung trap. A zero-damage trap is still worth a beat: the
// near-miss is part of the run, and the log reads wrong if the party walks
// through a trap room and nothing at all is said about it.
func beatTrap(userID id.UserID, run *DungeonRun, damage int) {
	if run == nil {
		return
	}
	hp, maxHP := dndHPSnapshot(userID)
	outcome := "sprung"
	if damage <= 0 {
		outcome = "avoided"
	}
	recordRunBeat(run.RunID, peteclient.RunBeat{
		Kind:       "trap",
		Room:       run.CurrentRoom + 1,
		TotalRooms: run.TotalRooms,
		RoomKind:   string(RoomTrap),
		Outcome:    outcome,
		Amount:     damage,
		HP:         hp,
		HPMax:      maxHP,
	})
}

// beatTreasure records one thing found and kept. One beat per item rather than a
// count: an item is a name, and the name is the whole reason anybody reads a
// loot line.
func beatTreasure(run *DungeonRun, item, source string) {
	if run == nil || item == "" {
		return
	}
	recordRunBeat(run.RunID, peteclient.RunBeat{
		Kind:       "treasure",
		Room:       run.CurrentRoom + 1,
		TotalRooms: run.TotalRooms,
		Target:     item,
		Outcome:    source, // "cache" | "boss" | "zone"
	})
}

// beatHaul records a room's auto-harvest take, one beat for the room rather than
// one per resource — this is background gathering, and a per-resource beat would
// bury the fights it happens between. Target names the biggest single yield so
// the line has a noun in it; Amount is the total.
func beatHaul(run *DungeonRun, sum autoHarvestSummary) {
	if run == nil || len(sum.Yields) == 0 {
		return
	}
	total := 0
	// Deterministic pick: biggest yield, ties broken by name, so re-running the
	// same room can't produce two different beats from the same map.
	keys := make([]string, 0, len(sum.Yields))
	for k, v := range sum.Yields {
		total += v
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if sum.Yields[keys[i]] != sum.Yields[keys[j]] {
			return sum.Yields[keys[i]] > sum.Yields[keys[j]]
		}
		return keys[i] < keys[j]
	})
	top := sum.Names[keys[0]]
	if top == "" {
		top = keys[0]
	}
	recordRunBeat(run.RunID, peteclient.RunBeat{
		Kind:       "haul",
		Room:       run.CurrentRoom + 1,
		TotalRooms: run.TotalRooms,
		Target:     top,
		Amount:     total,
		Count:      len(sum.Yields),
	})
}

// beatLock records a door the party had to deal with. Only the interesting
// outcomes reach here — an unlocked door is not an event.
func beatLock(run *DungeonRun, target, outcome string) {
	if run == nil {
		return
	}
	recordRunBeat(run.RunID, peteclient.RunBeat{
		Kind:       "lock",
		Room:       run.CurrentRoom + 1,
		TotalRooms: run.TotalRooms,
		Target:     target,
		Outcome:    outcome, // "picked" | "sealed"
	})
}

// beatRegion records a border crossing on a multi-region expedition. The run id
// changes at a crossing (each region gets its own run), so this beat closes one
// liveblog and the next run's start beat opens the next — naming the region
// ahead is what lets Pete stitch them into one journey.
func beatRegion(run *DungeonRun, from, to string) {
	if run == nil {
		return
	}
	recordRunBeat(run.RunID, peteclient.RunBeat{
		Kind:    "region",
		Region:  from,
		Target:  to,
		Outcome: "crossed",
	})
}

// beatRunEnd closes the story. outcome is the only field that matters and it is
// the one the whole log is read for.
//
// First writer wins, and that is load-bearing. A run ends once, but it passes
// through more than one place that could say so: a death in the combat resolver
// goes on to call abandonZoneRun, and a completed run gets retired by the
// expedition layer. The specific callers file first and know what happened; the
// generic funnels file "abandoned" and would otherwise overwrite them with the
// least informative answer available.
func beatRunEnd(run *DungeonRun, outcome string) {
	if run == nil || runHasEndBeat(run.RunID) {
		return
	}
	recordRunBeat(run.RunID, peteclient.RunBeat{
		Kind:       "end",
		Room:       run.CurrentRoom + 1,
		TotalRooms: run.TotalRooms,
		Outcome:    outcome, // "cleared" | "died" | "retreated" | "abandoned"
	})
}
