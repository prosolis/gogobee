package plugin

import (
	"math/rand/v2"
	"testing"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

func cleanupCombatSessions(uid id.UserID) {
	_, _ = db.Get().Exec(`DELETE FROM combat_session WHERE user_id = ?`, string(uid))
}

// ── Persistence layer ──────────────────────────────────────────────────────

func TestStartCombatSession_RoundTrip(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@combat-roundtrip:example.org")
	defer cleanupCombatSessions(uid)

	s, err := startCombatSession(uid, "run-1", "node-7", "owlbear", 80, 80, 120, 120)
	if err != nil {
		t.Fatalf("startCombatSession: %v", err)
	}
	if s.Status != CombatStatusActive || s.Phase != CombatPhasePlayerTurn {
		t.Errorf("fresh session: status=%q phase=%q", s.Status, s.Phase)
	}
	if s.Round != 1 {
		t.Errorf("round = %d, want 1", s.Round)
	}

	got, err := getActiveCombatSession(uid)
	if err != nil || got == nil {
		t.Fatalf("getActiveCombatSession: %v / %v", got, err)
	}
	if got.SessionID != s.SessionID {
		t.Errorf("id mismatch: %q vs %q", got.SessionID, s.SessionID)
	}
	if got.RunID != "run-1" || got.EncounterID != "node-7" || got.EnemyID != "owlbear" {
		t.Errorf("identity round-trip wrong: %+v", got)
	}
	if got.PlayerHP != 80 || got.PlayerHPMax != 80 || got.EnemyHP != 120 || got.EnemyHPMax != 120 {
		t.Errorf("hp round-trip wrong: %+v", got)
	}
	if len(got.TurnLog) != 0 {
		t.Errorf("expected empty turn log, got %v", got.TurnLog)
	}

	// Mutable-field persistence.
	got.Round = 3
	got.Phase = CombatPhaseRoundEnd
	got.PlayerHP = 40
	got.EnemyHP = 15
	got.Statuses = CombatStatuses{ActorStatuses: ActorStatuses{PoisonTicks: 2, PoisonDmg: 5}, Enraged: true}
	got.TurnLog = append(got.TurnLog, CombatEvent{Round: 3, Actor: "player", Action: "hit", Damage: 9})
	if err := saveCombatSession(got); err != nil {
		t.Fatalf("saveCombatSession: %v", err)
	}
	reloaded, err := getCombatSession(s.SessionID)
	if err != nil || reloaded == nil {
		t.Fatalf("getCombatSession: %v / %v", reloaded, err)
	}
	if reloaded.Round != 3 || reloaded.Phase != CombatPhaseRoundEnd {
		t.Errorf("round/phase not persisted: %+v", reloaded)
	}
	if reloaded.PlayerHP != 40 || reloaded.EnemyHP != 15 {
		t.Errorf("hp not persisted: %+v", reloaded)
	}
	if reloaded.Statuses != (CombatStatuses{ActorStatuses: ActorStatuses{PoisonTicks: 2, PoisonDmg: 5}, Enraged: true}) {
		t.Errorf("statuses not persisted: %+v", reloaded.Statuses)
	}
	if len(reloaded.TurnLog) != 1 || reloaded.TurnLog[0].Action != "hit" {
		t.Errorf("turn log not persisted: %+v", reloaded.TurnLog)
	}
}

func TestStartCombatSession_RejectsConcurrent(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@combat-concur:example.org")
	defer cleanupCombatSessions(uid)

	if _, err := startCombatSession(uid, "r", "n", "rat", 50, 50, 30, 30); err != nil {
		t.Fatal(err)
	}
	if _, err := startCombatSession(uid, "r", "n", "wolf", 50, 50, 30, 30); err != ErrCombatSessionAlreadyActive {
		t.Errorf("err = %v, want ErrCombatSessionAlreadyActive", err)
	}
}

func TestListExpiredCombatSessions(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@combat-sweep:example.org")
	fresh := id.UserID("@combat-fresh:example.org")
	defer cleanupCombatSessions(uid)
	defer cleanupCombatSessions(fresh)

	s, err := startCombatSession(uid, "r", "n", "boss", 60, 60, 200, 200)
	if err != nil {
		t.Fatal(err)
	}
	// A second, non-stale session should never show up in the list.
	if _, err := startCombatSession(fresh, "r2", "n2", "rat", 40, 40, 20, 20); err != nil {
		t.Fatal(err)
	}
	// Backdate expiry so the reaper considers the first one stale.
	if _, err := db.Get().Exec(
		`UPDATE combat_session SET expires_at = datetime('now', '-1 hour') WHERE session_id = ?`,
		s.SessionID); err != nil {
		t.Fatal(err)
	}

	expired, err := listExpiredCombatSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("expired count = %d, want 1", len(expired))
	}
	if expired[0].SessionID != s.SessionID {
		t.Errorf("expired[0] = %q, want %q", expired[0].SessionID, s.SessionID)
	}

	// markCombatSessionExpired is the non-auto-play fallback path.
	if err := markCombatSessionExpired(s.SessionID); err != nil {
		t.Fatalf("mark expired: %v", err)
	}
	if active, _ := getActiveCombatSession(uid); active != nil {
		t.Errorf("expected no active session after mark, got %+v", active)
	}
	reaped, _ := getCombatSession(s.SessionID)
	if reaped.Status != CombatStatusExpired || reaped.Phase != CombatPhaseOver {
		t.Errorf("reaped session: status=%q phase=%q", reaped.Status, reaped.Phase)
	}
	// The non-stale session is untouched.
	if again, _ := listExpiredCombatSessions(); len(again) != 0 {
		t.Errorf("expected 0 expired after mark, got %d", len(again))
	}
}

func TestHasActiveCombatSession(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@combat-guard:example.org")
	defer cleanupCombatSessions(uid)

	if hasActiveCombatSession(uid) {
		t.Fatal("no session yet, want false")
	}

	s, err := startCombatSession(uid, "r", "n", "boss", 60, 60, 120, 120)
	if err != nil {
		t.Fatal(err)
	}
	if !hasActiveCombatSession(uid) {
		t.Error("active session, want true")
	}

	// A terminal session no longer counts as active.
	s.Status = CombatStatusWon
	s.Phase = CombatPhaseOver
	if err := saveCombatSession(s); err != nil {
		t.Fatal(err)
	}
	if hasActiveCombatSession(uid) {
		t.Error("session is won, want false")
	}
}

// ── State machine ──────────────────────────────────────────────────────────

// turnSession builds an in-memory CombatSession for state-machine tests that
// don't touch the DB.
func turnSession(phase string, playerHP, enemyHP int) *CombatSession {
	return &CombatSession{
		SessionID: "test-session", UserID: "@t:x", Round: 1, Phase: phase,
		PlayerHP: playerHP, PlayerHPMax: playerHP,
		EnemyHP: enemyHP, EnemyHPMax: enemyHP,
		TurnLog: []CombatEvent{}, Status: CombatStatusActive,
	}
}

func stepEngine(sess *CombatSession, player, enemy *Combatant, action PlayerAction) ([]CombatEvent, error) {
	rng := rand.New(rand.NewPCG(42, phaseOrdinal(sess.Phase)))
	te := resumeTurnEngine(sess, []*Combatant{player}, enemy, rng)
	events, err := te.step(action)
	if err != nil {
		return nil, err
	}
	te.commit()
	return events, nil
}

func TestTurnEngine_PhaseProgression(t *testing.T) {
	// Pools large enough that no single phase can end the fight.
	sess := turnSession(CombatPhasePlayerTurn, 10000, 10000)
	player, enemy := basePlayer(), baseEnemy()

	if _, err := stepEngine(sess, &player, &enemy, PlayerAction{Kind: ActionAttack}); err != nil {
		t.Fatal(err)
	}
	if sess.Phase != CombatPhaseEnemyTurn {
		t.Fatalf("after player_turn: phase=%q, want enemy_turn", sess.Phase)
	}
	if _, err := stepEngine(sess, &player, &enemy, PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	if sess.Phase != CombatPhaseRoundEnd {
		t.Fatalf("after enemy_turn: phase=%q, want round_end", sess.Phase)
	}
	if _, err := stepEngine(sess, &player, &enemy, PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	if sess.Phase != CombatPhasePlayerTurn {
		t.Fatalf("after round_end: phase=%q, want player_turn", sess.Phase)
	}
	if sess.Round != 2 {
		t.Errorf("round = %d after a full cycle, want 2", sess.Round)
	}
	if sess.Status != CombatStatusActive {
		t.Errorf("status = %q, want active", sess.Status)
	}
}

func TestTurnEngine_PoisonTickAtRoundEnd(t *testing.T) {
	sess := turnSession(CombatPhaseRoundEnd, 50, 80)
	sess.Statuses = CombatStatuses{ActorStatuses: ActorStatuses{PoisonTicks: 2, PoisonDmg: 7}}
	player, enemy := basePlayer(), baseEnemy()

	events, err := stepEngine(sess, &player, &enemy, PlayerAction{})
	if err != nil {
		t.Fatal(err)
	}
	if sess.PlayerHP != 43 {
		t.Errorf("player_hp = %d, want 43 (50 - 7 poison)", sess.PlayerHP)
	}
	if sess.Statuses.PoisonTicks != 1 {
		t.Errorf("poison_ticks = %d, want 1", sess.Statuses.PoisonTicks)
	}
	if sess.Round != 2 || sess.Phase != CombatPhasePlayerTurn {
		t.Errorf("round=%d phase=%q, want 2/player_turn", sess.Round, sess.Phase)
	}
	if len(events) != 1 || events[0].Action != "poison_tick" || events[0].Damage != 7 {
		t.Errorf("expected one poison_tick event, got %+v", events)
	}
}

func TestTurnEngine_PoisonTickCanBeLethal(t *testing.T) {
	sess := turnSession(CombatPhaseRoundEnd, 4, 80)
	sess.Statuses = CombatStatuses{ActorStatuses: ActorStatuses{PoisonTicks: 1, PoisonDmg: 9}}
	player, enemy := basePlayer(), baseEnemy()

	if _, err := stepEngine(sess, &player, &enemy, PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	if sess.Status != CombatStatusLost {
		t.Errorf("status = %q, want lost (poison dropped player to 0)", sess.Status)
	}
	if sess.Phase != CombatPhaseOver {
		t.Errorf("phase = %q, want over", sess.Phase)
	}
	if sess.PlayerHP != 0 {
		t.Errorf("player_hp = %d, want 0", sess.PlayerHP)
	}
}

func TestTurnEngine_ConcentrationTickAtRoundEnd(t *testing.T) {
	sess := turnSession(CombatPhaseRoundEnd, 50, 80)
	sess.Statuses = CombatStatuses{ActorStatuses: ActorStatuses{ConcentrationDmg: 12}}
	player, enemy := basePlayer(), baseEnemy()

	events, err := stepEngine(sess, &player, &enemy, PlayerAction{})
	if err != nil {
		t.Fatal(err)
	}
	if sess.EnemyHP != 68 {
		t.Errorf("enemy_hp = %d, want 68 (80 - 12 aura)", sess.EnemyHP)
	}
	if sess.Statuses.ConcentrationDmg != 12 {
		t.Errorf("concentration_dmg = %d, want 12 (aura persists)", sess.Statuses.ConcentrationDmg)
	}
	if len(events) != 1 || events[0].Action != "concentration_tick" || events[0].Damage != 12 {
		t.Errorf("expected one concentration_tick event, got %+v", events)
	}
	if sess.Round != 2 || sess.Phase != CombatPhasePlayerTurn {
		t.Errorf("round=%d phase=%q, want 2/player_turn", sess.Round, sess.Phase)
	}
}

func TestTurnEngine_ConcentrationTickCanBeLethal(t *testing.T) {
	sess := turnSession(CombatPhaseRoundEnd, 50, 9)
	sess.Statuses = CombatStatuses{ActorStatuses: ActorStatuses{ConcentrationDmg: 12}}
	player, enemy := basePlayer(), baseEnemy()

	if _, err := stepEngine(sess, &player, &enemy, PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	if sess.Status != CombatStatusWon || sess.Phase != CombatPhaseOver {
		t.Errorf("status=%q phase=%q, want won/over (aura dropped enemy)", sess.Status, sess.Phase)
	}
	if sess.EnemyHP != 0 {
		t.Errorf("enemy_hp = %d, want 0", sess.EnemyHP)
	}
}

// A concentration damage cast lands its burst this round AND arms the aura so
// it re-ticks at every subsequent round_end.
func TestTurnEngine_ConcentrationCastArmsAura(t *testing.T) {
	sess := turnSession(CombatPhasePlayerTurn, 50, 200)
	player, enemy := basePlayer(), baseEnemy()

	eff := &turnActionEffect{
		Label: "Spirit Guardians", Action: "spell_cast",
		EnemyDamage: 15, ConcentrationDmg: 15,
	}
	if _, err := stepEngine(sess, &player, &enemy, PlayerAction{Kind: ActionCast, Effect: eff}); err != nil {
		t.Fatal(err)
	}
	if sess.EnemyHP != 185 {
		t.Errorf("enemy_hp = %d, want 185 (200 - 15 burst)", sess.EnemyHP)
	}
	if sess.Statuses.ConcentrationDmg != 15 {
		t.Fatalf("concentration_dmg = %d, want 15 (aura armed)", sess.Statuses.ConcentrationDmg)
	}
	// Drive to round_end and confirm the aura bites again.
	sess.Phase = CombatPhaseRoundEnd
	if _, err := stepEngine(sess, &player, &enemy, PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	if sess.EnemyHP != 170 {
		t.Errorf("enemy_hp = %d, want 170 (185 - 15 aura tick)", sess.EnemyHP)
	}
}

func TestTurnEngine_Flee(t *testing.T) {
	sess := turnSession(CombatPhasePlayerTurn, 50, 50)
	player, enemy := basePlayer(), baseEnemy()

	events, err := stepEngine(sess, &player, &enemy, PlayerAction{Kind: ActionFlee})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Status != CombatStatusFled || sess.Phase != CombatPhaseOver {
		t.Errorf("after flee: status=%q phase=%q", sess.Status, sess.Phase)
	}
	if len(events) != 1 || events[0].Action != "flee" {
		t.Errorf("expected one flee event, got %+v", events)
	}
}

func TestTurnEngine_PlayerWinsWhenEnemyDrops(t *testing.T) {
	// Enemy on 1 HP: any connecting player hit ends it. Drive the fight to a
	// terminal state and confirm the player wins (strong attacker vs. 1 HP).
	sess := turnSession(CombatPhasePlayerTurn, 100, 1)
	player, enemy := basePlayer(), baseEnemy()

	for i := 0; i < 12 && sess.Status == CombatStatusActive; i++ {
		if _, err := stepEngine(sess, &player, &enemy, PlayerAction{Kind: ActionAttack}); err != nil {
			t.Fatal(err)
		}
	}
	if sess.Status != CombatStatusWon {
		t.Fatalf("status = %q, want won", sess.Status)
	}
	if sess.Phase != CombatPhaseOver {
		t.Errorf("phase = %q, want over", sess.Phase)
	}
	if sess.EnemyHP > 0 {
		t.Errorf("enemy_hp = %d, want 0", sess.EnemyHP)
	}
}

func TestTurnEngine_StepRejectsTerminalSession(t *testing.T) {
	sess := turnSession(CombatPhaseOver, 50, 50)
	sess.Status = CombatStatusWon
	player, enemy := basePlayer(), baseEnemy()

	rng := rand.New(rand.NewPCG(1, 1))
	te := resumeTurnEngine(sess, []*Combatant{&player}, &enemy, rng)
	if _, err := te.step(PlayerAction{Kind: ActionAttack}); err != errCombatSessionOver {
		t.Errorf("err = %v, want errCombatSessionOver", err)
	}
}

// ── Fight-scoped buff persistence ──────────────────────────────────────────

func TestDiffTurnBuff(t *testing.T) {
	bs := CombatStats{AC: 14, AttackBonus: 3, MaxHP: 50, Speed: 10, CritRate: 0.05}
	bm := CombatModifiers{DamageBonus: 0.1, DamageReduct: 1.0}

	// A buff that bumps AC, grants ward charges, reduces damage taken, and
	// raises max HP (Aid-style → collapses to a heal).
	as, am := bs, bm
	as.AC = 16
	as.MaxHP = 55
	am.WardCharges = 2
	am.DamageReduct = 0.8
	d := diffTurnBuff(bs, as, bm, am)
	if d.dAC != 2 || d.ward != 2 || d.heal != 5 {
		t.Errorf("delta = %+v, want dAC 2 / ward 2 / heal 5", d)
	}
	if d.dReductMul < 0.79 || d.dReductMul > 0.81 {
		t.Errorf("dReductMul = %v, want ~0.8", d.dReductMul)
	}
	if !d.statComponent() || !d.any() {
		t.Errorf("buff with AC + reduct should report statComponent and any")
	}

	// A no-op buff (utility spell with no combat hook) diffs to nothing.
	none := diffTurnBuff(bs, bs, bm, bm)
	if none.statComponent() || none.any() {
		t.Errorf("no-op buff should report neither statComponent nor any: %+v", none)
	}
}

func TestApplyBuffDelta(t *testing.T) {
	var s CombatStatuses
	s.applyBuffDelta(turnBuffDelta{dAC: 2, ward: 1, dReductMul: 0.8, autoCrit: true})
	if s.BuffACBonus != 2 || s.WardCharges != 1 || !s.AutoCritFirst {
		t.Errorf("first delta not folded in: %+v", s)
	}
	if s.BuffDamageReductMul < 0.79 || s.BuffDamageReductMul > 0.81 {
		t.Errorf("BuffDamageReductMul = %v, want ~0.8", s.BuffDamageReductMul)
	}
	// A second buff stacks: deltas accumulate, reduction multipliers compound.
	s.applyBuffDelta(turnBuffDelta{dAC: 1, dReductMul: 0.5})
	if s.BuffACBonus != 3 {
		t.Errorf("BuffACBonus = %d, want 3 after stacking", s.BuffACBonus)
	}
	if s.BuffDamageReductMul < 0.39 || s.BuffDamageReductMul > 0.41 {
		t.Errorf("BuffDamageReductMul = %v, want ~0.4 (0.8 * 0.5)", s.BuffDamageReductMul)
	}
}

func TestApplySessionBuffs(t *testing.T) {
	player := basePlayer() // AC 0, AttackBonus 0, CritRate 0.05, Speed 10, DamageReduct 1.0
	applySessionBuffs(&player, ActorStatuses{
		BuffACBonus: 3, BuffAtkBonus: 2, BuffSpeedBonus: 5, BuffCritRate: 0.15,
		BuffDamageBonus: 0.25, BuffDamageReductMul: 0.8, BuffPetProc: 0.5, BuffPetDmg: 6,
	})
	if player.Stats.AC != 3 || player.Stats.AttackBonus != 2 || player.Stats.Speed != 15 {
		t.Errorf("stat deltas wrong: %+v", player.Stats)
	}
	if player.Stats.CritRate < 0.19 || player.Stats.CritRate > 0.21 {
		t.Errorf("CritRate = %v, want ~0.20", player.Stats.CritRate)
	}
	if player.Mods.DamageBonus != 0.25 || player.Mods.PetAttackProc != 0.5 || player.Mods.PetAttackDmg != 6 {
		t.Errorf("mod deltas wrong: %+v", player.Mods)
	}
	if player.Mods.DamageReduct < 0.79 || player.Mods.DamageReduct > 0.81 {
		t.Errorf("DamageReduct = %v, want ~0.8 (1.0 * 0.8)", player.Mods.DamageReduct)
	}
}

func TestSeedCombatSessionOneShots(t *testing.T) {
	// Arcane Ward is the one resource normally live at fight start.
	s := &CombatSession{}
	if !seedCombatSessionOneShots(s, CombatSeatSetup{Mods: CombatModifiers{ArcaneWardHP: 15}}) {
		t.Error("seed should report true when Arcane Ward is present")
	}
	if s.Statuses.ArcaneWardHP != 15 {
		t.Errorf("ArcaneWardHP = %d, want 15", s.Statuses.ArcaneWardHP)
	}
	// The armed ability rides along, so a rebuild mid-fight can re-apply it.
	armed := &CombatSession{}
	if !seedCombatSessionOneShots(armed, CombatSeatSetup{ArmedAbility: "rage"}) {
		t.Error("seed should report true when an ability was armed")
	}
	if armed.Statuses.ArmedAbility != "rage" {
		t.Errorf("ArmedAbility = %q, want rage", armed.Statuses.ArmedAbility)
	}
	// Nothing to seed → false, statuses untouched.
	empty := &CombatSession{}
	if seedCombatSessionOneShots(empty, CombatSeatSetup{}) {
		t.Error("seed should report false with no fight-start resources")
	}
}

// TestTurnEngine_OneShotsRoundTrip drives a no-op round_end step and confirms
// every fight-scoped one-shot survives the resume -> combatState -> commit
// cycle. Without that round-trip a Halfling could reroll a nat 1 — or a player
// re-bank a depleted ward charge — on every resumed round.
func TestTurnEngine_OneShotsRoundTrip(t *testing.T) {
	sess := turnSession(CombatPhaseRoundEnd, 50, 50)
	sess.Statuses = CombatStatuses{ActorStatuses: ActorStatuses{
		WardCharges: 3, SporeRounds: 2, ReflectFrac: 0.5, AutoCritFirst: true,
		ArcaneWardHP: 10, HealChargesLeft: 1,
		Raged: true, LuckyUsed: true, DeathSaveUsed: true, PendingRage: true,
		FirstAtkBonusUsed: true, AssassinateReroll: true, AssassinateBonus: true,
		// Buff stat deltas are owned by the command layer — commit must leave
		// them untouched rather than zeroing them on every step.
		BuffACBonus: 2, BuffDamageBonus: 0.15,
	}}
	want := sess.Statuses // round_end with no poison mutates none of these
	player, enemy := basePlayer(), baseEnemy()

	if _, err := stepEngine(sess, &player, &enemy, PlayerAction{}); err != nil {
		t.Fatal(err)
	}
	if sess.Statuses != want {
		t.Errorf("one-shots not round-tripped:\n got  %+v\n want %+v", sess.Statuses, want)
	}
}

// ── Pet proc (per-round) ───────────────────────────────────────────────────

// TestTurnEngine_PetStrike confirms a pet with a guaranteed proc strikes on
// every player-acting turn — the per-round roll model, matching auto-resolve.
func TestTurnEngine_PetStrike(t *testing.T) {
	// Pools huge so no phase can end the fight; isolate the pet behaviour.
	sess := turnSession(CombatPhasePlayerTurn, 100000, 100000)
	player, enemy := basePlayer(), baseEnemy()
	player.Mods.PetAttackProc = 1.0
	player.Mods.PetAttackDmg = 8

	petHitsOnPlayerTurn := func() int {
		events, err := stepEngine(sess, &player, &enemy, PlayerAction{Kind: ActionAttack})
		if err != nil {
			t.Fatal(err)
		}
		hits := 0
		for _, e := range events {
			if e.Action == "pet_attack" {
				hits++
				if e.Damage <= 0 {
					t.Errorf("pet_attack damage = %d, want > 0", e.Damage)
				}
			}
		}
		return hits
	}

	if got := petHitsOnPlayerTurn(); got != 1 {
		t.Fatalf("pet_attack events = %d on first player turn, want 1", got)
	}

	// Cycle back to the next player turn — the pet must strike again.
	for sess.Phase != CombatPhasePlayerTurn {
		if _, err := stepEngine(sess, &player, &enemy, PlayerAction{}); err != nil {
			t.Fatal(err)
		}
	}
	if got := petHitsOnPlayerTurn(); got != 1 {
		t.Errorf("pet_attack events = %d on second player turn, want 1 (per-round)", got)
	}
}

// TestTurnEngine_PetNoProc confirms a player without the pet proc never sees a
// pet strike.
func TestTurnEngine_PetNoProc(t *testing.T) {
	sess := turnSession(CombatPhasePlayerTurn, 100000, 100000)
	player, enemy := basePlayer(), baseEnemy()
	player.Mods.PetAttackProc = 0

	events, err := stepEngine(sess, &player, &enemy, PlayerAction{Kind: ActionAttack})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Action == "pet_attack" {
			t.Error("pet struck with zero PetAttackProc")
		}
	}
}

// TestTurnEngine_PetWhiff confirms a guaranteed whiff makes the enemy's turn a
// total miss — a pet_whiff event fires and the player takes no damage.
func TestTurnEngine_PetWhiff(t *testing.T) {
	sess := turnSession(CombatPhaseEnemyTurn, 100000, 100000)
	player, enemy := basePlayer(), baseEnemy()
	player.Mods.PetWhiffProc = 1.0
	enemy.Stats.AttackBonus = 50 // would connect easily if not whiffed
	startHP := sess.PlayerHP

	events, err := stepEngine(sess, &player, &enemy, PlayerAction{})
	if err != nil {
		t.Fatal(err)
	}
	whiffs := 0
	for _, e := range events {
		if e.Action == "pet_whiff" {
			whiffs++
		}
		if e.Actor == "enemy" && e.Action == "hit" {
			t.Error("enemy landed a hit despite a guaranteed pet whiff")
		}
	}
	if whiffs == 0 {
		t.Error("expected a pet_whiff event with PetWhiffProc 1.0")
	}
	if sess.PlayerHP != startHP {
		t.Errorf("player took %d damage on a whiffed turn, want 0", startHP-sess.PlayerHP)
	}
}

// TestTurnEngine_PetDeflect confirms a guaranteed deflect emits a pet_deflect
// event on a connecting enemy attack.
func TestTurnEngine_PetDeflect(t *testing.T) {
	sess := turnSession(CombatPhaseEnemyTurn, 100000, 100000)
	player, enemy := basePlayer(), baseEnemy()
	player.Mods.PetDeflectProc = 1.0
	enemy.Stats.AttackBonus = 50 // guarantee the swing connects

	events, err := stepEngine(sess, &player, &enemy, PlayerAction{})
	if err != nil {
		t.Fatal(err)
	}
	deflects := 0
	for _, e := range events {
		if e.Action == "pet_deflect" {
			deflects++
		}
	}
	if deflects == 0 {
		t.Error("expected a pet_deflect event with PetDeflectProc 1.0 on a connecting hit")
	}
}

func TestCombatSessionRNG_DeterministicPerPhase(t *testing.T) {
	sess := &CombatSession{SessionID: "abc123", Round: 2, Phase: CombatPhaseEnemyTurn}
	a := combatSessionRNG(sess)
	b := combatSessionRNG(sess)
	if a.Uint64() != b.Uint64() {
		t.Error("same session+round+phase should seed identical streams")
	}
	// A different phase of the same round must draw a distinct stream.
	sess.Phase = CombatPhasePlayerTurn
	c := combatSessionRNG(sess)
	sess.Phase = CombatPhaseEnemyTurn
	d := combatSessionRNG(sess)
	if c.Uint64() == d.Uint64() {
		t.Error("distinct phases of a round should seed distinct streams")
	}
}
