package plugin

// Phase G8f — Feywild Crossing branching graph.
//
// T4 zone. Shape: woven forks — two first-stage forks each lead to a
// second-stage fork; the inner nodes partially overlap so the player's
// first choice does not fully determine which second-stage rooms they
// can reach. Captures the Feywild theme of "the path you walked is
// not the path you arrived on."
//
//   entry → twilight_path → fork1
//                             ├─[CHA DC 14]── glamoured_grove ─┬─ hag_circle (elite) ─┐
//                             │                                └─[DEX DC 15]── time_eddy ─┤
//                             │                                                          ├── boss
//                             └─[Perception DC 14]── wisp_marsh ─┬─ hag_circle ──────────┤
//                                                                └─[Perception DC 16]── illusion_garden (secret) ─┘
//
// hag_circle is reachable from BOTH first-stage paths — the elite
// encounter is the "you'll meet her either way" room. time_eddy is
// only reachable via the grove (CHA path); illusion_garden is only
// reachable via the marsh (Perception path).
//
// First authored zone using LockStatCheck CHA — every prior secret/
// gate has been Perception/STR/INT/DEX. Both fork1 entries are locked
// (no LockNone option from fork1) which is the first instance of "no
// free choice"; the player must commit to the lock-check style they're
// stronger at.

func zoneFeywildCrossingGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "feywild_crossing.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Veil's Edge", PosX: 0, PosY: 2},
		{NodeID: "feywild_crossing.twilight_path", Kind: NodeKindExploration,
			Label: "Twilight Path", PosX: 1, PosY: 2},
		{NodeID: "feywild_crossing.fork1", Kind: NodeKindFork,
			Label: "The Bargain Crossroads", PosX: 2, PosY: 2},
		{NodeID: "feywild_crossing.glamoured_grove", Kind: NodeKindFork,
			Label: "Glamoured Grove", PosX: 3, PosY: 1},
		{NodeID: "feywild_crossing.wisp_marsh", Kind: NodeKindFork,
			Label: "Wisp Marsh", PosX: 3, PosY: 3},
		{NodeID: "feywild_crossing.time_eddy", Kind: NodeKindExploration,
			Label: "Time Eddy", PosX: 4, PosY: 0},
		{NodeID: "feywild_crossing.hag_circle", Kind: NodeKindElite,
			Label: "Hag Circle", PosX: 4, PosY: 2},
		{NodeID: "feywild_crossing.illusion_garden", Kind: NodeKindSecret,
			Label: "Illusion Garden", PosX: 4, PosY: 4,
			Content: ZoneNodeContent{LootBias: 2.0}},
		{NodeID: "feywild_crossing.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Court of the Antlered Queen", PosX: 5, PosY: 2},
	}
	edges := []ZoneEdge{
		{From: "feywild_crossing.entry", To: "feywild_crossing.twilight_path", Lock: LockNone},
		{From: "feywild_crossing.twilight_path", To: "feywild_crossing.fork1", Lock: LockNone},
		// Fork1 — both options are locked (CHA vs. Perception).
		{From: "feywild_crossing.fork1", To: "feywild_crossing.glamoured_grove",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "CHA", "dc": 14},
			Hint: "a starlight creature offering a deal — you'd have to play along", Weight: 1},
		{From: "feywild_crossing.fork1", To: "feywild_crossing.wisp_marsh",
			Lock: LockPerception, LockData: map[string]any{"dc": 14},
			Hint: "wisp-light flickering between two trees — they look like the same tree", Weight: 1},
		// Grove → hag_circle (open) or time_eddy (DEX).
		{From: "feywild_crossing.glamoured_grove", To: "feywild_crossing.hag_circle", Lock: LockNone, Weight: 1},
		{From: "feywild_crossing.glamoured_grove", To: "feywild_crossing.time_eddy",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "DEX", "dc": 15},
			Hint: "the seconds run sideways here — sidestep at the right one", Weight: 2},
		// Marsh → hag_circle (shared) or illusion_garden (high Perception secret).
		{From: "feywild_crossing.wisp_marsh", To: "feywild_crossing.hag_circle", Lock: LockNone, Weight: 1},
		{From: "feywild_crossing.wisp_marsh", To: "feywild_crossing.illusion_garden",
			Lock: LockPerception, LockData: map[string]any{"dc": 16},
			Hint: "a garden visible only when you don't look directly at it", Weight: 2},
		// All three second-stage nodes terminate at boss.
		{From: "feywild_crossing.hag_circle", To: "feywild_crossing.boss", Lock: LockNone},
		{From: "feywild_crossing.time_eddy", To: "feywild_crossing.boss", Lock: LockNone},
		{From: "feywild_crossing.illusion_garden", To: "feywild_crossing.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneFeywildCrossing, nodes, edges)
}

func init() {
	registerZoneGraph(zoneFeywildCrossingGraph())
}
