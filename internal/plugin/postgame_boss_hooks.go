package plugin

// Tier-6 postgame "Layer-2" boss mechanics (Phase P8).
//
// Layer 1 (shipped) is everything the stock engine expresses: a single
// MonsterAbility rider plus HP/AC/Attack/PhaseTwoAt on the stat block. Layer 2
// is the bespoke, per-boss stuff the plan promised — mechanics that read the
// *run* the player took to reach the boss, not just the fight in front of them.
//
// This file holds the PRE-COMBAT half of that seam: adjustments derived once
// from run state and folded into the boss's live Combatant before the fight
// resolves. It must be a PURE, IDEMPOTENT function of run state, because the
// turn engine rebuilds the enemy from the bestiary every single round
// (partyCombatantsForSession) — the boss's numbers are re-derived on every
// !attack. That is fine as long as the inputs are frozen for the duration of
// the fight, which they are: the boss room is terminal, so no more nodes are
// walked once the fight begins.
//
// In-combat Layer-2 hooks (Inversion Stitch, Amendment, Two Hearts) are a
// separate seam — a new MonsterAbility.Effect case in applyAbility, shared by
// both engines — and are not in this file.

// applyBossRunModifiers folds any pre-combat Layer-2 mechanic for bossID into
// the freshly-built enemy Combatant, reading the run the player took to get
// here. It is dispatched by bestiary ID and is a no-op for every enemy that
// isn't a hooked T6 boss, so the two build seams can call it unconditionally on
// whatever they just built. A nil enemy or nil run is a no-op.
func applyBossRunModifiers(bossID string, enemy *Combatant, run *DungeonRun) {
	if enemy == nil || run == nil {
		return
	}
	switch bossID {
	case "boss_aurvandryx":
		applyGreedTax(enemy, run)
	}
}

// ── Greed Tax — Aurvandryx, the Ember Before Fire (First Hoard) ─────────────
//
// The First Hoard carries the game's richest LootBias nodes on purpose (the
// gilded veins at 2.5–3.0; nowhere else in the game exceeds 1.0). Aurvandryx —
// the hoard's true owner, not its guard dog — takes the interest out of your
// hide: her Attack rises with how much of the gilded route you walked to reach
// her cradle. Strip the veins and meet a furious wyrm; take the vow-of-poverty
// line through the zone and fight her lean.
//
// Signal: the summed excess LootBias (above the 1.0 baseline) of every node the
// run walked — NOT run.LootCollected, which only tracks the boss-only signature
// manifest and is empty at the boss. A full-explore route through the First
// Hoard accrues ~3.5 richness (+7 Attack); the cap covers a maximal line.
//
// Calibration (P8 sim sweep, L20 party+Pete+pets): the full-explore route lands
// first_hoard at ~47% clear (mid-band 40–55), down from the taxless 56%. A lean
// prod route pays no tax and fights her at the taxless rate; a maximal route
// hits the cap. No base-stat retune was needed — the tax corrects the prior
// slight overshoot into the band. NOTE: the sim's autopilot explores the whole
// graph, so the sweep only exercises the full-route point; the lean/greedy
// spread is a prod-only player-agency lever the headless sim cannot walk.
const (
	// greedTaxRichnessMult converts summed route richness into Attack points.
	greedTaxRichnessMult = 2.0

	// greedTaxMaxAttack caps the tax so a maximal hoard-run meets a very hard
	// wyrm, never a literally-unbeatable one.
	greedTaxMaxAttack = 12
)

// greedRouteRichness sums the excess LootBias (above the 1.0 baseline) of every
// node the run has walked. Extracted for a deterministic unit test.
func greedRouteRichness(run *DungeonRun) float64 {
	g, ok := loadZoneGraph(run.ZoneID)
	if !ok {
		return 0
	}
	var rich float64
	for _, id := range run.VisitedNodes {
		if n, exists := g.Nodes[id]; exists && n.Content.LootBias > 1.0 {
			rich += n.Content.LootBias - 1.0
		}
	}
	return rich
}

// greedTaxAttack is the Attack surcharge for a route of the given richness.
func greedTaxAttack(richness float64) int {
	tax := int(richness * greedTaxRichnessMult)
	if tax > greedTaxMaxAttack {
		tax = greedTaxMaxAttack
	}
	if tax < 0 {
		tax = 0
	}
	return tax
}

func applyGreedTax(enemy *Combatant, run *DungeonRun) {
	enemy.Stats.Attack += greedTaxAttack(greedRouteRichness(run))
}
