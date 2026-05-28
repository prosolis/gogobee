package plugin

// Crypt of Valdris branching graph.
//
// Long-expedition plan D1-b: extended from the original 8-node POC sketch
// to the new T1 length band (12–14 rooms traversed). Topology preserves
// the original diamond — main_hall (elite) vs side_chapel (perception
// fork), with a Perception-DC-15 secret dangling off side_chapel — and
// adds the missing Trap anchor + extra exploration on both arms so the
// longest entry→boss walk lands at 13.
//
//   entry → processional_stair → bone_corridor →
//   grave_dust_trap (TRAP) → ossuary_walk → fork
//                              ├──[unlocked]── main_hall (ELITE)
//                              │                → reliquary_hall → ash_passage ──┐
//                              │                                                  ├── antechamber
//                              └──[Perception DC 12]── side_chapel                │   → choir_landing
//                                  │                    → catacomb_passage        │   → throne_steps
//                                  │                    → vault_landing ──────────┘   → boss
//                                  └──[Perception DC 15]── secret_chamber (SECRET) → antechamber
//
// Both main and side arms walk 3 mid-nodes after the fork; secret stays a
// 1-node side dangle off side_chapel that merges back at antechamber, so
// the Perception-DC-15 path is a loot dip rather than a length detour.

func zoneCryptValdrisGraph() ZoneGraph {
	nodes := []ZoneNode{
		{NodeID: "crypt_valdris.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Crypt Threshold", PosX: 0, PosY: 1},
		{NodeID: "crypt_valdris.processional_stair", Kind: NodeKindExploration,
			Label: "Processional Stair", PosX: 1, PosY: 1},
		{NodeID: "crypt_valdris.bone_corridor", Kind: NodeKindExploration,
			Label: "Bone Corridor", PosX: 2, PosY: 1},
		{NodeID: "crypt_valdris.grave_dust_trap", Kind: NodeKindTrap,
			Label: "Grave-Dust Cascade", PosX: 3, PosY: 1},
		{NodeID: "crypt_valdris.ossuary_walk", Kind: NodeKindExploration,
			Label: "Ossuary Walk", PosX: 4, PosY: 1},
		{NodeID: "crypt_valdris.fork", Kind: NodeKindFork,
			Label: "Branching Passage", PosX: 5, PosY: 1},

		// Left arm — default, contains the elite.
		{NodeID: "crypt_valdris.main_hall", Kind: NodeKindElite,
			Label: "Main Hall", PosX: 6, PosY: 0},
		{NodeID: "crypt_valdris.reliquary_hall", Kind: NodeKindExploration,
			Label: "Reliquary Hall", PosX: 7, PosY: 0},
		{NodeID: "crypt_valdris.ash_passage", Kind: NodeKindExploration,
			Label: "Ash Passage", PosX: 8, PosY: 0},

		// Right arm — perception-gated, elite-free.
		{NodeID: "crypt_valdris.side_chapel", Kind: NodeKindExploration,
			Label: "Side Chapel", PosX: 6, PosY: 2},
		{NodeID: "crypt_valdris.catacomb_passage", Kind: NodeKindExploration,
			Label: "Catacomb Passage", PosX: 7, PosY: 2},
		{NodeID: "crypt_valdris.vault_landing", Kind: NodeKindExploration,
			Label: "Vault Landing", PosX: 8, PosY: 2},

		// Secret dangle off side_chapel — high-DC Perception, loot-biased.
		{NodeID: "crypt_valdris.secret_chamber", Kind: NodeKindSecret,
			Label: "Hidden Reliquary", PosX: 7, PosY: 3,
			Content: ZoneNodeContent{LootBias: 2.0}},

		// Merge + approach.
		{NodeID: "crypt_valdris.antechamber", Kind: NodeKindMerge,
			Label: "Antechamber", PosX: 9, PosY: 1},
		{NodeID: "crypt_valdris.choir_landing", Kind: NodeKindExploration,
			Label: "Choir Landing", PosX: 10, PosY: 1},
		{NodeID: "crypt_valdris.throne_steps", Kind: NodeKindExploration,
			Label: "Throne Steps", PosX: 11, PosY: 1},
		{NodeID: "crypt_valdris.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Valdris's Sanctum", PosX: 12, PosY: 1},
	}
	edges := []ZoneEdge{
		{From: "crypt_valdris.entry", To: "crypt_valdris.processional_stair", Lock: LockNone},
		{From: "crypt_valdris.processional_stair", To: "crypt_valdris.bone_corridor", Lock: LockNone},
		{From: "crypt_valdris.bone_corridor", To: "crypt_valdris.grave_dust_trap", Lock: LockNone},
		{From: "crypt_valdris.grave_dust_trap", To: "crypt_valdris.ossuary_walk", Lock: LockNone},
		{From: "crypt_valdris.ossuary_walk", To: "crypt_valdris.fork", Lock: LockNone},

		{From: "crypt_valdris.fork", To: "crypt_valdris.main_hall", Lock: LockNone, Weight: 1},
		{From: "crypt_valdris.fork", To: "crypt_valdris.side_chapel",
			Lock: LockPerception, LockData: map[string]any{"dc": 12},
			Hint: "a faint draft from a crack in the wall", Weight: 2},

		{From: "crypt_valdris.main_hall", To: "crypt_valdris.reliquary_hall", Lock: LockNone},
		{From: "crypt_valdris.reliquary_hall", To: "crypt_valdris.ash_passage", Lock: LockNone},
		{From: "crypt_valdris.ash_passage", To: "crypt_valdris.antechamber", Lock: LockNone},

		{From: "crypt_valdris.side_chapel", To: "crypt_valdris.catacomb_passage", Lock: LockNone, Weight: 1},
		{From: "crypt_valdris.side_chapel", To: "crypt_valdris.secret_chamber",
			Lock: LockPerception, LockData: map[string]any{"dc": 15},
			Hint: "a faint scratching behind the altar", Weight: 2},
		{From: "crypt_valdris.catacomb_passage", To: "crypt_valdris.vault_landing", Lock: LockNone},
		{From: "crypt_valdris.vault_landing", To: "crypt_valdris.antechamber", Lock: LockNone},
		{From: "crypt_valdris.secret_chamber", To: "crypt_valdris.antechamber", Lock: LockNone},

		{From: "crypt_valdris.antechamber", To: "crypt_valdris.choir_landing", Lock: LockNone},
		{From: "crypt_valdris.choir_landing", To: "crypt_valdris.throne_steps", Lock: LockNone},
		{From: "crypt_valdris.throne_steps", To: "crypt_valdris.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneCryptValdris, nodes, edges)
}

func init() {
	registerZoneGraph(zoneCryptValdrisGraph())
}
