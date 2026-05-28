package plugin

// The Underforge branching graph.
//
// Long-expedition plan D1-c: extended from the original 10-node sketch
// to the new T3 length band (22–26 rooms traversed). Topology preserves
// the G8e design intent — a long one-way descent into Kharak Dûn with a
// single late 3-way antechamber fork — and adds the linear gauntlet
// length the new band asks for. Trap (Cooling River) and Elite (Magma
// Chamber) both sit on the descent; the 3-way fork is still the only
// decision in the zone, deferred to the room before the boss.
//
//   entry → sealed_threshold → wardstone_arch → forge_descent →
//   vapor_steps → slag_warrens → cooling_river (TRAP) → bellows_chamber
//   → cinder_walk → magma_chamber (ELITE) → emberforge_hall →
//   ashfall_corridor → smelting_vault → foreman_landing →
//   broken_crucible → great_anvil → resonance_passage → antechamber
//   (3-way fork)
//       ├─[unlocked]── direct_assault → forge_throat → ember_steps →
//       │              sealed_doors → boss
//       ├─[DEX DC 14]── catwalks → high_traverse → cinder_balcony →
//       │              vault_door → boss
//       └─[Perception DC 15]── forge_vault (SECRET) → secret_passage →
//                              makers_walk → vault_keyhole → boss
//
// The 17-node linear preamble is the zone's identity ("Kharak Dûn is a
// one-way descent — there is no scenic route, only the approach to what
// was sealed in"). The autopilot pitches 3 night-camps over the T3 band,
// breaking the descent into rest beats; the player still only picks the
// final fork. All three antechamber spokes are 4 mid-nodes so route
// choice is loot/encounter character, not shortcut economics.

func zoneUnderforgeGraph() ZoneGraph {
	nodes := []ZoneNode{
		// Linear descent (path positions 1–18).
		{NodeID: "underforge.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Sealed Threshold", PosX: 0, PosY: 1},
		{NodeID: "underforge.sealed_threshold", Kind: NodeKindExploration,
			Label: "Outer Wards", PosX: 1, PosY: 1},
		{NodeID: "underforge.wardstone_arch", Kind: NodeKindExploration,
			Label: "Wardstone Arch", PosX: 2, PosY: 1},
		{NodeID: "underforge.forge_descent", Kind: NodeKindExploration,
			Label: "Forge Descent", PosX: 3, PosY: 1},
		{NodeID: "underforge.vapor_steps", Kind: NodeKindExploration,
			Label: "Vapor Steps", PosX: 4, PosY: 1},
		{NodeID: "underforge.slag_warrens", Kind: NodeKindExploration,
			Label: "Slag Warrens", PosX: 5, PosY: 1},
		{NodeID: "underforge.cooling_river", Kind: NodeKindTrap,
			Label: "Cooling River", PosX: 6, PosY: 1},
		{NodeID: "underforge.bellows_chamber", Kind: NodeKindExploration,
			Label: "Bellows Chamber", PosX: 7, PosY: 1},
		{NodeID: "underforge.cinder_walk", Kind: NodeKindExploration,
			Label: "Cinder Walk", PosX: 8, PosY: 1},
		{NodeID: "underforge.magma_chamber", Kind: NodeKindElite,
			Label: "Magma Chamber", PosX: 9, PosY: 1},
		{NodeID: "underforge.emberforge_hall", Kind: NodeKindExploration,
			Label: "Emberforge Hall", PosX: 10, PosY: 1},
		{NodeID: "underforge.ashfall_corridor", Kind: NodeKindExploration,
			Label: "Ashfall Corridor", PosX: 11, PosY: 1},
		{NodeID: "underforge.smelting_vault", Kind: NodeKindExploration,
			Label: "Smelting Vault", PosX: 12, PosY: 1},
		{NodeID: "underforge.foreman_landing", Kind: NodeKindExploration,
			Label: "Foreman's Landing", PosX: 13, PosY: 1},
		{NodeID: "underforge.broken_crucible", Kind: NodeKindExploration,
			Label: "Broken Crucible", PosX: 14, PosY: 1},
		{NodeID: "underforge.great_anvil", Kind: NodeKindExploration,
			Label: "Great Anvil", PosX: 15, PosY: 1},
		{NodeID: "underforge.resonance_passage", Kind: NodeKindExploration,
			Label: "Resonance Passage", PosX: 16, PosY: 1},
		{NodeID: "underforge.antechamber", Kind: NodeKindFork,
			Label: "Antechamber of Kharak Dûn", PosX: 17, PosY: 1},

		// Antechamber — open spoke (direct assault).
		{NodeID: "underforge.direct_assault", Kind: NodeKindExploration,
			Label: "Direct Assault", PosX: 18, PosY: 0},
		{NodeID: "underforge.forge_throat", Kind: NodeKindExploration,
			Label: "Forge Throat", PosX: 19, PosY: 0},
		{NodeID: "underforge.ember_steps", Kind: NodeKindExploration,
			Label: "Ember Steps", PosX: 20, PosY: 0},
		{NodeID: "underforge.sealed_doors", Kind: NodeKindExploration,
			Label: "Sealed Doors", PosX: 21, PosY: 0},

		// Antechamber — DEX spoke (catwalks).
		{NodeID: "underforge.catwalks", Kind: NodeKindExploration,
			Label: "Forge Catwalks", PosX: 18, PosY: 1},
		{NodeID: "underforge.high_traverse", Kind: NodeKindExploration,
			Label: "High Traverse", PosX: 19, PosY: 1},
		{NodeID: "underforge.cinder_balcony", Kind: NodeKindExploration,
			Label: "Cinder Balcony", PosX: 20, PosY: 1},
		{NodeID: "underforge.vault_door", Kind: NodeKindExploration,
			Label: "Vault Door", PosX: 21, PosY: 1},

		// Antechamber — secret spoke (forge vault).
		{NodeID: "underforge.forge_vault", Kind: NodeKindSecret,
			Label: "Forge Vault", PosX: 18, PosY: 2,
			Content: ZoneNodeContent{LootBias: 2.0}},
		{NodeID: "underforge.secret_passage", Kind: NodeKindExploration,
			Label: "Secret Passage", PosX: 19, PosY: 2},
		{NodeID: "underforge.makers_walk", Kind: NodeKindExploration,
			Label: "Maker's Walk", PosX: 20, PosY: 2},
		{NodeID: "underforge.vault_keyhole", Kind: NodeKindExploration,
			Label: "Vault Keyhole", PosX: 21, PosY: 2},

		{NodeID: "underforge.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "The Sealed Hall", PosX: 22, PosY: 1},
	}
	edges := []ZoneEdge{
		// Linear descent.
		{From: "underforge.entry", To: "underforge.sealed_threshold", Lock: LockNone},
		{From: "underforge.sealed_threshold", To: "underforge.wardstone_arch", Lock: LockNone},
		{From: "underforge.wardstone_arch", To: "underforge.forge_descent", Lock: LockNone},
		{From: "underforge.forge_descent", To: "underforge.vapor_steps", Lock: LockNone},
		{From: "underforge.vapor_steps", To: "underforge.slag_warrens", Lock: LockNone},
		{From: "underforge.slag_warrens", To: "underforge.cooling_river", Lock: LockNone},
		{From: "underforge.cooling_river", To: "underforge.bellows_chamber", Lock: LockNone},
		{From: "underforge.bellows_chamber", To: "underforge.cinder_walk", Lock: LockNone},
		{From: "underforge.cinder_walk", To: "underforge.magma_chamber", Lock: LockNone},
		{From: "underforge.magma_chamber", To: "underforge.emberforge_hall", Lock: LockNone},
		{From: "underforge.emberforge_hall", To: "underforge.ashfall_corridor", Lock: LockNone},
		{From: "underforge.ashfall_corridor", To: "underforge.smelting_vault", Lock: LockNone},
		{From: "underforge.smelting_vault", To: "underforge.foreman_landing", Lock: LockNone},
		{From: "underforge.foreman_landing", To: "underforge.broken_crucible", Lock: LockNone},
		{From: "underforge.broken_crucible", To: "underforge.great_anvil", Lock: LockNone},
		{From: "underforge.great_anvil", To: "underforge.resonance_passage", Lock: LockNone},
		{From: "underforge.resonance_passage", To: "underforge.antechamber", Lock: LockNone},

		// Antechamber 3-way fork.
		{From: "underforge.antechamber", To: "underforge.direct_assault", Lock: LockNone, Weight: 1},
		{From: "underforge.antechamber", To: "underforge.catwalks",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "DEX", "dc": 14},
			Hint: "rusted catwalks above the magma — quick footing required", Weight: 2},
		{From: "underforge.antechamber", To: "underforge.forge_vault",
			Lock: LockPerception, LockData: map[string]any{"dc": 15},
			Hint: "a draft from a stone seam, where no draft should be", Weight: 3},

		// Direct assault spoke.
		{From: "underforge.direct_assault", To: "underforge.forge_throat", Lock: LockNone},
		{From: "underforge.forge_throat", To: "underforge.ember_steps", Lock: LockNone},
		{From: "underforge.ember_steps", To: "underforge.sealed_doors", Lock: LockNone},
		{From: "underforge.sealed_doors", To: "underforge.boss", Lock: LockNone},

		// Catwalks spoke.
		{From: "underforge.catwalks", To: "underforge.high_traverse", Lock: LockNone},
		{From: "underforge.high_traverse", To: "underforge.cinder_balcony", Lock: LockNone},
		{From: "underforge.cinder_balcony", To: "underforge.vault_door", Lock: LockNone},
		{From: "underforge.vault_door", To: "underforge.boss", Lock: LockNone},

		// Forge vault spoke (secret).
		{From: "underforge.forge_vault", To: "underforge.secret_passage", Lock: LockNone},
		{From: "underforge.secret_passage", To: "underforge.makers_walk", Lock: LockNone},
		{From: "underforge.makers_walk", To: "underforge.vault_keyhole", Lock: LockNone},
		{From: "underforge.vault_keyhole", To: "underforge.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneUnderforge, nodes, edges)
}

func init() {
	registerZoneGraph(zoneUnderforgeGraph())
}
