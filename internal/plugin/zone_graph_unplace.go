package plugin

// The Unplace branching graph (Tier 6, Phase P4).
//
// The unattended tear Belaxath left behind — geometry that gave up. Regions:
// The Fray → Escher's Debt → The Stitchworks → The Seam, on the shared T6
// skeleton. Longest walk = 46, in band.
//
// The Unplace's set-piece is its locks: every fork gates on a different way
// of not-trusting-the-walls (Perception to see the true corner, INT to
// out-think the geometry, DEX to move through a room that is also another
// room). The secret spurs are high-LootBias impossible-geometry pockets
// rather than named secrets — the reward is the shortcut nobody should be
// able to take.

func zoneUnplaceGraph() ZoneGraph {
	perc := func(dc int, hint string) pgLock {
		return pgLock{kind: LockPerception, lockData: map[string]any{"dc": dc}, hint: hint}
	}
	stat := func(s string, dc int, hint string) pgLock {
		return pgLock{kind: LockStatCheck, lockData: map[string]any{"stat": s, "dc": dc}, hint: hint}
	}
	return buildPostgameZoneGraph(pgZoneSpec{
		zoneID:  ZoneUnplace,
		regions: [4]string{"unplace_the_fray", "unplace_eschers_debt", "unplace_stitchworks", "unplace_the_seam"},
		entry:   "The Frayed Edge",
		preamble: [12]string{
			"The Fray", "Horizon Indoors", "The Borrowed Corner", "Room That Is Also",
			"Angleworn Passage", "The Miss You Took", "Fraying Snare", "Silhouette Hall",
			"The Uncounted Arms", "Escher's Approach", "Debt Landing", "The Owed Turn",
		},
		fork1: pgFork{
			label:     "The First Impossibility",
			freeLabel: [3]string{"Straight Enough Line", "Plain Corridor", "Debt Floor"},
			lockLabel: [3]string{"Corner-Behind-You", "The Pocket That Isn't", "Folded Alcove"},
			lock:      perc(16, "a corner you already passed is somehow ahead of you"),
		},
		r2gate: "Escher's Debt",
		r2build: [6]string{
			"Stair Up Into Down", "The Unpaid Landing", "Gallery of Wrong Angles",
			"Nalfeshnee Concourse", "The Recursive Hall", "Stitchworks Threshold",
		},
		fork2: pgFork{
			label:     "The Second Impossibility",
			freeLabel: [3]string{"Threadbare Walk", "Loose-Weave Hall", "Stitchworks Floor"},
			lockLabel: [3]string{"Seam-Behind-Seam", "The Unstitched Pocket", "Needled Recess"},
			lock:      stat("INT", 17, "the geometry has a rule; find it and the shortcut is legal"),
		},
		r3gate: "The Stitchworks",
		r3build: [6]string{
			"Rows of Half-Sewn Doors", "The Needle Gallery", "Marilith Loom",
			"Echo of Belaxath", "Seam Antechamber", "Threshold of the Tear",
		},
		capstone: pgCapstone{
			label:      "The Seam Fork",
			freeLabel:  [3]string{"Direct Seam-Walk", "The Open Tear", "Seam Floor"},
			lockALabel: [3]string{"Sideways Step", "The Room Behind This One", "Slipped Path"},
			lockA:      stat("DEX", 18, "step through the wall while it is also a door"),
			lockBLabel: [3]string{"Hairline Unreality", "The Undone Pocket", "Impossible Shortcut"},
			lockB:      perc(18, "there is a place that is not here, and it is closer than the door"),
		},
		merge: "The Seam",
		approach: [5]string{
			"The Far Half Speaking", "Sewn Into the Wound", "Needle-Rain Approach", "The Inside-Out Room", "The Last Stitch",
		},
		boss: "The Seamstress",
		overrides: map[string]pgNodeOverride{
			"f1b2":  {kind: NodeKindSecret, bias: 2.25},
			"f2b2":  {kind: NodeKindSecret, bias: 2.25},
			"cap32": {kind: NodeKindSecret, bias: 2.5},
		},
	})
}

func init() {
	registerZoneGraph(zoneUnplaceGraph())
}
