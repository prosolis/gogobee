package plugin

// Phase G8i — The Underdark branching graph (multi-region).
//
// T4 zone. Shape: 3-way regional fork — fork1 routes the player into
// one of three distinct regions (R1 deep, R2 drow, R3 illithid), each
// with its own internal mid-node, all converging at R4 (deep_throne).
//
//   R1 surface_tunnels:   entry → tunnel_descent → fork1
//   ┌─[CON DC 15]── deep_chasm (R1, rich harvest) ──────────┐
//   │                                                         │
//   ├─[unlocked]── drow_patrol (R2) → drow_captain (R2 elite)─┤
//   │                                                         ├── throne_approach (R4) → boss (R4)
//   └─[Perception DC 16]── psionic_corridor (R3) → mind_flayer (R3 elite)─┘
//
// Each colored arm crosses a region boundary, exercising the G6
// fireGraphRegionTransition hook end-to-end. The four authored regions
// match dnd_expedition_region.go:
//   R1 = underdark_surface_tunnels
//   R2 = underdark_drow_outpost
//   R3 = underdark_illithid_warren
//   R4 = underdark_deep_throne
//
// Distinct from prior zones in two ways:
// 1. First (and only) authored zone with non-empty RegionID on every
//    node. Region transitions fire on fork1→{drow_patrol|psionic_corridor}
//    and on the elite→throne_approach edges.
// 2. Convergent triangle: three colored arms, one merge.

func zoneUnderdarkGraph() ZoneGraph {
	r1 := "underdark_surface_tunnels"
	r2 := "underdark_drow_outpost"
	r3 := "underdark_illithid_warren"
	r4 := "underdark_deep_throne"

	nodes := []ZoneNode{
		{NodeID: "underdark.entry", Kind: NodeKindEntry, IsEntry: true, RegionID: r1,
			Label: "Cave Mouth", PosX: 0, PosY: 2},
		{NodeID: "underdark.tunnel_descent", Kind: NodeKindExploration, RegionID: r1,
			Label: "Tunnel Descent", PosX: 1, PosY: 2},
		{NodeID: "underdark.fork1", Kind: NodeKindFork, RegionID: r1,
			Label: "Three-Way Pass", PosX: 2, PosY: 2},
		{NodeID: "underdark.deep_chasm", Kind: NodeKindHarvest, RegionID: r1,
			Label: "Deep Chasm", PosX: 3, PosY: 2,
			Content: ZoneNodeContent{LootBias: 1.8}},
		{NodeID: "underdark.drow_patrol", Kind: NodeKindExploration, RegionID: r2,
			Label: "Drow Patrol", PosX: 3, PosY: 1},
		{NodeID: "underdark.drow_captain", Kind: NodeKindElite, RegionID: r2,
			Label: "Drow Captain's Camp", PosX: 4, PosY: 1},
		{NodeID: "underdark.psionic_corridor", Kind: NodeKindExploration, RegionID: r3,
			Label: "Psionic Corridor", PosX: 3, PosY: 3},
		{NodeID: "underdark.mind_flayer", Kind: NodeKindElite, RegionID: r3,
			Label: "Mind Flayer Elder", PosX: 4, PosY: 3},
		{NodeID: "underdark.throne_approach", Kind: NodeKindMerge, RegionID: r4,
			Label: "Throne Approach", PosX: 5, PosY: 2},
		{NodeID: "underdark.boss", Kind: NodeKindBoss, IsBoss: true, RegionID: r4,
			Label: "Deep Throne", PosX: 6, PosY: 2},
	}
	edges := []ZoneEdge{
		{From: "underdark.entry", To: "underdark.tunnel_descent", Lock: LockNone},
		{From: "underdark.tunnel_descent", To: "underdark.fork1", Lock: LockNone},
		// Fork1 — three regional arms.
		{From: "underdark.fork1", To: "underdark.drow_patrol", Lock: LockNone, Weight: 1},
		{From: "underdark.fork1", To: "underdark.psionic_corridor",
			Lock: LockPerception, LockData: map[string]any{"dc": 16},
			Hint: "a faint psionic pulse — somewhere a mind is broadcasting", Weight: 2},
		{From: "underdark.fork1", To: "underdark.deep_chasm",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "CON", "dc": 15},
			Hint: "a vertical shaft humming with cold air — the climb will hurt", Weight: 3},
		// R2 arm.
		{From: "underdark.drow_patrol", To: "underdark.drow_captain", Lock: LockNone},
		{From: "underdark.drow_captain", To: "underdark.throne_approach", Lock: LockNone},
		// R3 arm.
		{From: "underdark.psionic_corridor", To: "underdark.mind_flayer", Lock: LockNone},
		{From: "underdark.mind_flayer", To: "underdark.throne_approach", Lock: LockNone},
		// R1 deep arm — single node back to merge.
		{From: "underdark.deep_chasm", To: "underdark.throne_approach", Lock: LockNone},
		{From: "underdark.throne_approach", To: "underdark.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneUnderdark, nodes, edges)
}

func init() {
	registerZoneGraph(zoneUnderdarkGraph())
}
