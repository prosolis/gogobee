package plugin

// The Drowned Star branching graph (Tier 6, Phase P4).
//
// The abyssal trench where Seraphel rode her star down. Regions: The Long
// Sink → Pilgrim Trench → The Radiant Wreck → The Heart Chapel, on the
// shared T6 skeleton. Longest walk = 46, in band.
//
// A descent zone: the forks gate on holding your nerve in the dark (CON
// against the pressure, WIS against the false lights, CHA to walk the
// pilgrim route unmolested). The locked spurs are radiant pockets of the
// wreck — high LootBias, lit by the dying star.

func zoneDrownedStarGraph() ZoneGraph {
	stat := func(s string, dc int, hint string) pgLock {
		return pgLock{kind: LockStatCheck, lockData: map[string]any{"stat": s, "dc": dc}, hint: hint}
	}
	perc := func(dc int, hint string) pgLock {
		return pgLock{kind: LockPerception, lockData: map[string]any{"dc": dc}, hint: hint}
	}
	return buildPostgameZoneGraph(pgZoneSpec{
		zoneID:  ZoneDrownedStar,
		regions: [4]string{"star_long_sink", "star_pilgrim_trench", "star_radiant_wreck", "star_heart_chapel"},
		entry:   "The Waterline",
		preamble: [12]string{
			"The Long Sink", "Descending Dark", "Pressure Gate", "The Static Shoal",
			"Choir of Static", "Sunless Landing", "Undertow Snare", "The Glow Below",
			"Pilgrim's Approach", "Trench Mouth", "The Lit Road", "First Lantern",
		},
		fork1: pgFork{
			label:     "The First Descent",
			freeLabel: [3]string{"Marked Pilgrim Road", "Lantern-Lit Walk", "Trench Floor"},
			lockLabel: [3]string{"Off-Route Pocket", "The Radiant Wreck-Pocket", "Sunken Alcove"},
			lock:      stat("CON", 16, "the pressure here would fold a lesser diver — endure it and the wreck opens"),
		},
		r2gate: "Pilgrim Trench",
		r2build: [6]string{
			"Trench of the Followed Light", "The Angler-Knight's Walk", "False-Light Gallery",
			"Warden's Lure", "The Unseen-Again Hall", "Wreck Threshold",
		},
		fork2: pgFork{
			label:     "The Second Descent",
			freeLabel: [3]string{"Steady Pilgrim Line", "Trusted Dark", "Wreck Floor"},
			lockLabel: [3]string{"Halo-Fragment Recess", "The Star-Lit Vault", "Radiant Niche"},
			lock:      stat("WIS", 17, "one lantern is not a lure — know which and it leads true"),
		},
		r3gate: "The Radiant Wreck",
		r3build: [6]string{
			"The Star's Broken Hull", "Gallery of Ten Thousand Years", "Leviathanling Roost",
			"The Glowing Silhouette", "Chapel Antechamber", "Threshold of the Heart",
		},
		capstone: pgCapstone{
			label:      "The Chapel Fork",
			freeLabel:  [3]string{"Direct Chapel-Walk", "The Open Reredos", "Chapel Floor"},
			lockALabel: [3]string{"Pilgrim's Grace", "The Unmolested Route", "Sanctified Path"},
			lockA:      stat("CHA", 18, "walk the pilgrim route as a pilgrim and it lets you pass"),
			lockBLabel: [3]string{"Halo-Seam", "The Heart's Own Light", "Radiant Shortcut"},
			lockB:      perc(18, "a crack in the radiance where the star still leaks its oldest light"),
		},
		merge: "The Heart Chapel",
		approach: [5]string{
			"The Shielded Star", "Seraphel's Vigil", "Sanctified Undertow Walk", "The Grieving Light", "The Last Radiance",
		},
		boss: "Seraphel, the Light That Sank",
		overrides: map[string]pgNodeOverride{
			"f1b2":  {kind: NodeKindSecret, bias: 2.25},
			"f2b2":  {kind: NodeKindSecret, bias: 2.25},
			"cap32": {kind: NodeKindSecret, bias: 2.5},
		},
	})
}

func init() {
	registerZoneGraph(zoneDrownedStarGraph())
}
