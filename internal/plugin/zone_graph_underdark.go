package plugin

// The Underdark branching graph (multi-region).
//
// Long-expedition plan D1-d: extended from the original 10-node sketch
// to the new T4 length band (28–34 rooms traversed). Topology preserves
// the G8i design intent — three regional arms converge at the deep
// throne — and adds the linear depth each arm now needs to feel like a
// region in its own right rather than a single side-room.
//
//   R1 surface_tunnels preamble (8 nodes):
//     entry → tunnel_descent → moss_corridor → fungal_grove →
//     collapsed_arch (TRAP) → ledge_walk → shrine_to_lolth → fork1
//
//   Fork1 → three regional arms (each crosses a region boundary except
//   the deep_chasm arm, which intentionally stays in R1 — same as G8i):
//
//   R2 drow_outpost arm (12 nodes):
//     drow_patrol → drow_picket → corridor_of_eyes → spider_warren →
//     web_passage → drow_chapel → drow_armory → drow_captain (ELITE) →
//     captain_quarters → drow_descent → drow_passage → drow_gate
//
//   R3 illithid_warren arm (12 nodes):
//     psionic_corridor → whispering_hall → silenced_chamber →
//     mind_tank_room → thrall_pens → broodling_chamber →
//     mind_flayer (ELITE) → flayer_sanctum → illithid_descent →
//     illithid_passage → illithid_gate → illithid_threshold
//
//   R1 deep_chasm spur (4 nodes — short arm, the “rich-harvest if you
//   can take the CON-15 climb” route):
//     deep_chasm (HARVEST) → chasm_floor → chasm_bridge → chasm_ascent
//
//   R4 deep_throne tail (10 nodes including boss):
//     throne_approach (MERGE) → throne_stairs → throne_corridor →
//     throne_antechamber → throne_guard_post → throne_doors →
//     throne_gallery → throne_inner_hall → throne_steps → boss
//
// Longest entry→boss walk is via either R2 or R3 arm + R4 tail:
//   8 (preamble) + 12 (arm) + 10 (tail) = 30 nodes, inside [28,34].
// The deep_chasm spur reaches the boss in 8 + 4 + 10 = 22 nodes — a
// faster route that trades the elite encounter for the harvest spur,
// preserving the original “high-CON cost, no elite, denser loot”
// trade-off.
//
// Per-region totals (single-traverse): R1 surface_tunnels = 8 preamble,
// R1 chasm spur = 4 (only one is on any given walk), R2 = 12, R3 = 12,
// R4 = 10. Boundary transitions remain at fork1 → arm-entry and
// arm-tail → throne_approach; the G6 fireGraphRegionTransition hook
// still fires exactly once per fork choice (or twice if the player
// later !region-travels into a non-walked arm).
//
// Trap anchor (Collapsed Arch) is new — the original G8i graph had no
// Trap node. Placed in the R1 preamble so every walk hits it.

func zoneUnderdarkGraph() ZoneGraph {
	r1 := "underdark_surface_tunnels"
	r2 := "underdark_drow_outpost"
	r3 := "underdark_illithid_warren"
	r4 := "underdark_deep_throne"

	nodes := []ZoneNode{
		// R1 preamble.
		{NodeID: "underdark.entry", Kind: NodeKindEntry, IsEntry: true, RegionID: r1,
			Label: "Cave Mouth", PosX: 0, PosY: 2},
		{NodeID: "underdark.tunnel_descent", Kind: NodeKindExploration, RegionID: r1,
			Label: "Tunnel Descent", PosX: 1, PosY: 2},
		{NodeID: "underdark.moss_corridor", Kind: NodeKindExploration, RegionID: r1,
			Label: "Glowmoss Corridor", PosX: 2, PosY: 2},
		{NodeID: "underdark.fungal_grove", Kind: NodeKindExploration, RegionID: r1,
			Label: "Fungal Grove", PosX: 3, PosY: 2},
		{NodeID: "underdark.collapsed_arch", Kind: NodeKindTrap, RegionID: r1,
			Label: "Collapsed Arch", PosX: 4, PosY: 2},
		{NodeID: "underdark.ledge_walk", Kind: NodeKindExploration, RegionID: r1,
			Label: "Ledge Walk", PosX: 5, PosY: 2},
		{NodeID: "underdark.shrine_to_lolth", Kind: NodeKindExploration, RegionID: r1,
			Label: "Defaced Shrine", PosX: 6, PosY: 2},
		{NodeID: "underdark.fork1", Kind: NodeKindFork, RegionID: r1,
			Label: "Three-Way Pass", PosX: 7, PosY: 2},

		// R1 deep_chasm spur (HARVEST, stays in surface_tunnels).
		{NodeID: "underdark.deep_chasm", Kind: NodeKindHarvest, RegionID: r1,
			Label: "Deep Chasm", PosX: 8, PosY: 4,
			Content: ZoneNodeContent{LootBias: 1.8}},
		{NodeID: "underdark.chasm_floor", Kind: NodeKindExploration, RegionID: r1,
			Label: "Chasm Floor", PosX: 9, PosY: 4},
		{NodeID: "underdark.chasm_bridge", Kind: NodeKindExploration, RegionID: r1,
			Label: "Stone-Web Bridge", PosX: 10, PosY: 4},
		{NodeID: "underdark.chasm_ascent", Kind: NodeKindExploration, RegionID: r1,
			Label: "Chasm Ascent", PosX: 11, PosY: 4},

		// R2 drow_outpost arm.
		{NodeID: "underdark.drow_patrol", Kind: NodeKindExploration, RegionID: r2,
			Label: "Drow Patrol", PosX: 8, PosY: 0},
		{NodeID: "underdark.drow_picket", Kind: NodeKindExploration, RegionID: r2,
			Label: "Sentry Picket", PosX: 9, PosY: 0},
		{NodeID: "underdark.corridor_of_eyes", Kind: NodeKindExploration, RegionID: r2,
			Label: "Corridor of Eyes", PosX: 10, PosY: 0},
		{NodeID: "underdark.spider_warren", Kind: NodeKindExploration, RegionID: r2,
			Label: "Spider Warren", PosX: 11, PosY: 0},
		{NodeID: "underdark.web_passage", Kind: NodeKindExploration, RegionID: r2,
			Label: "Web Passage", PosX: 12, PosY: 0},
		{NodeID: "underdark.drow_chapel", Kind: NodeKindExploration, RegionID: r2,
			Label: "Lolth's Chapel", PosX: 13, PosY: 0},
		{NodeID: "underdark.drow_armory", Kind: NodeKindExploration, RegionID: r2,
			Label: "Drow Armory", PosX: 14, PosY: 0},
		{NodeID: "underdark.drow_captain", Kind: NodeKindElite, RegionID: r2,
			Label: "Drow Captain's Camp", PosX: 15, PosY: 0},
		{NodeID: "underdark.captain_quarters", Kind: NodeKindExploration, RegionID: r2,
			Label: "Captain's Quarters", PosX: 16, PosY: 0},
		{NodeID: "underdark.drow_descent", Kind: NodeKindExploration, RegionID: r2,
			Label: "Drow Descent", PosX: 17, PosY: 0},
		{NodeID: "underdark.drow_passage", Kind: NodeKindExploration, RegionID: r2,
			Label: "Lower Passage", PosX: 18, PosY: 0},
		{NodeID: "underdark.drow_gate", Kind: NodeKindExploration, RegionID: r2,
			Label: "Drow Gate", PosX: 19, PosY: 0},

		// R3 illithid_warren arm.
		{NodeID: "underdark.psionic_corridor", Kind: NodeKindExploration, RegionID: r3,
			Label: "Psionic Corridor", PosX: 8, PosY: 2},
		{NodeID: "underdark.whispering_hall", Kind: NodeKindExploration, RegionID: r3,
			Label: "Whispering Hall", PosX: 9, PosY: 2},
		{NodeID: "underdark.silenced_chamber", Kind: NodeKindExploration, RegionID: r3,
			Label: "Silenced Chamber", PosX: 10, PosY: 2},
		{NodeID: "underdark.mind_tank_room", Kind: NodeKindExploration, RegionID: r3,
			Label: "Brine Tanks", PosX: 11, PosY: 2},
		{NodeID: "underdark.thrall_pens", Kind: NodeKindExploration, RegionID: r3,
			Label: "Thrall Pens", PosX: 12, PosY: 2},
		{NodeID: "underdark.broodling_chamber", Kind: NodeKindExploration, RegionID: r3,
			Label: "Broodling Chamber", PosX: 13, PosY: 2},
		{NodeID: "underdark.mind_flayer", Kind: NodeKindElite, RegionID: r3,
			Label: "Mind Flayer Elder", PosX: 14, PosY: 2},
		{NodeID: "underdark.flayer_sanctum", Kind: NodeKindExploration, RegionID: r3,
			Label: "Flayer Sanctum", PosX: 15, PosY: 2},
		{NodeID: "underdark.illithid_descent", Kind: NodeKindExploration, RegionID: r3,
			Label: "Illithid Descent", PosX: 16, PosY: 2},
		{NodeID: "underdark.illithid_passage", Kind: NodeKindExploration, RegionID: r3,
			Label: "Slime Corridor", PosX: 17, PosY: 2},
		{NodeID: "underdark.illithid_gate", Kind: NodeKindExploration, RegionID: r3,
			Label: "Illithid Gate", PosX: 18, PosY: 2},
		{NodeID: "underdark.illithid_threshold", Kind: NodeKindExploration, RegionID: r3,
			Label: "Threshold of Thought", PosX: 19, PosY: 2},

		// R4 deep_throne tail.
		{NodeID: "underdark.throne_approach", Kind: NodeKindMerge, RegionID: r4,
			Label: "Throne Approach", PosX: 20, PosY: 2},
		{NodeID: "underdark.throne_stairs", Kind: NodeKindExploration, RegionID: r4,
			Label: "Obsidian Stairs", PosX: 21, PosY: 2},
		{NodeID: "underdark.throne_corridor", Kind: NodeKindExploration, RegionID: r4,
			Label: "Throne Corridor", PosX: 22, PosY: 2},
		{NodeID: "underdark.throne_antechamber", Kind: NodeKindExploration, RegionID: r4,
			Label: "Antechamber", PosX: 23, PosY: 2},
		{NodeID: "underdark.throne_guard_post", Kind: NodeKindExploration, RegionID: r4,
			Label: "Guard Post", PosX: 24, PosY: 2},
		{NodeID: "underdark.throne_doors", Kind: NodeKindExploration, RegionID: r4,
			Label: "Riven Doors", PosX: 25, PosY: 2},
		{NodeID: "underdark.throne_gallery", Kind: NodeKindExploration, RegionID: r4,
			Label: "Throne Gallery", PosX: 26, PosY: 2},
		{NodeID: "underdark.throne_inner_hall", Kind: NodeKindExploration, RegionID: r4,
			Label: "Inner Hall", PosX: 27, PosY: 2},
		{NodeID: "underdark.throne_steps", Kind: NodeKindExploration, RegionID: r4,
			Label: "Throne Steps", PosX: 28, PosY: 2},
		{NodeID: "underdark.boss", Kind: NodeKindBoss, IsBoss: true, RegionID: r4,
			Label: "Deep Throne", PosX: 29, PosY: 2},
	}
	edges := []ZoneEdge{
		// R1 preamble.
		{From: "underdark.entry", To: "underdark.tunnel_descent", Lock: LockNone},
		{From: "underdark.tunnel_descent", To: "underdark.moss_corridor", Lock: LockNone},
		{From: "underdark.moss_corridor", To: "underdark.fungal_grove", Lock: LockNone},
		{From: "underdark.fungal_grove", To: "underdark.collapsed_arch", Lock: LockNone},
		{From: "underdark.collapsed_arch", To: "underdark.ledge_walk", Lock: LockNone},
		{From: "underdark.ledge_walk", To: "underdark.shrine_to_lolth", Lock: LockNone},
		{From: "underdark.shrine_to_lolth", To: "underdark.fork1", Lock: LockNone},

		// Fork1 — three regional arms (unchanged lock/hint identity).
		{From: "underdark.fork1", To: "underdark.drow_patrol", Lock: LockNone, Weight: 1},
		{From: "underdark.fork1", To: "underdark.psionic_corridor",
			Lock: LockPerception, LockData: map[string]any{"dc": 16},
			Hint: "a faint psionic pulse — somewhere a mind is broadcasting", Weight: 2},
		{From: "underdark.fork1", To: "underdark.deep_chasm",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "CON", "dc": 15},
			Hint: "a vertical shaft humming with cold air — the climb will hurt", Weight: 3},

		// R1 spur.
		{From: "underdark.deep_chasm", To: "underdark.chasm_floor", Lock: LockNone},
		{From: "underdark.chasm_floor", To: "underdark.chasm_bridge", Lock: LockNone},
		{From: "underdark.chasm_bridge", To: "underdark.chasm_ascent", Lock: LockNone},
		{From: "underdark.chasm_ascent", To: "underdark.throne_approach", Lock: LockNone},

		// R2 arm.
		{From: "underdark.drow_patrol", To: "underdark.drow_picket", Lock: LockNone},
		{From: "underdark.drow_picket", To: "underdark.corridor_of_eyes", Lock: LockNone},
		{From: "underdark.corridor_of_eyes", To: "underdark.spider_warren", Lock: LockNone},
		{From: "underdark.spider_warren", To: "underdark.web_passage", Lock: LockNone},
		{From: "underdark.web_passage", To: "underdark.drow_chapel", Lock: LockNone},
		{From: "underdark.drow_chapel", To: "underdark.drow_armory", Lock: LockNone},
		{From: "underdark.drow_armory", To: "underdark.drow_captain", Lock: LockNone},
		{From: "underdark.drow_captain", To: "underdark.captain_quarters", Lock: LockNone},
		{From: "underdark.captain_quarters", To: "underdark.drow_descent", Lock: LockNone},
		{From: "underdark.drow_descent", To: "underdark.drow_passage", Lock: LockNone},
		{From: "underdark.drow_passage", To: "underdark.drow_gate", Lock: LockNone},
		{From: "underdark.drow_gate", To: "underdark.throne_approach", Lock: LockNone},

		// R3 arm.
		{From: "underdark.psionic_corridor", To: "underdark.whispering_hall", Lock: LockNone},
		{From: "underdark.whispering_hall", To: "underdark.silenced_chamber", Lock: LockNone},
		{From: "underdark.silenced_chamber", To: "underdark.mind_tank_room", Lock: LockNone},
		{From: "underdark.mind_tank_room", To: "underdark.thrall_pens", Lock: LockNone},
		{From: "underdark.thrall_pens", To: "underdark.broodling_chamber", Lock: LockNone},
		{From: "underdark.broodling_chamber", To: "underdark.mind_flayer", Lock: LockNone},
		{From: "underdark.mind_flayer", To: "underdark.flayer_sanctum", Lock: LockNone},
		{From: "underdark.flayer_sanctum", To: "underdark.illithid_descent", Lock: LockNone},
		{From: "underdark.illithid_descent", To: "underdark.illithid_passage", Lock: LockNone},
		{From: "underdark.illithid_passage", To: "underdark.illithid_gate", Lock: LockNone},
		{From: "underdark.illithid_gate", To: "underdark.illithid_threshold", Lock: LockNone},
		{From: "underdark.illithid_threshold", To: "underdark.throne_approach", Lock: LockNone},

		// R4 tail.
		{From: "underdark.throne_approach", To: "underdark.throne_stairs", Lock: LockNone},
		{From: "underdark.throne_stairs", To: "underdark.throne_corridor", Lock: LockNone},
		{From: "underdark.throne_corridor", To: "underdark.throne_antechamber", Lock: LockNone},
		{From: "underdark.throne_antechamber", To: "underdark.throne_guard_post", Lock: LockNone},
		{From: "underdark.throne_guard_post", To: "underdark.throne_doors", Lock: LockNone},
		{From: "underdark.throne_doors", To: "underdark.throne_gallery", Lock: LockNone},
		{From: "underdark.throne_gallery", To: "underdark.throne_inner_hall", Lock: LockNone},
		{From: "underdark.throne_inner_hall", To: "underdark.throne_steps", Lock: LockNone},
		{From: "underdark.throne_steps", To: "underdark.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneUnderdark, nodes, edges)
}

func init() {
	registerZoneGraph(zoneUnderdarkGraph())
}
