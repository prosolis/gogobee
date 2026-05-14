package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestEncounterIDForRoom(t *testing.T) {
	if got := encounterIDForRoom(0); got != "room0" {
		t.Errorf("encounterIDForRoom(0) = %q, want room0", got)
	}
	if encounterIDForRoom(3) == encounterIDForRoom(4) {
		t.Error("distinct rooms must produce distinct encounter ids")
	}
}

// ── getCombatSessionForEncounter ───────────────────────────────────────────

func TestGetCombatSessionForEncounter(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@combat-enc:example.org")
	defer cleanupCombatSessions(uid)

	if got, err := getCombatSessionForEncounter("run-enc", "room3"); err != nil || got != nil {
		t.Fatalf("unknown encounter: got %v / %v, want nil/nil", got, err)
	}

	s, err := startCombatSession(uid, "run-enc", "room3", "goblin", 40, 40, 12, 12)
	if err != nil {
		t.Fatal(err)
	}
	got, err := getCombatSessionForEncounter("run-enc", "room3")
	if err != nil || got == nil {
		t.Fatalf("getCombatSessionForEncounter: %v / %v", got, err)
	}
	if got.SessionID != s.SessionID {
		t.Errorf("id mismatch: %q vs %q", got.SessionID, s.SessionID)
	}
	if got, _ := getCombatSessionForEncounter("run-enc", "room9"); got != nil {
		t.Errorf("wrong room should not match: %+v", got)
	}

	// A terminal session is still returned — the room resolver relies on this
	// to tell "already won" apart from "not yet fought".
	s.Status = CombatStatusWon
	s.Phase = CombatPhaseOver
	if err := saveCombatSession(s); err != nil {
		t.Fatal(err)
	}
	got, err = getCombatSessionForEncounter("run-enc", "room3")
	if err != nil || got == nil || got.Status != CombatStatusWon {
		t.Errorf("terminal session lookup: %+v / %v", got, err)
	}
}

// ── runCombatRound ─────────────────────────────────────────────────────────

func TestRunCombatRound_FullRoundReturnsToPlayerTurn(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@combat-round:example.org")
	defer cleanupCombatSessions(uid)

	// HP pools large enough that one round can't end the fight.
	sess, err := startCombatSession(uid, "r", "room0", "goblin", 100000, 100000, 100000, 100000)
	if err != nil {
		t.Fatal(err)
	}
	player, enemy := basePlayer(), baseEnemy()

	events, err := runCombatRound(sess, &player, &enemy, PlayerAction{Kind: ActionAttack})
	if err != nil {
		t.Fatalf("runCombatRound: %v", err)
	}
	if !sess.IsActive() {
		t.Fatalf("status = %q, want active after one non-lethal round", sess.Status)
	}
	if sess.Phase != CombatPhasePlayerTurn {
		t.Errorf("phase = %q, want player_turn (round resolved fully)", sess.Phase)
	}
	if sess.Round != 2 {
		t.Errorf("round = %d, want 2 after one full round", sess.Round)
	}
	if len(events) == 0 {
		t.Error("expected the round to produce at least one event")
	}
	// The persisted row should match the in-memory session.
	reloaded, _ := getCombatSession(sess.SessionID)
	if reloaded == nil || reloaded.Round != 2 || reloaded.Phase != CombatPhasePlayerTurn {
		t.Errorf("round not persisted: %+v", reloaded)
	}
}

func TestRunCombatRound_FleeEndsFight(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@combat-flee:example.org")
	defer cleanupCombatSessions(uid)

	sess, err := startCombatSession(uid, "r", "room0", "goblin", 50, 50, 50, 50)
	if err != nil {
		t.Fatal(err)
	}
	player, enemy := basePlayer(), baseEnemy()

	events, err := runCombatRound(sess, &player, &enemy, PlayerAction{Kind: ActionFlee})
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

func TestRunCombatRound_PlayerWinsTerminates(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@combat-win:example.org")
	defer cleanupCombatSessions(uid)

	// Enemy on 1 HP, player with a huge pool: a connecting hit ends it, and
	// the player can't be dropped first. A few rounds covers attack misses.
	sess, err := startCombatSession(uid, "r", "room0", "goblin", 100000, 100000, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	player, enemy := basePlayer(), baseEnemy()

	for i := 0; i < 15 && sess.IsActive(); i++ {
		if _, err := runCombatRound(sess, &player, &enemy, PlayerAction{Kind: ActionAttack}); err != nil {
			t.Fatal(err)
		}
	}
	if sess.Status != CombatStatusWon {
		t.Fatalf("status = %q, want won", sess.Status)
	}
	if sess.Phase != CombatPhaseOver || sess.EnemyHP > 0 {
		t.Errorf("terminal state wrong: phase=%q enemyHP=%d", sess.Phase, sess.EnemyHP)
	}
}

// ── runCombatRound: !cast / !consume turn effects ─────────────────────────

func TestRunCombatRound_ConsumeHealClampsToMax(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@combat-heal:example.org")
	defer cleanupCombatSessions(uid)

	// Player at 10/50, big enemy pool so the round can't end the fight.
	sess, err := startCombatSession(uid, "r", "room0", "goblin", 10, 50, 100000, 100000)
	if err != nil {
		t.Fatal(err)
	}
	player, enemy := basePlayer(), baseEnemy()

	eff := &turnActionEffect{Action: "use_consumable", Label: "Spirit Tonic", PlayerHeal: 1000}
	if _, err := runCombatRound(sess, &player, &enemy, PlayerAction{Kind: ActionConsume, Effect: eff}); err != nil {
		t.Fatal(err)
	}
	// Heal of 1000 must clamp to PlayerHPMax, even after the enemy's swing this
	// round chipped some HP back off.
	if sess.PlayerHP > sess.PlayerHPMax {
		t.Errorf("playerHP = %d exceeds max %d", sess.PlayerHP, sess.PlayerHPMax)
	}
	if sess.PlayerHP <= 10 {
		t.Errorf("playerHP = %d, expected the heal to raise it above the 10 it started at", sess.PlayerHP)
	}
}

func TestRunCombatRound_CastDamageKillsEnemy(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@combat-cast-kill:example.org")
	defer cleanupCombatSessions(uid)

	sess, err := startCombatSession(uid, "r", "room0", "goblin", 100000, 100000, 30, 30)
	if err != nil {
		t.Fatal(err)
	}
	player, enemy := basePlayer(), baseEnemy()

	eff := &turnActionEffect{Action: "spell_cast", Label: "Fireball — 99 dmg", EnemyDamage: 99}
	events, err := runCombatRound(sess, &player, &enemy, PlayerAction{Kind: ActionCast, Effect: eff})
	if err != nil {
		t.Fatal(err)
	}
	if sess.Status != CombatStatusWon || sess.Phase != CombatPhaseOver {
		t.Errorf("after lethal cast: status=%q phase=%q", sess.Status, sess.Phase)
	}
	if sess.EnemyHP > 0 {
		t.Errorf("enemyHP = %d, want 0", sess.EnemyHP)
	}
	if len(events) != 1 || events[0].Action != "spell_cast" {
		t.Errorf("expected a single spell_cast event, got %+v", events)
	}
}

func TestRunCombatRound_CastControlSkipsEnemyTurn(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@combat-control:example.org")
	defer cleanupCombatSessions(uid)

	sess, err := startCombatSession(uid, "r", "room0", "goblin", 200, 200, 100000, 100000)
	if err != nil {
		t.Fatal(err)
	}
	player, enemy := basePlayer(), baseEnemy()
	hpBefore := sess.PlayerHP

	eff := &turnActionEffect{Action: "spell_cast", Label: "Hold Person — controlled", EnemySkip: true}
	events, err := runCombatRound(sess, &player, &enemy, PlayerAction{Kind: ActionCast, Effect: eff})
	if err != nil {
		t.Fatal(err)
	}
	// The enemy forfeits its swing this round, so the player takes no damage.
	if sess.PlayerHP != hpBefore {
		t.Errorf("playerHP = %d, want unchanged %d (enemy was held)", sess.PlayerHP, hpBefore)
	}
	held := false
	for _, ev := range events {
		if ev.Action == "spell_held" {
			held = true
		}
	}
	if !held {
		t.Errorf("expected a spell_held event, got %+v", events)
	}
	// The skip is a one-round effect — it must not persist into the next round.
	if sess.Statuses.EnemySkipNext {
		t.Error("EnemySkipNext should be cleared once the enemy turn consumed it")
	}
	if !sess.IsActive() || sess.Phase != CombatPhasePlayerTurn {
		t.Errorf("fight should continue: status=%q phase=%q", sess.Status, sess.Phase)
	}
}

// ── renderCombatRound ──────────────────────────────────────────────────────

func TestRenderCombatRound(t *testing.T) {
	if got := renderCombatRound(nil, "Hero", "Goblin"); !strings.Contains(got, "without a clean blow") {
		t.Errorf("empty events render = %q", got)
	}
	events := []CombatEvent{
		{Actor: "player", Action: "hit", Damage: 9},
		{Actor: "enemy", Action: "miss"},
		{Actor: "enemy", Action: "poison_tick", Damage: 3},
	}
	got := renderCombatRound(events, "Hero", "Goblin")
	if !strings.Contains(got, "Hero") || !strings.Contains(got, "9") {
		t.Errorf("player hit not rendered: %q", got)
	}
	if !strings.Contains(got, "misses") {
		t.Errorf("enemy miss not rendered: %q", got)
	}
	if !strings.Contains(got, "Poison") {
		t.Errorf("poison tick not rendered: %q", got)
	}
	// Unknown actor/action with no damage renders nothing (not a blank line).
	if line := renderCombatRoundEvent(CombatEvent{Actor: "system", Action: "timeout"}, "Hero", "Goblin"); line != "" {
		t.Errorf("unknown zero-damage event should render empty, got %q", line)
	}
}
