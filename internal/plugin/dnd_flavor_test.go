package plugin

import (
	"testing"

	"gogobee/internal/flavor"
)

// TestDnDFlavorPoolsNonEmpty: every pool we wired into the D&D layer must
// have at least one entry. Catches accidental deletions in the protected
// flavor files that would silently produce empty narrative.
func TestDnDFlavorPoolsNonEmpty(t *testing.T) {
	pools := map[string][]string{
		"CombatStart":         flavor.CombatStart,
		"CombatVictory":       flavor.CombatVictory,
		"BossDeath":           flavor.BossDeath,
		"PlayerDeath":         flavor.PlayerDeath,
		"ZoneComplete":        flavor.ZoneComplete,
		"Nat20":               flavor.Nat20,
		"Nat1":                flavor.Nat1,
		"LevelUp":             flavor.LevelUp,
		"ItemFound":           flavor.ItemFound,
		"RestShort":           flavor.RestShort,
		"RestLong":            flavor.RestLong,
		"HomeLongRest":        flavor.HomeLongRest,
		"MistyGreeting":       flavor.MistyGreeting,
		"MistyInsightSuccess": flavor.MistyInsightSuccess,
		"MistySkillFail":      flavor.MistySkillFail,
		"ArinaGreeting":       flavor.ArinaGreeting,
		"ArinaArcanaSuccess":  flavor.ArinaArcanaSuccess,
		"ArinaSkillFail":      flavor.ArinaSkillFail,
		"ExpeditionStart":     flavor.ExpeditionStart,
	}
	for name, pool := range pools {
		if len(pool) == 0 {
			t.Errorf("flavor pool %s is empty — protected file deletion?", name)
		}
		for i, s := range pool {
			if s == "" {
				t.Errorf("flavor.%s[%d] is empty string", name, i)
			}
		}
	}
}

// TestDnDFlavorHelpersReturnNonEmpty: every helper must produce a non-empty
// string when its underlying pool has entries.
func TestDnDFlavorHelpersReturnNonEmpty(t *testing.T) {
	cases := map[string]func() string{
		"dndCombatOpeningLine":       dndCombatOpeningLine,
		"dndZoneCompleteLine":        dndZoneCompleteLine,
		"dndNat20Line":               dndNat20Line,
		"dndNat1Line":                dndNat1Line,
		"dndLevelUpFlavorLine":       dndLevelUpFlavorLine,
		"dndItemFoundLine":           dndItemFoundLine,
		"dndRestShortFlavorLine":     dndRestShortFlavorLine,
		"dndCombatClosingLine_Win":   func() string { return dndCombatClosingLine(true, false) },
		"dndCombatClosingLine_Boss":  func() string { return dndCombatClosingLine(true, true) },
		"dndCombatClosingLine_Death": func() string { return dndCombatClosingLine(false, false) },
		"dndRestLongFlavorLine_Home": func() string { return dndRestLongFlavorLine(true) },
		"dndRestLongFlavorLine_Inn":  func() string { return dndRestLongFlavorLine(false) },
	}
	for name, fn := range cases {
		// Run several times — flavor pulls a random pool member, so a single
		// call can flake if the pool happens to have an empty string. The
		// pool-non-empty test above guards against that, but be belt-and-suspenders.
		for i := 0; i < 10; i++ {
			if got := fn(); got == "" {
				t.Errorf("%s returned empty (call #%d)", name, i)
				break
			}
		}
	}
}

// TestDnDGreetingPoolUnion: combined Misty/Arina greeting pools include both
// legacy openings and the D&D-flavored entries.
func TestDnDGreetingPoolUnion(t *testing.T) {
	mistyPool := dndMistyGreetingPool()
	if len(mistyPool) != len(flavor.MistyGreeting)+len(mistyOpenings) {
		t.Errorf("Misty pool union size = %d; expected %d (flavor %d + legacy %d)",
			len(mistyPool), len(flavor.MistyGreeting)+len(mistyOpenings),
			len(flavor.MistyGreeting), len(mistyOpenings))
	}
	arinaPool := dndArinaGreetingPool()
	if len(arinaPool) != len(flavor.ArinaGreeting)+len(arinaOpenings) {
		t.Errorf("Arina pool union size = %d; expected %d", len(arinaPool),
			len(flavor.ArinaGreeting)+len(arinaOpenings))
	}
}

func TestDnDItalicize(t *testing.T) {
	if got := dndItalicize(""); got != "" {
		t.Errorf("italicize(empty) = %q, want empty", got)
	}
	if got := dndItalicize("  "); got != "" {
		t.Errorf("italicize(whitespace) = %q, want empty", got)
	}
	got := dndItalicize("hello")
	if got != "\n\n_hello_" {
		t.Errorf("italicize = %q, want \\n\\n_hello_", got)
	}
}
