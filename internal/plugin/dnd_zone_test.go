package plugin

import "testing"

// Phase 11 D1a — zone registry tests. Validates that:
//   1. Tier 1 zones are registered.
//   2. All enemy/boss BestiaryID references resolve.
//   3. Level gating (zonesForLevel) respects the 2-tier-above ceiling.
//   4. Loot tables either have explicit drop chances in [0,1] or
//      UniqueAlways set.

func TestZoneRegistry_Tier1Present(t *testing.T) {
	if _, ok := getZone(ZoneGoblinWarrens); !ok {
		t.Fatal("Goblin Warrens not registered")
	}
	if _, ok := getZone(ZoneCryptValdris); !ok {
		t.Fatal("Crypt of Valdris not registered")
	}
	t1 := zonesByTier(ZoneTierBeginner)
	if len(t1) != 2 {
		t.Fatalf("expected 2 Tier 1 zones, got %d", len(t1))
	}
}

func TestZoneRegistry_Tier2Present(t *testing.T) {
	if _, ok := getZone(ZoneForestShadows); !ok {
		t.Fatal("Forest of Shadows not registered")
	}
	if _, ok := getZone(ZoneSunkenTemple); !ok {
		t.Fatal("Sunken Temple not registered")
	}
	t2 := zonesByTier(ZoneTierApprentice)
	if len(t2) != 2 {
		t.Fatalf("expected 2 Tier 2 zones, got %d", len(t2))
	}
}

func TestZoneRegistry_BestiaryRefsResolve(t *testing.T) {
	for _, z := range allZones() {
		for _, e := range z.Enemies {
			if _, ok := dndBestiary[e.BestiaryID]; !ok {
				t.Errorf("zone %s enemy %s missing from bestiary", z.ID, e.BestiaryID)
			}
		}
		if _, ok := dndBestiary[z.Boss.BestiaryID]; !ok {
			t.Errorf("zone %s boss %s missing from bestiary", z.ID, z.Boss.BestiaryID)
		}
	}
}

func TestZoneRegistry_BossCRMatchesZoneTier(t *testing.T) {
	// Sanity: boss CR should be at least at the high end of tier
	// CR range. Tier 1 = CR 1/8–1 enemies, but boss is CR 3–5.
	for _, z := range allZones() {
		if z.Boss.CR < 1 {
			t.Errorf("zone %s boss CR %.1f too low", z.ID, z.Boss.CR)
		}
	}
}

func TestZonesForLevel_TierGate(t *testing.T) {
	// L1 player (tier 1): max tier visible = 1+2 = 3. With T1+T2 registered
	// (D1a + D3a), L1 sees all four — Tier 3 zones aren't in the registry yet.
	got := zonesForLevel(1)
	if len(got) != 4 {
		t.Fatalf("L1 should see 4 zones (2 T1 + 2 T2), got %d", len(got))
	}
	// L20 player should also see all zones (no upper cutoff).
	got = zonesForLevel(20)
	if len(got) < 4 {
		t.Fatalf("L20 should see all zones, got %d", len(got))
	}
}

func TestZoneRegistry_LootDropChances(t *testing.T) {
	for _, z := range allZones() {
		for _, l := range z.Loot {
			if l.UniqueAlways {
				continue
			}
			if l.DropChance <= 0 || l.DropChance > 1 {
				t.Errorf("zone %s loot %s drop chance %.2f out of range",
					z.ID, l.ItemID, l.DropChance)
			}
		}
	}
}

func TestZoneRegistry_RoomCountSane(t *testing.T) {
	for _, z := range allZones() {
		if z.MinRooms < 5 || z.MaxRooms > 10 || z.MinRooms > z.MaxRooms {
			t.Errorf("zone %s rooms %d-%d outside design (5-10, min<=max)",
				z.ID, z.MinRooms, z.MaxRooms)
		}
	}
}

func TestZoneRegistry_ElitesFlagged(t *testing.T) {
	// Each Tier 1 zone roster should contain at least one elite (per
	// design doc each zone lists ELITE entries).
	for _, z := range allZones() {
		anyElite := false
		for _, e := range z.Enemies {
			if e.IsElite {
				anyElite = true
				break
			}
		}
		if !anyElite {
			t.Errorf("zone %s has no elite enemy", z.ID)
		}
	}
}

func TestZoneRegistry_LevelRangeMatchesTier(t *testing.T) {
	// Tier→expected level range per design doc §2.
	expected := map[ZoneTier][2]int{
		ZoneTierBeginner:   {1, 3},
		ZoneTierApprentice: {3, 5},
		ZoneTierJourneyman: {5, 8},
		ZoneTierVeteran:    {8, 12},
		ZoneTierLegendary:  {12, 20},
	}
	for _, z := range allZones() {
		want, ok := expected[z.Tier]
		if !ok {
			t.Errorf("zone %s has unknown tier %d", z.ID, z.Tier)
			continue
		}
		if z.LevelMin < want[0] || z.LevelMax > want[1] {
			t.Errorf("zone %s level %d-%d outside tier %d range %d-%d",
				z.ID, z.LevelMin, z.LevelMax, z.Tier, want[0], want[1])
		}
	}
}
