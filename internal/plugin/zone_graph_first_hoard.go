package plugin

// The First Hoard branching graph (Tier 6, Phase P4).
//
// Aurvandryx's gilded clutch under Infernus Peak. Regions: The Cooling
// Throne → Gilded Gullet → The Clutch Vaults → Cradle of the First Flame,
// on the shared T6 skeleton. Longest walk = 46, in band.
//
// Signature set-piece: the game-max LootBias route. Per the plan, the First
// Hoard carries the richest nodes in the game (2.5–3.0) because the boss's
// Layer-2 Greed Tax makes rich routes self-balancing — gild yourself and
// pay for it at Aurvandryx, or take the lean line. The free capstone (the
// deepest gilded vein, cap12) sits at the 3.0 ceiling; two buildup nodes
// carry 2.5/2.75. Forks gate on physical checks (STR/CON) — you force the
// vault, you don't sneak it.

func zoneFirstHoardGraph() ZoneGraph {
	stat := func(s string, dc int, hint string) pgLock {
		return pgLock{kind: LockStatCheck, lockData: map[string]any{"stat": s, "dc": dc}, hint: hint}
	}
	return buildPostgameZoneGraph(pgZoneSpec{
		zoneID:  ZoneFirstHoard,
		regions: [4]string{"hoard_cooling_throne", "hoard_gilded_gullet", "hoard_clutch_vaults", "hoard_cradle_first_flame"},
		entry:   "The Still-Warm Gate",
		preamble: [12]string{
			"Infernax's Corpse-Terrain", "The Cooling Throne", "Scale-River Ford", "Ashfall Landing",
			"The Guard-Dog's Rest", "Ember Colonnade", "Slag Pit", "Choir-Drake Roost",
			"The Gilded Descent", "Minted Approach", "Coin-Drift Hall", "Throat of the Mountain",
		},
		fork1: pgFork{
			label:     "The First Swallow",
			freeLabel: [3]string{"Lean Passage", "Bare Gullet", "Gullet Floor"},
			lockLabel: [3]string{"Vault-Seam", "The Gilded Vein", "Coin-Choked Stair"},
			lock:      stat("STR", 16, "a wall of fused coin — force it and the hoard spills"),
		},
		r2gate: "Gilded Gullet",
		r2build: [6]string{
			"Hall of Minted Antibodies", "The Simulacrum Font", "Mirror-Mint Gallery",
			"Executor's Approach", "The Scale-Debt Walk", "Vault Threshold",
		},
		fork2: pgFork{
			label:     "The Second Swallow",
			freeLabel: [3]string{"Poverty's Line", "Empty Clutch-Walk", "Clutch Floor"},
			lockLabel: [3]string{"Cracked Egg-Vault", "The Gilded Reliquary", "Gold-Sealed Nest"},
			lock:      stat("CON", 16, "the heat here would cook a lesser body — endure it for the richest vault"),
		},
		r3gate: "The Clutch Vaults",
		r3build: [6]string{
			"Rows of Gilded Eggs", "The Resonance Nursery", "Choir-Cracked Stone",
			"Wyrm-Sworn Landing", "Cradle Antechamber", "Threshold of the First Flame",
		},
		capstone: pgCapstone{
			label:      "The Cradle Fork",
			freeLabel:  [3]string{"The Deep Vein", "Mother-Lode Gallery", "Cradle Floor"},
			lockALabel: [3]string{"Ember Squeeze", "The Molten Reliquary", "Scale-Sworn Path"},
			lockA:      stat("STR", 18, "prise the last vault open before she wakes"),
			lockBLabel: [3]string{"Heat-Shimmer Recess", "The Poverty Shrine", "Vowed Approach"},
			lockB:      stat("WIS", 18, "leave the gold — the lean route arrives unburdened"),
		},
		merge: "Cradle of the First Flame",
		approach: [5]string{
			"The Warming Nest", "Clutch of the First Wyrm", "Ember-Before-Fire Walk", "The Progenitor's Shadow", "The First Breath",
		},
		boss: "Aurvandryx, the Ember Before Fire",
		overrides: map[string]pgNodeOverride{
			"cap12": {bias: 3.0},  // deepest gilded vein — game-max
			"f1b2":  {bias: 2.75}, // The Gilded Vein
			"r2b2":  {bias: 2.5},  // The Simulacrum Font
		},
	})
}

func init() {
	registerZoneGraph(zoneFirstHoardGraph())
}
