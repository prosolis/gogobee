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
		if m.Ability != nil {
			t.Errorf("%s: Ability is set — the generator never wires abilities", id)
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
