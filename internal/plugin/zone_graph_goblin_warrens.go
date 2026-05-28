package plugin

// Goblin Warrens branching graph.
//
// Long-expedition plan D1: extended from the original 7-node sketch to
// the new T1 length band (12–14 rooms traversed). Topology preserves the
// original single-fork "Perception skips the elite" shape — the warband
// branch is the default path and houses the zone's only Elite; the
// collapsed-shaft branch is hint-gated, elite-free, and roughly equal in
// node count so the two routes feel balanced.
//
//   entry → tunnel_descent → guard_post → snare_trap (TRAP) →
//   patrol_route → cavern_junction (FORK)
//                          ├──[unlocked]── warband_pit (ELITE)
//                          │                → trophy_chamber → kennel_path ──┐
//                          │                                                  ├── warchief_hall
//                          └──[Perception DC 12]── collapsed_shaft             │   → ritual_dais
//                                → fungus_grotto → service_kitchens ──────────┘   → throne_approach
//                                                                                  → boss
//
// Per-tier room budget: 16 graph nodes total; longest-path traversal is
// 13 (entry-side 5 + fork 1 + branch 3 + post-merge 4), which lands in
// the §2 T1 band [12,14] for both branches.

func zoneGoblinWarrensGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "goblin_warrens.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Tunnel Mouth", PosX: 0, PosY: 1},
		{NodeID: "goblin_warrens.tunnel_descent", Kind: NodeKindExploration,
			Label: "Slick Descent", PosX: 1, PosY: 1},
		{NodeID: "goblin_warrens.guard_post", Kind: NodeKindExploration,
			Label: "Goblin Guard Post", PosX: 2, PosY: 1},
		{NodeID: "goblin_warrens.snare_trap", Kind: NodeKindTrap,
			Label: "Tripwire Snare", PosX: 3, PosY: 1},
		{NodeID: "goblin_warrens.patrol_route", Kind: NodeKindExploration,
			Label: "Patrol Route", PosX: 4, PosY: 1},
		{NodeID: "goblin_warrens.cavern_junction", Kind: NodeKindFork,
			Label: "Cavern Junction", PosX: 5, PosY: 1},

		// Left branch — default, contains the elite.
		{NodeID: "goblin_warrens.warband_pit", Kind: NodeKindElite,
			Label: "Warband Pit", PosX: 6, PosY: 0},
		{NodeID: "goblin_warrens.trophy_chamber", Kind: NodeKindExploration,
			Label: "Trophy Chamber", PosX: 7, PosY: 0},
		{NodeID: "goblin_warrens.kennel_path", Kind: NodeKindExploration,
			Label: "Wolf Kennel Path", PosX: 8, PosY: 0},

		// Right branch — perception-gated, elite-free.
		{NodeID: "goblin_warrens.collapsed_shaft", Kind: NodeKindExploration,
			Label: "Collapsed Shaft", PosX: 6, PosY: 2},
		{NodeID: "goblin_warrens.fungus_grotto", Kind: NodeKindExploration,
			Label: "Fungus Grotto", PosX: 7, PosY: 2},
		{NodeID: "goblin_warrens.service_kitchens", Kind: NodeKindExploration,
			Label: "Goblin Kitchens", PosX: 8, PosY: 2},

		// Post-merge approach.
		{NodeID: "goblin_warrens.warchief_hall", Kind: NodeKindMerge,
			Label: "Warchief's Antechamber", PosX: 9, PosY: 1},
		{NodeID: "goblin_warrens.ritual_dais", Kind: NodeKindExploration,
			Label: "Ritual Dais", PosX: 10, PosY: 1},
		{NodeID: "goblin_warrens.throne_approach", Kind: NodeKindExploration,
			Label: "Throne Approach", PosX: 11, PosY: 1},
		{NodeID: "goblin_warrens.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Grol's Throne", PosX: 12, PosY: 1},
	}
	edges := []ZoneEdge{
		{From: "goblin_warrens.entry", To: "goblin_warrens.tunnel_descent", Lock: LockNone},
		{From: "goblin_warrens.tunnel_descent", To: "goblin_warrens.guard_post", Lock: LockNone},
		{From: "goblin_warrens.guard_post", To: "goblin_warrens.snare_trap", Lock: LockNone},
		{From: "goblin_warrens.snare_trap", To: "goblin_warrens.patrol_route", Lock: LockNone},
		{From: "goblin_warrens.patrol_route", To: "goblin_warrens.cavern_junction", Lock: LockNone},

		{From: "goblin_warrens.cavern_junction", To: "goblin_warrens.warband_pit", Lock: LockNone, Weight: 1},
		{From: "goblin_warrens.cavern_junction", To: "goblin_warrens.collapsed_shaft",
			Lock: LockPerception, LockData: map[string]any{"dc": 12},
			Hint: "a muffled scrape behind a heap of fallen timbers", Weight: 2},

		{From: "goblin_warrens.warband_pit", To: "goblin_warrens.trophy_chamber", Lock: LockNone},
		{From: "goblin_warrens.trophy_chamber", To: "goblin_warrens.kennel_path", Lock: LockNone},
		{From: "goblin_warrens.kennel_path", To: "goblin_warrens.warchief_hall", Lock: LockNone},

		{From: "goblin_warrens.collapsed_shaft", To: "goblin_warrens.fungus_grotto", Lock: LockNone},
		{From: "goblin_warrens.fungus_grotto", To: "goblin_warrens.service_kitchens", Lock: LockNone},
		{From: "goblin_warrens.service_kitchens", To: "goblin_warrens.warchief_hall", Lock: LockNone},

		{From: "goblin_warrens.warchief_hall", To: "goblin_warrens.ritual_dais", Lock: LockNone},
		{From: "goblin_warrens.ritual_dais", To: "goblin_warrens.throne_approach", Lock: LockNone},
		{From: "goblin_warrens.throne_approach", To: "goblin_warrens.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneGoblinWarrens, nodes, edges)
}

func init() {
	registerZoneGraph(zoneGoblinWarrensGraph())
}
