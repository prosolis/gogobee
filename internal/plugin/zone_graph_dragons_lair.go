package plugin

// Phase G8g — Dragon's Lair (Infernus Peak) branching graph.
//
// T5 zone. Shape: long approach + mid-zone fork (converging at the
// elite) + capstone 3-way fork at the boss antechamber. Combines two
// distinct fork styles in one zone: a binary-converging fork and a
// triple-spread capstone — escalating the player's decision-weight as
// they descend.
//
//   entry → kobold_warrens → drake_pens → fork1 (binary, converges)
//                                            ├─[unlocked]── ash_bridge ────┐
//                                            └─[Perception DC 15]── treasure_vault ─┤
//                                                                                   ↓
//                                                                       wyrmlings_nest (elite young red dragon)
//                                                                                   ↓
//                                                                                fork2 (capstone, 3-way)
//                                                                                   ├─[unlocked]── direct_confrontation → boss
//                                                                                   ├─[CHA DC 16]── dragon_bargain → boss
//                                                                                   └─[Perception DC 17]── hoard_pillar (secret) → boss
//
// First authored zone with two distinct fork stages where the first
// converges (diamond-style) and the second spreads (3-way capstone).
// Earlier zones used either-or: stacked equal-width hubs (Manor) or
// late-only fork (Underforge). The escalation pattern ("commit, then
// commit harder") is unique to T5.

func zoneDragonsLairGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "dragons_lair.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Mountain Pass", PosX: 0, PosY: 1},
		{NodeID: "dragons_lair.kobold_warrens", Kind: NodeKindExploration,
			Label: "Kobold Warrens", PosX: 1, PosY: 1},
		{NodeID: "dragons_lair.drake_pens", Kind: NodeKindExploration,
			Label: "Drake Pens", PosX: 2, PosY: 1},
		{NodeID: "dragons_lair.fork1", Kind: NodeKindFork,
			Label: "The Cinder Crossing", PosX: 3, PosY: 1},
		{NodeID: "dragons_lair.ash_bridge", Kind: NodeKindTrap,
			Label: "Ash Bridge", PosX: 4, PosY: 0},
		{NodeID: "dragons_lair.treasure_vault", Kind: NodeKindExploration,
			Label: "Treasure Vault", PosX: 4, PosY: 2},
		{NodeID: "dragons_lair.wyrmlings_nest", Kind: NodeKindElite,
			Label: "Wyrmling's Nest", PosX: 5, PosY: 1},
		{NodeID: "dragons_lair.fork2", Kind: NodeKindFork,
			Label: "Hoard Approach", PosX: 6, PosY: 1},
		{NodeID: "dragons_lair.direct_confrontation", Kind: NodeKindExploration,
			Label: "Direct Approach", PosX: 7, PosY: 0},
		{NodeID: "dragons_lair.dragon_bargain", Kind: NodeKindExploration,
			Label: "Words With Infernax", PosX: 7, PosY: 1},
		{NodeID: "dragons_lair.hoard_pillar", Kind: NodeKindSecret,
			Label: "Hidden Hoard Pillar", PosX: 7, PosY: 2,
			Content: ZoneNodeContent{LootBias: 2.5}},
		{NodeID: "dragons_lair.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Infernax's Crown", PosX: 8, PosY: 1},
	}
	edges := []ZoneEdge{
		{From: "dragons_lair.entry", To: "dragons_lair.kobold_warrens", Lock: LockNone},
		{From: "dragons_lair.kobold_warrens", To: "dragons_lair.drake_pens", Lock: LockNone},
		{From: "dragons_lair.drake_pens", To: "dragons_lair.fork1", Lock: LockNone},
		// Fork1 — converges at wyrmlings_nest.
		{From: "dragons_lair.fork1", To: "dragons_lair.ash_bridge", Lock: LockNone, Weight: 1},
		{From: "dragons_lair.fork1", To: "dragons_lair.treasure_vault",
			Lock: LockPerception, LockData: map[string]any{"dc": 15},
			Hint: "a draft from a side passage — and the dull glint of gold beyond it", Weight: 2},
		{From: "dragons_lair.ash_bridge", To: "dragons_lair.wyrmlings_nest", Lock: LockNone},
		{From: "dragons_lair.treasure_vault", To: "dragons_lair.wyrmlings_nest", Lock: LockNone},
		{From: "dragons_lair.wyrmlings_nest", To: "dragons_lair.fork2", Lock: LockNone},
		// Fork2 — capstone 3-way.
		{From: "dragons_lair.fork2", To: "dragons_lair.direct_confrontation", Lock: LockNone, Weight: 1},
		{From: "dragons_lair.fork2", To: "dragons_lair.dragon_bargain",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "CHA", "dc": 16},
			Hint: "Infernax speaks. The voice fills the chamber. You could speak back.", Weight: 2},
		{From: "dragons_lair.fork2", To: "dragons_lair.hoard_pillar",
			Lock: LockPerception, LockData: map[string]any{"dc": 17},
			Hint: "a single coin standing on edge — a draft, not a tremor", Weight: 3},
		{From: "dragons_lair.direct_confrontation", To: "dragons_lair.boss", Lock: LockNone},
		{From: "dragons_lair.dragon_bargain", To: "dragons_lair.boss", Lock: LockNone},
		{From: "dragons_lair.hoard_pillar", To: "dragons_lair.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneDragonsLair, nodes, edges)
}

func init() {
	registerZoneGraph(zoneDragonsLairGraph())
}
