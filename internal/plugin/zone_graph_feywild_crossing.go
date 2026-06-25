package plugin

// Feywild Crossing branching graph.
//
// Long-expedition plan D1-d: extended from the original 9-node sketch
// to the new T4 length band (28–34 rooms traversed). Topology preserves
// the G8f design intent — woven first-stage forks (CHA vs Perception,
// no LockNone), hag_circle shared between the two first-stage paths,
// time_eddy grove-exclusive, illusion_garden marsh-exclusive — and
// adds the linear depth each arm now needs.
//
// New in D1-d: a single fae_court MERGE node gathers all three second-
// stage endings (hag_circle / time_eddy / illusion_garden) before the
// final walk to boss. The Court of the Antlered Queen is one venue, so
// the convergence reads thematically; this also avoids triplicating the
// long pre-boss approach.
//
// Also new: cursed_thicket TRAP anchor in the preamble — every walk
// hits it. The original G8f graph had no Trap node.
//
// D10 anchor variety (in-place kind swaps, no length change): the two
// first-stage approaches each gain one distinct anchor so the fork
// choice carries a flavor, not just a skill gate —
//   - Grove approach: Singing Orchard becomes an ELITE (the CHA-bonus
//     route is guarded; you pay for the bargain in blood).
//   - Marsh approach: Mire Steps becomes a TRAP (the free route is
//     hazardous footing rather than a fight).
// The time_eddy / illusion_garden exclusive endings stay anchor-light so
// they keep their "skip the hag elite for loot" trade-off.
//
//   Preamble (10 nodes, ending in fork1):
//     entry → twilight_path → veiled_glade → faerie_lights →
//     cursed_thicket (TRAP) → revel_road → moonshadow_bridge →
//     bargain_walk → fey_market → fork1
//
//   Fork1 → marsh (free default) | grove (CHA DC 14 bonus — the bargain):
//
//   Grove approach (8 nodes, CHA DC 14 bonus):
//     grove_threshold → starlight_path → singing_orchard → mirror_pond
//     → prismatic_arbor → moonpetal_clearing → dawnglow_walk →
//     glamoured_grove (fork2a)
//
//   Marsh approach (8 nodes, Perception DC 14):
//     marsh_threshold → wisp_lights → fog_basin → moaning_reeds →
//     mire_steps → submerged_stones → drowned_grove → wisp_marsh
//     (fork2b)
//
//   Fork2a (glamoured_grove) →
//     LockNone:        hag_corridor_a → dryad_promenade → moonbridge_a → hag_circle
//     LockStatCheck DEX 15: eddy_corridor → splintered_minute → unwoven_step → time_eddy
//
//   Fork2b (wisp_marsh) →
//     LockNone:        hag_corridor_b → fen_walk → moonbridge_b → hag_circle  (SHARED)
//     LockPerception 16: garden_approach → mirror_walk → unseen_path → illusion_garden
//
//   Post-elite/secret tails to fae_court MERGE (2 mid-nodes each):
//     hag_circle → courtly_walk → hawthorn_arch → fae_court
//     time_eddy → backward_corridor → reverberation_hall → fae_court
//     illusion_garden → false_arbor → unseen_atrium → fae_court
//
//   Final approach (4 mid-nodes + boss):
//     fae_court → antlered_approach → queens_promenade → throne_steps →
//     glamoured_dais → boss
//
// Longest entry→boss walk lands at 30 nodes (e.g. preamble 10 + grove
// approach 8 + eddy spur 3 + time_eddy 1 + eddy tail 2 + fae_court 1 +
// final approach 4 + boss 1 = 30), inside [28,34]. All four meaningful
// walks (grove/marsh × hag/exclusive) hit the same 30-node target by
// construction.

func zoneFeywildCrossingGraph() ZoneGraph {
	nodes := []ZoneNode{
		// Preamble.
		{NodeID: "feywild_crossing.entry", Kind: NodeKindEntry, IsEntry: true,
			Label: "Veil's Edge", PosX: 0, PosY: 2},
		{NodeID: "feywild_crossing.twilight_path", Kind: NodeKindExploration,
			Label: "Twilight Path", PosX: 1, PosY: 2},
		{NodeID: "feywild_crossing.veiled_glade", Kind: NodeKindExploration,
			Label: "Veiled Glade", PosX: 2, PosY: 2},
		{NodeID: "feywild_crossing.faerie_lights", Kind: NodeKindExploration,
			Label: "Faerie Lights", PosX: 3, PosY: 2},
		{NodeID: "feywild_crossing.cursed_thicket", Kind: NodeKindTrap,
			Label: "Cursed Thicket", PosX: 4, PosY: 2},
		{NodeID: "feywild_crossing.revel_road", Kind: NodeKindExploration,
			Label: "The Revel Road", PosX: 5, PosY: 2},
		{NodeID: "feywild_crossing.moonshadow_bridge", Kind: NodeKindExploration,
			Label: "Moonshadow Bridge", PosX: 6, PosY: 2},
		{NodeID: "feywild_crossing.bargain_walk", Kind: NodeKindExploration,
			Label: "The Bargain Walk", PosX: 7, PosY: 2},
		{NodeID: "feywild_crossing.fey_market", Kind: NodeKindExploration,
			Label: "Hush Market", PosX: 8, PosY: 2},
		{NodeID: "feywild_crossing.fork1", Kind: NodeKindFork,
			Label: "The Bargain Crossroads", PosX: 9, PosY: 2},

		// Grove approach (CHA route to glamoured_grove).
		{NodeID: "feywild_crossing.grove_threshold", Kind: NodeKindExploration,
			Label: "Grove Threshold", PosX: 10, PosY: 0},
		{NodeID: "feywild_crossing.starlight_path", Kind: NodeKindExploration,
			Label: "Starlight Path", PosX: 11, PosY: 0},
		{NodeID: "feywild_crossing.singing_orchard", Kind: NodeKindElite,
			Label: "Singing Orchard", PosX: 12, PosY: 0},
		{NodeID: "feywild_crossing.mirror_pond", Kind: NodeKindExploration,
			Label: "Mirror Pond", PosX: 13, PosY: 0},
		{NodeID: "feywild_crossing.prismatic_arbor", Kind: NodeKindExploration,
			Label: "Prismatic Arbor", PosX: 14, PosY: 0},
		{NodeID: "feywild_crossing.moonpetal_clearing", Kind: NodeKindExploration,
			Label: "Moonpetal Clearing", PosX: 15, PosY: 0},
		{NodeID: "feywild_crossing.dawnglow_walk", Kind: NodeKindExploration,
			Label: "Dawnglow Walk", PosX: 16, PosY: 0},
		{NodeID: "feywild_crossing.glamoured_grove", Kind: NodeKindFork,
			Label: "Glamoured Grove", PosX: 17, PosY: 0},

		// Marsh approach (Perception route to wisp_marsh).
		{NodeID: "feywild_crossing.marsh_threshold", Kind: NodeKindExploration,
			Label: "Marsh Threshold", PosX: 10, PosY: 4},
		{NodeID: "feywild_crossing.wisp_lights", Kind: NodeKindExploration,
			Label: "Wisp Lights", PosX: 11, PosY: 4},
		{NodeID: "feywild_crossing.fog_basin", Kind: NodeKindExploration,
			Label: "Fog Basin", PosX: 12, PosY: 4},
		{NodeID: "feywild_crossing.moaning_reeds", Kind: NodeKindExploration,
			Label: "Moaning Reeds", PosX: 13, PosY: 4},
		{NodeID: "feywild_crossing.mire_steps", Kind: NodeKindTrap,
			Label: "Mire Steps", PosX: 14, PosY: 4},
		{NodeID: "feywild_crossing.submerged_stones", Kind: NodeKindExploration,
			Label: "Submerged Stones", PosX: 15, PosY: 4},
		{NodeID: "feywild_crossing.drowned_grove", Kind: NodeKindExploration,
			Label: "Drowned Grove", PosX: 16, PosY: 4},
		{NodeID: "feywild_crossing.wisp_marsh", Kind: NodeKindFork,
			Label: "Wisp Marsh", PosX: 17, PosY: 4},

		// Grove → hag_circle spur.
		{NodeID: "feywild_crossing.hag_corridor_a", Kind: NodeKindExploration,
			Label: "Whispering Hedge", PosX: 18, PosY: 1},
		{NodeID: "feywild_crossing.dryad_promenade", Kind: NodeKindExploration,
			Label: "Dryad Promenade", PosX: 19, PosY: 1},
		{NodeID: "feywild_crossing.moonbridge_a", Kind: NodeKindExploration,
			Label: "Moonbridge", PosX: 20, PosY: 1},

		// Grove → time_eddy spur.
		{NodeID: "feywild_crossing.eddy_corridor", Kind: NodeKindExploration,
			Label: "Eddy Corridor", PosX: 18, PosY: 0},
		{NodeID: "feywild_crossing.splintered_minute", Kind: NodeKindExploration,
			Label: "Splintered Minute", PosX: 19, PosY: 0},
		{NodeID: "feywild_crossing.unwoven_step", Kind: NodeKindExploration,
			Label: "Unwoven Step", PosX: 20, PosY: 0},
		{NodeID: "feywild_crossing.time_eddy", Kind: NodeKindExploration,
			Label: "Time Eddy", PosX: 21, PosY: 0},

		// Marsh → hag_circle spur.
		{NodeID: "feywild_crossing.hag_corridor_b", Kind: NodeKindExploration,
			Label: "Sunken Causeway", PosX: 18, PosY: 3},
		{NodeID: "feywild_crossing.fen_walk", Kind: NodeKindExploration,
			Label: "Fen Walk", PosX: 19, PosY: 3},
		{NodeID: "feywild_crossing.moonbridge_b", Kind: NodeKindExploration,
			Label: "Drowned Moonbridge", PosX: 20, PosY: 3},

		// Marsh → illusion_garden spur.
		{NodeID: "feywild_crossing.garden_approach", Kind: NodeKindExploration,
			Label: "Garden Approach", PosX: 18, PosY: 4},
		{NodeID: "feywild_crossing.mirror_walk", Kind: NodeKindExploration,
			Label: "Mirror Walk", PosX: 19, PosY: 4},
		{NodeID: "feywild_crossing.unseen_path", Kind: NodeKindExploration,
			Label: "Unseen Path", PosX: 20, PosY: 4},
		{NodeID: "feywild_crossing.illusion_garden", Kind: NodeKindSecret,
			Label: "Illusion Garden", PosX: 21, PosY: 4,
			Content: ZoneNodeContent{LootBias: 2.0}},

		// hag_circle (shared elite).
		{NodeID: "feywild_crossing.hag_circle", Kind: NodeKindElite,
			Label: "Hag Circle", PosX: 21, PosY: 2},

		// Hag tail.
		{NodeID: "feywild_crossing.courtly_walk", Kind: NodeKindExploration,
			Label: "Courtly Walk", PosX: 22, PosY: 2},
		{NodeID: "feywild_crossing.hawthorn_arch", Kind: NodeKindExploration,
			Label: "Hawthorn Arch", PosX: 23, PosY: 2},

		// Eddy tail.
		{NodeID: "feywild_crossing.backward_corridor", Kind: NodeKindExploration,
			Label: "Backward Corridor", PosX: 22, PosY: 0},
		{NodeID: "feywild_crossing.reverberation_hall", Kind: NodeKindExploration,
			Label: "Reverberation Hall", PosX: 23, PosY: 0},

		// Garden tail.
		{NodeID: "feywild_crossing.false_arbor", Kind: NodeKindExploration,
			Label: "False Arbor", PosX: 22, PosY: 4},
		{NodeID: "feywild_crossing.unseen_atrium", Kind: NodeKindExploration,
			Label: "Unseen Atrium", PosX: 23, PosY: 4},

		// Final approach.
		{NodeID: "feywild_crossing.fae_court", Kind: NodeKindMerge,
			Label: "Fae Court", PosX: 24, PosY: 2},
		{NodeID: "feywild_crossing.antlered_approach", Kind: NodeKindExploration,
			Label: "Antlered Approach", PosX: 25, PosY: 2},
		{NodeID: "feywild_crossing.queens_promenade", Kind: NodeKindExploration,
			Label: "Queen's Promenade", PosX: 26, PosY: 2},
		{NodeID: "feywild_crossing.throne_steps", Kind: NodeKindExploration,
			Label: "Throne Steps", PosX: 27, PosY: 2},
		{NodeID: "feywild_crossing.glamoured_dais", Kind: NodeKindExploration,
			Label: "Glamoured Dais", PosX: 28, PosY: 2},
		{NodeID: "feywild_crossing.boss", Kind: NodeKindBoss, IsBoss: true,
			Label: "Court of the Antlered Queen", PosX: 29, PosY: 2},
	}
	edges := []ZoneEdge{
		// Preamble.
		{From: "feywild_crossing.entry", To: "feywild_crossing.twilight_path", Lock: LockNone},
		{From: "feywild_crossing.twilight_path", To: "feywild_crossing.veiled_glade", Lock: LockNone},
		{From: "feywild_crossing.veiled_glade", To: "feywild_crossing.faerie_lights", Lock: LockNone},
		{From: "feywild_crossing.faerie_lights", To: "feywild_crossing.cursed_thicket", Lock: LockNone},
		{From: "feywild_crossing.cursed_thicket", To: "feywild_crossing.revel_road", Lock: LockNone},
		{From: "feywild_crossing.revel_road", To: "feywild_crossing.moonshadow_bridge", Lock: LockNone},
		{From: "feywild_crossing.moonshadow_bridge", To: "feywild_crossing.bargain_walk", Lock: LockNone},
		{From: "feywild_crossing.bargain_walk", To: "feywild_crossing.fey_market", Lock: LockNone},
		{From: "feywild_crossing.fey_market", To: "feywild_crossing.fork1", Lock: LockNone},

		// Fork1 — marsh is the free default path; grove is a CHA-gated
		// bonus route (the fey bargain). (Both were skill-locked originally,
		// with no LockNone exit — a soft-lock: any character failing both
		// the CHA and Perception checks was permanently stranded here, since
		// fork rolls are deterministic with no retry. Every other zone fork
		// has a free path; freeing marsh restores that invariant while
		// keeping the signature CHA fey-bargain route as a bonus.)
		{From: "feywild_crossing.fork1", To: "feywild_crossing.grove_threshold",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "CHA", "dc": 14},
			Hint: "a starlight creature offering a deal — you'd have to play along", Weight: 1},
		{From: "feywild_crossing.fork1", To: "feywild_crossing.marsh_threshold",
			Lock: LockNone,
			Hint: "wisp-light flickering between two trees — pick your way through", Weight: 1},

		// Grove approach.
		{From: "feywild_crossing.grove_threshold", To: "feywild_crossing.starlight_path", Lock: LockNone},
		{From: "feywild_crossing.starlight_path", To: "feywild_crossing.singing_orchard", Lock: LockNone},
		{From: "feywild_crossing.singing_orchard", To: "feywild_crossing.mirror_pond", Lock: LockNone},
		{From: "feywild_crossing.mirror_pond", To: "feywild_crossing.prismatic_arbor", Lock: LockNone},
		{From: "feywild_crossing.prismatic_arbor", To: "feywild_crossing.moonpetal_clearing", Lock: LockNone},
		{From: "feywild_crossing.moonpetal_clearing", To: "feywild_crossing.dawnglow_walk", Lock: LockNone},
		{From: "feywild_crossing.dawnglow_walk", To: "feywild_crossing.glamoured_grove", Lock: LockNone},

		// Marsh approach.
		{From: "feywild_crossing.marsh_threshold", To: "feywild_crossing.wisp_lights", Lock: LockNone},
		{From: "feywild_crossing.wisp_lights", To: "feywild_crossing.fog_basin", Lock: LockNone},
		{From: "feywild_crossing.fog_basin", To: "feywild_crossing.moaning_reeds", Lock: LockNone},
		{From: "feywild_crossing.moaning_reeds", To: "feywild_crossing.mire_steps", Lock: LockNone},
		{From: "feywild_crossing.mire_steps", To: "feywild_crossing.submerged_stones", Lock: LockNone},
		{From: "feywild_crossing.submerged_stones", To: "feywild_crossing.drowned_grove", Lock: LockNone},
		{From: "feywild_crossing.drowned_grove", To: "feywild_crossing.wisp_marsh", Lock: LockNone},

		// glamoured_grove (fork2a) → hag spur (open) | eddy spur (DEX).
		{From: "feywild_crossing.glamoured_grove", To: "feywild_crossing.hag_corridor_a", Lock: LockNone, Weight: 1},
		{From: "feywild_crossing.glamoured_grove", To: "feywild_crossing.eddy_corridor",
			Lock: LockStatCheck, LockData: map[string]any{"stat": "DEX", "dc": 15},
			Hint: "the seconds run sideways here — sidestep at the right one", Weight: 2},
		{From: "feywild_crossing.hag_corridor_a", To: "feywild_crossing.dryad_promenade", Lock: LockNone},
		{From: "feywild_crossing.dryad_promenade", To: "feywild_crossing.moonbridge_a", Lock: LockNone},
		{From: "feywild_crossing.moonbridge_a", To: "feywild_crossing.hag_circle", Lock: LockNone},
		{From: "feywild_crossing.eddy_corridor", To: "feywild_crossing.splintered_minute", Lock: LockNone},
		{From: "feywild_crossing.splintered_minute", To: "feywild_crossing.unwoven_step", Lock: LockNone},
		{From: "feywild_crossing.unwoven_step", To: "feywild_crossing.time_eddy", Lock: LockNone},

		// wisp_marsh (fork2b) → hag spur (open) | garden spur (Perception secret).
		{From: "feywild_crossing.wisp_marsh", To: "feywild_crossing.hag_corridor_b", Lock: LockNone, Weight: 1},
		{From: "feywild_crossing.wisp_marsh", To: "feywild_crossing.garden_approach",
			Lock: LockPerception, LockData: map[string]any{"dc": 16},
			Hint: "a garden visible only when you don't look directly at it", Weight: 2},
		{From: "feywild_crossing.hag_corridor_b", To: "feywild_crossing.fen_walk", Lock: LockNone},
		{From: "feywild_crossing.fen_walk", To: "feywild_crossing.moonbridge_b", Lock: LockNone},
		{From: "feywild_crossing.moonbridge_b", To: "feywild_crossing.hag_circle", Lock: LockNone},
		{From: "feywild_crossing.garden_approach", To: "feywild_crossing.mirror_walk", Lock: LockNone},
		{From: "feywild_crossing.mirror_walk", To: "feywild_crossing.unseen_path", Lock: LockNone},
		{From: "feywild_crossing.unseen_path", To: "feywild_crossing.illusion_garden", Lock: LockNone},

		// Post-second-stage tails into fae_court merge.
		{From: "feywild_crossing.hag_circle", To: "feywild_crossing.courtly_walk", Lock: LockNone},
		{From: "feywild_crossing.courtly_walk", To: "feywild_crossing.hawthorn_arch", Lock: LockNone},
		{From: "feywild_crossing.hawthorn_arch", To: "feywild_crossing.fae_court", Lock: LockNone},
		{From: "feywild_crossing.time_eddy", To: "feywild_crossing.backward_corridor", Lock: LockNone},
		{From: "feywild_crossing.backward_corridor", To: "feywild_crossing.reverberation_hall", Lock: LockNone},
		{From: "feywild_crossing.reverberation_hall", To: "feywild_crossing.fae_court", Lock: LockNone},
		{From: "feywild_crossing.illusion_garden", To: "feywild_crossing.false_arbor", Lock: LockNone},
		{From: "feywild_crossing.false_arbor", To: "feywild_crossing.unseen_atrium", Lock: LockNone},
		{From: "feywild_crossing.unseen_atrium", To: "feywild_crossing.fae_court", Lock: LockNone},

		// Final approach.
		{From: "feywild_crossing.fae_court", To: "feywild_crossing.antlered_approach", Lock: LockNone},
		{From: "feywild_crossing.antlered_approach", To: "feywild_crossing.queens_promenade", Lock: LockNone},
		{From: "feywild_crossing.queens_promenade", To: "feywild_crossing.throne_steps", Lock: LockNone},
		{From: "feywild_crossing.throne_steps", To: "feywild_crossing.glamoured_dais", Lock: LockNone},
		{From: "feywild_crossing.glamoured_dais", To: "feywild_crossing.boss", Lock: LockNone},
	}
	return BuildGraph(ZoneFeywildCrossing, nodes, edges)
}

func init() {
	registerZoneGraph(zoneFeywildCrossingGraph())
}
