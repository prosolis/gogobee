package plugin

// Phase G8e — The Underforge branching graph.
//
// T3 zone. Shape: pre-boss gauntlet — long linear preamble through the
// forge-city, then a single 3-way antechamber fork right before the
// boss. The plan §G8 calls for "two forks for T2+"; this zone bends
// that to a single late fork with three options because the gauntlet
// shape is the design intent (Kharak Dûn is a one-way descent — there
// is no scenic route, only the approach to what was sealed in). The
// triple-option antechamber preserves player agency at the
// commitment moment.
//
//   entry → sealed_gate → forge_descent → cooling_river → magma_chamber (elite) → antechamber (3-way)
//                                                                                    ├─[unlocked]── direct_assault → boss
//                                                                                    ├─[DEX DC 14]── catwalks → boss
//                                                                                    └─[Perception DC 15]── forge_vault (secret) → boss
//
// Distinct from prior zones in two ways:
// 1. Five-node linear preamble (no zone yet has > 2 linear preamble nodes).
// 2. Fork is delayed to the final node before boss; the !zone map
//    therefore renders as a long horizontal stem with a 3-leaf fan at
//    the right edge.

func zoneUnderforgeGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "underforge.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Sealed Threshold", PosX: 0, PosY: 1},
		{NodeID: "underforge.sealed_gate", Kind: NodeKindExploration,
			Label: "Sealed Gate", PosX: 1, PosY: 1},
		{NodeID: "underforge.forge_descent", Kind: NodeKindExploration,
			Label: "Forge Descent", PosX: 2, PosY: 1},
		{NodeID: "underforge.cooling_river", Kind: NodeKindTrap,
			Label: "Cooling River", PosX: 3, PosY: 1},
		{NodeID: "underforge.magma_chamber", Kind: NodeKindElite,
			Label: "Magma Chamber", PosX: 4, PosY: 1},
		{NodeID: "underforge.antechamber", Kind: NodeKindFork,
			Label: "Antechamber of Kharak Dûn", PosX: 5, PosY: 1},
		{NodeID: "underforge.direct_assault", Kind: NodeKindExploration,
			Label: "Direct Assault", PosX: 6, PosY: 0},
		{NodeID: "underforge.catwalks", Kind: NodeKindExploration,
			Label: "Forge Catwalks", PosX: 6, PosY: 1},
		{NodeID: "underforge.forge_vault", Kind: NodeKindSecret,
			Label: "Forge Vault", PosX: 6, PosY: 2,
			Content: ZoneNodeContent{LootBias: 2.0}},
		{NodeID: "underforge.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "The Sealed Hall", PosX: 7, PosY: 1},
	}
	edges := []ZoneEdge{
		{From: "underforge.entry", To: "underforge.sealed_gate", Lock: LockNone},
		{From: "underforge.sealed_gate", To: "underforge.forge_descent", Lock: LockNone},
		{From: "underforge.forge_descent", To: "underforge.cooling_river", Lock: LockNone},
		{From: "underforge.cooling_river", To: "underforge.magma_chamber", Lock: LockNone},
		{From: "underforge.magma_chamber", To: "underforge.antechamber", Lock: LockNone},
		{From: "underforge.antechamber", To: "underforge.direct_assault", Lock: LockNone, Weight: 1},
		{From: "underforge.antechamber", To: "underforge.catwalks",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "DEX", "dc": 14},
			Hint: "rusted catwalks above the magma — quick footing required", Weight: 2},
		{From: "underforge.antechamber", To: "underforge.forge_vault",
			Lock: LockPerception, LockData: map[string]any{"dc": 15},
			Hint: "a draft from a stone seam, where no draft should be", Weight: 2},
		{From: "underforge.direct_assault", To: "underforge.boss", Lock: LockNone},
		{From: "underforge.catwalks", To: "underforge.boss", Lock: LockNone},
		{From: "underforge.forge_vault", To: "underforge.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneUnderforge, nodes, edges)
}

func init() {
	registerZoneGraph(zoneUnderforgeGraph())
}
