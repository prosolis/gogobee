package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// §6 — the caster casts in an auto-resolved room fight.
//
// Before this, the ONLY spell that could land in an ordinary room was one the
// player hand-queued with `!cast` before walking in. On autopilot nobody queues
// one, so a cleric fought every room of every expedition with a mace, lost room
// fights a fighter never loses, and a room loss ends the whole expedition.

func autocastCleric(t *testing.T, tag string) (id.UserID, *DnDCharacter) {
	t.Helper()
	p := &AdventurePlugin{}
	s := &SimRunner{P: p}
	uid := id.UserID("@autocast-" + tag + ":example.org")
	c, err := s.BuildCharacter(uid, ClassCleric, 10)
	if err != nil {
		t.Fatal(err)
	}
	return uid, c
}

func usedSlots(t *testing.T, uid id.UserID) int {
	t.Helper()
	slots, err := getSpellSlots(uid)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, pair := range slots {
		total += pair[1]
	}
	return total
}

func TestAutoCast_CasterCastsAndPaysForIt(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	uid, c := autocastCleric(t, "pays")

	var mods CombatModifiers
	stats := CombatStats{MaxHP: 100, AC: 14}
	enemy := CombatStats{MaxHP: 100, AC: 14}

	if !p.autoCastForAutoResolve(uid, c, &stats, &mods, &enemy) {
		t.Fatal("a level-10 cleric with a full slot pool cast nothing")
	}
	// It has to be PAID for. pickBestDamageSpell reads the theoretical class slot
	// table; if we had reused it here the spell would be infinite — the same
	// free-lunch bug that gave the companion an unlimited body. The slot is spent
	// whether or not the spell connects, exactly as it is at a real table.
	if used := usedSlots(t, uid); used != 1 {
		t.Fatalf("slots used = %d, want exactly 1 — the spell must cost a real slot", used)
	}
}

// The slot has to buy damage. Not on any GIVEN cast — under the room slot cap a
// cleric reaches for inflict_wounds, which is an attack-roll spell and is
// allowed to miss — but a caster who spends its pool and never scratches
// anything would be strictly worse than swinging the mace.
func TestAutoCast_SpellActuallyDealsDamage(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	uid, c := autocastCleric(t, "damage")

	landed := 0
	for i := 0; i < 40; i++ {
		var mods CombatModifiers
		stats := CombatStats{MaxHP: 100, AC: 14}
		enemy := CombatStats{MaxHP: 1000, AC: 10}
		if !p.autoCastForAutoResolve(uid, c, &stats, &mods, &enemy) {
			continue
		}
		if mods.SpellPreDamage > 0 || enemy.MaxHP < 1000 {
			landed++
		}
	}
	if landed == 0 {
		t.Fatal("40 casts against AC 10 and not one dealt a point of damage — " +
			"the caster is spending slots on air")
	}
}

// The pool is finite: cast far more times than the caster has slots, and the
// ledger must converge rather than run forever. This is the free-lunch guard —
// reusing the harness's pickBestDamageSpell (which reads the theoretical class
// slot table, not the row) would cast an unlimited leveled spell every room.
//
// It converges BELOW the full pool, and that is correct: a cleric owns no damage
// spell at slot 2 or 4 (guiding_bolt and inflict_wounds are L1, spirit_guardians
// L3, flame_strike L5), and a trash room never upcasts. Those slots are not
// wasted — the turn engine upcasts them at the elite and the boss, which is what
// they are for.
func TestAutoCast_SlotPoolIsFiniteAndConverges(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	uid, c := autocastCleric(t, "finite")

	before, err := getSpellSlots(uid)
	if err != nil {
		t.Fatal(err)
	}
	pool := 0
	for _, pair := range before {
		pool += pair[0]
	}
	if pool == 0 {
		t.Fatal("test cleric has no slots at all")
	}

	cast := func(n int) {
		for i := 0; i < n; i++ {
			var mods CombatModifiers
			stats := CombatStats{MaxHP: 100, AC: 14}
			enemy := CombatStats{MaxHP: 1000, AC: 14}
			p.autoCastForAutoResolve(uid, c, &stats, &mods, &enemy)
		}
	}

	cast(pool + 10)
	drained := usedSlots(t, uid)
	if drained == 0 {
		t.Fatal("the cleric never spent a slot — it is not really casting")
	}
	if drained > pool {
		t.Fatalf("slots used = %d > pool %d — slots are being conjured", drained, pool)
	}

	// Dry means dry: another twenty rooms must not find another leveled slot.
	cast(20)
	if after := usedSlots(t, uid); after != drained {
		t.Fatalf("slots used went %d → %d after the pool was dry — a leveled spell is being cast for free",
			drained, after)
	}

	// And a dry caster still throws a cantrip rather than reverting to a stick.
	var mods CombatModifiers
	stats := CombatStats{MaxHP: 100, AC: 14}
	enemy := CombatStats{MaxHP: 1000, AC: 14}
	if !p.autoCastForAutoResolve(uid, c, &stats, &mods, &enemy) {
		t.Fatal("an out-of-slots caster cast nothing at all; a cantrip is free")
	}
}

// A hand-queued spell wins. applyPendingCast already owns that cast; auto-casting
// on top of it would fire two spells in one action.
func TestAutoCast_HandQueuedSpellWins(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	uid, c := autocastCleric(t, "queued")

	c.PendingCast = encodePendingCast(PendingCast{SpellID: "guiding_bolt", SlotLevel: 1})
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	var mods CombatModifiers
	stats := CombatStats{MaxHP: 100, AC: 14}
	enemy := CombatStats{MaxHP: 100, AC: 14}
	if p.autoCastForAutoResolve(uid, c, &stats, &mods, &enemy) {
		t.Fatal("auto-cast fired on top of a hand-queued spell — that is two spells in one action")
	}
	if used := usedSlots(t, uid); used != 0 {
		t.Fatalf("slots used = %d; the auto-cast must not spend anything when the player already queued", used)
	}
}

// A martial has no spellbook and must be left exactly as it was — the balance
// corpus rests on the martials not moving.
func TestAutoCast_MartialIsUntouched(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	s := &SimRunner{P: p}
	uid := id.UserID("@autocast-fighter:example.org")
	c, err := s.BuildCharacter(uid, ClassFighter, 10)
	if err != nil {
		t.Fatal(err)
	}
	var mods CombatModifiers
	stats := CombatStats{MaxHP: 100, AC: 14}
	enemy := CombatStats{MaxHP: 100, AC: 14}
	if p.autoCastForAutoResolve(uid, c, &stats, &mods, &enemy) {
		t.Fatal("a fighter cast a spell")
	}
	if mods.SpellPreDamage != 0 {
		t.Fatalf("fighter picked up %d spell pre-damage", mods.SpellPreDamage)
	}
}

// Never upcast in a trash room: the big slots are what the elite and the boss
// are for, and the turn engine spends them there.
func TestAutoCast_DoesNotBurnTheBigSlots(t *testing.T) {
	setupEmptyTestDB(t)
	uid, c := autocastCleric(t, "native")

	spell, slot, ok := pickAutoResolveSpell(uid, c)
	if !ok {
		t.Fatal("cleric picked nothing")
	}
	if slot != spell.Level {
		t.Fatalf("%s cast at slot %d, native level %d — an ordinary room must not upcast",
			spell.ID, slot, spell.Level)
	}
}

// The boss kit survives the walk in. This is the regression the first cut of §6
// shipped and the sweep caught: with no cap, a caster spent its whole pool on
// goblins and reached the boss empty — room deaths fell but boss deaths tripled
// (mage 19 → 61, sorcerer 15 → 65). "Never upcast" is not a reserve, because a
// mage's native-level spells ARE its boss kit.
//
// So: grind a full expedition's worth of rooms, and every slot above the cap
// must still be sitting there untouched when the dragon opens its eyes.
func TestAutoCast_ReservesTheBossSlots(t *testing.T) {
	for _, class := range []DnDClass{ClassMage, ClassSorcerer, ClassCleric, ClassWarlock} {
		t.Run(string(class), func(t *testing.T) {
			setupEmptyTestDB(t)
			p := &AdventurePlugin{}
			s := &SimRunner{P: p}
			uid := id.UserID("@autocast-reserve-" + string(class) + ":example.org")
			c, err := s.BuildCharacter(uid, class, 12)
			if err != nil {
				t.Fatal(err)
			}

			// A long expedition: far more rooms than the caster has slots.
			for i := 0; i < 60; i++ {
				var mods CombatModifiers
				stats := CombatStats{MaxHP: 100, AC: 14}
				enemy := CombatStats{MaxHP: 1000, AC: 14}
				p.autoCastForAutoResolve(uid, c, &stats, &mods, &enemy)
			}

			slots, err := getSpellSlots(uid)
			if err != nil {
				t.Fatal(err)
			}
			for lvl, pair := range slots {
				if lvl <= roomSlotCap {
					continue
				}
				if pair[1] != 0 {
					t.Errorf("level-%d slots: %d of %d spent in ordinary rooms — "+
						"that is the boss's kit, and the caster will arrive empty",
						lvl, pair[1], pair[0])
				}
			}
		})
	}
}
