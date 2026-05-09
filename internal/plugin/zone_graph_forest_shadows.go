package plugin

// Phase G8b — Forest of Shadows branching graph.
//
// T2 zone. Per plan §G8 ("two forks for T2+") and the G8 design
// session: deliberately a different shape from the Crypt diamond.
//
// Shape: asymmetric main fork + WIS-gated secret pre-boss.
//
//   entry → twisted_path → fork1
//                            ├─[unlocked, w=1]── grove_descent → dryad_circle (elite) ─┐
//                            │                                                          ├── fork2
//                            └─[Perception DC 13, w=2]── thorn_tunnel ──────────────────┘
//                                                                                      │
//                                              ┌─[unlocked, w=1]── boss ────────────────┘
//                                              │
//                                              └─[WIS DC 14, w=2]── sapling_shrine (secret) → boss
//
// Asymmetry: long branch is two mid-nodes (grove_descent → dryad_circle
// (elite)), short branch is one (thorn_tunnel). Map renders the long
// branch as a "+1 column" arm so the topology reads at a glance.
// Secret uses LockStatCheck WIS (not Perception) to exercise the
// stat-check lock kind — Crypt's secret is Perception, so this is the
// first authored zone using stat_check.

func zoneForestShadowsGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "forest_shadows.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Forest Edge", PosX: 0, PosY: 1},
		{NodeID: "forest_shadows.twisted_path", Kind: NodeKindExploration,
			Label: "Twisted Path", PosX: 1, PosY: 1},
		{NodeID: "forest_shadows.fork1", Kind: NodeKindFork,
			Label: "Splintered Trail", PosX: 2, PosY: 1},
		{NodeID: "forest_shadows.grove_descent", Kind: NodeKindExploration,
			Label: "Grove Descent", PosX: 3, PosY: 0},
		{NodeID: "forest_shadows.dryad_circle", Kind: NodeKindElite,
			Label: "Dryad's Circle", PosX: 4, PosY: 0},
		{NodeID: "forest_shadows.thorn_tunnel", Kind: NodeKindExploration,
			Label: "Thorn Tunnel", PosX: 3, PosY: 2},
		{NodeID: "forest_shadows.fork2", Kind: NodeKindFork,
			Label: "Hollow King's Approach", PosX: 5, PosY: 1},
		{NodeID: "forest_shadows.sapling_shrine", Kind: NodeKindSecret,
			Label: "Sapling Shrine", PosX: 6, PosY: 2,
			Content: ZoneNodeContent{LootBias: 2.0}},
		{NodeID: "forest_shadows.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Throne of Antlers", PosX: 7, PosY: 1},
	}
	edges := []ZoneEdge{
		{From: "forest_shadows.entry", To: "forest_shadows.twisted_path", Lock: LockNone},
		{From: "forest_shadows.twisted_path", To: "forest_shadows.fork1", Lock: LockNone},
		{From: "forest_shadows.fork1", To: "forest_shadows.grove_descent", Lock: LockNone, Weight: 1},
		{From: "forest_shadows.fork1", To: "forest_shadows.thorn_tunnel",
			Lock: LockPerception, LockData: map[string]any{"dc": 13},
			Hint: "a deer-track winding into thicker brush", Weight: 2},
		{From: "forest_shadows.grove_descent", To: "forest_shadows.dryad_circle", Lock: LockNone},
		{From: "forest_shadows.dryad_circle", To: "forest_shadows.fork2", Lock: LockNone},
		{From: "forest_shadows.thorn_tunnel", To: "forest_shadows.fork2", Lock: LockNone},
		{From: "forest_shadows.fork2", To: "forest_shadows.boss", Lock: LockNone, Weight: 1},
		{From: "forest_shadows.fork2", To: "forest_shadows.sapling_shrine",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "WIS", "dc": 14},
			Hint: "a humming you can almost place — antlers, somewhere", Weight: 2},
		{From: "forest_shadows.sapling_shrine", To: "forest_shadows.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneForestShadows, nodes, edges)
}

func init() {
	registerZoneGraph(zoneForestShadowsGraph())
}
