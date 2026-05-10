package plugin

// Phase G8h — The Abyss Portal branching graph.
//
// T5 zone. Shape: three sequential forks at increasing depth — binary,
// binary, ternary capstone. The player makes more decisions in this
// zone than in any other (3 separate fork prompts), matching the
// "things keep getting worse" thematic descent.
//
//   entry → fractured_threshold → fork1 (binary)
//                                   ├─[unlocked]── burning_wastes ─┐
//                                   └─[Perception DC 16]── silent_chambers ─┤
//                                                                           ↓
//                                                                       fork2_a (binary)
//                                                                           ├─[unlocked]── vrock_aerie (elite marilith) ─┐
//                                                                           └─[CON DC 16]── mind_corridor ───────────────┤
//                                                                                                                          ↓
//                                                                                                                       fork3 (3-way capstone)
//                                                                                                                           ├─[unlocked]── direct_assault → boss
//                                                                                                                           ├─[CHA DC 18]── usurper_throne → boss
//                                                                                                                           └─[Perception DC 18]── reality_seam (secret) → boss
//
// First authored zone with three sequential forks AND first use of
// LockStatCheck CON — completing the full ability roster (STR/DEX/
// CON/INT/WIS/CHA all appear in shipping zones by G8h). reality_seam
// LootBias 3.0 reflects the Abyss capstone secret.

func zoneAbyssPortalGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "abyss_portal.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "The Open Door", PosX: 0, PosY: 2},
		{NodeID: "abyss_portal.fractured_threshold", Kind: NodeKindExploration,
			Label: "Fractured Threshold", PosX: 1, PosY: 2},
		{NodeID: "abyss_portal.fork1", Kind: NodeKindFork,
			Label: "First Reality-Break", PosX: 2, PosY: 2},
		{NodeID: "abyss_portal.burning_wastes", Kind: NodeKindExploration,
			Label: "Burning Wastes", PosX: 3, PosY: 1},
		{NodeID: "abyss_portal.silent_chambers", Kind: NodeKindExploration,
			Label: "Silent Chambers", PosX: 3, PosY: 3},
		{NodeID: "abyss_portal.fork2", Kind: NodeKindFork,
			Label: "Second Reality-Break", PosX: 4, PosY: 2},
		{NodeID: "abyss_portal.vrock_aerie", Kind: NodeKindElite,
			Label: "Marilith's Aerie", PosX: 5, PosY: 1},
		{NodeID: "abyss_portal.mind_corridor", Kind: NodeKindExploration,
			Label: "Corridor of Whispers", PosX: 5, PosY: 3},
		{NodeID: "abyss_portal.fork3", Kind: NodeKindFork,
			Label: "Third Reality-Break", PosX: 6, PosY: 2},
		{NodeID: "abyss_portal.direct_assault", Kind: NodeKindExploration,
			Label: "Direct Assault", PosX: 7, PosY: 1},
		{NodeID: "abyss_portal.usurper_throne", Kind: NodeKindExploration,
			Label: "Usurper's Approach", PosX: 7, PosY: 2},
		{NodeID: "abyss_portal.reality_seam", Kind: NodeKindSecret,
			Label: "Reality Seam", PosX: 7, PosY: 3,
			Content: ZoneNodeContent{LootBias: 3.0}},
		{NodeID: "abyss_portal.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Belaxath's Throne", PosX: 8, PosY: 2},
	}
	edges := []ZoneEdge{
		{From: "abyss_portal.entry", To: "abyss_portal.fractured_threshold", Lock: LockNone},
		{From: "abyss_portal.fractured_threshold", To: "abyss_portal.fork1", Lock: LockNone},
		// Fork1 — perception side path.
		{From: "abyss_portal.fork1", To: "abyss_portal.burning_wastes", Lock: LockNone, Weight: 1},
		{From: "abyss_portal.fork1", To: "abyss_portal.silent_chambers",
			Lock: LockPerception, LockData: map[string]any{"dc": 16},
			Hint: "a silence so complete you can hear your own pulse — and something else's", Weight: 2},
		{From: "abyss_portal.burning_wastes", To: "abyss_portal.fork2", Lock: LockNone},
		{From: "abyss_portal.silent_chambers", To: "abyss_portal.fork2", Lock: LockNone},
		// Fork2 — CON wall (psychic pressure).
		{From: "abyss_portal.fork2", To: "abyss_portal.vrock_aerie", Lock: LockNone, Weight: 1},
		{From: "abyss_portal.fork2", To: "abyss_portal.mind_corridor",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "CON", "dc": 16},
			Hint: "the air thickens — your bones know the wrong direction is also down", Weight: 2},
		{From: "abyss_portal.vrock_aerie", To: "abyss_portal.fork3", Lock: LockNone},
		{From: "abyss_portal.mind_corridor", To: "abyss_portal.fork3", Lock: LockNone},
		// Fork3 — capstone 3-way.
		{From: "abyss_portal.fork3", To: "abyss_portal.direct_assault", Lock: LockNone, Weight: 1},
		{From: "abyss_portal.fork3", To: "abyss_portal.usurper_throne",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "CHA", "dc": 18},
			Hint: "Belaxath has not noticed you yet — claim authority before he does", Weight: 2},
		{From: "abyss_portal.fork3", To: "abyss_portal.reality_seam",
			Lock: LockPerception, LockData: map[string]any{"dc": 18},
			Hint: "a hairline crack in the air itself — somewhere there is a place that is not here", Weight: 3},
		{From: "abyss_portal.direct_assault", To: "abyss_portal.boss", Lock: LockNone},
		{From: "abyss_portal.usurper_throne", To: "abyss_portal.boss", Lock: LockNone},
		{From: "abyss_portal.reality_seam", To: "abyss_portal.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneAbyssPortal, nodes, edges)
}

func init() {
	registerZoneGraph(zoneAbyssPortalGraph())
}
