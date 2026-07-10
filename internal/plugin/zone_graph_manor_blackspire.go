package plugin

// Haunted Manor of Blackspire branching graph.
//
// Long-expedition plan D1-c: extended from the original 11-node sketch to
// the new T3 length band (22–26 rooms traversed). Topology preserves the
// G8d design intent — two stacked 3-way forks ("first zone where the
// player picks twice in a row from a wide menu") with full lock-kind
// coverage (LockNone, LockPerception, LockStatCheck, LockLevelMin) — and
// adds the missing Trap anchor + extends every branch to 3 mid-nodes so
// the longest entry→boss walk lands at 23.
//
//   entry → grounds_walk → carriage_path → manor_gate → foyer →
//   drawing_room → gallery_corridor → cursed_threshold (TRAP) →
//   grand_staircase → great_hall (3-way fork)
//       ├─[unlocked]── portrait_gallery → cold_solarium → west_landing ─┐
//       ├─[Perception DC 14]── locked_study → study_archive →           │
//       │                       secretary_hall ───────────────────────  ├── second_floor_landing
//       └─[INT DC 14]── forbidden_library → library_loft →               │
//                       reading_rotunda ──────────────────────────────  ┘
//                                                                      │
//                       → portrait_landing → upper_hall (3-way fork)   │
//       ├─[unlocked]── master_bedroom (ELITE) → grim_balcony →         │
//       │              moonlit_passage ───────────────────────────────  ┐
//       ├─[Perception DC 15]── hidden_oratory (SECRET) → choir_balcony  │
//       │                       → reliquary_dust ───────────────────── ├── spire_corridor
//       └─[LevelMin 7]── tower_observatory → spiral_ascent →            │
//                        telescope_attic ─────────────────────────────  ┘
//                       → ancestral_gallery → sanctum_threshold → boss
//
// All six fork branches are 3 mid-nodes long so any route through the
// manor lands at the same 23-node traversal length — the player's choice
// is about loot/encounter character, not shortcut economics. The ELITE
// hangs off the open spoke of upper_hall (cheapest gate, hardest fight);
// the SECRET sits behind the highest-DC Perception with its loot bias
// preserved.
//
// N5/D4 adds a fourth upper_hall spoke: the Sealed Reliquary, a cross-zone
// key vault (LockKey "sunken sigil", earned in the Sunken Temple's Coral
// Reliquary). A one-node loot shortcut back to spire_corridor — reachable
// only by a player who found the sigil, so it never shows on the longest
// path and the 23-node band is unchanged.

func zoneManorBlackspireGraph() ZoneGraph {
	nodes := []ZoneNode{
		// Pre-fork preamble (path positions 1–10).
		{NodeID: "manor_blackspire.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Manor Gate", PosX: 0, PosY: 2},
		{NodeID: "manor_blackspire.grounds_walk", Kind: NodeKindExploration,
			Label: "Overgrown Grounds", PosX: 1, PosY: 2},
		{NodeID: "manor_blackspire.carriage_path", Kind: NodeKindExploration,
			Label: "Carriage Path", PosX: 2, PosY: 2},
		{NodeID: "manor_blackspire.manor_gate", Kind: NodeKindExploration,
			Label: "Iron Gate", PosX: 3, PosY: 2},
		{NodeID: "manor_blackspire.foyer", Kind: NodeKindExploration,
			Label: "Foyer", PosX: 4, PosY: 2},
		{NodeID: "manor_blackspire.drawing_room", Kind: NodeKindExploration,
			Label: "Drawing Room", PosX: 5, PosY: 2},
		{NodeID: "manor_blackspire.gallery_corridor", Kind: NodeKindExploration,
			Label: "Gallery Corridor", PosX: 6, PosY: 2},
		{NodeID: "manor_blackspire.cursed_threshold", Kind: NodeKindTrap,
			Label: "Cursed Threshold", PosX: 7, PosY: 2},
		{NodeID: "manor_blackspire.grand_staircase", Kind: NodeKindExploration,
			Label: "Grand Staircase", PosX: 8, PosY: 2},
		{NodeID: "manor_blackspire.great_hall", Kind: NodeKindFork,
			Label: "Great Hall", PosX: 9, PosY: 2},

		// great_hall — open spoke (portrait gallery).
		{NodeID: "manor_blackspire.portrait_gallery", Kind: NodeKindExploration,
			Label: "Portrait Gallery", PosX: 10, PosY: 0},
		{NodeID: "manor_blackspire.cold_solarium", Kind: NodeKindExploration,
			Label: "Cold Solarium", PosX: 11, PosY: 0},
		{NodeID: "manor_blackspire.west_landing", Kind: NodeKindExploration,
			Label: "West Landing", PosX: 12, PosY: 0},

		// great_hall — perception spoke (locked study).
		{NodeID: "manor_blackspire.locked_study", Kind: NodeKindExploration,
			Label: "Locked Study", PosX: 10, PosY: 2},
		{NodeID: "manor_blackspire.study_archive", Kind: NodeKindExploration,
			Label: "Study Archive", PosX: 11, PosY: 2},
		{NodeID: "manor_blackspire.secretary_hall", Kind: NodeKindExploration,
			Label: "Secretary's Hall", PosX: 12, PosY: 2},

		// great_hall — INT spoke (forbidden library).
		{NodeID: "manor_blackspire.forbidden_library", Kind: NodeKindExploration,
			Label: "Forbidden Library", PosX: 10, PosY: 4},
		{NodeID: "manor_blackspire.library_loft", Kind: NodeKindExploration,
			Label: "Library Loft", PosX: 11, PosY: 4},
		{NodeID: "manor_blackspire.reading_rotunda", Kind: NodeKindExploration,
			Label: "Reading Rotunda", PosX: 12, PosY: 4},

		// Merge + bridge to upper_hall.
		{NodeID: "manor_blackspire.second_floor_landing", Kind: NodeKindMerge,
			Label: "Second-Floor Landing", PosX: 13, PosY: 2},
		{NodeID: "manor_blackspire.portrait_landing", Kind: NodeKindExploration,
			Label: "Portrait Landing", PosX: 14, PosY: 2},
		{NodeID: "manor_blackspire.upper_hall", Kind: NodeKindFork,
			Label: "Upper Hall", PosX: 15, PosY: 2},

		// upper_hall — open spoke (elite).
		{NodeID: "manor_blackspire.master_bedroom", Kind: NodeKindElite,
			Label: "Master Bedroom", PosX: 16, PosY: 0},
		{NodeID: "manor_blackspire.grim_balcony", Kind: NodeKindExploration,
			Label: "Grim Balcony", PosX: 17, PosY: 0},
		{NodeID: "manor_blackspire.moonlit_passage", Kind: NodeKindExploration,
			Label: "Moonlit Passage", PosX: 18, PosY: 0},

		// upper_hall — perception spoke (secret).
		{NodeID: "manor_blackspire.hidden_oratory", Kind: NodeKindSecret,
			Label: "Hidden Oratory", PosX: 16, PosY: 2,
			Content: ZoneNodeContent{LootBias: 2.0}},
		{NodeID: "manor_blackspire.choir_balcony", Kind: NodeKindExploration,
			Label: "Choir Balcony", PosX: 17, PosY: 2},
		{NodeID: "manor_blackspire.reliquary_dust", Kind: NodeKindExploration,
			Label: "Reliquary Dust", PosX: 18, PosY: 2},

		// upper_hall — cross-zone key spoke (N5/D4). A Sunken Temple sigil
		// opens this vault; a loot-rich shortcut back to the spire corridor.
		{NodeID: "manor_blackspire.sealed_reliquary", Kind: NodeKindSecret,
			Label: "Sealed Reliquary", PosX: 16, PosY: -2,
			Content: ZoneNodeContent{LootBias: 2.2}},

		// upper_hall — level-min spoke (tower).
		{NodeID: "manor_blackspire.tower_observatory", Kind: NodeKindExploration,
			Label: "Tower Observatory", PosX: 16, PosY: 4},
		{NodeID: "manor_blackspire.spiral_ascent", Kind: NodeKindExploration,
			Label: "Spiral Ascent", PosX: 17, PosY: 4},
		{NodeID: "manor_blackspire.telescope_attic", Kind: NodeKindExploration,
			Label: "Telescope Attic", PosX: 18, PosY: 4},

		// Merge + boss approach.
		{NodeID: "manor_blackspire.spire_corridor", Kind: NodeKindMerge,
			Label: "Spire Corridor", PosX: 19, PosY: 2},
		{NodeID: "manor_blackspire.ancestral_gallery", Kind: NodeKindExploration,
			Label: "Ancestral Gallery", PosX: 20, PosY: 2},
		{NodeID: "manor_blackspire.sanctum_threshold", Kind: NodeKindExploration,
			Label: "Sanctum Threshold", PosX: 21, PosY: 2},
		{NodeID: "manor_blackspire.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Aldric's Sanctum", PosX: 22, PosY: 2},
	}
	edges := []ZoneEdge{
		// Preamble (linear).
		{From: "manor_blackspire.entry", To: "manor_blackspire.grounds_walk", Lock: LockNone},
		{From: "manor_blackspire.grounds_walk", To: "manor_blackspire.carriage_path", Lock: LockNone},
		{From: "manor_blackspire.carriage_path", To: "manor_blackspire.manor_gate", Lock: LockNone},
		{From: "manor_blackspire.manor_gate", To: "manor_blackspire.foyer", Lock: LockNone},
		{From: "manor_blackspire.foyer", To: "manor_blackspire.drawing_room", Lock: LockNone},
		{From: "manor_blackspire.drawing_room", To: "manor_blackspire.gallery_corridor", Lock: LockNone},
		{From: "manor_blackspire.gallery_corridor", To: "manor_blackspire.cursed_threshold", Lock: LockNone},
		{From: "manor_blackspire.cursed_threshold", To: "manor_blackspire.grand_staircase", Lock: LockNone},
		{From: "manor_blackspire.grand_staircase", To: "manor_blackspire.great_hall", Lock: LockNone},

		// great_hall fork — three spokes (open, perception 14, INT 14).
		{From: "manor_blackspire.great_hall", To: "manor_blackspire.portrait_gallery", Lock: LockNone, Weight: 1},
		{From: "manor_blackspire.great_hall", To: "manor_blackspire.locked_study",
			Lock: LockPerception, LockData: map[string]any{"dc": 14},
			Hint: "an out-of-place draft from a doorframe", Weight: 2},
		{From: "manor_blackspire.great_hall", To: "manor_blackspire.forbidden_library",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "INT", "dc": 14},
			Hint: "a runed door — you'd need to read it", Weight: 3},

		// Portrait spoke.
		{From: "manor_blackspire.portrait_gallery", To: "manor_blackspire.cold_solarium", Lock: LockNone},
		{From: "manor_blackspire.cold_solarium", To: "manor_blackspire.west_landing", Lock: LockNone},
		{From: "manor_blackspire.west_landing", To: "manor_blackspire.second_floor_landing", Lock: LockNone},

		// Study spoke.
		{From: "manor_blackspire.locked_study", To: "manor_blackspire.study_archive", Lock: LockNone},
		{From: "manor_blackspire.study_archive", To: "manor_blackspire.secretary_hall", Lock: LockNone},
		{From: "manor_blackspire.secretary_hall", To: "manor_blackspire.second_floor_landing", Lock: LockNone},

		// Library spoke.
		{From: "manor_blackspire.forbidden_library", To: "manor_blackspire.library_loft", Lock: LockNone},
		{From: "manor_blackspire.library_loft", To: "manor_blackspire.reading_rotunda", Lock: LockNone},
		{From: "manor_blackspire.reading_rotunda", To: "manor_blackspire.second_floor_landing", Lock: LockNone},

		// Bridge to upper_hall.
		{From: "manor_blackspire.second_floor_landing", To: "manor_blackspire.portrait_landing", Lock: LockNone},
		{From: "manor_blackspire.portrait_landing", To: "manor_blackspire.upper_hall", Lock: LockNone},

		// upper_hall fork — three spokes (open elite, perception 15 secret, level-min 7).
		{From: "manor_blackspire.upper_hall", To: "manor_blackspire.master_bedroom", Lock: LockNone, Weight: 1},
		{From: "manor_blackspire.upper_hall", To: "manor_blackspire.hidden_oratory",
			Lock: LockPerception, LockData: map[string]any{"dc": 15},
			Hint: "a candle behind a panel that shouldn't have a back", Weight: 2},
		{From: "manor_blackspire.upper_hall", To: "manor_blackspire.tower_observatory",
			Lock: LockLevelMin, LockData: map[string]any{"min_level": 7},
			Hint: "a spiral stair that creaks ominously — climb only if you trust your footing", Weight: 3},
		{From: "manor_blackspire.upper_hall", To: "manor_blackspire.sealed_reliquary",
			Lock: LockKey, LockData: map[string]any{"key_id": "sunken sigil"},
			Hint: "a panel scored with a drowned god's mark — you'd need its twin to open it", Weight: 4},

		// Master Bedroom spoke (elite).
		{From: "manor_blackspire.master_bedroom", To: "manor_blackspire.grim_balcony", Lock: LockNone},
		{From: "manor_blackspire.grim_balcony", To: "manor_blackspire.moonlit_passage", Lock: LockNone},
		{From: "manor_blackspire.moonlit_passage", To: "manor_blackspire.spire_corridor", Lock: LockNone},

		// Hidden Oratory spoke (secret).
		{From: "manor_blackspire.hidden_oratory", To: "manor_blackspire.choir_balcony", Lock: LockNone},
		{From: "manor_blackspire.choir_balcony", To: "manor_blackspire.reliquary_dust", Lock: LockNone},
		{From: "manor_blackspire.reliquary_dust", To: "manor_blackspire.spire_corridor", Lock: LockNone},

		// Sealed Reliquary spoke (cross-zone key — Sunken Sigil).
		{From: "manor_blackspire.sealed_reliquary", To: "manor_blackspire.spire_corridor", Lock: LockNone},

		// Tower spoke.
		{From: "manor_blackspire.tower_observatory", To: "manor_blackspire.spiral_ascent", Lock: LockNone},
		{From: "manor_blackspire.spiral_ascent", To: "manor_blackspire.telescope_attic", Lock: LockNone},
		{From: "manor_blackspire.telescope_attic", To: "manor_blackspire.spire_corridor", Lock: LockNone},

		// Boss approach.
		{From: "manor_blackspire.spire_corridor", To: "manor_blackspire.ancestral_gallery", Lock: LockNone},
		{From: "manor_blackspire.ancestral_gallery", To: "manor_blackspire.sanctum_threshold", Lock: LockNone},
		{From: "manor_blackspire.sanctum_threshold", To: "manor_blackspire.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneManorBlackspire, nodes, edges)
}

func init() {
	registerZoneGraph(zoneManorBlackspireGraph())
}
