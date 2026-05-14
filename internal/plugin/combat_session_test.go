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
	got.Statuses = CombatStatuses{PoisonTicks: 2, PoisonDmg: 5, Enraged: true}
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
	if reloaded.Statuses != (CombatStatuses{PoisonTicks: 2, PoisonDmg: 5, Enraged: true}) {
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

func TestSweepExpiredCombatSessions(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@combat-sweep:example.org")
	defer cleanupCombatSessions(uid)

	s, err := startCombatSession(uid, "r", "n", "boss", 60, 60, 200, 200)
	if err != nil {
		t.Fatal(err)
	}
	// Backdate expiry so the reaper considers it stale.
	if _, err := db.Get().Exec(
		`UPDATE combat_session SET expires_at = datetime('now', '-1 hour') WHERE session_id = ?`,
		s.SessionID); err != nil {
		t.Fatal(err)
	}
	n, err := sweepExpiredCombatSessions()
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if n < 1 {
		t.Errorf("sweep count = %d, want >= 1", n)
	}
	if active, _ := getActiveCombatSession(uid); active != nil {
		t.Errorf("expected no active session after sweep, got %+v", active)
	}
	reaped, _ := getCombatSession(s.SessionID)
	if reaped.Status != CombatStatusExpired || reaped.Phase != CombatPhaseOver {
		t.Errorf("reaped session: status=%q phase=%q", reaped.Status, reaped.Phase)
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
	te := resumeTurnEngine(sess, player, enemy, rng)
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
	sess.Statuses = CombatStatuses{PoisonTicks: 2, PoisonDmg: 7}
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
	sess.Statuses = CombatStatuses{PoisonTicks: 1, PoisonDmg: 9}
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
	te := resumeTurnEngine(sess, &player, &enemy, rng)
	if _, err := te.step(PlayerAction{Kind: ActionAttack}); err != errCombatSessionOver {
		t.Errorf("err = %v, want errCombatSessionOver", err)
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
