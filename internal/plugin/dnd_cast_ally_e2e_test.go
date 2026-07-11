package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// The whole `!cast cure wounds @friend` path, out of combat, through the real
// handler: parse → class/known/prepared gates → slot debit → the ally's sheet.
//
// The seams are unit-tested next door; this exists for the one thing they cannot
// see — that the slot ledger and the HP write agree about whether the heal
// happened. A refund that does not fire is a slot a player paid for nothing, and
// a refund that fires twice is a free heal.

// castingCleric turns an existing sheet into a cleric who knows and has prepared
// Cure Wounds, with slots to spend.
func castingCleric(t *testing.T, uid id.UserID) {
	t.Helper()
	c, err := LoadDnDCharacter(uid)
	if err != nil || c == nil {
		t.Fatalf("load %s: %v", uid, err)
	}
	c.Class = ClassCleric
	c.Level = 5
	c.WIS = 16
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := setSpellSlotsForCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := addKnownSpell(uid, "cure_wounds", "class", true); err != nil {
		t.Fatal(err)
	}
	if err := setSpellPrepared(uid, "cure_wounds", true); err != nil {
		t.Fatal(err)
	}
}

func slotsUsed(t *testing.T, uid id.UserID, level int) int {
	t.Helper()
	slots, err := getSpellSlots(uid)
	if err != nil {
		t.Fatal(err)
	}
	return slots[level][1]
}

func setCharHP(t *testing.T, uid id.UserID, hp int) {
	t.Helper()
	c, err := LoadDnDCharacter(uid)
	if err != nil || c == nil {
		t.Fatalf("load %s: %v", uid, err)
	}
	c.HPCurrent = hp
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
}

func TestCastAlly_OutOfCombat_HealsTheFriendAndSpendsOneSlot(t *testing.T) {
	setupEmptyTestDB(t)
	leader, member := seatedMember(t, "e2eheal")
	castingCleric(t, leader)
	setCharHP(t, member, 5) // of 20

	p := &AdventurePlugin{}
	if err := p.handleDnDCastCmd(MessageContext{Sender: leader}, "cure wounds @"+member.Localpart()); err != nil {
		t.Fatalf("cast: %v", err)
	}

	mc, _ := LoadDnDCharacter(member)
	if mc.HPCurrent <= 5 {
		t.Fatalf("the friend is still on %d HP; the heal landed on nobody", mc.HPCurrent)
	}
	// The caster must NOT have healed themselves — the bug this whole section exists for.
	if lc, _ := LoadDnDCharacter(leader); lc.HPCurrent != lc.HPMax {
		t.Fatalf("caster HP moved to %d/%d; the heal landed on the caster", lc.HPCurrent, lc.HPMax)
	}
	if used := slotsUsed(t, leader, 1); used != 1 {
		t.Fatalf("level-1 slots used = %d, want exactly 1", used)
	}
}

// Naming a full-HP ally must cost nothing: the slot is debited before the target
// is touched, so this is the refund path firing for real.
func TestCastAlly_OutOfCombat_FullHPAllyRefundsTheSlot(t *testing.T) {
	setupEmptyTestDB(t)
	leader, member := seatedMember(t, "e2efull")
	castingCleric(t, leader)

	p := &AdventurePlugin{}
	if err := p.handleDnDCastCmd(MessageContext{Sender: leader}, "cure wounds @"+member.Localpart()); err != nil {
		t.Fatalf("cast: %v", err)
	}
	if used := slotsUsed(t, leader, 1); used != 0 {
		t.Fatalf("level-1 slots used = %d after healing a full-HP ally; the slot was not refunded", used)
	}
}

// A target nobody can reach is refused before anything is spent.
func TestCastAlly_OutOfCombat_StrangerCostsNothing(t *testing.T) {
	setupEmptyTestDB(t)
	leader, _ := seatedMember(t, "e2estranger")
	castingCleric(t, leader)

	p := &AdventurePlugin{}
	if err := p.handleDnDCastCmd(MessageContext{Sender: leader}, "cure wounds @nobody"); err != nil {
		t.Fatalf("cast: %v", err)
	}
	if used := slotsUsed(t, leader, 1); used != 0 {
		t.Fatalf("level-1 slots used = %d after naming a stranger; nothing should have been spent", used)
	}
}

// A target on a spell that cannot use one is refused, rather than quietly
// dropping the target and firing the spell at the caster — which is what the old
// "accept and ignore" parse did.
func TestCastAlly_OutOfCombat_TargetOnANonHealIsRefused(t *testing.T) {
	setupEmptyTestDB(t)
	leader, member := seatedMember(t, "e2enonheal")
	castingCleric(t, leader)
	if err := addKnownSpell(leader, "guiding_bolt", "class", true); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	if err := p.handleDnDCastCmd(MessageContext{Sender: leader}, "guiding bolt @"+member.Localpart()); err != nil {
		t.Fatalf("cast: %v", err)
	}
	if used := slotsUsed(t, leader, 1); used != 0 {
		t.Fatalf("level-1 slots used = %d; a targeted non-heal must be refused before it is paid for", used)
	}
	if lc, _ := LoadDnDCharacter(leader); lc.PendingCast != "" {
		t.Fatalf("guiding bolt queued as %q; the target should have refused the cast outright", lc.PendingCast)
	}
}
