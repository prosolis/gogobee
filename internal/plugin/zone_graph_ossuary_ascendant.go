package plugin

// The Ossuary Ascendant branching graph (Tier 6, Phase P4).
//
// Valdris's inverted bone-cathedral. Four regions — Bonefall Steps →
// Cathedral of Marrow → Reliquary Vaults → The Apotheosis Engine — on the
// shared T6 skeleton (buildPostgameZoneGraph). Longest walk = 46, in band.
//
// Signature set-piece: the three Phylactery Verses. Each is a NodeKindSecret
// reached through a LockPerception spur (one per fork), matching the plan's
// "three SECRET nodes, Verses found before the boss strip a Legendary
// Resistance / rebirth proc." The gate rides the fork→threshold edge; the
// edge into the Verse itself is unlocked. Full-clear explorers who find all
// three fight a mortal; speedrunners fight a god. Secret suffixes: f1b2,
// f2b2, cap32 (the middle node of each Perception spur).

func zoneOssuaryAscendantGraph() ZoneGraph {
	perc := func(dc int, hint string) pgLock {
		return pgLock{kind: LockPerception, lockData: map[string]any{"dc": dc}, hint: hint}
	}
	return buildPostgameZoneGraph(pgZoneSpec{
		zoneID:  ZoneOssuaryAscendant,
		regions: [4]string{"ossuary_bonefall_steps", "ossuary_cathedral_marrow", "ossuary_reliquary_vaults", "ossuary_apotheosis_engine"},
		entry:   "The Inverted Threshold",
		preamble: [12]string{
			"Bonefall Steps", "The Rising Ossuary", "Marrow Stair", "Chancel of Ash",
			"Ribvault Landing", "The Patient Gallery", "Reliquary Snare", "Choirless Nave",
			"Ascending Transept", "The Counted Dead", "Vertebral Climb", "First Landing",
		},
		fork1: pgFork{
			label:     "The First Ascent",
			freeLabel: [3]string{"Open Stair", "Bonelit Passage", "Cathedral Approach"},
			lockLabel: [3]string{"Hairline Recess", "Verse of the Waiting Grave", "Sealed Alcove"},
			lock:      perc(16, "a seam in the marrow where the wall was never quite finished"),
		},
		r2gate: "Cathedral of Marrow",
		r2build: [6]string{
			"Nave of Fused Clergy", "The Censer Walk", "Grave-Smoke Aisle",
			"Choir of the Pre-Blessed", "Cardinal's Approach", "The Marrow Altar",
		},
		fork2: pgFork{
			label:     "The Second Ascent",
			freeLabel: [3]string{"Vault Stair", "Donor-Bone Hall", "Reliquary Approach"},
			lockLabel: [3]string{"Cracked Reliquary", "Verse of the Bait Long Set", "Dust-Sealed Niche"},
			lock:      perc(17, "one reliquary is hollow where the others are full"),
		},
		r3gate: "Reliquary Vaults",
		r3build: [6]string{
			"The Coffin-Lid Wall", "Hall of Riveted Knights", "Gallery of the Crypt-Fallen",
			"The Unoccupied Armor", "Engine Antechamber", "Threshold of Ascension",
		},
		capstone: pgCapstone{
			label:      "The Apotheosis Fork",
			freeLabel:  [3]string{"Direct Ascent", "The Final Stair", "Engine Floor"},
			lockALabel: [3]string{"Deacon's Whisper", "The Named Dead", "Consecrated Path"},
			lockA:      pgLock{kind: LockStatCheck, lockData: map[string]any{"stat": "WIS", "dc": 18}, hint: "the choir sings your name in rounds — answer it and it opens"},
			lockBLabel: [3]string{"Vertebral Fault", "Verse of the Homecoming", "The Last Recess"},
			lockB:      perc(18, "the highest vertebra of the cathedral was set wrong on purpose"),
		},
		merge: "The Apotheosis Engine",
		approach: [5]string{
			"Marrow Conduit", "The Ascending Choir", "Phylactery Cradle", "Vertebra of Godhood", "The Final Blessing",
		},
		boss: "Valdris, At Last",
		overrides: map[string]pgNodeOverride{
			"f1b2":  {kind: NodeKindSecret, bias: 2.0},
			"f2b2":  {kind: NodeKindSecret, bias: 2.0},
			"cap32": {kind: NodeKindSecret, bias: 2.0},
		},
	})
}

func init() {
	registerZoneGraph(zoneOssuaryAscendantGraph())
}
