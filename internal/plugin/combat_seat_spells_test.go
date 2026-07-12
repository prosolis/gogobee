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

	// The slot came off the expedition's ledger — which is where it has to live, so
	// that it is still spent in the NEXT fight of the same run.
	if used := companionSlotsForRun(ct.sess.RunID); used[1] != 1 {
		t.Errorf("companion spent %v level-1 slots on the run's ledger, want 1", used[1])
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

	total := companionSlotPool(ClassCleric, 6, [6]int{})[1][0]
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

// His pool does NOT refill between fights, and it DOES come back at camp.
//
// This is the one the sweep caught and the unit tests did not. The first cut of
// the spellbook parked his ledger on his combat seat — and a seat is per-session,
// so every fight opened a fresh one and he walked in with full slots. A human
// cleric rations a single pool across the whole run; rationing it IS the caster's
// game. Handed an infinite pool, a gearless level-penalized hireling out-cleared a
// same-level human cleric by 15pp in the sim.
func TestCompanionSpells_PoolIsRationedAcrossTheRun(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	leader := id.UserID("@lead:example.org")
	runID := hireForFight(t, "exp-ration", leader, ClassCleric, 6)

	// Fight one: spend every level-1 slot he owns.
	first := petePartyFight(t, p, runID, leader, 20)
	total := companionSlotPool(ClassCleric, 6, [6]int{})[1][0]
	for range total {
		if ok, err := consumeSeatSlot(first.sess, 1, companionUserID(), 1); err != nil || !ok {
			t.Fatalf("consume: %v (%v)", ok, err)
		}
	}

	// Fight two, same run — a NEW combat session, which is exactly what used to
	// hand him a fresh pool.
	if err := markCombatSessionExpired(first.sess.SessionID); err != nil {
		t.Fatal(err)
	}
	second := petePartyFight(t, p, runID, leader, 20)
	if ok, _ := consumeSeatSlot(second.sess, 1, companionUserID(), 1); ok {
		t.Error("the companion's spell slots refilled between fights — he is an infinite caster")
	}

	// ...and camp gives them back, the same way it does for every human.
	if err := refreshCompanionSlots("exp-ration"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := consumeSeatSlot(second.sess, 1, companionUserID(), 1); !ok {
		t.Error("camp did not restore the companion's slots — his pool only ever goes down")
	}
}

// He carries his wounds between fights, and camp patches him up.
//
// He used to re-seat at full max HP for every single fight — "he arrives fresh
// next time" was the close-out's stated intent — which is an infinite body. A
// player bleeds across a 30-room run and only heals at camp; the hireling soaked
// his share of every fight and then reset. In the sim his party fled 5 runs out of
// 640 where the same party with a *human* cleric fled 56.
func TestCompanion_CarriesWoundsBetweenFights(t *testing.T) {
	setupEmptyTestDB(t)
	leader := id.UserID("@lead:example.org")
	seedExpedition(t, "exp-hp", leader, "active")
	seatLeaderFixture(t, "exp-hp")
	if err := hireCompanion("exp-hp", ClassCleric, 6); err != nil {
		t.Fatal(err)
	}

	// A fresh hire is at full.
	if got := companionSeatHP("exp-hp", 100); got != 100 {
		t.Errorf("a fresh hire seats at %d/100 HP, want full", got)
	}

	// He walks out of a fight on 30 HP; he walks into the next one on 30 HP.
	if err := setCompanionHP("exp-hp", 30); err != nil {
		t.Fatal(err)
	}
	if got := companionSeatHP("exp-hp", 100); got != 30 {
		t.Errorf("he re-seated at %d/100 HP after ending a fight on 30 — the wound did not carry", got)
	}

	// Dropped in a fight, he comes back on his feet but barely — not as a corpse
	// (there is no companion-death rule) and not at full.
	if err := setCompanionHP("exp-hp", 0); err != nil {
		t.Fatal(err)
	}
	if got := companionSeatHP("exp-hp", 100); got != 1 {
		t.Errorf("after being dropped he seats at %d/100 HP, want 1", got)
	}

	// Camp puts him right, exactly as it does every human.
	if err := refreshCompanionHP("exp-hp"); err != nil {
		t.Fatal(err)
	}
	if got := companionSeatHP("exp-hp", 100); got != 100 {
		t.Errorf("camp left him on %d/100 HP — his body only ever goes down", got)
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
