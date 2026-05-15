package main

import (
	"strings"
	"testing"
)

// TestCleanDescStripsJargon — the S3 acceptance criteria translated into
// regression cases. Any phrase listed here must not survive the sanitizer.
func TestCleanDescStripsJargon(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		mustNot []string
	}{
		{
			name: "saving throw + DC",
			in:   "Each creature in the area must make a Dexterity saving throw (DC 15) or take damage.",
			mustNot: []string{
				"saving throw", "DC 15", "DC15",
			},
		},
		{
			name:    "saving throws plural",
			in:      "You have advantage on saving throws against spells.",
			mustNot: []string{"saving throws"},
		},
		{
			name:    "within range",
			in:      "Hurl a bolt at a creature within range.",
			mustNot: []string{"within range"},
		},
		{
			name:    "5-foot radius",
			in:      "Each creature in a 5-foot radius takes damage.",
			mustNot: []string{"5-foot"},
		},
		{
			name:    "spell slot upcast clause",
			in:      "When you cast this spell using a spell slot of 2nd level or higher, the damage increases.",
			mustNot: []string{"spell slot"},
		},
		{
			name:    "constitution score",
			in:      "Your Constitution score is 19 while you wear this amulet.",
			mustNot: []string{"Constitution score"},
		},
		{
			name:    "(save DC X) parenthetical",
			in:      "You can use an action to cast the detect thoughts spell (save DC 13) from it.",
			mustNot: []string{"(save", "DC 13"},
		},
		{
			name:    "wisdom save",
			in:      "Forces a wisdom save against the caster.",
			mustNot: []string{"wisdom save"},
		},
		{
			name:    "out to a range of feet",
			in:      "You have darkvision out to a range of 60 feet.",
			mustNot: []string{"feet", "out to a range"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanDesc(tc.in)
			low := strings.ToLower(got)
			for _, ban := range tc.mustNot {
				if strings.Contains(low, strings.ToLower(ban)) {
					t.Errorf("cleanDesc kept banned phrase %q\n  in:  %q\n  out: %q", ban, tc.in, got)
				}
			}
		})
	}
}

// TestCleanDescRepairsOrphans — the strippers leave dangling stubs ("must
// make.", " and."); the post-pass is meant to tidy them so player-facing
// prose doesn't end mid-clause.
func TestCleanDescRepairsOrphans(t *testing.T) {
	in := "You gain a +1 bonus to ability checks and saving throws."
	got := cleanDesc(in)
	if strings.HasSuffix(got, " and.") {
		t.Errorf("orphan trailing 'and.' survived: %q", got)
	}
	in2 := "Up to three creatures of your choice that you can see must make charisma saving throws."
	got2 := cleanDesc(in2)
	if strings.Contains(strings.ToLower(got2), "must make.") {
		t.Errorf("orphan 'must make.' survived: %q", got2)
	}
}

// TestStripNameParenthetical — bestiary R21 / magic-item alias names.
func TestStripNameParenthetical(t *testing.T) {
	cases := map[string]string{
		"Giant Rat (Diseased)":          "Giant Rat",
		"Deep Gnome (Svirfneblin)":      "Deep Gnome",
		"Stone of Good Luck (Luckstone)": "Stone of Good Luck",
		"Ordinary Name":                 "Ordinary Name",
		"":                              "",
	}
	for in, want := range cases {
		if got := stripNameParenthetical(in); got != want {
			t.Errorf("stripNameParenthetical(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSpellOverrideWins — the curated descriptions for the SRD-only default
// spells must beat the auto first-sentence path.
func TestSpellOverrideWins(t *testing.T) {
	got := spellDescription("vicious_mockery", "anything could go here")
	if got != spellDescOverride["vicious_mockery"] {
		t.Errorf("override didn't win: got %q", got)
	}
	// Unknown ID falls through to cleanDesc.
	in := "Hurl a bolt at a creature within range."
	got = spellDescription("unknown_spell_xyz", in)
	if strings.Contains(got, "within range") {
		t.Errorf("fallthrough didn't sanitize: %q", got)
	}
}
