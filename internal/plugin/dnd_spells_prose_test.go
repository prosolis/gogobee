package plugin

import (
	"regexp"
	"testing"
)

// TestSpellDescriptionsAreJargonFree scans every reachable spell Description
// in the merged registry (SRD + hand-authored overlay) for D&D jargon that
// leaks into player-facing chat via dnd_cast.go's !cast confirmation and the
// !spellbook list view. Per feedback_accessibility_over_dnd_crunch, the
// player surface should be verb/outcome-led — no math, no dice, no saves.
//
// Failures usually mean either (a) a new SRD entry was added without an
// overlay shim in dnd_spells_data.go, or (b) an overlay Description was
// edited back into crunch. Fix in dnd_spells_data.go; do NOT touch the
// generated dnd_spells_srd_data.go directly (it's regenerated from Open5e).
//
// Upcast strings are NOT tested — they're never displayed (grep
// `\.Upcast` across the codebase to confirm; only the import generator
// reads them as of 2026-05-15).
func TestSpellDescriptionsAreJargonFree(t *testing.T) {
	bans := []struct {
		name string
		re   *regexp.Regexp
	}{
		{"saving throw", regexp.MustCompile(`(?i)saving throw`)},
		{"spell slot of", regexp.MustCompile(`(?i)spell slot of`)},
		{"ability modifier", regexp.MustCompile(`(?i)ability modifier`)},
		{"hit points equal to", regexp.MustCompile(`(?i)hit points equal to`)},
		{"NdN dice notation", regexp.MustCompile(`\b\d+d\d+\b`)},
		{"bare die notation", regexp.MustCompile(`\bd(4|6|8|10|12|20|100)\b`)},
	}

	for id, s := range dndSpellRegistry {
		if s.Description == "" {
			continue
		}
		for _, b := range bans {
			if b.re.MatchString(s.Description) {
				t.Errorf("spell %q Description leaks %q jargon: %q",
					id, b.name, s.Description)
			}
		}
	}
}
