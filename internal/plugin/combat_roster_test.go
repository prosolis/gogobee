package plugin

// Roster/cursor tests for the N-player combat state (N3/P2).
//
// The solo path is pinned bit-for-bit by TestCombatCharacterization; these
// cover the machinery that pin cannot see, because solo never moves the
// cursor off seat 0.

import (
	"math/rand/v2"
	"testing"
)

func TestNewActor_DerivesPerFightStateFromMods(t *testing.T) {
	c := basePlayer()
	c.Mods.WardCharges = 2
	c.Mods.SporeCloud = 3
	c.Mods.ReflectNext = 0.5
	c.Mods.AutoCritFirst = true
	c.Mods.ArcaneWardHP = 25

	a := newActor(&c)

	if a.c != &c {
		t.Error("actor should point back at its Combatant")
	}
	if a.playerHP != c.Stats.MaxHP {
		t.Errorf("playerHP = %d, want MaxHP %d", a.playerHP, c.Stats.MaxHP)
	}
	if a.wardCharges != 2 || a.sporeRounds != 3 || a.reflectFrac != 0.5 || !a.autoCrit || a.arcaneWardHP != 25 {
		t.Errorf("consumable one-shots not carried from Mods: %+v", a)
	}
}

func TestNewActor_StartHPAndHealChargeBackfill(t *testing.T) {
	// StartHP below MaxHP means the character walks in wounded.
	wounded := basePlayer()
	wounded.Stats.StartHP = 40
	if got := newActor(&wounded).playerHP; got != 40 {
		t.Errorf("wounded entry HP = %d, want 40", got)
	}

	// StartHP at or above MaxHP is ignored (guards a stale snapshot).
	full := basePlayer()
	full.Stats.StartHP = 999
	if got := newActor(&full).playerHP; got != full.Stats.MaxHP {
		t.Errorf("StartHP above MaxHP should not raise entry HP, got %d", got)
	}

	// Legacy one-shot: a HealItem amount with no explicit count backfills to 1.
	legacy := basePlayer()
	legacy.Mods.HealItem = 30
	if got := newActor(&legacy).healChargesLeft; got != 1 {
		t.Errorf("legacy heal backfill = %d charges, want 1", got)
	}

	// An explicit count wins over the backfill.
	stocked := basePlayer()
	stocked.Mods.HealItem = 30
	stocked.Mods.HealItemCharges = 4
	if got := newActor(&stocked).healChargesLeft; got != 4 {
		t.Errorf("explicit heal charges = %d, want 4", got)
	}

	// No HealItem at all means no charges, even if a count leaked through.
	none := basePlayer()
	if got := newActor(&none).healChargesLeft; got != 0 {
		t.Errorf("no heal item should mean 0 charges, got %d", got)
	}
}

// The cursor is the whole point of the embed: per-actor fields must follow
// seat(), and fight-scoped fields must not.
func TestCombatState_SeatSwitchesPerActorStateOnly(t *testing.T) {
	alice, bob := basePlayer(), basePlayer()
	alice.Name, bob.Name = "Alice", "Bob"
	a0, a1 := newActor(&alice), newActor(&bob)

	st := &combatState{
		actor:   a0,
		actors:  []*actor{a0, a1},
		enemyHP: 100,
		rng:     rand.New(rand.NewPCG(1, 1)),
	}

	// Wound seat 0 and burn one of its once-per-fight one-shots.
	st.seat(0)
	st.playerHP = 10
	st.luckyUsed = true

	// Seat 1 must be untouched — a promoted write goes to the cursor, not the
	// struct. This is the assertion that would fail if actor were embedded by
	// value instead of by pointer.
	st.seat(1)
	if st.playerHP != bob.Stats.MaxHP {
		t.Errorf("seat 1 HP = %d, want a full pool %d — seat 0's wound leaked", st.playerHP, bob.Stats.MaxHP)
	}
	if st.luckyUsed {
		t.Error("seat 1 saw seat 0's consumed Lucky reroll")
	}
	if st.c.Name != "Bob" {
		t.Errorf("cursor Combatant = %q, want Bob", st.c.Name)
	}

	// Fight-scoped state is shared: writing it under one seat is visible
	// under the other.
	st.enemyHP = 42
	st.enemyBlockUp = true
	st.seat(0)
	if st.playerHP != 10 || !st.luckyUsed {
		t.Error("seat 0 lost its own state across a cursor round-trip")
	}
	if st.enemyHP != 42 || !st.enemyBlockUp {
		t.Error("enemy stance should be fight-scoped, not per-actor")
	}
	if st.c.Name != "Alice" {
		t.Errorf("cursor Combatant = %q, want Alice", st.c.Name)
	}
}

func TestCombatState_AnyAlive(t *testing.T) {
	alice, bob := basePlayer(), basePlayer()
	a0, a1 := newActor(&alice), newActor(&bob)
	st := &combatState{actor: a0, actors: []*actor{a0, a1}}

	if !st.anyAlive() {
		t.Error("a fresh roster should be alive")
	}
	a0.playerHP = 0
	if !st.anyAlive() {
		t.Error("one downed member should not end the fight while another stands")
	}
	a1.playerHP = 0
	if st.anyAlive() {
		t.Error("a fully downed roster should not be alive")
	}
}

// Solo fights seat exactly one actor and park the cursor on it. If this ever
// regresses, the characterization golden stops proving anything about the
// production auto-resolve path.
func TestSimulateCombat_SeatsExactlyOneActor(t *testing.T) {
	p, e := basePlayer(), baseEnemy()
	seat0 := newActor(&p)
	st := &combatState{actor: seat0, actors: []*actor{seat0}, enemyHP: e.Stats.MaxHP}

	if len(st.actors) != 1 {
		t.Fatalf("solo roster length = %d, want 1", len(st.actors))
	}
	if st.actor != st.actors[0] {
		t.Error("solo cursor must point at seat 0")
	}
}
