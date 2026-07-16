package plugin

import "testing"

func TestGreedTaxAttack(t *testing.T) {
	cases := []struct {
		rich float64
		want int
	}{
		{rich: -1, want: 0},
		{rich: 0, want: 0},
		{rich: 0.4, want: 0}, // 0.4*2 = 0.8, floors to 0
		{rich: 0.5, want: 1}, // one point per 0.5 richness
		{rich: 3.5, want: 7}, // the sim's full-explore route
		{rich: 5.25, want: 10},
		{rich: 6, want: 12},   // exactly the cap
		{rich: 100, want: 12}, // capped
	}
	for _, c := range cases {
		if got := greedTaxAttack(c.rich); got != c.want {
			t.Errorf("greedTaxAttack(%.2f) = %d, want %d", c.rich, got, c.want)
		}
	}
}

func TestGreedRouteRichness(t *testing.T) {
	// The First Hoard's gilded veins are the only >1.0 LootBias nodes in the
	// game: cap12@3.0, f1b2@2.75, r2b2@2.5 (see zone_graph_first_hoard.go). The
	// graph builder prefixes every node id with the zone id + ".".
	const pre = "first_hoard."

	// The full gilded route sums every vein's excess bias.
	all := &DungeonRun{ZoneID: ZoneFirstHoard, VisitedNodes: []string{pre + "entry", pre + "cap12", pre + "f1b2", pre + "r2b2", pre + "boss"}}
	if got := greedRouteRichness(all); !approxEq(got, 5.25) {
		t.Errorf("full route richness = %.2f, want 5.25", got)
	}

	// A single vein contributes only its own excess.
	one := &DungeonRun{ZoneID: ZoneFirstHoard, VisitedNodes: []string{pre + "entry", pre + "cap12", pre + "boss"}}
	if got := greedRouteRichness(one); !approxEq(got, 2.0) {
		t.Errorf("one-vein (cap12@3.0) richness = %.2f, want 2.0", got)
	}

	// A lean line that never touches a vein pays nothing.
	lean := &DungeonRun{ZoneID: ZoneFirstHoard, VisitedNodes: []string{pre + "entry", pre + "p1", pre + "p2", pre + "boss"}}
	if got := greedRouteRichness(lean); !approxEq(got, 0) {
		t.Errorf("lean route richness = %.2f, want 0", got)
	}

	// An unknown zone has no graph and no richness.
	if got := greedRouteRichness(&DungeonRun{ZoneID: "nope", VisitedNodes: []string{"x"}}); got != 0 {
		t.Errorf("unknown-zone richness = %.2f, want 0", got)
	}
}

func TestApplyBossRunModifiers_GreedTax(t *testing.T) {
	richRun := &DungeonRun{ZoneID: ZoneFirstHoard, VisitedNodes: []string{"first_hoard.cap12", "first_hoard.f1b2", "first_hoard.r2b2"}} // 5.25 rich → +10 Attack
	leanRun := &DungeonRun{ZoneID: ZoneFirstHoard, VisitedNodes: []string{"first_hoard.entry", "first_hoard.boss"}}                     // 0 rich → +0

	// Aurvandryx's Attack rises with the gilded route walked into her cradle.
	enemy := &Combatant{Stats: CombatStats{Attack: 45}}
	applyBossRunModifiers("boss_aurvandryx", enemy, richRun)
	if enemy.Stats.Attack != 45+10 {
		t.Errorf("greedy-route Aurvandryx Attack = %d, want %d", enemy.Stats.Attack, 45+10)
	}

	// A lean run leaves her at her base Attack.
	lean := &Combatant{Stats: CombatStats{Attack: 45}}
	applyBossRunModifiers("boss_aurvandryx", lean, leanRun)
	if lean.Stats.Attack != 45 {
		t.Errorf("lean-run Aurvandryx Attack = %d, want 45", lean.Stats.Attack)
	}

	// Every non-hooked enemy is untouched, even on the richest route.
	other := &Combatant{Stats: CombatStats{Attack: 45}}
	applyBossRunModifiers("boss_seamstress", other, richRun)
	if other.Stats.Attack != 45 {
		t.Errorf("non-hooked boss Attack = %d, want 45 (no-op)", other.Stats.Attack)
	}

	// A nil run is a no-op, not a panic.
	nilRun := &Combatant{Stats: CombatStats{Attack: 45}}
	applyBossRunModifiers("boss_aurvandryx", nilRun, nil)
	if nilRun.Stats.Attack != 45 {
		t.Errorf("nil-run Attack = %d, want 45 (no-op)", nilRun.Stats.Attack)
	}
}

func approxEq(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}
