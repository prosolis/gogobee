package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

// §1 — the cleric fix.
//
// Until this landed, EVERY heal in the combat engine was self-scoped:
// MistyHealProc healed the actor, HealItem fired the actor's own trigger, and
// turnActionEffect.PlayerHeal wrote to the acting seat. There was no action of any
// kind that could touch another seat's HP. A party cleric — the class whose entire
// identity is keeping other people upright — could not put one hit point on a
// friend, and N3 shipped that way without a single test going red, because party
// combat had no golden.
//
// These tests exist so that can never be true again.

// startAllyHealFight seats a healer at 0 and a hurt friend at 1.
func startAllyHealFight(t *testing.T, p *AdventurePlugin, hurtHP int) *CombatSession {
	t.Helper()
	healer := basePlayer()
	friend := basePlayer()
	enemy := Combatant{Name: "dummy", Stats: CombatStats{MaxHP: 800, AC: 10, Attack: 1, AttackBonus: 1}}

	// Distinct ids per fight: one active session per player is enforced, so two
	// fights sharing a healer would collide.
	tag := strings.ReplaceAll(t.Name(), "/", "_")
	sess, err := p.startPartyCombatSession("run-"+tag, "enc", "goblin", &enemy, []CombatSeatSetup{
		{UserID: healerID(tag), HP: 100, HPMax: 100, Mods: healer.Mods, C: &healer},
		{UserID: friendID(tag), HP: hurtHP, HPMax: 100, Mods: friend.Mods, C: &friend},
	})
	if err != nil {
		t.Fatal(err)
	}
	return sess
}

func healerID(tag string) id.UserID { return id.UserID("@healer-" + tag + ":example.org") }
func friendID(tag string) id.UserID { return id.UserID("@friend-" + tag + ":example.org") }

func TestAllyHeal_LandsOnTheFriendNotTheCaster(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	sess := startAllyHealFight(t, p, 20)

	healer, friend := basePlayer(), basePlayer()
	players := []*Combatant{&healer, &friend}
	enemy := Combatant{Name: "dummy", Stats: CombatStats{MaxHP: 800, AC: 10, Attack: 1, AttackBonus: 1}}
	ct := &combatTurn{sess: sess, players: players, enemy: &enemy, seat: 0, uid: healerID(strings.ReplaceAll(t.Name(), "/", "_"))}

	casterBefore, friendBefore := sess.seatHP(0), sess.seatHP(1)

	_, err := p.driveCombatRound(ct, PlayerAction{
		Kind: ActionCast,
		Effect: &turnActionEffect{
			Label: "Cure Wounds", Action: "spell_cast",
			AllyHeal: 30, AllySeat: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := sess.seatHP(1); got != friendBefore+30 {
		t.Errorf("friend HP = %d, want %d — the heal did not reach them. "+
			"This is the bug: a cleric who cannot heal anyone but themselves.",
			got, friendBefore+30)
	}
	if got := sess.seatHP(0); got > casterBefore {
		t.Errorf("caster HP = %d (was %d) — the heal landed on the caster instead of the target",
			got, casterBefore)
	}
}

// A heal cannot exceed the target's ceiling, and cannot raise the dead. Death is
// terminal for the fight — the close-out marks them, the hospital takes them — and
// a heal that resurrected a corpse would quietly rewrite the loss rules every
// close-out path depends on.
func TestAllyHeal_CapsAtMaxAndWillNotRaiseTheDead(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}

	t.Run("caps at max", func(t *testing.T) {
		sess := startAllyHealFight(t, p, 90)
		healer, friend := basePlayer(), basePlayer()
		ct := &combatTurn{sess: sess, players: []*Combatant{&healer, &friend},
			enemy: &Combatant{Name: "d", Stats: CombatStats{MaxHP: 800, AC: 10, Attack: 1, AttackBonus: 1}},
			seat:  0, uid: healerID(strings.ReplaceAll(t.Name(), "/", "_"))}

		if _, err := p.driveCombatRound(ct, PlayerAction{Kind: ActionCast,
			Effect: &turnActionEffect{Label: "Cure", Action: "spell_cast", AllyHeal: 500, AllySeat: 1},
		}); err != nil {
			t.Fatal(err)
		}
		if got, want := sess.seatHP(1), sess.seatHPMax(1); got > want {
			t.Errorf("friend healed to %d over a max of %d", got, want)
		}
	})

	t.Run("will not raise the dead", func(t *testing.T) {
		sess := startAllyHealFight(t, p, 0) // seat 1 is already down
		healer, friend := basePlayer(), basePlayer()
		ct := &combatTurn{sess: sess, players: []*Combatant{&healer, &friend},
			enemy: &Combatant{Name: "d", Stats: CombatStats{MaxHP: 800, AC: 10, Attack: 1, AttackBonus: 1}},
			seat:  0, uid: healerID(strings.ReplaceAll(t.Name(), "/", "_"))}

		if _, err := p.driveCombatRound(ct, PlayerAction{Kind: ActionCast,
			Effect: &turnActionEffect{Label: "Cure", Action: "spell_cast", AllyHeal: 50, AllySeat: 1},
		}); err != nil {
			t.Fatal(err)
		}
		if got := sess.seatHP(1); got > 0 {
			t.Errorf("a downed seat was healed to %d — healing keeps people up, it does not bring them back", got)
		}
	})
}

// The target parser resolves against the people in the fight, and leaves anything
// it does not recognise on the string for the spell parser — so every existing
// `!cast` form still means what it always meant.
func TestSplitCastTarget(t *testing.T) {
	setupEmptyTestDB(t)
	p := &AdventurePlugin{}
	sess := startAllyHealFight(t, p, 50)
	healer, friend := basePlayer(), basePlayer()
	healer.Name, friend.Name = "Ayla", "Bram"
	ct := &combatTurn{sess: sess, players: []*Combatant{&healer, &friend},
		enemy: &Combatant{Name: "d"}, seat: 0, uid: healerID(strings.ReplaceAll(t.Name(), "/", "_"))}

	tName := t.Name()
	tests := []struct {
		name      string
		in        string
		wantArgs  string
		wantSeat  int
		wantError bool
	}{
		{"no target", "cure wounds", "cure wounds", -1, false},
		{"slot level is not a target", "fireball 3", "fireball 3", -1, false},
		{"@mention by display name", "cure wounds @Bram", "cure wounds", 1, false},
		// The flag !help has advertised all along, and that parseCombatCast used to
		// accept and silently throw away ("reserved for SP3").
		{"--target flag", "cure wounds --target @Bram", "cure wounds", 1, false},
		{"--target mid-string", "cure wounds --target Bram", "cure wounds", 1, false},
		{"--target with no name", "cure wounds --target", "", -1, true},
		{"bare display name", "cure wounds Bram", "cure wounds", 1, false},
		{"by localpart", "cure wounds @" + friendID(strings.ReplaceAll(tName, "/", "_")).Localpart(), "cure wounds", 1, false},
		{"targeting yourself is just casting it", "cure wounds @Ayla", "cure wounds", -1, false},
		{"@mention of a stranger is an error", "cure wounds @nobody", "", -1, true},
		{"an unknown bare word is left for the spell parser", "mass cure wounds", "mass cure wounds", -1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args, seat, errMsg := splitCastTarget(ct, 0, tc.in)
			if tc.wantError {
				if errMsg == "" {
					t.Fatalf("splitCastTarget(%q) gave no error; a mistyped @mention would silently waste the slot", tc.in)
				}
				return
			}
			if errMsg != "" {
				t.Fatalf("splitCastTarget(%q) errored: %s", tc.in, errMsg)
			}
			if args != tc.wantArgs || seat != tc.wantSeat {
				t.Errorf("splitCastTarget(%q) = (%q, %d), want (%q, %d)",
					tc.in, args, seat, tc.wantArgs, tc.wantSeat)
			}
		})
	}
}
