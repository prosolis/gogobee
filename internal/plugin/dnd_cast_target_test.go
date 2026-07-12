package plugin

import (
	"errors"
	"testing"

	"maunium.net/go/mautrix/id"
)

// The out-of-combat half of §1. `--target` was parsed and thrown away here since
// SP2 ("reserved for SP3, accept and ignore"), so a party cleric standing over a
// bleeding friend between fights could do precisely nothing for them.

func TestSplitOutOfCombatTarget(t *testing.T) {
	cases := []struct {
		name, args, wantRest, wantTarget string
	}{
		{"flag", "cure wounds --target @alex", "cure wounds", "alex"},
		{"flag mid-string", "cure wounds --target @alex --upcast 3", "cure wounds --upcast 3", "alex"},
		{"flag without the @", "cure wounds --target alex", "cure wounds", "alex"},
		{"trailing mention", "cure wounds @alex", "cure wounds", "alex"},
		{"no target", "cure wounds", "cure wounds", ""},
		// A trailing bare word is a spell word, not a name — the `@` is what makes
		// it a target out of combat. Getting this wrong eats half of "cure wounds".
		{"bare trailing word is not a target", "healing word", "healing word", ""},
		{"upcast digit is not a target", "cure wounds --upcast 3", "cure wounds --upcast 3", ""},
		{"dangling flag", "cure wounds --target", "cure wounds --target", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rest, target := splitOutOfCombatTarget(tc.args)
			if rest != tc.wantRest || target != tc.wantTarget {
				t.Fatalf("splitOutOfCombatTarget(%q) = (%q, %q), want (%q, %q)",
					tc.args, rest, target, tc.wantRest, tc.wantTarget)
			}
		})
	}
}

// The target set is the expedition, not the world: you can heal the person you
// are travelling with, and nobody else.
func TestResolveCastTarget_OnlyThePartyIsReachable(t *testing.T) {
	setupEmptyTestDB(t)
	leader, member := seatedMember(t, "casttarget")

	uid, errMsg := resolveCastTargetOnExpedition(leader, member.Localpart())
	if errMsg != "" || uid != member {
		t.Fatalf("leader → member = (%q, %q); want the member, no error", uid, errMsg)
	}

	// Somebody with a sheet, but not on this expedition.
	stranger := id.UserID("@stranger-casttarget:example.org")
	zoneCmdTestCharacter(t, stranger, 1)
	if _, errMsg := resolveCastTargetOnExpedition(leader, stranger.Localpart()); errMsg == "" {
		t.Fatal("healed a stranger across the world; only the party is reachable")
	}

	// Naming yourself is just casting it on yourself: no target, no error. A
	// refusal here would make `!cast cure wounds @me` cost a slot for nothing.
	if uid, errMsg := resolveCastTargetOnExpedition(leader, leader.Localpart()); uid != "" || errMsg != "" {
		t.Fatalf("self-target = (%q, %q); want the self-heal path, silently", uid, errMsg)
	}
}

func TestHealPartyMember_ClampsAtMaxAndWillNotRaiseTheDead(t *testing.T) {
	setupEmptyTestDB(t)
	target := id.UserID("@heal-target:example.org")
	zoneCmdTestCharacter(t, target, 1) // HPMax 20

	setHP := func(hp int) {
		c, err := LoadDnDCharacter(target)
		if err != nil || c == nil {
			t.Fatalf("load: %v", err)
		}
		c.HPCurrent = hp
		if err := SaveDnDCharacter(c); err != nil {
			t.Fatal(err)
		}
	}

	// A heal bigger than the wound tops out at max rather than overflowing.
	setHP(15)
	before, after, maxHP, err := healPartyMember(target, 100)
	if err != nil || before != 15 || after != 20 || maxHP != 20 {
		t.Fatalf("overheal = (%d → %d / %d, %v); want clamped to 20", before, after, maxHP, err)
	}

	// Already full: decline so the caller can refund. Spending a slot to heal
	// zero HP is the kind of thing players never forgive.
	if _, _, _, err := healPartyMember(target, 10); !errors.Is(err, errHealTargetFull) {
		t.Fatalf("healing a full-HP ally = %v; want errHealTargetFull", err)
	}

	// Down: a heal is not a resurrection. Same rule the combat path holds.
	setHP(0)
	if _, _, _, err := healPartyMember(target, 10); !errors.Is(err, errHealTargetDown) {
		t.Fatalf("healing a downed ally = %v; want errHealTargetDown", err)
	}
	if c, _ := LoadDnDCharacter(target); c == nil || c.HPCurrent != 0 {
		t.Fatal("the downed ally was raised by a heal")
	}
}
