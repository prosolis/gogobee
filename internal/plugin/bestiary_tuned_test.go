package plugin

import "testing"

// TestTunedBestiarySRD_Populated guards the generated tuned table: every SRD
// monster should derive a structurally sane DnDMonsterTemplate. Regenerate with
// `go run ./cmd/open5e-import gen tuned` if the SRD set or formula changes.
func TestTunedBestiarySRD_Populated(t *testing.T) {
	if len(tunedBestiarySRD) < 300 {
		t.Fatalf("tunedBestiarySRD has %d entries, want the full SRD set (~322)", len(tunedBestiarySRD))
	}
	for id, m := range tunedBestiarySRD {
		if m.ID != id {
			t.Errorf("%s: ID field = %q, want map key", id, m.ID)
		}
		if m.Name == "" {
			t.Errorf("%s: empty Name", id)
		}
		if m.HP <= 0 {
			t.Errorf("%s: HP=%d, want positive", id, m.HP)
		}
		if m.AC < 10 {
			t.Errorf("%s: AC=%d, want >= 10 (engine minimum)", id, m.AC)
		}
		if m.Attack < 1 {
			t.Errorf("%s: Attack=%d, want >= 1", id, m.Attack)
		}
		if m.Speed < 6 || m.Speed > 18 {
			t.Errorf("%s: Speed=%d, want within [6,18]", id, m.Speed)
		}
		// Ability is optional (nil when the creature's traits are all
		// non-combat), but a wired one must be structurally sane.
		if m.Ability != nil {
			a := m.Ability
			if a.Name == "" {
				t.Errorf("%s: Ability has empty Name", id)
			}
			if !tunedAbilityPhases[a.Phase] {
				t.Errorf("%s: Ability.Phase=%q, want a known combat phase", id, a.Phase)
			}
			if a.ProcChance <= 0 || a.ProcChance > 1 {
				t.Errorf("%s: Ability.ProcChance=%v, want within (0,1]", id, a.ProcChance)
			}
			if !tunedAbilityEffects[a.Effect] {
				t.Errorf("%s: Ability.Effect=%q, want an engine-recognised effect", id, a.Effect)
			}
		}
	}
}

// tunedAbilityPhases / tunedAbilityEffects are the values the trait classifier
// (abilityFromTraits in cmd/open5e-import/tuned.go) is allowed to emit. The
// effect set is the subset of applyAbility's vocabulary the classifier uses;
// if a new trait rule introduces another effect, add it here too.
var tunedAbilityPhases = map[string]bool{
	"opening": true, "clash": true, "decisive": true, "any": true,
}

var tunedAbilityEffects = map[string]bool{
	"death_aoe": true, "aoe": true, "survive_at_1": true, "regenerate": true,
	"spell_resist": true, "bonus_damage": true, "stun": true, "enrage": true,
	"retaliate": true, "advantage": true, "evade": true,
}

// TestTunedBestiarySRD_AbilityWiring spot-checks the trait→ability classifier:
// a creature with a combat trait gets the expected effect, and one whose traits
// are all non-combat stays nil.
func TestTunedBestiarySRD_AbilityWiring(t *testing.T) {
	cases := []struct {
		id     string
		effect string // "" means Ability should be nil
	}{
		{"troll", "regenerate"}, // Regeneration
		{"goblin", "evade"},     // Nimble Escape
		{"balor", "death_aoe"},  // Death Throes (priority over Magic Resistance)
		{"lich", "aoe"},         // Spellcasting
		{"awakened_shrub", ""},  // False Appearance — no rule, stays nil
		{"badger", ""},          // Keen Smell — non-combat
	}
	for _, tc := range cases {
		m, ok := tunedBestiarySRD[tc.id]
		if !ok {
			t.Errorf("%s missing from tuned bestiary", tc.id)
			continue
		}
		switch {
		case tc.effect == "" && m.Ability != nil:
			t.Errorf("%s: Ability=%+v, want nil", tc.id, m.Ability)
		case tc.effect != "" && m.Ability == nil:
			t.Errorf("%s: Ability is nil, want effect %q", tc.id, tc.effect)
		case tc.effect != "" && m.Ability.Effect != tc.effect:
			t.Errorf("%s: Ability.Effect=%q, want %q", tc.id, m.Ability.Effect, tc.effect)
		}
	}
}

// TestTunedBestiarySRD_MergedIntoRoster confirms the init-time merge ran and
// that hand-authored dndBestiary entries win over the generated baseline.
func TestTunedBestiarySRD_MergedIntoRoster(t *testing.T) {
	// A tuned-only creature (no hand-authored entry) should be in the roster.
	if _, ok := dndBestiary["tarrasque"]; !ok {
		t.Error("tarrasque missing from dndBestiary — tuned merge did not run")
	}
	// Owlbear is hand-authored; the roster entry must keep the playtested
	// values (and its wired Ability), not the generated baseline.
	ob, ok := dndBestiary["owlbear"]
	if !ok {
		t.Fatal("owlbear missing from dndBestiary")
	}
	if ob.Ability == nil {
		t.Error("owlbear lost its hand-authored Ability — tuned baseline clobbered the roster entry")
	}
}

// TestAttackByCR_KnownAnchors spot-checks the Attack curve against playtested
// roster values so a formula regression surfaces as a mismatch.
func TestTunedBestiarySRD_KnownEntry(t *testing.T) {
	ab, ok := tunedBestiarySRD["aboleth"]
	if !ok {
		t.Fatal("aboleth missing from tuned bestiary")
	}
	// HP/AC verbatim SRD; Attack from the CR-10 anchor; AttackBonus from the
	// hardest-hitting attack (Tentacle, +9).
	if ab.HP != 135 || ab.AC != 17 || ab.Attack != 16 || ab.AttackBonus != 9 {
		t.Errorf("aboleth = HP %d / AC %d / Attack %d / AB %d, want 135 / 17 / 16 / 9",
			ab.HP, ab.AC, ab.Attack, ab.AttackBonus)
	}
	if ab.XPValue != 5900 {
		t.Errorf("aboleth XPValue = %d, want 5900", ab.XPValue)
	}
}
