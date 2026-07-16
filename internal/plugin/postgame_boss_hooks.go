package plugin

// Tier-6 postgame "Layer-2" boss mechanics (Phase P8).
//
// Layer 1 (shipped) is everything the stock engine expresses: a single
// MonsterAbility rider plus HP/AC/Attack/PhaseTwoAt on the stat block. Layer 2
// is the bespoke, per-boss stuff the plan promised — mechanics that read the
// *run* the player took to reach the boss, not just the fight in front of them.
//
// It holds BOTH halves of that seam:
//
//   - PRE-COMBAT (applyBossRunModifiers / seedBossRunStatuses): adjustments
//     derived once from run state and folded into the boss's live Combatant (or
//     the fresh session) before the fight resolves. applyBossRunModifiers must be
//     a PURE, IDEMPOTENT function of run state, because the turn engine rebuilds
//     the enemy from the bestiary every single round (partyCombatantsForSession)
//     — the boss's numbers are re-derived on every !attack. That is fine as long
//     as the inputs are frozen for the fight, which they are: the boss room is
//     terminal, so no more nodes are walked once the fight begins.
//
//   - IN-COMBAT (applyBossInCombatRoundEnd): round-boundary mechanics that read
//     and mutate the live combatState — HP snapshots, once-only rewinds, escalating
//     timers. Called from the turn engine's stepRoundEnd after the round's damage
//     has settled. Any state it spends round-trips through CombatStatuses so a
//     suspend/resume can't replay it.
//
// The remaining planned in-combat mechanics (Inversion Stitch, Two Hearts) are
// still a separate seam — a new MonsterAbility.Effect case in applyAbility — and
// are not in this file yet.

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

// ── Phylactery Verses — Valdris, At Last (The Ossuary Ascendant) ────────────
//
// The plan Valdris has been running since he "died" in the T1 Crypt: the
// phylactery shard players looted for years was bait, and he has rebuilt as a
// true lich. His rebirths are bound into three Verses hidden in the cathedral,
// each a NodeKindSecret behind a Perception gate. Every Verse a player finds and
// walks before the fight UNBINDS one rebirth; every Verse they skip leaves it
// armed. Full-clear explorers strip all three and fight a mortal lich;
// speedrunners who blow past the secrets fight a god who will not stay down.
//
// This is the mirror-image of the Greed Tax's design axis: the Tax punishes
// greedy exploration, the Verses reward thorough exploration. Both are prod
// player-agency levers the headless sim only samples at whatever route its
// autopilot happens to walk.
//
// Unlike the Greed Tax (a pure per-round Attack recompute), a rebirth is spent
// mid-fight, so it is stateful: seedBossRunStatuses freezes the charge count
// onto the session ONCE at fight start, and the turn engine round-trips the
// live count through CombatStatuses. It must NOT be re-derived on the per-round
// enemy rebuild, or spent rebirths would come back every round.
const (
	// phylacteryReviveFrac is the fraction of the boss's (party-scaled) max HP
	// each unbound rebirth restores him to — a real second wind, not the 1-HP
	// stay of survive_at_1.
	phylacteryReviveFrac = 4 // 1/4 == 25%
)

// phylacteryReviveCharges is the number of rebirths still armed on Valdris: one
// per zone Verse (NodeKindSecret) the run has NOT visited. The Ossuary's only
// secret nodes are the three Verses (the shared builder stamps none by default),
// so counting unvisited secrets is exactly "unbound rebirths". Zero for any
// non-Valdris boss or a nil run. A pure function of frozen run state.
func phylacteryReviveCharges(bossID string, run *DungeonRun) int {
	if bossID != "boss_valdris_ascendant" || run == nil {
		return 0
	}
	g, ok := loadZoneGraph(run.ZoneID)
	if !ok {
		return 0
	}
	visited := make(map[string]bool, len(run.VisitedNodes))
	for _, id := range run.VisitedNodes {
		visited[id] = true
	}
	charges := 0
	for id, n := range g.Nodes {
		if n.Kind == NodeKindSecret && !visited[id] {
			charges++
		}
	}
	return charges
}

// seedBossRunStatuses folds any ONCE-AT-FIGHT-START Layer-2 boss state into the
// freshly-created session, reading the run the player took to get here. It is
// the stateful counterpart to applyBossRunModifiers (which is a per-round pure
// recompute): whatever it seeds here is mutated by the fight and round-tripped,
// never re-derived. enemyMaxHP is the party-scaled pool already persisted onto
// the session. Returns whether it changed anything (so the caller can skip a
// redundant save). No-op — false — for every non-hooked boss.
func seedBossRunStatuses(sess *CombatSession, bossID string, enemyMaxHP int, run *DungeonRun) bool {
	if sess == nil {
		return false
	}
	if charges := phylacteryReviveCharges(bossID, run); charges > 0 {
		sess.Statuses.EnemyReviveCharges = charges
		sess.Statuses.EnemyReviveHP = max(1, enemyMaxHP/phylacteryReviveFrac)
		return true
	}
	return false
}

// ── Amendment — The Custodian of the Last Hour (The Last Meridian) ───────────
//
// The Custodian has concluded its contract is complete and is dismantling the
// hours on its way out — including, once, the last few minutes of its own fight.
// Its true durability is HIDDEN in the opening rounds: it snapshots its HP at the
// end of round 3, and the first time the party knocks it into phase 2 it *rewinds
// itself* to that snapshot — once. Front-loaded burst is partially refunded (the
// damage past the snapshot is undone), so sustained builds that can grind through
// the extra pool shine over glass-cannon alpha strikes. A soft "midnight timer"
// then leans on stalls: every round past round 20 the Custodian's Attack climbs,
// so a fight that can't close still resolves before the clock runs out.
//
// This is the first IN-COMBAT Layer-2 mechanic. Unlike the pre-combat hooks it
// reads/mutates the live combatState at round end, and its once-only state
// (EnemyRewindHP snapshot + EnemyRewindUsed) round-trips through CombatStatuses so
// a suspend/resume can't hand the boss a second rewind. Dispatched by bestiary ID,
// so it is a no-op for every other enemy.
const (
	// custodianSnapshotRound is the round whose end HP the Custodian rewinds to.
	// Snapshotting late enough that a normal party has committed real damage, but
	// early enough that the refund still matters, is the whole point — the fight's
	// true length is concealed until the rewind fires.
	custodianSnapshotRound = 3

	// custodianPhaseTwoFrac must track The Custodian's PhaseTwoAt in
	// postgame_zone_defs.go (0.45): the rewind is meant to fire exactly as the
	// party crosses the boss into phase 2. Kept as a local constant because the
	// turn engine resolves off the bestiary template + session, not the
	// ZoneDefinition, so the threshold isn't otherwise in reach at round end.
	custodianPhaseTwoFrac = 0.45

	// midnightAtkStep / midnightAtkCap: the soft closing-time timer. Every round
	// past midnightTimerAfter adds midnightAtkStep to the boss's Attack (via the
	// shared EnemyAtkBuff, which enemyAttackStat already folds in), capped so a
	// truly stalled fight still ends without the number becoming meaningless.
	midnightTimerAfter = 20
	midnightAtkStep    = 2
	midnightAtkCap     = 40
)

// applyBossInCombatRoundEnd resolves the round-boundary in-combat Layer-2
// mechanics for the enemy the turn engine is fighting. It runs after the round's
// damage has settled and only while the enemy still stands. Dispatched by
// bestiary ID; a no-op for every enemy that isn't a hooked T6 boss.
func applyBossInCombatRoundEnd(st *combatState, bossID string, enemyMaxHP int) {
	if st == nil {
		return
	}
	switch bossID {
	case "boss_custodian":
		applyAmendment(st, enemyMaxHP)
	}
}

// applyAmendment resolves the Custodian's Amendment (round-3 snapshot + once-only
// phase-2 rewind) and its soft midnight timer, given the round that just finished
// (st.round, pre-increment) and the boss's party-scaled MaxHP.
func applyAmendment(st *combatState, enemyMaxHP int) {
	// Snapshot the boss's HP at the end of round 3 — the concealed "true length".
	if st.round == custodianSnapshotRound && st.enemyRewindHP == 0 {
		st.enemyRewindHP = st.enemyHP
	}
	// The first time the party crosses the boss into phase 2, rewind to the
	// snapshot — once. Guarded on snapshot > current so a party that never got
	// below the round-3 HP (and a fight bursted into phase 2 before the snapshot
	// was even taken, EnemyRewindHP == 0) can't be handed a free heal.
	phaseTwo := int(custodianPhaseTwoFrac * float64(enemyMaxHP))
	if !st.enemyRewindUsed && st.enemyRewindHP > st.enemyHP && st.enemyHP <= phaseTwo {
		st.enemyHP = min(enemyMaxHP, st.enemyRewindHP)
		st.enemyRewindUsed = true
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: CombatPhaseRoundEnd, Actor: "enemy", Action: "amendment_rewind",
			PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}
	// Soft midnight timer: closing time leans on a stalled fight.
	if st.round >= midnightTimerAfter && st.enemyAtkBuff < midnightAtkCap {
		st.enemyAtkBuff = min(midnightAtkCap, st.enemyAtkBuff+midnightAtkStep)
		st.events = append(st.events, CombatEvent{
			Round: st.round, Phase: CombatPhaseRoundEnd, Actor: "enemy", Action: "midnight_toll",
			Damage: midnightAtkStep, PlayerHP: st.playerHP, EnemyHP: st.enemyHP,
		})
	}
}
