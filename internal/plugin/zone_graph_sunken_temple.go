package plugin

// Sunken Temple of Dar'eth branching graph.
//
// Long-expedition plan D1-b: extended from the original 10-node sketch to
// the new T2 length band (16–20 rooms traversed). Topology preserves the
// G8c design intent — sequential forks with no mid-path merge — and adds
// the missing Trap anchor + deeper pre-fork and per-leaf chains. The
// four leaf chains still terminate at the single boss room (validator
// requires exactly one boss).
//
//   entry → flooded_atrium → tide_passage → kelp_trap (TRAP) →
//   barnacled_steps → drowned_atrium → sunken_nave → fork1
//                  ├─[unlocked, w=1]── dry_corridor → silent_columns →
//                  │     echoing_vault → fork2a
//                  │           ├─[unlocked]── kuo_toa_pen (ELITE) →
//                  │           │     trophy_pool → idol_court → boss
//                  │           └─[STR DC 13]── glyph_chamber → silt_arch → boss
//                  └─[Perception DC 13, w=2]── submerged_passage →
//                        coral_lattice → drowned_nave → fork2b
//                              ├─[unlocked]── aboleth_thralls →
//                              │     thrall_basin → sodden_hall → boss
//                              └─[Perception DC 15]── coral_reliquary (SECRET) → boss
//
// Distinct from the Crypt diamond and the Forest asymmetric-diamond: no
// two paths share a mid-node, so the !zone map paints two parallel "Y"
// subtrees instead of a converging branch. Longest entry→boss = 16.

func zoneSunkenTempleGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "sunken_temple.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Tide-Stained Threshold", PosX: 0, PosY: 2},
		{NodeID: "sunken_temple.flooded_atrium", Kind: NodeKindExploration,
			Label: "Flooded Atrium", PosX: 1, PosY: 2},
		{NodeID: "sunken_temple.tide_passage", Kind: NodeKindExploration,
			Label: "Tide Passage", PosX: 2, PosY: 2},
		{NodeID: "sunken_temple.kelp_trap", Kind: NodeKindTrap,
			Label: "Kelp-Snare", PosX: 3, PosY: 2},
		{NodeID: "sunken_temple.barnacled_steps", Kind: NodeKindExploration,
			Label: "Barnacled Steps", PosX: 4, PosY: 2},
		{NodeID: "sunken_temple.drowned_atrium", Kind: NodeKindExploration,
			Label: "Drowned Atrium", PosX: 5, PosY: 2},
		{NodeID: "sunken_temple.sunken_nave", Kind: NodeKindExploration,
			Label: "Sunken Nave", PosX: 6, PosY: 2},
		{NodeID: "sunken_temple.fork1", Kind: NodeKindFork,
			Label: "Split Stair", PosX: 7, PosY: 2},

		// Dry subtree.
		{NodeID: "sunken_temple.dry_corridor", Kind: NodeKindExploration,
			Label: "Dry Corridor", PosX: 8, PosY: 0},
		{NodeID: "sunken_temple.silent_columns", Kind: NodeKindExploration,
			Label: "Silent Columns", PosX: 9, PosY: 0},
		{NodeID: "sunken_temple.echoing_vault", Kind: NodeKindExploration,
			Label: "Echoing Vault", PosX: 10, PosY: 0},
		{NodeID: "sunken_temple.fork2a", Kind: NodeKindFork,
			Label: "Dry Crossing", PosX: 11, PosY: 0},
		{NodeID: "sunken_temple.kuo_toa_pen", Kind: NodeKindElite,
			Label: "Kuo-toa Pen", PosX: 12, PosY: -1},
		{NodeID: "sunken_temple.trophy_pool", Kind: NodeKindExploration,
			Label: "Trophy Pool", PosX: 13, PosY: -1},
		{NodeID: "sunken_temple.idol_court", Kind: NodeKindExploration,
			Label: "Idol Court", PosX: 14, PosY: -1},
		{NodeID: "sunken_temple.glyph_chamber", Kind: NodeKindExploration,
			Label: "Glyph Chamber", PosX: 12, PosY: 1},
		{NodeID: "sunken_temple.silt_arch", Kind: NodeKindExploration,
			Label: "Silt Arch", PosX: 13, PosY: 1},

		// Wet subtree.
		{NodeID: "sunken_temple.submerged_passage", Kind: NodeKindExploration,
			Label: "Submerged Passage", PosX: 8, PosY: 4},
		{NodeID: "sunken_temple.coral_lattice", Kind: NodeKindExploration,
			Label: "Coral Lattice", PosX: 9, PosY: 4},
		{NodeID: "sunken_temple.drowned_nave", Kind: NodeKindExploration,
			Label: "Drowned Nave", PosX: 10, PosY: 4},
		{NodeID: "sunken_temple.fork2b", Kind: NodeKindFork,
			Label: "Submerged Crossing", PosX: 11, PosY: 4},
		{NodeID: "sunken_temple.aboleth_thralls", Kind: NodeKindExploration,
			Label: "Thrall Pool", PosX: 12, PosY: 3},
		{NodeID: "sunken_temple.thrall_basin", Kind: NodeKindExploration,
			Label: "Thrall Basin", PosX: 13, PosY: 3},
		{NodeID: "sunken_temple.sodden_hall", Kind: NodeKindExploration,
			Label: "Sodden Hall", PosX: 14, PosY: 3},
		{NodeID: "sunken_temple.coral_reliquary", Kind: NodeKindSecret,
			Label: "Coral Reliquary", PosX: 12, PosY: 5,
			Content: ZoneNodeContent{LootBias: 1.8}},

		{NodeID: "sunken_temple.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Aboleth's Pool", PosX: 15, PosY: 2},
	}
	edges := []ZoneEdge{
		{From: "sunken_temple.entry", To: "sunken_temple.flooded_atrium", Lock: LockNone},
		{From: "sunken_temple.flooded_atrium", To: "sunken_temple.tide_passage", Lock: LockNone},
		{From: "sunken_temple.tide_passage", To: "sunken_temple.kelp_trap", Lock: LockNone},
		{From: "sunken_temple.kelp_trap", To: "sunken_temple.barnacled_steps", Lock: LockNone},
		{From: "sunken_temple.barnacled_steps", To: "sunken_temple.drowned_atrium", Lock: LockNone},
		{From: "sunken_temple.drowned_atrium", To: "sunken_temple.sunken_nave", Lock: LockNone},
		{From: "sunken_temple.sunken_nave", To: "sunken_temple.fork1", Lock: LockNone},

		{From: "sunken_temple.fork1", To: "sunken_temple.dry_corridor", Lock: LockNone, Weight: 1},
		{From: "sunken_temple.fork1", To: "sunken_temple.submerged_passage",
			Lock: LockPerception, LockData: map[string]any{"dc": 13},
			Hint: "wet stone glistens down a side passage", Weight: 2},

		{From: "sunken_temple.dry_corridor", To: "sunken_temple.silent_columns", Lock: LockNone},
		{From: "sunken_temple.silent_columns", To: "sunken_temple.echoing_vault", Lock: LockNone},
		{From: "sunken_temple.echoing_vault", To: "sunken_temple.fork2a", Lock: LockNone},

		{From: "sunken_temple.fork2a", To: "sunken_temple.kuo_toa_pen", Lock: LockNone, Weight: 1},
		{From: "sunken_temple.fork2a", To: "sunken_temple.glyph_chamber",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "STR", "dc": 13},
			Hint: "a stone door wedged half-shut by silt", Weight: 2},

		{From: "sunken_temple.kuo_toa_pen", To: "sunken_temple.trophy_pool", Lock: LockNone},
		{From: "sunken_temple.trophy_pool", To: "sunken_temple.idol_court", Lock: LockNone},
		{From: "sunken_temple.idol_court", To: "sunken_temple.boss", Lock: LockNone},

		{From: "sunken_temple.glyph_chamber", To: "sunken_temple.silt_arch", Lock: LockNone},
		{From: "sunken_temple.silt_arch", To: "sunken_temple.boss", Lock: LockNone},

		{From: "sunken_temple.submerged_passage", To: "sunken_temple.coral_lattice", Lock: LockNone},
		{From: "sunken_temple.coral_lattice", To: "sunken_temple.drowned_nave", Lock: LockNone},
		{From: "sunken_temple.drowned_nave", To: "sunken_temple.fork2b", Lock: LockNone},

		{From: "sunken_temple.fork2b", To: "sunken_temple.aboleth_thralls", Lock: LockNone, Weight: 1},
		{From: "sunken_temple.fork2b", To: "sunken_temple.coral_reliquary",
			Lock: LockPerception, LockData: map[string]any{"dc": 15},
			Hint: "a coral arch glittering under the surface", Weight: 2},

		{From: "sunken_temple.aboleth_thralls", To: "sunken_temple.thrall_basin", Lock: LockNone},
		{From: "sunken_temple.thrall_basin", To: "sunken_temple.sodden_hall", Lock: LockNone},
		{From: "sunken_temple.sodden_hall", To: "sunken_temple.boss", Lock: LockNone},

		{From: "sunken_temple.coral_reliquary", To: "sunken_temple.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneSunkenTemple, nodes, edges)
}

func init() {
	registerZoneGraph(zoneSunkenTempleGraph())
}
