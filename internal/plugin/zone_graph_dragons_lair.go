package plugin

// Dragon's Lair (Infernus Peak) branching graph — multi-region.
//
// Long-expedition plan D1-e: extended from the original 12-node sketch
// to the new T5 length band (36–44 rooms traversed). Topology preserves
// the G8g design intent — long approach, mid-zone binary-converging
// fork, capstone 3-way fork — and adds the linear depth each region
// now needs to feel like its own sub-dungeon. D1-e also backfills the
// missing RegionID authoring per dnd_expedition_region.go: every node
// carries a valid RegionID matching the registry.
//
// The original "fork2 capstone leaves go straight to boss" shape is
// adjusted so the three capstones now converge at an infernax_doors
// MERGE in R4, then walk a short throne approach to boss. Three
// distinct paths to boss are preserved (each via its own capstone
// node); the merge just consolidates the long final hall instead of
// triplicating it.
//
//   R1 kobold_warrens preamble (10 nodes):
//     entry → kobold_lookout → kobold_warrens → smoke_choked_hall →
//     ash_chapel → kobold_warchief_camp → ember_corridor →
//     obsidian_steps → cinder_passage → kobold_descent
//
//   R2 drake_pens region (13 nodes; walk length 10):
//     drake_pens → drake_grooming_pit → drake_holding_run →
//     drake_yards → drake_handlers_hall → drake_armory → fork1
//
//   R2 fork1 — binary, converges at wyrmlings_nest (R3 boundary):
//     ash_bridge spur (unlocked, TRAP):
//       ash_bridge (TRAP) → burning_span → cinder_walk
//     treasure_vault spur (Perception DC 15, denser loot):
//       treasure_vault → coin_strewn_hall → vault_passage
//
//   R3 the_vault region (9 nodes; walk length 9):
//     wyrmlings_nest (ELITE) → hoard_outer → coin_river → hoard_arch →
//     flame_corridor → infernax_threshold → molten_steps → inferno_walk
//     → fork2
//
//   R4 infernax_chamber — capstone 3-way fork:
//     direct_confrontation spur (unlocked):
//       charge_walk → direct_confrontation → broken_chamber
//     dragon_bargain spur (CHA DC 16):
//       speakers_step → dragon_bargain → audience_hall
//     hoard_pillar spur (Perception DC 17, SECRET LootBias 2.5):
//       hidden_passage → hoard_pillar (SECRET) → secret_arch
//
//   R4 final approach (MERGE + boss; walk 6 after capstone spur):
//     infernax_doors (MERGE) → throne_ascension →
//     molten_throne_approach → crown_steps → final_step → boss
//
// Longest entry→boss walk: 10 (R1) + 10 (R2) + 9 (R3) + 10 (R4) = 39
// nodes, inside [36, 44]. The two R2 spurs and three R4 capstones each
// reach the boss in the same node count by construction.
//
// D10 anchor variety (in-place kind swaps, no length change): the two
// loot-leaning branches each gain a guard, so "the richer route costs
// more" —
//   - Coin-Strewn Hall (treasure_vault spur) becomes an ELITE; the
//     Perception loot route is now guarded, mirroring the ash_bridge
//     spur's TRAP so both fork1 branches carry an anchor.
//   - Hidden Passage (hoard_pillar SECRET capstone spur) becomes a
//     TRAP guarding the densest loot in the zone.
// The direct-confrontation and dragon-bargain routes stay clean, so the
// fork choice trades safety for loot.

func zoneDragonsLairGraph() ZoneGraph {
	r1 := "dragons_lair_kobold_warrens"
	r2 := "dragons_lair_drake_pens"
	r3 := "dragons_lair_the_vault"
	r4 := "dragons_lair_infernax_chamber"

	nodes := []ZoneNode{
		// R1 kobold_warrens preamble.
		{NodeID: "dragons_lair.entry", Kind: NodeKindEntry, IsEntry: true, RegionID: r1,
			Label: "Mountain Pass", PosX: 0, PosY: 1},
		{NodeID: "dragons_lair.kobold_lookout", Kind: NodeKindExploration, RegionID: r1,
			Label: "Kobold Lookout", PosX: 1, PosY: 1},
		{NodeID: "dragons_lair.kobold_warrens", Kind: NodeKindExploration, RegionID: r1,
			Label: "Kobold Warrens", PosX: 2, PosY: 1},
		{NodeID: "dragons_lair.smoke_choked_hall", Kind: NodeKindExploration, RegionID: r1,
			Label: "Smoke-Choked Hall", PosX: 3, PosY: 1},
		{NodeID: "dragons_lair.ash_chapel", Kind: NodeKindExploration, RegionID: r1,
			Label: "Ash Chapel", PosX: 4, PosY: 1},
		{NodeID: "dragons_lair.kobold_warchief_camp", Kind: NodeKindExploration, RegionID: r1,
			Label: "Warchief's Camp", PosX: 5, PosY: 1},
		{NodeID: "dragons_lair.ember_corridor", Kind: NodeKindExploration, RegionID: r1,
			Label: "Ember Corridor", PosX: 6, PosY: 1},
		{NodeID: "dragons_lair.obsidian_steps", Kind: NodeKindExploration, RegionID: r1,
			Label: "Obsidian Steps", PosX: 7, PosY: 1},
		{NodeID: "dragons_lair.cinder_passage", Kind: NodeKindExploration, RegionID: r1,
			Label: "Cinder Passage", PosX: 8, PosY: 1},
		{NodeID: "dragons_lair.kobold_descent", Kind: NodeKindExploration, RegionID: r1,
			Label: "Kobold Descent", PosX: 9, PosY: 1},

		// R2 drake_pens region.
		{NodeID: "dragons_lair.drake_pens", Kind: NodeKindExploration, RegionID: r2,
			Label: "Drake Pens", PosX: 10, PosY: 1},
		{NodeID: "dragons_lair.drake_grooming_pit", Kind: NodeKindExploration, RegionID: r2,
			Label: "Grooming Pit", PosX: 11, PosY: 1},
		{NodeID: "dragons_lair.drake_holding_run", Kind: NodeKindExploration, RegionID: r2,
			Label: "Holding Run", PosX: 12, PosY: 1},
		{NodeID: "dragons_lair.drake_yards", Kind: NodeKindExploration, RegionID: r2,
			Label: "Drake Yards", PosX: 13, PosY: 1},
		{NodeID: "dragons_lair.drake_handlers_hall", Kind: NodeKindExploration, RegionID: r2,
			Label: "Handler's Hall", PosX: 14, PosY: 1},
		{NodeID: "dragons_lair.drake_armory", Kind: NodeKindExploration, RegionID: r2,
			Label: "Drake Armory", PosX: 15, PosY: 1},
		{NodeID: "dragons_lair.fork1", Kind: NodeKindFork, RegionID: r2,
			Label: "The Cinder Crossing", PosX: 16, PosY: 1},

		// R2 ash_bridge spur (TRAP, unlocked).
		{NodeID: "dragons_lair.ash_bridge", Kind: NodeKindTrap, RegionID: r2,
			Label: "Ash Bridge", PosX: 17, PosY: 0},
		{NodeID: "dragons_lair.burning_span", Kind: NodeKindExploration, RegionID: r2,
			Label: "Burning Span", PosX: 18, PosY: 0},
		{NodeID: "dragons_lair.cinder_walk", Kind: NodeKindExploration, RegionID: r2,
			Label: "Cinder Walk", PosX: 19, PosY: 0},

		// R2 treasure_vault spur (Perception 15, loot).
		{NodeID: "dragons_lair.treasure_vault", Kind: NodeKindExploration, RegionID: r2,
			Label: "Treasure Vault", PosX: 17, PosY: 2,
			Content: ZoneNodeContent{LootBias: 1.5}},
		{NodeID: "dragons_lair.coin_strewn_hall", Kind: NodeKindElite, RegionID: r2,
			Label: "Coin-Strewn Hall", PosX: 18, PosY: 2},
		{NodeID: "dragons_lair.vault_passage", Kind: NodeKindExploration, RegionID: r2,
			Label: "Vault Passage", PosX: 19, PosY: 2},

		// R3 the_vault region.
		{NodeID: "dragons_lair.wyrmlings_nest", Kind: NodeKindElite, RegionID: r3,
			Label: "Wyrmling's Nest", PosX: 20, PosY: 1},
		{NodeID: "dragons_lair.hoard_outer", Kind: NodeKindExploration, RegionID: r3,
			Label: "Outer Hoard", PosX: 21, PosY: 1},
		{NodeID: "dragons_lair.coin_river", Kind: NodeKindExploration, RegionID: r3,
			Label: "Coin River", PosX: 22, PosY: 1},
		{NodeID: "dragons_lair.hoard_arch", Kind: NodeKindExploration, RegionID: r3,
			Label: "Hoard Arch", PosX: 23, PosY: 1},
		{NodeID: "dragons_lair.flame_corridor", Kind: NodeKindExploration, RegionID: r3,
			Label: "Flame Corridor", PosX: 24, PosY: 1},
		{NodeID: "dragons_lair.infernax_threshold", Kind: NodeKindExploration, RegionID: r3,
			Label: "Infernax's Threshold", PosX: 25, PosY: 1},
		{NodeID: "dragons_lair.molten_steps", Kind: NodeKindExploration, RegionID: r3,
			Label: "Molten Steps", PosX: 26, PosY: 1},
		{NodeID: "dragons_lair.inferno_walk", Kind: NodeKindExploration, RegionID: r3,
			Label: "Inferno Walk", PosX: 27, PosY: 1},
		{NodeID: "dragons_lair.fork2", Kind: NodeKindFork, RegionID: r3,
			Label: "Hoard Approach", PosX: 28, PosY: 1},

		// R4 direct_confrontation spur.
		{NodeID: "dragons_lair.charge_walk", Kind: NodeKindExploration, RegionID: r4,
			Label: "Charge Walk", PosX: 29, PosY: 0},
		{NodeID: "dragons_lair.direct_confrontation", Kind: NodeKindExploration, RegionID: r4,
			Label: "Direct Approach", PosX: 30, PosY: 0},
		{NodeID: "dragons_lair.broken_chamber", Kind: NodeKindExploration, RegionID: r4,
			Label: "Broken Chamber", PosX: 31, PosY: 0},

		// R4 dragon_bargain spur (CHA 16).
		{NodeID: "dragons_lair.speakers_step", Kind: NodeKindExploration, RegionID: r4,
			Label: "Speaker's Step", PosX: 29, PosY: 1},
		{NodeID: "dragons_lair.dragon_bargain", Kind: NodeKindExploration, RegionID: r4,
			Label: "Words With Infernax", PosX: 30, PosY: 1},
		{NodeID: "dragons_lair.audience_hall", Kind: NodeKindExploration, RegionID: r4,
			Label: "Audience Hall", PosX: 31, PosY: 1},

		// R4 hoard_pillar spur (Perception 17, SECRET).
		{NodeID: "dragons_lair.hidden_passage", Kind: NodeKindTrap, RegionID: r4,
			Label: "Hidden Passage", PosX: 29, PosY: 2},
		{NodeID: "dragons_lair.hoard_pillar", Kind: NodeKindSecret, RegionID: r4,
			Label: "Hidden Hoard Pillar", PosX: 30, PosY: 2,
			Content: ZoneNodeContent{LootBias: 2.5}},
		{NodeID: "dragons_lair.secret_arch", Kind: NodeKindExploration, RegionID: r4,
			Label: "Secret Arch", PosX: 31, PosY: 2},

		// R4 final approach.
		{NodeID: "dragons_lair.infernax_doors", Kind: NodeKindMerge, RegionID: r4,
			Label: "The Infernax Doors", PosX: 32, PosY: 1},
		{NodeID: "dragons_lair.throne_ascension", Kind: NodeKindExploration, RegionID: r4,
			Label: "Throne Ascension", PosX: 33, PosY: 1},
		{NodeID: "dragons_lair.molten_throne_approach", Kind: NodeKindExploration, RegionID: r4,
			Label: "Molten Throne Approach", PosX: 34, PosY: 1},
		{NodeID: "dragons_lair.crown_steps", Kind: NodeKindExploration, RegionID: r4,
			Label: "Crown Steps", PosX: 35, PosY: 1},
		{NodeID: "dragons_lair.final_step", Kind: NodeKindExploration, RegionID: r4,
			Label: "Final Step", PosX: 36, PosY: 1},
		{NodeID: "dragons_lair.boss", Kind: NodeKindBoss, IsBoss: true, RegionID: r4,
			Label: "Infernax's Crown", PosX: 37, PosY: 1},
	}
	edges := []ZoneEdge{
		// R1 preamble.
		{From: "dragons_lair.entry", To: "dragons_lair.kobold_lookout", Lock: LockNone},
		{From: "dragons_lair.kobold_lookout", To: "dragons_lair.kobold_warrens", Lock: LockNone},
		{From: "dragons_lair.kobold_warrens", To: "dragons_lair.smoke_choked_hall", Lock: LockNone},
		{From: "dragons_lair.smoke_choked_hall", To: "dragons_lair.ash_chapel", Lock: LockNone},
		{From: "dragons_lair.ash_chapel", To: "dragons_lair.kobold_warchief_camp", Lock: LockNone},
		{From: "dragons_lair.kobold_warchief_camp", To: "dragons_lair.ember_corridor", Lock: LockNone},
		{From: "dragons_lair.ember_corridor", To: "dragons_lair.obsidian_steps", Lock: LockNone},
		{From: "dragons_lair.obsidian_steps", To: "dragons_lair.cinder_passage", Lock: LockNone},
		{From: "dragons_lair.cinder_passage", To: "dragons_lair.kobold_descent", Lock: LockNone},

		// R1 → R2 transition.
		{From: "dragons_lair.kobold_descent", To: "dragons_lair.drake_pens", Lock: LockNone},

		// R2 buildup to fork1.
		{From: "dragons_lair.drake_pens", To: "dragons_lair.drake_grooming_pit", Lock: LockNone},
		{From: "dragons_lair.drake_grooming_pit", To: "dragons_lair.drake_holding_run", Lock: LockNone},
		{From: "dragons_lair.drake_holding_run", To: "dragons_lair.drake_yards", Lock: LockNone},
		{From: "dragons_lair.drake_yards", To: "dragons_lair.drake_handlers_hall", Lock: LockNone},
		{From: "dragons_lair.drake_handlers_hall", To: "dragons_lair.drake_armory", Lock: LockNone},
		{From: "dragons_lair.drake_armory", To: "dragons_lair.fork1", Lock: LockNone},

		// Fork1 — binary, converges at wyrmlings_nest (R3 boundary).
		{From: "dragons_lair.fork1", To: "dragons_lair.ash_bridge", Lock: LockNone, Weight: 1},
		{From: "dragons_lair.fork1", To: "dragons_lair.treasure_vault",
			Lock: LockPerception, LockData: map[string]any{"dc": 15},
			Hint: "a draft from a side passage — and the dull glint of gold beyond it", Weight: 2},

		// ash_bridge spur.
		{From: "dragons_lair.ash_bridge", To: "dragons_lair.burning_span", Lock: LockNone},
		{From: "dragons_lair.burning_span", To: "dragons_lair.cinder_walk", Lock: LockNone},
		{From: "dragons_lair.cinder_walk", To: "dragons_lair.wyrmlings_nest", Lock: LockNone},

		// treasure_vault spur.
		{From: "dragons_lair.treasure_vault", To: "dragons_lair.coin_strewn_hall", Lock: LockNone},
		{From: "dragons_lair.coin_strewn_hall", To: "dragons_lair.vault_passage", Lock: LockNone},
		{From: "dragons_lair.vault_passage", To: "dragons_lair.wyrmlings_nest", Lock: LockNone},

		// R3 the_vault buildup to fork2.
		{From: "dragons_lair.wyrmlings_nest", To: "dragons_lair.hoard_outer", Lock: LockNone},
		{From: "dragons_lair.hoard_outer", To: "dragons_lair.coin_river", Lock: LockNone},
		{From: "dragons_lair.coin_river", To: "dragons_lair.hoard_arch", Lock: LockNone},
		{From: "dragons_lair.hoard_arch", To: "dragons_lair.flame_corridor", Lock: LockNone},
		{From: "dragons_lair.flame_corridor", To: "dragons_lair.infernax_threshold", Lock: LockNone},
		{From: "dragons_lair.infernax_threshold", To: "dragons_lair.molten_steps", Lock: LockNone},
		{From: "dragons_lair.molten_steps", To: "dragons_lair.inferno_walk", Lock: LockNone},
		{From: "dragons_lair.inferno_walk", To: "dragons_lair.fork2", Lock: LockNone},

		// Fork2 — capstone 3-way (R3 → R4).
		{From: "dragons_lair.fork2", To: "dragons_lair.charge_walk", Lock: LockNone, Weight: 1},
		{From: "dragons_lair.fork2", To: "dragons_lair.speakers_step",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "CHA", "dc": 16},
			Hint: "Infernax speaks. The voice fills the chamber. You could speak back.", Weight: 2},
		{From: "dragons_lair.fork2", To: "dragons_lair.hidden_passage",
			Lock: LockPerception, LockData: map[string]any{"dc": 17},
			Hint: "a single coin standing on edge — a draft, not a tremor", Weight: 3},

		// direct_confrontation spur → merge.
		{From: "dragons_lair.charge_walk", To: "dragons_lair.direct_confrontation", Lock: LockNone},
		{From: "dragons_lair.direct_confrontation", To: "dragons_lair.broken_chamber", Lock: LockNone},
		{From: "dragons_lair.broken_chamber", To: "dragons_lair.infernax_doors", Lock: LockNone},

		// dragon_bargain spur → merge.
		{From: "dragons_lair.speakers_step", To: "dragons_lair.dragon_bargain", Lock: LockNone},
		{From: "dragons_lair.dragon_bargain", To: "dragons_lair.audience_hall", Lock: LockNone},
		{From: "dragons_lair.audience_hall", To: "dragons_lair.infernax_doors", Lock: LockNone},

		// hoard_pillar spur → merge.
		{From: "dragons_lair.hidden_passage", To: "dragons_lair.hoard_pillar", Lock: LockNone},
		{From: "dragons_lair.hoard_pillar", To: "dragons_lair.secret_arch", Lock: LockNone},
		{From: "dragons_lair.secret_arch", To: "dragons_lair.infernax_doors", Lock: LockNone},

		// R4 final approach.
		{From: "dragons_lair.infernax_doors", To: "dragons_lair.throne_ascension", Lock: LockNone},
		{From: "dragons_lair.throne_ascension", To: "dragons_lair.molten_throne_approach", Lock: LockNone},
		{From: "dragons_lair.molten_throne_approach", To: "dragons_lair.crown_steps", Lock: LockNone},
		{From: "dragons_lair.crown_steps", To: "dragons_lair.final_step", Lock: LockNone},
		{From: "dragons_lair.final_step", To: "dragons_lair.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneDragonsLair, nodes, edges)
}

func init() {
	registerZoneGraph(zoneDragonsLairGraph())
}
