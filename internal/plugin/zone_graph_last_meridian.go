package plugin

// The Last Meridian branching graph (Tier 6, Phase P4).
//
// A necropolis-observatory being decommissioned hour by hour. Regions: The
// Dusk Colonnade → Gallery of Spent Hours → The Escapement → One Minute To
// Midnight, on the shared T6 skeleton. Longest walk = 46, in band.
//
// A time zone: the forks gate on reading the clockwork's rhythm (INT for the
// escapement's rule, DEX to slip a stolen turn, WIS to keep your own hour
// when the Hour Thief reaches for it). The locked spurs are pockets of
// unspent time — high LootBias hours the Custodian hasn't dismantled yet.

func zoneLastMeridianGraph() ZoneGraph {
	stat := func(s string, dc int, hint string) pgLock {
		return pgLock{kind: LockStatCheck, lockData: map[string]any{"stat": s, "dc": dc}, hint: hint}
	}
	perc := func(dc int, hint string) pgLock {
		return pgLock{kind: LockPerception, lockData: map[string]any{"dc": dc}, hint: hint}
	}
	return buildPostgameZoneGraph(pgZoneSpec{
		zoneID:  ZoneLastMeridian,
		regions: [4]string{"meridian_dusk_colonnade", "meridian_gallery_spent_hours", "meridian_escapement", "meridian_one_minute"},
		entry:   "The Dusk Gate",
		preamble: [12]string{
			"The Dusk Colonnade", "Hall of Winding Nothing", "The Hour Thief's Trail", "Candle-Shortened Walk",
			"Horologist's Ghost-Aisle", "The Stolen Present", "Pickpocket's Snare", "Colonnade of Spent Light",
			"The Gallery Approach", "Gallery Mouth", "Hall of Spent Hours", "First Struck Bell",
		},
		fork1: pgFork{
			label:     "The First Hour",
			freeLabel: [3]string{"On-Time Passage", "The Kept Hour", "Gallery Floor"},
			lockLabel: [3]string{"Guarded Recess", "The Unspent Hour-Pocket", "Ticking Alcove"},
			lock:      stat("WIS", 16, "keep your own hour when the Thief reaches for it and a door stays open"),
		},
		r2gate: "Gallery of Spent Hours",
		r2build: [6]string{
			"Rows of Stopped Clocks", "The Twin Wardens' Door", "Changing of the Guard",
			"Shift-Schedule Hall", "The Nothing Behind", "Escapement Threshold",
		},
		fork2: pgFork{
			label:     "The Second Hour",
			freeLabel: [3]string{"Steady Escapement Walk", "The Even-Round Hall", "Escapement Floor"},
			lockLabel: [3]string{"Gear-Seam", "The Unwound Vault", "Pendulum Niche"},
			lock:      stat("INT", 17, "the pendulum only swings on even rounds — read the rhythm and slip past"),
		},
		r3gate: "The Escapement",
		r3build: [6]string{
			"The Original Pendulum", "Gallery of Its Legs", "Verdigris Colossus Walk",
			"Orrery-Ring Landing", "Midnight Antechamber", "Threshold of the Last Hour",
		},
		capstone: pgCapstone{
			label:      "The Midnight Fork",
			freeLabel:  [3]string{"Direct Midnight-Walk", "The Open Escapement", "Midnight Floor"},
			lockALabel: [3]string{"Borrowed Second", "The Slipped Turn", "Stolen-Time Path"},
			lockA:      stat("DEX", 18, "steal a second from the clock and step through before it notices"),
			lockBLabel: [3]string{"Unwound Seam", "The Hour Never Struck", "Unspent Shortcut"},
			lockB:      perc(18, "one hour on the great face was never dismantled — it is still open"),
		},
		merge: "One Minute To Midnight",
		approach: [5]string{
			"The Apologizing Golem", "Orrery of the Last Hour", "Chime Approach", "The Rewinding Second", "The Clock Stops",
		},
		boss: "The Custodian of the Last Hour",
		overrides: map[string]pgNodeOverride{
			"f1b2":  {kind: NodeKindSecret, bias: 2.25},
			"f2b2":  {kind: NodeKindSecret, bias: 2.25},
			"cap32": {kind: NodeKindSecret, bias: 2.5},
		},
	})
}

func init() {
	registerZoneGraph(zoneLastMeridianGraph())
}
