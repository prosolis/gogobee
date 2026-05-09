package plugin

// Phase G8d — Haunted Manor of Blackspire branching graph.
//
// T3 zone. Shape: stacked 3-way "hub" forks. The plan calls T2+ for
// "two forks", and a Manor of corridors and locked doors fits a wide-
// fork hub-and-spoke pattern more naturally than a binary fork. Two
// hub nodes (great_hall, upper_hall), each with three options:
//
//   entry → foyer → great_hall (3-way)
//                      ├─[unlocked]── portrait_gallery ─┐
//                      ├─[Perception DC 14]── study ────┼── upper_hall (3-way)
//                      └─[INT DC 14]── library ─────────┘     ├─[unlocked]── master_bedroom (elite) → boss
//                                                              ├─[Perception DC 15]── hidden_oratory (secret) → boss
//                                                              └─[LevelMin 7]── tower_observatory → boss
//
// Two consecutive 3-way fork prompts make this the first zone where
// the player picks twice in a row from a wide menu — distinct from the
// binary forks of G8a/b and the parallel Y-trees of G8c. Exercises
// LockLevelMin (first authored use) so all four non-trivial lock kinds
// (Perception, StatCheck, LevelMin, plus implicit LockNone) appear by
// G8d.

func zoneManorBlackspireGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "manor_blackspire.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Manor Gate", PosX: 0, PosY: 2},
		{NodeID: "manor_blackspire.foyer", Kind: NodeKindExploration,
			Label: "Foyer", PosX: 1, PosY: 2},
		{NodeID: "manor_blackspire.great_hall", Kind: NodeKindFork,
			Label: "Great Hall", PosX: 2, PosY: 2},
		{NodeID: "manor_blackspire.portrait_gallery", Kind: NodeKindExploration,
			Label: "Portrait Gallery", PosX: 3, PosY: 1},
		{NodeID: "manor_blackspire.study", Kind: NodeKindExploration,
			Label: "Locked Study", PosX: 3, PosY: 2},
		{NodeID: "manor_blackspire.library", Kind: NodeKindExploration,
			Label: "Forbidden Library", PosX: 3, PosY: 3},
		{NodeID: "manor_blackspire.upper_hall", Kind: NodeKindFork,
			Label: "Upper Hall", PosX: 4, PosY: 2},
		{NodeID: "manor_blackspire.master_bedroom", Kind: NodeKindElite,
			Label: "Master Bedroom", PosX: 5, PosY: 1},
		{NodeID: "manor_blackspire.tower_observatory", Kind: NodeKindExploration,
			Label: "Tower Observatory", PosX: 5, PosY: 2},
		{NodeID: "manor_blackspire.hidden_oratory", Kind: NodeKindSecret,
			Label: "Hidden Oratory", PosX: 5, PosY: 3,
			Content: ZoneNodeContent{LootBias: 2.0}},
		{NodeID: "manor_blackspire.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Aldric's Sanctum", PosX: 6, PosY: 2},
	}
	edges := []ZoneEdge{
		{From: "manor_blackspire.entry", To: "manor_blackspire.foyer", Lock: LockNone},
		{From: "manor_blackspire.foyer", To: "manor_blackspire.great_hall", Lock: LockNone},
		// Great Hall — 3-way: portrait (open), study (perception), library (intelligence)
		{From: "manor_blackspire.great_hall", To: "manor_blackspire.portrait_gallery", Lock: LockNone, Weight: 1},
		{From: "manor_blackspire.great_hall", To: "manor_blackspire.study",
			Lock: LockPerception, LockData: map[string]any{"dc": 14},
			Hint: "an out-of-place draft from a doorframe", Weight: 2},
		{From: "manor_blackspire.great_hall", To: "manor_blackspire.library",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "INT", "dc": 14},
			Hint: "a runed door — you'd need to read it", Weight: 2},
		// All three converge at upper_hall.
		{From: "manor_blackspire.portrait_gallery", To: "manor_blackspire.upper_hall", Lock: LockNone},
		{From: "manor_blackspire.study", To: "manor_blackspire.upper_hall", Lock: LockNone},
		{From: "manor_blackspire.library", To: "manor_blackspire.upper_hall", Lock: LockNone},
		// Upper Hall — 3-way: master_bedroom (open elite), hidden_oratory (high perception secret), tower (level-gated)
		{From: "manor_blackspire.upper_hall", To: "manor_blackspire.master_bedroom", Lock: LockNone, Weight: 1},
		{From: "manor_blackspire.upper_hall", To: "manor_blackspire.hidden_oratory",
			Lock: LockPerception, LockData: map[string]any{"dc": 15},
			Hint: "a candle behind a panel that shouldn't have a back", Weight: 2},
		{From: "manor_blackspire.upper_hall", To: "manor_blackspire.tower_observatory",
			Lock: LockLevelMin, LockData: map[string]any{"min_level": 7},
			Hint: "a spiral stair that creaks ominously — climb only if you trust your footing", Weight: 3},
		{From: "manor_blackspire.master_bedroom", To: "manor_blackspire.boss", Lock: LockNone},
		{From: "manor_blackspire.hidden_oratory", To: "manor_blackspire.boss", Lock: LockNone},
		{From: "manor_blackspire.tower_observatory", To: "manor_blackspire.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneManorBlackspire, nodes, edges)
}

func init() {
	registerZoneGraph(zoneManorBlackspireGraph())
}
