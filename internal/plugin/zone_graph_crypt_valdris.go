package plugin

// Phase G7 — Crypt of Valdris POC graph.
//
// First hand-authored branching graph. Topology per
// gogobee_branching_zones_plan.md §3:
//
//   entry → corridor → fork
//                       ├──[unlocked]── main_hall (elite) ──┐
//                       │                                    ├── antechamber → boss
//                       └──[Perception DC 12]── side_chapel ─┘
//                                                  │
//                                                  └──[Perception DC 15]── secret_chamber → antechamber
//
// Two paths to the boss (main_hall vs side_chapel); secret_chamber
// dangles off side_chapel for an extra Perception gate with loot.
// Registration is unconditional — the runtime gate
// (GOGOBEE_BRANCHING_ZONES) decides whether new runs adopt this graph
// or stay on the legacy linear template.

func zoneCryptValdrisGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "crypt_valdris.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Crypt Threshold", PosX: 0, PosY: 1},
		{NodeID: "crypt_valdris.corridor", Kind: NodeKindExploration,
			Label: "Bone Corridor", PosX: 1, PosY: 1},
		{NodeID: "crypt_valdris.fork", Kind: NodeKindFork,
			Label: "Branching Passage", PosX: 2, PosY: 1},
		{NodeID: "crypt_valdris.main_hall", Kind: NodeKindElite,
			Label: "Main Hall", PosX: 3, PosY: 0},
		{NodeID: "crypt_valdris.side_chapel", Kind: NodeKindExploration,
			Label: "Side Chapel", PosX: 3, PosY: 2},
		{NodeID: "crypt_valdris.secret_chamber", Kind: NodeKindSecret,
			Label: "Hidden Reliquary", PosX: 4, PosY: 3,
			Content: ZoneNodeContent{LootBias: 2.0}},
		{NodeID: "crypt_valdris.antechamber", Kind: NodeKindMerge,
			Label: "Antechamber", PosX: 5, PosY: 1},
		{NodeID: "crypt_valdris.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Valdris's Sanctum", PosX: 6, PosY: 1},
	}
	edges := []ZoneEdge{
		{From: "crypt_valdris.entry", To: "crypt_valdris.corridor", Lock: LockNone},
		{From: "crypt_valdris.corridor", To: "crypt_valdris.fork", Lock: LockNone},
		{From: "crypt_valdris.fork", To: "crypt_valdris.main_hall", Lock: LockNone, Weight: 1},
		{From: "crypt_valdris.fork", To: "crypt_valdris.side_chapel",
			Lock: LockPerception, LockData: map[string]any{"dc": 12},
			Hint: "a faint draft from a crack in the wall", Weight: 2},
		{From: "crypt_valdris.main_hall", To: "crypt_valdris.antechamber", Lock: LockNone},
		{From: "crypt_valdris.side_chapel", To: "crypt_valdris.antechamber", Lock: LockNone, Weight: 1},
		{From: "crypt_valdris.side_chapel", To: "crypt_valdris.secret_chamber",
			Lock: LockPerception, LockData: map[string]any{"dc": 15},
			Hint: "a faint scratching behind the altar", Weight: 2},
		{From: "crypt_valdris.secret_chamber", To: "crypt_valdris.antechamber", Lock: LockNone},
		{From: "crypt_valdris.antechamber", To: "crypt_valdris.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneCryptValdris, nodes, edges)
}

func init() {
	registerZoneGraph(zoneCryptValdrisGraph())
}
