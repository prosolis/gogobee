package plugin

// Phase G8a — Goblin Warrens branching graph.
//
// First non-POC zone. T1 design constraint per
// gogobee_branching_zones_plan.md §G8: one fork per zone for T1, no
// secret (the secret pattern is exercised by Crypt of Valdris; Goblin
// Warrens deliberately validates the bare single-fork shape on a
// non-POC zone so we can confirm legacy compileLegacyZoneGraph fully
// hands off to the authored graph end-to-end).
//
//   entry → guard_post → fork
//                          ├──[unlocked]── warband_pit (elite) ──┐
//                          │                                      ├── warchief_hall → boss
//                          └──[Perception DC 12]── collapsed_shaft ┘
//
// Both branches reach the boss; Perception side is hinted (per plan:
// "Locked paths should always have a hint — cruel design otherwise").

func zoneGoblinWarrensGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "goblin_warrens.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Tunnel Mouth", PosX: 0, PosY: 1},
		{NodeID: "goblin_warrens.guard_post", Kind: NodeKindExploration,
			Label: "Goblin Guard Post", PosX: 1, PosY: 1},
		{NodeID: "goblin_warrens.fork", Kind: NodeKindFork,
			Label: "Cavern Junction", PosX: 2, PosY: 1},
		{NodeID: "goblin_warrens.warband_pit", Kind: NodeKindElite,
			Label: "Warband Pit", PosX: 3, PosY: 0},
		{NodeID: "goblin_warrens.collapsed_shaft", Kind: NodeKindExploration,
			Label: "Collapsed Shaft", PosX: 3, PosY: 2},
		{NodeID: "goblin_warrens.warchief_hall", Kind: NodeKindMerge,
			Label: "Warchief's Hall", PosX: 4, PosY: 1},
		{NodeID: "goblin_warrens.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Grol's Throne", PosX: 5, PosY: 1},
	}
	edges := []ZoneEdge{
		{From: "goblin_warrens.entry", To: "goblin_warrens.guard_post", Lock: LockNone},
		{From: "goblin_warrens.guard_post", To: "goblin_warrens.fork", Lock: LockNone},
		{From: "goblin_warrens.fork", To: "goblin_warrens.warband_pit", Lock: LockNone, Weight: 1},
		{From: "goblin_warrens.fork", To: "goblin_warrens.collapsed_shaft",
			Lock: LockPerception, LockData: map[string]any{"dc": 12},
			Hint: "a muffled scrape behind a heap of fallen timbers", Weight: 2},
		{From: "goblin_warrens.warband_pit", To: "goblin_warrens.warchief_hall", Lock: LockNone},
		{From: "goblin_warrens.collapsed_shaft", To: "goblin_warrens.warchief_hall", Lock: LockNone},
		{From: "goblin_warrens.warchief_hall", To: "goblin_warrens.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneGoblinWarrens, nodes, edges)
}

func init() {
	registerZoneGraph(zoneGoblinWarrensGraph())
}
