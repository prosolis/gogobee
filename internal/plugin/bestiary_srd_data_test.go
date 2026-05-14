package plugin

import "testing"

// TestSRDStagingBestiary_Populated guards the generated raw SRD staging table:
// the full SRD dump should be present and structurally sane. Regenerate with
// `go run ./cmd/open5e-import (fetch|gen) bestiary` if the SRD set changes.
func TestSRDStagingBestiary_Populated(t *testing.T) {
	if len(srdStagingBestiary) < 300 {
		t.Fatalf("srdStagingBestiary has %d entries, want the full SRD set (~322)", len(srdStagingBestiary))
	}

	for slug, m := range srdStagingBestiary {
		if m.Slug != slug {
			t.Errorf("%s: Slug field = %q, want map key", slug, m.Slug)
		}
		if m.Name == "" {
			t.Errorf("%s: empty Name", slug)
		}
		if m.HP <= 0 || m.AC <= 0 {
			t.Errorf("%s: HP=%d AC=%d, want both positive", slug, m.HP, m.AC)
		}
		for _, a := range m.Attacks {
			// AvgDamage is precomputed; a parsed attack should carry dice and a
			// non-negative average (flat negative bonuses can zero it, not less).
			if a.DamageDice == "" {
				t.Errorf("%s: attack %q has no DamageDice", slug, a.Name)
			}
			if a.AvgDamage < 0 {
				t.Errorf("%s: attack %q AvgDamage=%d, want >= 0", slug, a.Name, a.AvgDamage)
			}
		}
	}
}

// TestSRDStagingBestiary_KnownEntry spot-checks a canonical stat block so a
// classifier regression surfaces as a value mismatch, not a silent drift.
func TestSRDStagingBestiary_KnownEntry(t *testing.T) {
	ab, ok := srdStagingBestiary["aboleth"]
	if !ok {
		t.Fatal("aboleth missing from staging bestiary")
	}
	if ab.HP != 135 || ab.AC != 17 || ab.CR != 10 {
		t.Errorf("aboleth = HP %d / AC %d / CR %v, want 135 / 17 / 10", ab.HP, ab.AC, ab.CR)
	}
	if ab.XP != 5900 {
		t.Errorf("aboleth XP = %d, want 5900 (derived from CR 10)", ab.XP)
	}
	if ab.Multiattack == "" {
		t.Error("aboleth should have a Multiattack description")
	}
	if !ab.Legendary {
		t.Error("aboleth should be flagged Legendary")
	}
	// Tentacle: +9 to hit, 2d6+5 → avg 12.
	var tentacle *SRDStatAttack
	for i := range ab.Attacks {
		if ab.Attacks[i].Name == "Tentacle" {
			tentacle = &ab.Attacks[i]
		}
	}
	if tentacle == nil {
		t.Fatal("aboleth missing Tentacle attack")
	}
	if tentacle.AttackBonus != 9 || tentacle.DamageDice != "2d6" || tentacle.AvgDamage != 12 {
		t.Errorf("aboleth Tentacle = %+v, want +9 / 2d6 / avg 12", *tentacle)
	}
}
