package plugin

// Forest of Shadows branching graph.
//
// Long-expedition plan D1-b: extended from the original 9-node sketch to
// the new T2 length band (16–20 rooms traversed). Topology preserves the
// G8 design intent — asymmetric main fork (long branch carries the
// elite) + WIS-gated secret hanging pre-boss — and adds the missing Trap
// anchor + extra exploration on both arms and the boss approach.
//
//   entry → mossy_threshold → twisted_path → root_snare (TRAP) →
//   creeping_brush → dappled_hollow → witchlight_clearing → fork1
//                            ├─[unlocked, w=1]── grove_descent →
//                            │     dryad_circle (ELITE) → torn_meadow ──┐
//                            │                                          ├── fork2
//                            └─[Perception DC 13, w=2]── thorn_tunnel   │
//                                  → briar_warren ────────────────────────┘
//                                                                    │
//                              ┌─[unlocked, w=1]── ritual_glade →     │
//                              │   antler_path → hollow_steps → boss ─┘
//                              │
//                              └─[WIS DC 14, w=2]── sapling_shrine (SECRET) → boss
//
// Asymmetry preserved: long branch is 3 mid-nodes (grove_descent →
// dryad_circle (elite) → torn_meadow), short branch is 2 (thorn_tunnel →
// briar_warren). Longest entry→boss walk = 16 (long branch path).

func zoneForestShadowsGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "forest_shadows.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Forest Edge", PosX: 0, PosY: 1},
		{NodeID: "forest_shadows.mossy_threshold", Kind: NodeKindExploration,
			Label: "Mossy Threshold", PosX: 1, PosY: 1},
		{NodeID: "forest_shadows.twisted_path", Kind: NodeKindExploration,
			Label: "Twisted Path", PosX: 2, PosY: 1},
		{NodeID: "forest_shadows.root_snare", Kind: NodeKindTrap,
			Label: "Root Snare", PosX: 3, PosY: 1},
		{NodeID: "forest_shadows.creeping_brush", Kind: NodeKindExploration,
			Label: "Creeping Brush", PosX: 4, PosY: 1},
		{NodeID: "forest_shadows.dappled_hollow", Kind: NodeKindExploration,
			Label: "Dappled Hollow", PosX: 5, PosY: 1},
		{NodeID: "forest_shadows.witchlight_clearing", Kind: NodeKindExploration,
			Label: "Witchlight Clearing", PosX: 6, PosY: 1},
		{NodeID: "forest_shadows.fork1", Kind: NodeKindFork,
			Label: "Splintered Trail", PosX: 7, PosY: 1},

		// Long branch (3 mid-nodes, carries the elite).
		{NodeID: "forest_shadows.grove_descent", Kind: NodeKindExploration,
			Label: "Grove Descent", PosX: 8, PosY: 0},
		{NodeID: "forest_shadows.dryad_circle", Kind: NodeKindElite,
			Label: "Dryad's Circle", PosX: 9, PosY: 0},
		{NodeID: "forest_shadows.torn_meadow", Kind: NodeKindExploration,
			Label: "Torn Meadow", PosX: 10, PosY: 0},

		// Short branch (2 mid-nodes, perception-gated).
		{NodeID: "forest_shadows.thorn_tunnel", Kind: NodeKindExploration,
			Label: "Thorn Tunnel", PosX: 8, PosY: 2},
		{NodeID: "forest_shadows.briar_warren", Kind: NodeKindExploration,
			Label: "Briar Warren", PosX: 9, PosY: 2},

		{NodeID: "forest_shadows.fork2", Kind: NodeKindFork,
			Label: "Hollow King's Approach", PosX: 11, PosY: 1},

		// Post-fork2 main approach.
		{NodeID: "forest_shadows.ritual_glade", Kind: NodeKindExploration,
			Label: "Ritual Glade", PosX: 12, PosY: 1},
		{NodeID: "forest_shadows.antler_path", Kind: NodeKindExploration,
			Label: "Antler Path", PosX: 13, PosY: 1},
		{NodeID: "forest_shadows.hollow_steps", Kind: NodeKindExploration,
			Label: "Hollow Steps", PosX: 14, PosY: 1},

		// Secret dangle off fork2 — WIS gate, loot-biased, shortcut to boss.
		{NodeID: "forest_shadows.sapling_shrine", Kind: NodeKindSecret,
			Label: "Sapling Shrine", PosX: 13, PosY: 2,
			Content: ZoneNodeContent{LootBias: 2.0}},

		{NodeID: "forest_shadows.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Throne of Antlers", PosX: 15, PosY: 1},
	}
	edges := []ZoneEdge{
		{From: "forest_shadows.entry", To: "forest_shadows.mossy_threshold", Lock: LockNone},
		{From: "forest_shadows.mossy_threshold", To: "forest_shadows.twisted_path", Lock: LockNone},
		{From: "forest_shadows.twisted_path", To: "forest_shadows.root_snare", Lock: LockNone},
		{From: "forest_shadows.root_snare", To: "forest_shadows.creeping_brush", Lock: LockNone},
		{From: "forest_shadows.creeping_brush", To: "forest_shadows.dappled_hollow", Lock: LockNone},
		{From: "forest_shadows.dappled_hollow", To: "forest_shadows.witchlight_clearing", Lock: LockNone},
		{From: "forest_shadows.witchlight_clearing", To: "forest_shadows.fork1", Lock: LockNone},

		{From: "forest_shadows.fork1", To: "forest_shadows.grove_descent", Lock: LockNone, Weight: 1},
		{From: "forest_shadows.fork1", To: "forest_shadows.thorn_tunnel",
			Lock: LockPerception, LockData: map[string]any{"dc": 13},
			Hint: "a deer-track winding into thicker brush", Weight: 2},

		{From: "forest_shadows.grove_descent", To: "forest_shadows.dryad_circle", Lock: LockNone},
		{From: "forest_shadows.dryad_circle", To: "forest_shadows.torn_meadow", Lock: LockNone},
		{From: "forest_shadows.torn_meadow", To: "forest_shadows.fork2", Lock: LockNone},

		{From: "forest_shadows.thorn_tunnel", To: "forest_shadows.briar_warren", Lock: LockNone},
		{From: "forest_shadows.briar_warren", To: "forest_shadows.fork2", Lock: LockNone},

		{From: "forest_shadows.fork2", To: "forest_shadows.ritual_glade", Lock: LockNone, Weight: 1},
		{From: "forest_shadows.fork2", To: "forest_shadows.sapling_shrine",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "WIS", "dc": 14},
			Hint: "a humming you can almost place — antlers, somewhere", Weight: 2},

		{From: "forest_shadows.ritual_glade", To: "forest_shadows.antler_path", Lock: LockNone},
		{From: "forest_shadows.antler_path", To: "forest_shadows.hollow_steps", Lock: LockNone},
		{From: "forest_shadows.hollow_steps", To: "forest_shadows.boss", Lock: LockNone},
		{From: "forest_shadows.sapling_shrine", To: "forest_shadows.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneForestShadows, nodes, edges)
}

func init() {
	registerZoneGraph(zoneForestShadowsGraph())
}
