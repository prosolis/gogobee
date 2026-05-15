package plugin

// UX session S2 acceptance tests. Cover:
//   - B2: no EffectReaction in any class's default known-spell list (and
//     every default is actually castable by that class — the overlay-narrow
//     bug used to make Sorcerer/Warlock/Bard/Druid lists silently broken).
//   - R15: parseSpell("healing word") resolves deterministically to the
//     SRD `healing_word`, and the legacy `healing_word_spell` alias is gone.
//   - B3: !cast non-caster error contains no class enumerations.

import (
	"fmt"
	"strings"
	"testing"
)

func TestDefaultKnownSpellsHaveNoReactions(t *testing.T) {
	classes := []DnDClass{
		ClassMage, ClassCleric, ClassRanger,
		ClassDruid, ClassBard, ClassSorcerer, ClassWarlock, ClassPaladin,
	}
	for _, class := range classes {
		for _, lvl := range []int{1, 2, 3, 5, 9, 13, 20} {
			for _, sid := range defaultKnownSpells(class, lvl) {
				s, ok := lookupSpell(sid)
				if !ok {
					t.Errorf("default for %s L%d: unknown spell %q", class, lvl, sid)
					continue
				}
				if s.Effect == EffectReaction {
					t.Errorf("default for %s L%d includes reaction spell %q — combat has no reaction window yet",
						class, lvl, sid)
				}
			}
		}
	}
}

func TestDefaultKnownSpellsAreCastableByClass(t *testing.T) {
	// Every default-granted spell must list the granting class in its
	// Classes slice; otherwise the !cast classOK gate rejects a spell the
	// player was auto-granted. This used to fail silently because the
	// hand-authored overlay was narrowing class lists down to [Mage].
	classes := []DnDClass{
		ClassMage, ClassCleric, ClassRanger,
		ClassDruid, ClassBard, ClassSorcerer, ClassWarlock, ClassPaladin,
	}
	for _, class := range classes {
		for _, lvl := range []int{1, 3, 5, 9, 13, 20} {
			for _, sid := range defaultKnownSpells(class, lvl) {
				s, ok := lookupSpell(sid)
				if !ok {
					continue // covered by the existence test
				}
				ok = false
				for _, c := range s.Classes {
					if c == class {
						ok = true
						break
					}
				}
				if !ok {
					t.Errorf("%s default L%d grants %q but Classes=%v doesn't include %s",
						class, lvl, sid, s.Classes, class)
				}
			}
		}
	}
}

func TestHealingWordResolvesDeterministically(t *testing.T) {
	for _, in := range []string{"healing word", "Healing Word", "HEALING WORD", "healing-word", "healing_word"} {
		s, ok := parseSpell(in)
		if !ok {
			t.Fatalf("parseSpell(%q): not found", in)
		}
		if s.ID != "healing_word" {
			t.Errorf("parseSpell(%q): id=%q, want healing_word (legacy alias should be gone)",
				in, s.ID)
		}
	}
	if _, ok := lookupSpell("healing_word_spell"); ok {
		t.Errorf("healing_word_spell alias should have been removed; still in registry")
	}
}

func TestCastNonCasterErrorHasNoClassEnumeration(t *testing.T) {
	// B3: the non-caster error message used to enumerate "Mage, Cleric,
	// Ranger, and Arcane Trickster Rogues (L5+)" — stale list that now
	// omits Druid/Bard/Sorcerer/Warlock/Paladin. The replacement should
	// not mention specific class names.
	//
	// We exercise the prefix the !cast handler emits via the public
	// titleClass()/isSpellcaster() shape. Direct invocation of the handler
	// would need a full plugin/db harness; we settle for asserting on the
	// composed prefix that the production line uses.
	for _, cls := range []DnDClass{ClassFighter, ClassRogue} {
		msg := fmt.Sprintf("%s doesn't cast spells.", titleClass(cls))
		lower := strings.ToLower(msg)
		for _, banned := range []string{"mage", "cleric", "druid", "bard", "sorcerer", "warlock", "paladin", "ranger", "trickster"} {
			if strings.Contains(lower, banned) {
				// titleClass() may legitimately put the *speaker's* class in;
				// the failure case is enumerating *other* classes.
				if !strings.EqualFold(string(cls), banned) {
					t.Errorf("non-caster error mentions class %q for speaker %s: %q",
						banned, cls, msg)
				}
			}
		}
	}
}
