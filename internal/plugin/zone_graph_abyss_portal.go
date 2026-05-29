package plugin

// The Abyss Portal branching graph — multi-region.
//
// Long-expedition plan D1-e: extended from the original 13-node sketch
// to the new T5 length band (36–44 rooms traversed). Topology preserves
// the G8h design intent — three sequential forks at increasing depth
// (binary, binary, ternary capstone) and CON stat-check coverage — and
// adds the linear depth each region now needs to feel like its own
// sub-dungeon. D1-e also backfills the missing RegionID authoring per
// dnd_expedition_region.go: every node carries a valid RegionID
// matching the registry.
//
// The original "fork3 capstone leaves go straight to boss" shape is
// adjusted so the three capstones now converge at a belaxath_doors
// MERGE in R4, then walk a short tear-approach to boss. Three distinct
// paths to boss are preserved (each via its own capstone node).
//
//   R1 outer_rift preamble (10 nodes):
//     entry → shattered_path → fractured_threshold → screaming_passage
//     → ember_walk → outer_rift_descent → tear_in_stone → ruined_arch
//     → quasit_outpost → fork1
//
//   R2 demon_assembly region:
//     fork1 — binary, both spurs converge in R2:
//       burning_wastes spur (unlocked):
//         burning_wastes → cinder_field → obsidian_plain
//       silent_chambers spur (Perception DC 16):
//         silent_chambers → hush_corridor → listening_room
//     R2 buildup to fork2 (6 nodes):
//       demon_assembly_gate → assembly_outskirts → nalfeshnee_corridor
//       → vrock_pillars → echo_atrium → assembly_descent → fork2
//
//   R3 wardens_post region:
//     fork2 — binary, both spurs converge in R3:
//       vrock_aerie spur (unlocked, ELITE):
//         vrock_approach → vrock_aerie (ELITE) → vrock_descent
//       mind_corridor spur (CON DC 16):
//         mind_threshold → mind_corridor → echo_passage
//     R3 buildup to fork3 (6 nodes):
//       wardens_outer_post → wardens_hall → wardens_chapel →
//       wardens_descent → tear_approach → wardens_threshold → fork3
//
//   R4 the_tear — capstone 3-way fork:
//     direct_assault spur (unlocked):
//       void_charge → direct_assault → fissure_walk
//     usurper_throne spur (CHA DC 18):
//       throne_walk → usurper_throne → claimed_path
//     reality_seam spur (Perception DC 18, SECRET LootBias 3.0):
//       seam_threshold → reality_seam (SECRET) → through_the_seam
//
//   R4 final approach (MERGE + boss; walk 6 after capstone spur):
//     belaxath_doors (MERGE) → outer_tear → inner_tear → tear_steps →
//     final_threshold → boss
//
// Longest entry→boss walk: 10 (R1) + 10 (R2) + 10 (R3) + 10 (R4) = 40
// nodes, inside [36, 44]. All four meaningful walks (fork1 × fork2 ×
// capstone) hit the same node count by construction.
//
// D10 anchor variety (in-place kind swaps, no length change): the Abyss
// shipped with NO trap node and only the fork2 vrock ELITE, so D10 adds
// two traps and a region-guardian elite —
//   - Hush Corridor (fork1 silent_chambers Perception-loot spur) and
//     Seam Threshold (fork3 reality_seam SECRET spur) become TRAPs, so
//     the two loot/secret branches each carry a hazard.
//   - Warden's Hall (R3 wardens_post buildup, main path) becomes an
//     ELITE — the region literally named for its wardens finally has
//     one, giving every walk a mid-zone region-guardian.
// The burning_wastes / mind_corridor / void_charge / usurper_throne
// routes stay clean, preserving the safe-vs-loot fork trade-off.

func zoneAbyssPortalGraph() ZoneGraph {
	r1 := "abyss_outer_rift"
	r2 := "abyss_demon_assembly"
	r3 := "abyss_wardens_post"
	r4 := "abyss_the_tear"

	nodes := []ZoneNode{
		// R1 outer_rift preamble.
		{NodeID: "abyss_portal.entry", Kind: NodeKindEntry, IsEntry: true, RegionID: r1,
			Label: "The Open Door", PosX: 0, PosY: 2},
		{NodeID: "abyss_portal.shattered_path", Kind: NodeKindExploration, RegionID: r1,
			Label: "Shattered Path", PosX: 1, PosY: 2},
		{NodeID: "abyss_portal.fractured_threshold", Kind: NodeKindExploration, RegionID: r1,
			Label: "Fractured Threshold", PosX: 2, PosY: 2},
		{NodeID: "abyss_portal.screaming_passage", Kind: NodeKindExploration, RegionID: r1,
			Label: "Screaming Passage", PosX: 3, PosY: 2},
		{NodeID: "abyss_portal.ember_walk", Kind: NodeKindExploration, RegionID: r1,
			Label: "Ember Walk", PosX: 4, PosY: 2},
		{NodeID: "abyss_portal.outer_rift_descent", Kind: NodeKindExploration, RegionID: r1,
			Label: "Rift Descent", PosX: 5, PosY: 2},
		{NodeID: "abyss_portal.tear_in_stone", Kind: NodeKindExploration, RegionID: r1,
			Label: "Tear in Stone", PosX: 6, PosY: 2},
		{NodeID: "abyss_portal.ruined_arch", Kind: NodeKindExploration, RegionID: r1,
			Label: "Ruined Arch", PosX: 7, PosY: 2},
		{NodeID: "abyss_portal.quasit_outpost", Kind: NodeKindExploration, RegionID: r1,
			Label: "Quasit Outpost", PosX: 8, PosY: 2},
		{NodeID: "abyss_portal.fork1", Kind: NodeKindFork, RegionID: r1,
			Label: "First Reality-Break", PosX: 9, PosY: 2},

		// R2 demon_assembly — fork1 spurs.
		{NodeID: "abyss_portal.burning_wastes", Kind: NodeKindExploration, RegionID: r2,
			Label: "Burning Wastes", PosX: 10, PosY: 1},
		{NodeID: "abyss_portal.cinder_field", Kind: NodeKindExploration, RegionID: r2,
			Label: "Cinder Field", PosX: 11, PosY: 1},
		{NodeID: "abyss_portal.obsidian_plain", Kind: NodeKindExploration, RegionID: r2,
			Label: "Obsidian Plain", PosX: 12, PosY: 1},

		{NodeID: "abyss_portal.silent_chambers", Kind: NodeKindExploration, RegionID: r2,
			Label: "Silent Chambers", PosX: 10, PosY: 3},
		{NodeID: "abyss_portal.hush_corridor", Kind: NodeKindTrap, RegionID: r2,
			Label: "Hush Corridor", PosX: 11, PosY: 3},
		{NodeID: "abyss_portal.listening_room", Kind: NodeKindExploration, RegionID: r2,
			Label: "Listening Room", PosX: 12, PosY: 3},

		// R2 buildup to fork2.
		{NodeID: "abyss_portal.demon_assembly_gate", Kind: NodeKindExploration, RegionID: r2,
			Label: "Demon Assembly Gate", PosX: 13, PosY: 2},
		{NodeID: "abyss_portal.assembly_outskirts", Kind: NodeKindExploration, RegionID: r2,
			Label: "Assembly Outskirts", PosX: 14, PosY: 2},
		{NodeID: "abyss_portal.nalfeshnee_corridor", Kind: NodeKindExploration, RegionID: r2,
			Label: "Nalfeshnee Corridor", PosX: 15, PosY: 2},
		{NodeID: "abyss_portal.vrock_pillars", Kind: NodeKindExploration, RegionID: r2,
			Label: "Vrock Pillars", PosX: 16, PosY: 2},
		{NodeID: "abyss_portal.echo_atrium", Kind: NodeKindExploration, RegionID: r2,
			Label: "Echo Atrium", PosX: 17, PosY: 2},
		{NodeID: "abyss_portal.assembly_descent", Kind: NodeKindExploration, RegionID: r2,
			Label: "Assembly Descent", PosX: 18, PosY: 2},
		{NodeID: "abyss_portal.fork2", Kind: NodeKindFork, RegionID: r2,
			Label: "Second Reality-Break", PosX: 19, PosY: 2},

		// R3 wardens_post — fork2 spurs.
		{NodeID: "abyss_portal.vrock_approach", Kind: NodeKindExploration, RegionID: r3,
			Label: "Vrock Approach", PosX: 20, PosY: 1},
		{NodeID: "abyss_portal.vrock_aerie", Kind: NodeKindElite, RegionID: r3,
			Label: "Marilith's Aerie", PosX: 21, PosY: 1},
		{NodeID: "abyss_portal.vrock_descent", Kind: NodeKindExploration, RegionID: r3,
			Label: "Vrock Descent", PosX: 22, PosY: 1},

		{NodeID: "abyss_portal.mind_threshold", Kind: NodeKindExploration, RegionID: r3,
			Label: "Mind Threshold", PosX: 20, PosY: 3},
		{NodeID: "abyss_portal.mind_corridor", Kind: NodeKindExploration, RegionID: r3,
			Label: "Corridor of Whispers", PosX: 21, PosY: 3},
		{NodeID: "abyss_portal.echo_passage", Kind: NodeKindExploration, RegionID: r3,
			Label: "Echo Passage", PosX: 22, PosY: 3},

		// R3 buildup to fork3.
		{NodeID: "abyss_portal.wardens_outer_post", Kind: NodeKindExploration, RegionID: r3,
			Label: "Outer Warden Post", PosX: 23, PosY: 2},
		{NodeID: "abyss_portal.wardens_hall", Kind: NodeKindElite, RegionID: r3,
			Label: "Warden's Hall", PosX: 24, PosY: 2},
		{NodeID: "abyss_portal.wardens_chapel", Kind: NodeKindExploration, RegionID: r3,
			Label: "Warden's Chapel", PosX: 25, PosY: 2},
		{NodeID: "abyss_portal.wardens_descent", Kind: NodeKindExploration, RegionID: r3,
			Label: "Warden's Descent", PosX: 26, PosY: 2},
		{NodeID: "abyss_portal.tear_approach", Kind: NodeKindExploration, RegionID: r3,
			Label: "Tear Approach", PosX: 27, PosY: 2},
		{NodeID: "abyss_portal.wardens_threshold", Kind: NodeKindExploration, RegionID: r3,
			Label: "Warden's Threshold", PosX: 28, PosY: 2},
		{NodeID: "abyss_portal.fork3", Kind: NodeKindFork, RegionID: r3,
			Label: "Third Reality-Break", PosX: 29, PosY: 2},

		// R4 capstone spurs.
		{NodeID: "abyss_portal.void_charge", Kind: NodeKindExploration, RegionID: r4,
			Label: "Void Charge", PosX: 30, PosY: 1},
		{NodeID: "abyss_portal.direct_assault", Kind: NodeKindExploration, RegionID: r4,
			Label: "Direct Assault", PosX: 31, PosY: 1},
		{NodeID: "abyss_portal.fissure_walk", Kind: NodeKindExploration, RegionID: r4,
			Label: "Fissure Walk", PosX: 32, PosY: 1},

		{NodeID: "abyss_portal.throne_walk", Kind: NodeKindExploration, RegionID: r4,
			Label: "Throne Walk", PosX: 30, PosY: 2},
		{NodeID: "abyss_portal.usurper_throne", Kind: NodeKindExploration, RegionID: r4,
			Label: "Usurper's Approach", PosX: 31, PosY: 2},
		{NodeID: "abyss_portal.claimed_path", Kind: NodeKindExploration, RegionID: r4,
			Label: "Claimed Path", PosX: 32, PosY: 2},

		{NodeID: "abyss_portal.seam_threshold", Kind: NodeKindTrap, RegionID: r4,
			Label: "Seam Threshold", PosX: 30, PosY: 3},
		{NodeID: "abyss_portal.reality_seam", Kind: NodeKindSecret, RegionID: r4,
			Label: "Reality Seam", PosX: 31, PosY: 3,
			Content: ZoneNodeContent{LootBias: 3.0}},
		{NodeID: "abyss_portal.through_the_seam", Kind: NodeKindExploration, RegionID: r4,
			Label: "Through the Seam", PosX: 32, PosY: 3},

		// R4 final approach.
		{NodeID: "abyss_portal.belaxath_doors", Kind: NodeKindMerge, RegionID: r4,
			Label: "The Belaxath Doors", PosX: 33, PosY: 2},
		{NodeID: "abyss_portal.outer_tear", Kind: NodeKindExploration, RegionID: r4,
			Label: "Outer Tear", PosX: 34, PosY: 2},
		{NodeID: "abyss_portal.inner_tear", Kind: NodeKindExploration, RegionID: r4,
			Label: "Inner Tear", PosX: 35, PosY: 2},
		{NodeID: "abyss_portal.tear_steps", Kind: NodeKindExploration, RegionID: r4,
			Label: "Tear Steps", PosX: 36, PosY: 2},
		{NodeID: "abyss_portal.final_threshold", Kind: NodeKindExploration, RegionID: r4,
			Label: "Final Threshold", PosX: 37, PosY: 2},
		{NodeID: "abyss_portal.boss", Kind: NodeKindBoss, IsBoss: true, RegionID: r4,
			Label: "Belaxath's Throne", PosX: 38, PosY: 2},
	}
	edges := []ZoneEdge{
		// R1 preamble.
		{From: "abyss_portal.entry", To: "abyss_portal.shattered_path", Lock: LockNone},
		{From: "abyss_portal.shattered_path", To: "abyss_portal.fractured_threshold", Lock: LockNone},
		{From: "abyss_portal.fractured_threshold", To: "abyss_portal.screaming_passage", Lock: LockNone},
		{From: "abyss_portal.screaming_passage", To: "abyss_portal.ember_walk", Lock: LockNone},
		{From: "abyss_portal.ember_walk", To: "abyss_portal.outer_rift_descent", Lock: LockNone},
		{From: "abyss_portal.outer_rift_descent", To: "abyss_portal.tear_in_stone", Lock: LockNone},
		{From: "abyss_portal.tear_in_stone", To: "abyss_portal.ruined_arch", Lock: LockNone},
		{From: "abyss_portal.ruined_arch", To: "abyss_portal.quasit_outpost", Lock: LockNone},
		{From: "abyss_portal.quasit_outpost", To: "abyss_portal.fork1", Lock: LockNone},

		// Fork1 — binary (R1 → R2).
		{From: "abyss_portal.fork1", To: "abyss_portal.burning_wastes", Lock: LockNone, Weight: 1},
		{From: "abyss_portal.fork1", To: "abyss_portal.silent_chambers",
			Lock: LockPerception, LockData: map[string]any{"dc": 16},
			Hint: "a silence so complete you can hear your own pulse — and something else's", Weight: 2},

		// burning_wastes spur → demon_assembly_gate.
		{From: "abyss_portal.burning_wastes", To: "abyss_portal.cinder_field", Lock: LockNone},
		{From: "abyss_portal.cinder_field", To: "abyss_portal.obsidian_plain", Lock: LockNone},
		{From: "abyss_portal.obsidian_plain", To: "abyss_portal.demon_assembly_gate", Lock: LockNone},

		// silent_chambers spur → demon_assembly_gate.
		{From: "abyss_portal.silent_chambers", To: "abyss_portal.hush_corridor", Lock: LockNone},
		{From: "abyss_portal.hush_corridor", To: "abyss_portal.listening_room", Lock: LockNone},
		{From: "abyss_portal.listening_room", To: "abyss_portal.demon_assembly_gate", Lock: LockNone},

		// R2 buildup to fork2.
		{From: "abyss_portal.demon_assembly_gate", To: "abyss_portal.assembly_outskirts", Lock: LockNone},
		{From: "abyss_portal.assembly_outskirts", To: "abyss_portal.nalfeshnee_corridor", Lock: LockNone},
		{From: "abyss_portal.nalfeshnee_corridor", To: "abyss_portal.vrock_pillars", Lock: LockNone},
		{From: "abyss_portal.vrock_pillars", To: "abyss_portal.echo_atrium", Lock: LockNone},
		{From: "abyss_portal.echo_atrium", To: "abyss_portal.assembly_descent", Lock: LockNone},
		{From: "abyss_portal.assembly_descent", To: "abyss_portal.fork2", Lock: LockNone},

		// Fork2 — binary (R2 → R3).
		{From: "abyss_portal.fork2", To: "abyss_portal.vrock_approach", Lock: LockNone, Weight: 1},
		{From: "abyss_portal.fork2", To: "abyss_portal.mind_threshold",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "CON", "dc": 16},
			Hint: "the air thickens — your bones know the wrong direction is also down", Weight: 2},

		// vrock_aerie spur → wardens_outer_post.
		{From: "abyss_portal.vrock_approach", To: "abyss_portal.vrock_aerie", Lock: LockNone},
		{From: "abyss_portal.vrock_aerie", To: "abyss_portal.vrock_descent", Lock: LockNone},
		{From: "abyss_portal.vrock_descent", To: "abyss_portal.wardens_outer_post", Lock: LockNone},

		// mind_corridor spur → wardens_outer_post.
		{From: "abyss_portal.mind_threshold", To: "abyss_portal.mind_corridor", Lock: LockNone},
		{From: "abyss_portal.mind_corridor", To: "abyss_portal.echo_passage", Lock: LockNone},
		{From: "abyss_portal.echo_passage", To: "abyss_portal.wardens_outer_post", Lock: LockNone},

		// R3 buildup to fork3.
		{From: "abyss_portal.wardens_outer_post", To: "abyss_portal.wardens_hall", Lock: LockNone},
		{From: "abyss_portal.wardens_hall", To: "abyss_portal.wardens_chapel", Lock: LockNone},
		{From: "abyss_portal.wardens_chapel", To: "abyss_portal.wardens_descent", Lock: LockNone},
		{From: "abyss_portal.wardens_descent", To: "abyss_portal.tear_approach", Lock: LockNone},
		{From: "abyss_portal.tear_approach", To: "abyss_portal.wardens_threshold", Lock: LockNone},
		{From: "abyss_portal.wardens_threshold", To: "abyss_portal.fork3", Lock: LockNone},

		// Fork3 — capstone 3-way (R3 → R4).
		{From: "abyss_portal.fork3", To: "abyss_portal.void_charge", Lock: LockNone, Weight: 1},
		{From: "abyss_portal.fork3", To: "abyss_portal.throne_walk",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "CHA", "dc": 18},
			Hint: "Belaxath has not noticed you yet — claim authority before he does", Weight: 2},
		{From: "abyss_portal.fork3", To: "abyss_portal.seam_threshold",
			Lock: LockPerception, LockData: map[string]any{"dc": 18},
			Hint: "a hairline crack in the air itself — somewhere there is a place that is not here", Weight: 3},

		// direct_assault spur → merge.
		{From: "abyss_portal.void_charge", To: "abyss_portal.direct_assault", Lock: LockNone},
		{From: "abyss_portal.direct_assault", To: "abyss_portal.fissure_walk", Lock: LockNone},
		{From: "abyss_portal.fissure_walk", To: "abyss_portal.belaxath_doors", Lock: LockNone},

		// usurper_throne spur → merge.
		{From: "abyss_portal.throne_walk", To: "abyss_portal.usurper_throne", Lock: LockNone},
		{From: "abyss_portal.usurper_throne", To: "abyss_portal.claimed_path", Lock: LockNone},
		{From: "abyss_portal.claimed_path", To: "abyss_portal.belaxath_doors", Lock: LockNone},

		// reality_seam spur → merge.
		{From: "abyss_portal.seam_threshold", To: "abyss_portal.reality_seam", Lock: LockNone},
		{From: "abyss_portal.reality_seam", To: "abyss_portal.through_the_seam", Lock: LockNone},
		{From: "abyss_portal.through_the_seam", To: "abyss_portal.belaxath_doors", Lock: LockNone},

		// R4 final approach.
		{From: "abyss_portal.belaxath_doors", To: "abyss_portal.outer_tear", Lock: LockNone},
		{From: "abyss_portal.outer_tear", To: "abyss_portal.inner_tear", Lock: LockNone},
		{From: "abyss_portal.inner_tear", To: "abyss_portal.tear_steps", Lock: LockNone},
		{From: "abyss_portal.tear_steps", To: "abyss_portal.final_threshold", Lock: LockNone},
		{From: "abyss_portal.final_threshold", To: "abyss_portal.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneAbyssPortal, nodes, edges)
}

func init() {
	registerZoneGraph(zoneAbyssPortalGraph())
}
