package plugin

import "testing"

func TestBestiaryAllWellFormed(t *testing.T) {
	if len(dndBestiary) == 0 {
		t.Fatal("dndBestiary empty")
	}
	for id, m := range dndBestiary {
		if m.ID != id {
			t.Errorf("%s: ID mismatch (%s)", id, m.ID)
		}
		if m.Name == "" {
			t.Errorf("%s: empty Name", id)
		}
		if m.HP < 1 {
			t.Errorf("%s: HP=%d", id, m.HP)
		}
		if m.AC < 10 {
			t.Errorf("%s: AC=%d (want ≥ 10)", id, m.AC)
		}
		if m.Attack < 1 {
			t.Errorf("%s: Attack=%d", id, m.Attack)
		}
		if m.XPValue < 0 {
			t.Errorf("%s: XPValue=%d", id, m.XPValue)
		}
	}
}

func TestBestiaryByCR(t *testing.T) {
	low := dndBestiaryByCR(1.0)
	for _, m := range low {
		if m.CR > 1.0 {
			t.Errorf("CR filter leaked: %s CR=%v", m.Name, m.CR)
		}
	}
	// Cap above CR 24 to cover Infernax (Ancient Red Dragon, T5 boss).
	high := dndBestiaryByCR(30.0)
	if len(high) != len(dndBestiary) {
		t.Errorf("CR≤30 filter returned %d, want all %d", len(high), len(dndBestiary))
	}
}

func TestBestiaryToCombatStats(t *testing.T) {
	dragon := dndBestiary["ancient_dragon"]
	stats, mods := dragon.toCombatStats()
	if stats.MaxHP != 367 {
		t.Errorf("dragon HP = %d, want 367", stats.MaxHP)
	}
	if stats.AC != 22 {
		t.Errorf("dragon AC = %d, want 22", stats.AC)
	}
	if stats.AttackBonus != 11 {
		t.Errorf("dragon AttackBonus = %d, want 11 (capped at 11 by 2026-05-10 rebalance)", stats.AttackBonus)
	}
	if mods.DamageReduct != 1.0 {
		t.Errorf("DamageReduct = %v, want 1.0", mods.DamageReduct)
	}
}

func TestBestiaryStarterMonsters(t *testing.T) {
	// Section 8.2 of v1.0 lists 6 starter monsters; verify all present.
	wantIDs := []string{"goblin", "skeleton", "orc_grunt", "troll", "wyvern", "ancient_dragon"}
	for _, id := range wantIDs {
		if _, ok := dndBestiary[id]; !ok {
			t.Errorf("starter bestiary missing: %s", id)
		}
	}
}
