package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// Grim Harvest on the turn-based surface.
//
// The auto-resolve path stashes the killing spell's slot level on the
// fight-start CombatModifiers, where the close-out reads it. A turn-based fight
// has nowhere to keep that: it rebuilds its combatants from the session row on
// every !attack / !cast. resolveTurnSpell computed the stash into a local mod
// set and dropped it on the floor, so a Necromancy Mage who killed with a spell
// never healed — on a class that already trails.
//
// The stash now rides on the casting seat's ActorStatuses, and the close-out
// asks whether the *last* spell_cast is the one that dropped the enemy to 0.
//
// (Empowered Evocation and Overchannel were never broken here: they only move
// mods.SpellPreDamage, which resolveTurnSpell already returns as EnemyDamage.)

// necromancer is a Mage who has reached L5 Grim Harvest.
func necromancer(t *testing.T, uid id.UserID, level int) *DnDCharacter {
	t.Helper()
	if err := createAdvCharacter(uid, string(uid)); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassMage, Subclass: SubclassNecromancy, Level: level,
		STR: 8, DEX: 12, CON: 12, INT: 16, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 10, ArmorClass: 12,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	return c
}

// tough is a stat block a spell can be cast at without any of it mattering.
func tough() CombatStats { return CombatStats{MaxHP: 40, AC: 10} }

// ── the leak itself ──────────────────────────────────────────────────────────

// resolveTurnSpell must hand the stash back to its caller. Before this, the
// slot level lived and died inside the function.
func TestResolveTurnSpell_CarriesTheGrimHarvestStashOut(t *testing.T) {
	c := &DnDCharacter{Class: ClassMage, Subclass: SubclassNecromancy, Level: 5, INT: 16}

	// magic_missile: auto-hit, force damage, upcast to a 3rd-level slot.
	spell, _ := lookupSpell("magic_missile")
	enemy := tough()
	out, ok := resolveTurnSpell(c, spell, 3, &enemy)
	if !ok {
		t.Fatal("magic_missile should be a supported turn spell")
	}
	if out.GrimHarvestSlot != 3 {
		t.Errorf("GrimHarvestSlot = %d, want 3 (the slot it was cast at)", out.GrimHarvestSlot)
	}
	if out.GrimHarvestNecrotic {
		t.Error("magic_missile deals force damage — Necrotic should be false")
	}

	// blight: auto-hit, necrotic — the flag that triples the heal.
	spell, _ = lookupSpell("blight")
	enemy = tough()
	out, _ = resolveTurnSpell(c, spell, 4, &enemy)
	if out.GrimHarvestSlot != 4 || !out.GrimHarvestNecrotic {
		t.Errorf("blight L4: slot=%d necrotic=%v, want 4/true", out.GrimHarvestSlot, out.GrimHarvestNecrotic)
	}
}

// Nobody else stashes. A Mage below L5 has not learned the harvest, and an
// Evocation Mage never will.
func TestResolveTurnSpell_NoStashForAnyoneElse(t *testing.T) {
	spell, _ := lookupSpell("magic_missile")
	for _, c := range []*DnDCharacter{
		{Class: ClassMage, Subclass: SubclassNecromancy, Level: 4, INT: 16},
		{Class: ClassMage, Subclass: SubclassEvocation, Level: 12, INT: 16},
		{Class: ClassSorcerer, Level: 12, CHA: 16},
	} {
		enemy := tough()
		out, _ := resolveTurnSpell(c, spell, 3, &enemy)
		if out.GrimHarvestSlot != 0 {
			t.Errorf("%s/%s L%d stashed slot %d, want 0",
				c.Class, c.Subclass, c.Level, out.GrimHarvestSlot)
		}
	}
}

// ── which cast counts ────────────────────────────────────────────────────────

// The killing-blow check used to break on the first spell_cast it saw. In a
// turn-based fight the mage casts every round, so the first one is an opening
// cantrip that left the enemy standing — and it vetoed the heal the killing
// spell had earned. It is the last cast that has to be lethal.
func TestGrimHarvestHeal_ReadsTheLastCastNotTheFirst(t *testing.T) {
	c := &DnDCharacter{Class: ClassMage, Subclass: SubclassNecromancy, Level: 5, INT: 16}
	mods := CombatModifiers{GrimHarvestSlot: 2, GrimHarvestNecrotic: true}

	result := CombatResult{PlayerWon: true, Events: []CombatEvent{
		{Round: 1, Action: "spell_cast", EnemyHP: 22}, // opening cantrip, enemy stands
		{Round: 2, Action: "enemy_attack", EnemyHP: 22},
		{Round: 2, Action: "spell_cast", EnemyHP: 0}, // this one killed it
	}}
	if got := grimHarvestHeal(c, result, mods); got != 6 {
		t.Errorf("heal = %d, want 6 (3× a 2nd-level necrotic slot)", got)
	}
}

// The mirror: the mage softened it up with a spell, then finished it with a
// weapon. No spell landed the blow, so no harvest.
func TestGrimHarvestHeal_WeaponKillAfterASpellDoesNotHarvest(t *testing.T) {
	c := &DnDCharacter{Class: ClassMage, Subclass: SubclassNecromancy, Level: 5, INT: 16}
	mods := CombatModifiers{GrimHarvestSlot: 2, GrimHarvestNecrotic: true}

	result := CombatResult{PlayerWon: true, Events: []CombatEvent{
		{Round: 1, Action: "spell_cast", EnemyHP: 9},
		{Round: 2, Action: "player_attack", EnemyHP: 0},
	}}
	if got := grimHarvestHeal(c, result, mods); got != 0 {
		t.Errorf("heal = %d, want 0 — the sword landed the blow, not the spell", got)
	}
}

// ── the seam through the session ─────────────────────────────────────────────

// The stash has to survive the round-trip the fight puts it through: parked on
// the seat by the cast, carried across commit()'s snapshot, read back by the
// close-out.
func TestSeatFightStartMods_ReadsTheStashOffTheSeat(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@seatstash:example.org")
	c := necromancer(t, uid, 5)

	sess := &CombatSession{UserID: string(uid), Status: CombatStatusWon}
	sess.Statuses.GrimHarvestSlot = 3
	sess.Statuses.GrimHarvestNecrotic = true

	mods := seatFightStartMods(sess, 0, c)
	if mods.GrimHarvestSlot != 3 || !mods.GrimHarvestNecrotic {
		t.Fatalf("seatFightStartMods lost the stash: %+v", mods)
	}

	// snapshotActor rebuilds ActorStatuses from combatState each commit; fields
	// with no combatState counterpart must be carried over from the prior
	// snapshot, the way ArmedAbility is.
	kept := snapshotActor(&actor{}, sess.Statuses.ActorStatuses)
	if kept.GrimHarvestSlot != 3 || !kept.GrimHarvestNecrotic {
		t.Errorf("commit() dropped the stash: %+v", kept)
	}
}

// End to end at the close-out: a necromancer who ended the fight at 10/30 HP
// with a lethal 2nd-level necrotic spell walks away with 6 HP back.
func TestPostCombatBookkeepingForSeat_GrimHarvestHealsOnASpellKill(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@harvest:example.org")
	necromancer(t, uid, 5)

	sess := &CombatSession{
		UserID: string(uid), Status: CombatStatusWon, Round: 3,
		PlayerHP: 10, PlayerHPMax: 30, EnemyHP: 0,
		TurnLog: []CombatEvent{
			{Round: 1, Action: "spell_cast", EnemyHP: 14},
			{Round: 3, Action: "spell_cast", EnemyHP: 0},
		},
	}
	sess.Statuses.GrimHarvestSlot = 2
	sess.Statuses.GrimHarvestNecrotic = true

	(&AdventurePlugin{}).postCombatBookkeepingForSeat(sess, 0)

	got, _ := LoadDnDCharacter(uid)
	if got.HPCurrent != 16 {
		t.Errorf("HPCurrent = %d after a turn-based spell kill, want 16 (10 + 3×2)", got.HPCurrent)
	}
}

// A seat with no stash — the mage never cast, or missed every time — is left
// exactly as the fight left them.
func TestPostCombatBookkeepingForSeat_NoStashNoHeal(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@nostash:example.org")
	necromancer(t, uid, 5)

	sess := &CombatSession{
		UserID: string(uid), Status: CombatStatusWon, Round: 2,
		PlayerHP: 10, PlayerHPMax: 30, EnemyHP: 0,
		TurnLog: []CombatEvent{{Round: 2, Action: "player_attack", EnemyHP: 0}},
	}
	(&AdventurePlugin{}).postCombatBookkeepingForSeat(sess, 0)

	got, _ := LoadDnDCharacter(uid)
	if got.HPCurrent != 10 {
		t.Errorf("HPCurrent = %d, want 10 — nothing was stashed", got.HPCurrent)
	}
}

// Party fights: the stash is per-seat, so seat 1's harvest cannot heal seat 0.
// This is the same class of bug P5 fixed for mid-fight buffs.
func TestGrimHarvestStash_IsPerSeat(t *testing.T) {
	setupEmptyTestDB(t)
	leader, member := id.UserID("@leader:example.org"), id.UserID("@member:example.org")
	fightTestChar(t, leader, 30)
	necromancer(t, member, 5)

	sess := &CombatSession{
		UserID: string(leader), Status: CombatStatusWon, Round: 2,
		PlayerHP: 20, PlayerHPMax: 30, EnemyHP: 0,
		Participants: []CombatParticipant{{Seat: 1, UserID: string(member), HP: 10, HPMax: 30}},
		TurnLog: []CombatEvent{
			{Round: 2, Seat: 1, Action: "spell_cast", EnemyHP: 0},
		},
	}
	sess.Participants[0].Statuses.GrimHarvestSlot = 2
	sess.Participants[0].Statuses.GrimHarvestNecrotic = true

	p := &AdventurePlugin{}
	p.postCombatBookkeepingForSeat(sess, 0)
	p.postCombatBookkeepingForSeat(sess, 1)

	gotLeader, _ := LoadDnDCharacter(leader)
	if gotLeader.HPCurrent != 30 {
		t.Errorf("leader HPCurrent = %d, want 30 — the member's harvest is not theirs", gotLeader.HPCurrent)
	}
	gotMember, _ := LoadDnDCharacter(member)
	if gotMember.HPCurrent != 16 {
		t.Errorf("member HPCurrent = %d, want 16 (10 + 3×2)", gotMember.HPCurrent)
	}
}
