package plugin

import (
	"strings"
	"testing"
)

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

func TestPhylacteryReviveCharges(t *testing.T) {
	// The Ossuary's three Verses are its only secret nodes: f1b2, f2b2, cap32
	// (see zone_graph_ossuary_ascendant.go). Node ids are zone-prefixed.
	const pre = "ossuary_ascendant."

	// Skip every Verse → all three rebirths stay armed (fight a god).
	skip := &DungeonRun{ZoneID: ZoneOssuaryAscendant, VisitedNodes: []string{pre + "entry", pre + "p1", pre + "boss"}}
	if got := phylacteryReviveCharges("boss_valdris_ascendant", skip); got != 3 {
		t.Errorf("skip-route charges = %d, want 3", got)
	}

	// Find one Verse → one rebirth unbound, two remain.
	one := &DungeonRun{ZoneID: ZoneOssuaryAscendant, VisitedNodes: []string{pre + "entry", pre + "f1b2", pre + "boss"}}
	if got := phylacteryReviveCharges("boss_valdris_ascendant", one); got != 2 {
		t.Errorf("one-Verse charges = %d, want 2", got)
	}

	// Full-clear explorer finds all three → 0 rebirths, a mortal lich.
	full := &DungeonRun{ZoneID: ZoneOssuaryAscendant, VisitedNodes: []string{pre + "entry", pre + "f1b2", pre + "f2b2", pre + "cap32", pre + "boss"}}
	if got := phylacteryReviveCharges("boss_valdris_ascendant", full); got != 0 {
		t.Errorf("full-clear charges = %d, want 0", got)
	}

	// Only Valdris carries the Verses; a nil run and any other boss are 0.
	if got := phylacteryReviveCharges("boss_aurvandryx", skip); got != 0 {
		t.Errorf("non-Valdris boss charges = %d, want 0", got)
	}
	if got := phylacteryReviveCharges("boss_valdris_ascendant", nil); got != 0 {
		t.Errorf("nil-run charges = %d, want 0", got)
	}
}

func TestSeedBossRunStatuses_Phylactery(t *testing.T) {
	const pre = "ossuary_ascendant."
	skip := &DungeonRun{ZoneID: ZoneOssuaryAscendant, VisitedNodes: []string{pre + "entry", pre + "boss"}} // 3 unfound

	// A skip-route seeds all three rebirths and the 25%-of-max revive pool.
	sess := &CombatSession{}
	if !seedBossRunStatuses(sess, "boss_valdris_ascendant", 560, skip) {
		t.Fatal("skip-route seed returned false, want true (charges seeded)")
	}
	if sess.Statuses.EnemyReviveCharges != 3 {
		t.Errorf("seeded charges = %d, want 3", sess.Statuses.EnemyReviveCharges)
	}
	if sess.Statuses.EnemyReviveHP != 140 { // 560/4
		t.Errorf("seeded revive HP = %d, want 140", sess.Statuses.EnemyReviveHP)
	}

	// A full-clear seeds nothing and reports no change.
	full := &DungeonRun{ZoneID: ZoneOssuaryAscendant, VisitedNodes: []string{pre + "f1b2", pre + "f2b2", pre + "cap32"}}
	clean := &CombatSession{}
	if seedBossRunStatuses(clean, "boss_valdris_ascendant", 560, full) {
		t.Error("full-clear seed returned true, want false (no rebirths)")
	}
	if clean.Statuses.EnemyReviveCharges != 0 {
		t.Errorf("full-clear seeded charges = %d, want 0", clean.Statuses.EnemyReviveCharges)
	}

	// Non-hooked boss: no-op even on a skip route.
	other := &CombatSession{}
	if seedBossRunStatuses(other, "boss_seamstress", 560, skip) {
		t.Error("non-hooked boss seed returned true, want false")
	}
}

// TestEnemyDown_PhylacteryRebirth exercises the engine primitive: each seeded
// charge revives the boss to the revive pool once, and the fight ends only when
// the last charge is spent.
func TestEnemyDown_PhylacteryRebirth(t *testing.T) {
	player := &Combatant{Name: "Hero", IsPlayer: true, Stats: CombatStats{MaxHP: 100, AC: 15, Attack: 10}}
	seat0 := newActor(player)
	st := &combatState{actor: seat0, actors: []*actor{seat0}, enemyReviveCharges: 2, enemyReviveHP: 140}

	// First lethal blow: a charge spends, boss revives to the pool, not down.
	st.enemyHP = 0
	if enemyDown(st, "test") {
		t.Fatal("first killing blow reported down, want rebirth")
	}
	if st.enemyHP != 140 || st.enemyReviveCharges != 1 {
		t.Errorf("after 1st rebirth: HP=%d charges=%d, want 140/1", st.enemyHP, st.enemyReviveCharges)
	}

	// Second lethal blow: last charge spends, revives again.
	st.enemyHP = -5
	if enemyDown(st, "test") {
		t.Fatal("second killing blow reported down, want rebirth")
	}
	if st.enemyHP != 140 || st.enemyReviveCharges != 0 {
		t.Errorf("after 2nd rebirth: HP=%d charges=%d, want 140/0", st.enemyHP, st.enemyReviveCharges)
	}

	// Third lethal blow: no charges left, the lich stays dead.
	st.enemyHP = 0
	if !enemyDown(st, "test") {
		t.Error("charge-less killing blow reported alive, want down")
	}

	// A rebirth event was emitted per revival (two), for the narrator.
	rebirths := 0
	for _, e := range st.events {
		if e.Action == "phylactery_rebirth" {
			rebirths++
		}
	}
	if rebirths != 2 {
		t.Errorf("phylactery_rebirth events = %d, want 2", rebirths)
	}
}

func approxEq(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}

// custodianState builds a live combatState seated against a Custodian-shaped
// enemy (MaxHP given), at the given round and current enemy HP, for direct
// applyAmendment tests. Uses the real turn engine so the embedded actor/roster
// are fully formed.
func custodianState(t *testing.T, round, enemyHP, enemyMaxHP int) *combatState {
	t.Helper()
	sess := turnSession(CombatPhaseRoundEnd, 10000, enemyHP)
	sess.Round = round
	sess.EnemyID = "boss_custodian"
	p := basePlayer()
	e := baseEnemy()
	e.Stats.MaxHP = enemyMaxHP
	te := resumeTurnEngine(sess, []*Combatant{&p}, &e, combatSessionStepRNG(sess, enemySeat))
	te.st.enemyHP = enemyHP
	return te.st
}

func countEvents(events []CombatEvent, action string) int {
	n := 0
	for _, ev := range events {
		if ev.Action == action {
			n++
		}
	}
	return n
}

func TestApplyAmendment_SnapshotAtRoundThree(t *testing.T) {
	// Before round 3: no snapshot.
	st := custodianState(t, 2, 400, 500)
	applyAmendment(st, 500)
	if st.enemyRewindHP != 0 {
		t.Errorf("round 2 snapshot = %d, want 0 (not yet)", st.enemyRewindHP)
	}
	// End of round 3: snapshot the current HP.
	st = custodianState(t, 3, 380, 500)
	applyAmendment(st, 500)
	if st.enemyRewindHP != 380 {
		t.Errorf("round 3 snapshot = %d, want 380", st.enemyRewindHP)
	}
	// A later round does not re-snapshot over the captured value.
	st.round = 5
	st.enemyHP = 200
	applyAmendment(st, 500)
	if st.enemyRewindHP != 380 {
		t.Errorf("post-capture snapshot = %d, want it frozen at 380", st.enemyRewindHP)
	}
}

func TestApplyAmendment_RewindOnceAtPhaseTwo(t *testing.T) {
	// Snapshot 400; boss now at 200, below the 0.45*500=225 phase-two line.
	st := custodianState(t, 6, 200, 500)
	st.enemyRewindHP = 400
	applyAmendment(st, 500)
	if st.enemyHP != 400 {
		t.Errorf("post-rewind HP = %d, want restored to snapshot 400", st.enemyHP)
	}
	if !st.enemyRewindUsed {
		t.Error("rewind did not mark itself used")
	}
	if countEvents(st.events, "amendment_rewind") != 1 {
		t.Errorf("amendment_rewind events = %d, want 1", countEvents(st.events, "amendment_rewind"))
	}
	// A second crossing does not rewind again.
	st.enemyHP = 150
	mark := len(st.events)
	applyAmendment(st, 500)
	if st.enemyHP != 150 {
		t.Errorf("second-crossing HP = %d, want left at 150 (rewind spent)", st.enemyHP)
	}
	if len(st.events) != mark {
		t.Error("a spent rewind emitted another event")
	}
}

func TestApplyAmendment_NoRewindAbovePhaseTwo(t *testing.T) {
	// Boss above the phase-two line: no rewind even with a snapshot in hand.
	st := custodianState(t, 6, 300, 500) // 300 > 225
	st.enemyRewindHP = 450
	applyAmendment(st, 500)
	if st.enemyHP != 300 || st.enemyRewindUsed {
		t.Errorf("HP=%d used=%v; want no rewind above phase two", st.enemyHP, st.enemyRewindUsed)
	}
}

func TestApplyAmendment_NoRewindWithoutSnapshot(t *testing.T) {
	// Bursted into phase two before round 3: no snapshot, so no free heal.
	st := custodianState(t, 2, 100, 500)
	applyAmendment(st, 500)
	if st.enemyHP != 100 || st.enemyRewindUsed {
		t.Errorf("HP=%d used=%v; want no rewind without a snapshot", st.enemyHP, st.enemyRewindUsed)
	}
}

func TestApplyAmendment_MidnightTimer(t *testing.T) {
	// Before round 20: no attack climb.
	st := custodianState(t, 19, 300, 500)
	applyAmendment(st, 500)
	if st.enemyAtkBuff != 0 {
		t.Errorf("round 19 atk buff = %d, want 0", st.enemyAtkBuff)
	}
	// Round 20+: +2 per round, capped.
	st = custodianState(t, 20, 300, 500)
	applyAmendment(st, 500)
	if st.enemyAtkBuff != midnightAtkStep {
		t.Errorf("round 20 atk buff = %d, want %d", st.enemyAtkBuff, midnightAtkStep)
	}
	if countEvents(st.events, "midnight_toll") != 1 {
		t.Errorf("midnight_toll events = %d, want 1", countEvents(st.events, "midnight_toll"))
	}
	// The cap holds against a truly endless stall.
	st.enemyAtkBuff = midnightAtkCap
	mark := len(st.events)
	applyAmendment(st, 500)
	if st.enemyAtkBuff != midnightAtkCap {
		t.Errorf("capped atk buff = %d, want %d", st.enemyAtkBuff, midnightAtkCap)
	}
	if len(st.events) != mark {
		t.Error("midnight timer emitted a toll after hitting the cap")
	}
}

func TestApplyBossInCombatRoundEnd_DispatchesOnlyCustodian(t *testing.T) {
	// A non-Custodian enemy is untouched even at a rewind-eligible HP.
	st := custodianState(t, 6, 200, 500)
	st.enemyRewindHP = 400
	applyBossInCombatRoundEnd(st, "boss_seraphel", 500)
	if st.enemyHP != 200 || st.enemyRewindUsed {
		t.Errorf("HP=%d used=%v; want no-op for a non-Custodian boss", st.enemyHP, st.enemyRewindUsed)
	}
	// The Custodian id does resolve the hook.
	applyBossInCombatRoundEnd(st, "boss_custodian", 500)
	if st.enemyHP != 400 || !st.enemyRewindUsed {
		t.Errorf("HP=%d used=%v; want the rewind for the Custodian", st.enemyHP, st.enemyRewindUsed)
	}
}

// ── Inversion Stitch (Seamstress) ────────────────────────────────────────────

// seamstressState builds a fully-formed combatState (embedded actor + roster) so
// the event helpers that read st.playerHP resolve — a bare struct-literal state
// has a nil actor cursor.
func seamstressState(t *testing.T, enemyHP, enemyMaxHP int) *combatState {
	t.Helper()
	sess := turnSession(CombatPhaseRoundEnd, 10000, enemyHP)
	sess.EnemyID = "boss_seamstress"
	p := basePlayer()
	e := baseEnemy()
	e.Stats.MaxHP = enemyMaxHP
	te := resumeTurnEngine(sess, []*Combatant{&p}, &e, combatSessionStepRNG(sess, enemySeat))
	te.st.enemyHP = enemyHP
	return te.st
}

func TestApplyInversionStitch_NoOpAbovePhaseTwo(t *testing.T) {
	// Above the 0.35*500=175 phase-two line the room stays right-side-out.
	st := seamstressState(t, 300, 500)
	applyInversionStitch(st, 500)
	if st.inversionTelegraph || st.inversionActive != 0 || len(st.events) != 0 {
		t.Errorf("above phase two: telegraph=%v active=%d events=%d, want all zero",
			st.inversionTelegraph, st.inversionActive, len(st.events))
	}
}

func TestApplyInversionStitch_Cadence(t *testing.T) {
	// Below the phase-two line the room cycles warn(1) → sting(pulse) → warn(1)…
	st := seamstressState(t, 150, 500) // 150 < 0.35*500 = 175

	// Round end 1: no pulse yet, so telegraph the first one.
	applyInversionStitch(st, 500)
	if !st.inversionTelegraph || st.inversionActive != 0 {
		t.Fatalf("after warn: telegraph=%v active=%d, want telegraph=true active=0",
			st.inversionTelegraph, st.inversionActive)
	}
	if countEvents(st.events, "inversion_telegraph") != 1 {
		t.Fatalf("telegraph events = %d, want 1", countEvents(st.events, "inversion_telegraph"))
	}

	// Round end 2: the telegraphed pulse activates for its full duration.
	applyInversionStitch(st, 500)
	if st.inversionActive != inversionPulseRounds || st.inversionTelegraph {
		t.Fatalf("after activate: active=%d telegraph=%v, want active=%d telegraph=false",
			st.inversionActive, st.inversionTelegraph, inversionPulseRounds)
	}
	if countEvents(st.events, "inversion_stitch") != 1 {
		t.Fatalf("stitch events = %d, want 1", countEvents(st.events, "inversion_stitch"))
	}

	// The pulse counts down one round per round end, staying active until it lapses.
	for r := inversionPulseRounds - 1; r >= 1; r-- {
		applyInversionStitch(st, 500)
		if st.inversionActive != r {
			t.Fatalf("mid-pulse active = %d, want %d", st.inversionActive, r)
		}
	}

	// The round the pulse lapses (active 1→0) a fresh warn is scheduled in the
	// same round end — the repeating rhythm, never two silent rounds in a row.
	applyInversionStitch(st, 500)
	if st.inversionActive != 0 || !st.inversionTelegraph {
		t.Fatalf("after pulse: active=%d telegraph=%v, want active=0 telegraph=true",
			st.inversionActive, st.inversionTelegraph)
	}
	if countEvents(st.events, "inversion_telegraph") != 2 {
		t.Fatalf("telegraph events = %d, want 2 (the next pulse warned)",
			countEvents(st.events, "inversion_telegraph"))
	}
}

func TestApplyBossInCombatRoundEnd_DispatchesSeamstress(t *testing.T) {
	// A non-Seamstress boss at a phase-two HP is untouched.
	st := seamstressState(t, 150, 500)
	applyBossInCombatRoundEnd(st, "boss_aurvandryx", 500)
	if st.inversionTelegraph || len(st.events) != 0 {
		t.Errorf("non-Seamstress boss triggered inversion (telegraph=%v events=%d)",
			st.inversionTelegraph, len(st.events))
	}
	// The Seamstress id resolves the hook.
	applyBossInCombatRoundEnd(st, "boss_seamstress", 500)
	if !st.inversionTelegraph {
		t.Error("boss_seamstress did not schedule the inversion telegraph")
	}
}

// TestInversionStitch_HealsSting drives a real round with an active pulse seeded
// and confirms both the self-heal and the ally-heal land as wounds, floored at 1.
func TestInversionStitch_HealsSting(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}

	t.Run("ally heal stings the friend", func(t *testing.T) {
		sess := startAllyHealFight(t, p, 50) // friend hurt to 50/100
		sess.Statuses.InversionActive = 1    // room is inside-out this round
		healer, friend := basePlayer(), basePlayer()
		ct := &combatTurn{sess: sess, players: []*Combatant{&healer, &friend},
			enemy: &Combatant{Name: "dummy", Stats: CombatStats{MaxHP: 800, AC: 10, Attack: 1, AttackBonus: 1}},
			seat:  0, uid: healerID(strings.ReplaceAll(t.Name(), "/", "_"))}
		before := sess.seatHP(1)

		events, err := p.driveCombatRound(ct, PlayerAction{Kind: ActionCast,
			Effect: &turnActionEffect{Label: "Cure", Action: "spell_cast", AllyHeal: 30, AllySeat: 1}})
		if err != nil {
			t.Fatal(err)
		}
		if got := sess.seatHP(1); got >= before {
			t.Errorf("friend HP = %d (was %d) — an inverted heal must wound, not mend", got, before)
		}
		if countEvents(events, "heal_inverted") != 1 {
			t.Errorf("heal_inverted events = %d, want 1", countEvents(events, "heal_inverted"))
		}
	})

	t.Run("self heal stings the acting seat, floored at 1", func(t *testing.T) {
		// A self-heal (PlayerHeal, no AllySeat) lands on whichever seat is acting;
		// assert the inversion fired (heal_inverted) and no seat was driven to 0 —
		// the sting denies sustain, it never kills.
		sess := startAllyHealFight(t, p, 100)
		sess.Statuses.InversionActive = 1
		healer, friend := basePlayer(), basePlayer()
		ct := &combatTurn{sess: sess, players: []*Combatant{&healer, &friend},
			enemy: &Combatant{Name: "dummy", Stats: CombatStats{MaxHP: 800, AC: 10, Attack: 1, AttackBonus: 1}},
			seat:  0, uid: healerID(strings.ReplaceAll(t.Name(), "/", "_"))}

		events, err := p.driveCombatRound(ct, PlayerAction{Kind: ActionCast,
			Effect: &turnActionEffect{Label: "Cure Self", Action: "spell_cast", PlayerHeal: 30}})
		if err != nil {
			t.Fatal(err)
		}
		if countEvents(events, "heal_inverted") != 1 {
			t.Errorf("heal_inverted events = %d, want 1 — the self-heal did not invert", countEvents(events, "heal_inverted"))
		}
		for seat := 0; seat < 2; seat++ {
			if got := sess.seatHP(seat); got < 1 {
				t.Errorf("seat %d HP = %d — the sting must floor at 1, never kill", seat, got)
			}
		}
	})
}

func TestTurnEngine_AmendmentRoundTrips(t *testing.T) {
	// Drive a real Custodian round-end and confirm the once-only rewind is
	// captured through CombatStatuses (a suspend/resume can't replay it).
	sess := turnSession(CombatPhaseRoundEnd, 10000, 200)
	sess.Round = 6
	sess.EnemyID = "boss_custodian"
	sess.EnemyHPMax = 500
	sess.Statuses.EnemyRewindHP = 400 // snapshot taken earlier in the fight
	p := basePlayer()
	e := baseEnemy()
	e.Stats.MaxHP = 500
	te := resumeTurnEngine(sess, []*Combatant{&p}, &e, combatSessionStepRNG(sess, enemySeat))
	if _, err := te.step(PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	te.commit()

	if sess.EnemyHP != 400 {
		t.Errorf("committed EnemyHP = %d, want the 400 rewind pool", sess.EnemyHP)
	}
	if !sess.Statuses.EnemyRewindUsed {
		t.Error("EnemyRewindUsed did not persist through commit")
	}
	if countEvents(sess.TurnLog, "amendment_rewind") != 1 {
		t.Errorf("amendment_rewind events = %d, want 1", countEvents(sess.TurnLog, "amendment_rewind"))
	}
}

func TestTurnEngine_InversionRoundTrips(t *testing.T) {
	// Drive a real Seamstress round-end in phase 2 and confirm the scheduled
	// telegraph persists through CombatStatuses (a suspend/resume keeps the cadence).
	sess := turnSession(CombatPhaseRoundEnd, 10000, 150) // 150 < 0.35*500 = 175
	sess.EnemyID = "boss_seamstress"
	sess.EnemyHPMax = 500
	p := basePlayer()
	e := baseEnemy()
	e.Stats.MaxHP = 500
	te := resumeTurnEngine(sess, []*Combatant{&p}, &e, combatSessionStepRNG(sess, enemySeat))
	if _, err := te.step(PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	te.commit()

	if !sess.Statuses.InversionTelegraph {
		t.Error("InversionTelegraph did not persist through commit")
	}
	if countEvents(sess.TurnLog, "inversion_telegraph") != 1 {
		t.Errorf("inversion_telegraph events = %d, want 1", countEvents(sess.TurnLog, "inversion_telegraph"))
	}
}
