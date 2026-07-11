package plugin

import (
	"strings"
	"testing"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// The hired companion casts.
//
// He could not, and the reason was one line: every spell lookup in the engine is
// keyed on a user id and answered by a dnd_* table, and he has no rows in any of
// them — deliberately, because a sheet on disk is what would turn him into a real
// character everywhere. So the picker's first statement (`LoadDnDCharacter(uid)`)
// came back nil and returned "attack", every turn, for the whole fight.
//
// Role-fill hands a lone martial a Cleric. So the common case of the feature was a
// healer who could not heal, in a party engine that had only just learned to let
// anyone heal anyone (§1). These pin both halves: that he picks the heal, and that
// picking it leaves no trace of him in the database.

// hireForFight seeds an expedition with the companion hired into it, and returns
// the run id his loadout is resolved against.
func hireForFight(t *testing.T, expID string, owner id.UserID, class DnDClass, level int) string {
	t.Helper()
	seedExpedition(t, expID, owner, "active")
	seatLeaderFixture(t, expID)
	if err := hireCompanion(expID, class, level); err != nil {
		t.Fatalf("hireCompanion: %v", err)
	}
	return "run-" + expID
}

// petePartyFight seats a hurt leader and the companion, and returns the turn.
func petePartyFight(t *testing.T, p *AdventurePlugin, runID string, leader id.UserID, leaderHP int) *combatTurn {
	t.Helper()
	monster := dndBestiary["goblin"]
	lead, _, _ := p.companionCombatant(ClassFighter, 8, monster, 2, 0)
	lead.Name = "lead"
	pete, _, _ := p.companionCombatant(ClassCleric, 6, monster, 2, 0)
	pete.Name = companionDisplayName
	enemy := Combatant{Name: "x", Stats: CombatStats{MaxHP: 500, AC: 12, Attack: 4, AttackBonus: 4}}

	sess, err := p.startPartyCombatSession(runID, "enc", "goblin", &enemy, []CombatSeatSetup{
		{UserID: leader, HP: leaderHP, HPMax: 100, Mods: lead.Mods, C: &lead},
		{UserID: companionUserID(), HP: 60, HPMax: 60, Mods: pete.Mods, C: &pete, EngineDriven: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &combatTurn{
		sess: sess, players: []*Combatant{&lead, &pete}, enemy: &enemy,
		seat: 1, uid: companionUserID(),
	}
}

// The headline: a hired Cleric, watching the leader bleed out, casts a heal on him.
// Before the seat-scoped spellbook this returned ("attack", "") — he swung a mace
// at the boss while the man who paid for him died.
func TestCompanionSpells_HiredClericHealsTheLeader(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	leader := id.UserID("@lead:example.org")
	runID := hireForFight(t, "exp-heal", leader, ClassCleric, 6)
	ct := petePartyFight(t, p, runID, leader, 20) // leader at 20/100 — well under the 45% bar

	kind, arg := p.pickAutoCombatActionForSeat(companionUserID(), ct.sess, 1)
	if kind != "cast" {
		t.Fatalf("the hired cleric picked %q %q with the leader at 20%% HP — he must heal", kind, arg)
	}
	if !strings.Contains(arg, "@lead") {
		t.Fatalf("cast arg = %q, want the heal aimed at the leader's seat", arg)
	}

	// And it lands on the leader, not on himself.
	action, settle, msg := p.castActionForSeat(ct, 1, arg)
	if msg != "" {
		t.Fatalf("castActionForSeat refused the companion's own pick: %s", msg)
	}
	settle(true)
	eff := action.Effect
	if eff == nil || eff.AllyHeal <= 0 {
		t.Fatalf("effect = %+v, want an ally heal", eff)
	}
	if eff.AllySeat != 0 {
		t.Errorf("heal landed on seat %d, want seat 0 (the leader)", eff.AllySeat)
	}
	if eff.PlayerHeal != 0 {
		t.Errorf("the heal also healed the caster (%d HP) — it was redirected, not copied", eff.PlayerHeal)
	}
}

// His slots are spent on his seat, and he leaves no rows behind.
//
// This is the invariant with teeth. castActionForSeat used to load the caster via
// ensureCharForDnDCmd, whose auto-migration branch — handed a user with no sheet —
// *builds one at level 1 and saves it*. Pointed at the companion that silently
// makes him a player: a dnd_character row, a player_meta seed, a spellbook, and a
// level-1 chassis in place of the level he was hired at.
func TestCompanionSpells_SpendsHisSeatNotTheDatabase(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	leader := id.UserID("@lead:example.org")
	runID := hireForFight(t, "exp-rows", leader, ClassCleric, 6)
	ct := petePartyFight(t, p, runID, leader, 20)

	action, settle, msg := p.castActionForSeat(ct, 1, "cure_wounds @lead")
	if msg != "" {
		t.Fatalf("the hired cleric could not cast cure wounds: %s", msg)
	}
	settle(true)
	if action.Effect == nil || action.Effect.AllyHeal <= 0 {
		t.Fatalf("effect = %+v, want an ally heal", action.Effect)
	}

	// The slot came off his seat's ledger.
	if used := ct.sess.actorStatusesPtr(1).SlotsUsed[1]; used != 1 {
		t.Errorf("companion spent %d level-1 slots on his seat, want 1", used)
	}
	// ...and not off the leader's, which is the bug the seat-scoping exists to
	// prevent: one shared @pete id across every expedition that hires him.
	if used := ct.sess.actorStatusesPtr(0).SlotsUsed[1]; used != 0 {
		t.Errorf("the companion's cast debited the LEADER's seat (%d slots)", used)
	}

	for _, table := range []string{"dnd_character", "dnd_known_spells", "dnd_spell_slots", "player_meta"} {
		var n int
		if err := db.Get().QueryRow(
			`SELECT COUNT(*) FROM `+table+` WHERE user_id = ?`, string(companionUserID()),
		).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("casting wrote %d %s row(s) for the companion — he is not a player", n, table)
		}
	}
}

// He runs dry like anybody else, and then he swings. A companion with an infinite
// spell pool is the "carry" the whole design says he must never be.
func TestCompanionSpells_RunsOutOfSlots(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	leader := id.UserID("@lead:example.org")
	runID := hireForFight(t, "exp-dry", leader, ClassCleric, 6)
	ct := petePartyFight(t, p, runID, leader, 20)

	total := companionSlotPool(ClassCleric, 6, ActorStatuses{})[1][0]
	if total <= 0 {
		t.Fatal("a level-6 cleric has no level-1 slots — the slot table did not resolve")
	}
	for i := range total {
		if ok, err := consumeSeatSlot(ct.sess, 1, companionUserID(), 1); err != nil || !ok {
			t.Fatalf("slot %d/%d: consume = %v (%v), want it to succeed", i+1, total, ok, err)
		}
	}
	if ok, _ := consumeSeatSlot(ct.sess, 1, companionUserID(), 1); ok {
		t.Fatalf("the companion cast a %d-th level-1 spell out of a %d-slot pool", total+1, total)
	}

	// The refund half: a round that fails to resolve gives the slot back.
	if err := refundSeatSlot(ct.sess, 1, companionUserID(), 1); err != nil {
		t.Fatal(err)
	}
	if ok, _ := consumeSeatSlot(ct.sess, 1, companionUserID(), 1); !ok {
		t.Error("a refunded slot was not castable again")
	}
}

// A hired martial is still a martial: the spellbook is the class's, not a blanket
// grant, so hiring a Fighter does not quietly buy a caster.
func TestCompanionSpells_MartialHasNoSpellbook(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	leader := id.UserID("@lead:example.org")
	runID := hireForFight(t, "exp-mart", leader, ClassFighter, 6)
	ct := petePartyFight(t, p, runID, leader, 20)

	if known, _ := seatKnownSpells(ct.sess, 1, companionUserID()); len(known) != 0 {
		t.Errorf("a hired Fighter knows %d spells, want none", len(known))
	}
	if _, _, msg := p.castActionForSeat(ct, 1, "cure_wounds @lead"); msg == "" {
		t.Error("a hired Fighter cast cure wounds")
	}
	kind, _ := p.pickAutoCombatActionForSeat(companionUserID(), ct.sess, 1)
	if kind != "attack" {
		t.Errorf("a hired Fighter picked %q, want attack", kind)
	}
}
