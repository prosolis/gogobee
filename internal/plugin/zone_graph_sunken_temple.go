package plugin

// Phase G8c — Sunken Temple of Dar'eth branching graph.
//
// T2 zone. Shape: sequential forks (no mid-path merge). Four distinct
// paths to the boss; the player commits at fork1 (dry vs. wet) and
// commits again at fork2a/fork2b. Path is unique up to the boss room.
//
//   entry → flooded_atrium → fork1
//                              ├─[unlocked, w=1]── fork2a (dry)
//                              │                      ├─[unlocked]── kuo_toa_pen (elite) → boss
//                              │                      └─[STR DC 13]── glyph_chamber → boss
//                              └─[Perception DC 13, w=2]── fork2b (wet)
//                                                     ├─[unlocked]── aboleth_thralls → boss
//                                                     └─[Perception DC 15]── coral_reliquary (secret) → boss
//
// Distinct from the Crypt diamond and the Forest asymmetric-diamond:
// no two paths share a mid-node, so the !zone map paints two parallel
// "Y" subtrees instead of a converging branch. The four leaves all
// terminate at the boss (validator requires exactly one boss node).

func zoneSunkenTempleGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "sunken_temple.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Tide-Stained Threshold", PosX: 0, PosY: 2},
		{NodeID: "sunken_temple.flooded_atrium", Kind: NodeKindExploration,
			Label: "Flooded Atrium", PosX: 1, PosY: 2},
		{NodeID: "sunken_temple.fork1", Kind: NodeKindFork,
			Label: "Split Stair", PosX: 2, PosY: 2},
		{NodeID: "sunken_temple.fork2a", Kind: NodeKindFork,
			Label: "Dry Crossing", PosX: 3, PosY: 0},
		{NodeID: "sunken_temple.fork2b", Kind: NodeKindFork,
			Label: "Submerged Crossing", PosX: 3, PosY: 4},
		{NodeID: "sunken_temple.kuo_toa_pen", Kind: NodeKindElite,
			Label: "Kuo-toa Pen", PosX: 4, PosY: 0},
		{NodeID: "sunken_temple.glyph_chamber", Kind: NodeKindExploration,
			Label: "Glyph Chamber", PosX: 4, PosY: 1},
		{NodeID: "sunken_temple.aboleth_thralls", Kind: NodeKindExploration,
			Label: "Thrall Pool", PosX: 4, PosY: 3},
		{NodeID: "sunken_temple.coral_reliquary", Kind: NodeKindSecret,
			Label: "Coral Reliquary", PosX: 4, PosY: 4,
			Content: ZoneNodeContent{LootBias: 1.8}},
		{NodeID: "sunken_temple.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Aboleth's Pool", PosX: 5, PosY: 2},
	}
	edges := []ZoneEdge{
		{From: "sunken_temple.entry", To: "sunken_temple.flooded_atrium", Lock: LockNone},
		{From: "sunken_temple.flooded_atrium", To: "sunken_temple.fork1", Lock: LockNone},
		{From: "sunken_temple.fork1", To: "sunken_temple.fork2a", Lock: LockNone, Weight: 1},
		{From: "sunken_temple.fork1", To: "sunken_temple.fork2b",
			Lock: LockPerception, LockData: map[string]any{"dc": 13},
			Hint: "wet stone glistens down a side passage", Weight: 2},
		{From: "sunken_temple.fork2a", To: "sunken_temple.kuo_toa_pen", Lock: LockNone, Weight: 1},
		{From: "sunken_temple.fork2a", To: "sunken_temple.glyph_chamber",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "STR", "dc": 13},
			Hint: "a stone door wedged half-shut by silt", Weight: 2},
		{From: "sunken_temple.fork2b", To: "sunken_temple.aboleth_thralls", Lock: LockNone, Weight: 1},
		{From: "sunken_temple.fork2b", To: "sunken_temple.coral_reliquary",
			Lock: LockPerception, LockData: map[string]any{"dc": 15},
			Hint: "a coral arch glittering under the surface", Weight: 2},
		{From: "sunken_temple.kuo_toa_pen", To: "sunken_temple.boss", Lock: LockNone},
		{From: "sunken_temple.glyph_chamber", To: "sunken_temple.boss", Lock: LockNone},
		{From: "sunken_temple.aboleth_thralls", To: "sunken_temple.boss", Lock: LockNone},
		{From: "sunken_temple.coral_reliquary", To: "sunken_temple.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneSunkenTemple, nodes, edges)
}

func init() {
	registerZoneGraph(zoneSunkenTempleGraph())
}
