package plugin

import (
	"math/rand/v2"
	"testing"
)

// ── §5 Drop tier brackets ──────────────────────────────────────────────────

func TestDropTierFromCR_Brackets(t *testing.T) {
	cases := []struct {
		name           string
		cr             float32
		isBoss         bool
		wantGuaranteed LootTier
		wantBonus      LootTier
		wantDropChance float64
	}{
		{"low cr no guarantee", 0.5, false, LootTierCommon, "", 0.70},
		{"cr 1 still common only", 1.0, false, LootTierCommon, "", 0.70},
		{"cr 2 guarantees common", 2.0, false, LootTierCommon, LootTierUncommon, 1.0},
		{"cr 5 uncommon", 5.0, false, LootTierUncommon, LootTierRare, 1.0},
		{"cr 9 rare", 9.0, false, LootTierRare, LootTierEpic, 1.0},
		{"cr 13 epic", 13.0, false, LootTierEpic, LootTierLegendary, 1.0},
		{"boss min rare", 4.0, true, LootTierRare, LootTierEpic, 1.0},
		{"high cr boss still rare floor", 20.0, true, LootTierRare, LootTierEpic, 1.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, b, _, dc := dropTierFromCR(c.cr, c.isBoss)
			if g != c.wantGuaranteed || b != c.wantBonus || dc != c.wantDropChance {
				t.Errorf("got (%s,%s,%g) want (%s,%s,%g)",
					g, b, dc, c.wantGuaranteed, c.wantBonus, c.wantDropChance)
			}
		})
	}
}

// ── Loot table coverage ─────────────────────────────────────────────────────

func TestZoneLootTables_AllZonesAllTiers(t *testing.T) {
	zones := []ZoneID{
		ZoneGoblinWarrens, ZoneCryptValdris, ZoneForestShadows,
		ZoneSunkenTemple, ZoneManorBlackspire, ZoneUnderforge,
		ZoneUnderdark, ZoneFeywildCrossing, ZoneDragonsLair, ZoneAbyssPortal,
	}
	tiers := []LootTier{
		LootTierCommon, LootTierUncommon, LootTierRare, LootTierEpic, LootTierLegendary,
	}
	for _, z := range zones {
		table, ok := zoneLootTables[z]
		if !ok {
			t.Errorf("zone %s missing loot table", z)
			continue
		}
		for _, tier := range tiers {
			if len(table[tier]) == 0 {
				t.Errorf("zone %s has empty %s slate", z, tier)
			}
		}
	}
}

func TestZoneLootTables_SellValuesInRarityBrackets(t *testing.T) {
	bands := map[LootTier][2]int{
		LootTierCommon:    {5, 15},
		LootTierUncommon:  {25, 60},
		LootTierRare:      {100, 300},
		LootTierEpic:      {500, 1200},
		LootTierLegendary: {3000, 10000},
	}
	for zid, table := range zoneLootTables {
		for tier, slate := range table {
			lo, hi := bands[tier][0], bands[tier][1]
			for _, e := range slate {
				if e.BaseValue < lo || e.BaseValue > hi {
					t.Errorf("zone %s tier %s item %s value %d outside [%d,%d]",
						zid, tier, e.Name, e.BaseValue, lo, hi)
				}
			}
		}
	}
}

// ── rollZoneLoot behavior ───────────────────────────────────────────────────

func TestRollZoneLoot_LowCRRespectsDropChance(t *testing.T) {
	// CR 0.5 has 70% drop chance — over many rolls we should see misses.
	rng := rand.New(rand.NewPCG(1, 2))
	misses := 0
	hits := 0
	for i := 0; i < 1000; i++ {
		_, _, ok := rollZoneLoot(ZoneGoblinWarrens, 0.5, false, rng)
		if ok {
			hits++
		} else {
			misses++
		}
	}
	if misses == 0 {
		t.Errorf("expected some misses at CR 0.5, got 0/%d", hits+misses)
	}
	if hits == 0 {
		t.Errorf("expected some hits at CR 0.5, got 0/%d", hits+misses)
	}
}

func TestRollZoneLoot_BossAlwaysDropsRareOrBetter(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 9))
	for i := 0; i < 200; i++ {
		_, tier, ok := rollZoneLoot(ZoneAbyssPortal, 20, true, rng)
		if !ok {
			t.Fatalf("boss drop missed (i=%d)", i)
		}
		if tier != LootTierRare && tier != LootTierEpic {
			t.Fatalf("boss tier = %s, want Rare or Epic", tier)
		}
	}
}

func TestRollZoneLoot_HighCROccasionallyUpgradesToLegendary(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 13))
	saw := false
	for i := 0; i < 5000; i++ {
		_, tier, ok := rollZoneLoot(ZoneDragonsLair, 15, false, rng)
		if ok && tier == LootTierLegendary {
			saw = true
			break
		}
	}
	if !saw {
		t.Errorf("expected at least one Legendary upgrade at CR 15 over 5000 rolls")
	}
}

func TestRollZoneLoot_UnknownZoneReturnsFalse(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	_, _, ok := rollZoneLoot(ZoneID("nonexistent_zone"), 5, false, rng)
	if ok {
		t.Errorf("expected ok=false for unknown zone")
	}
}

// ── Zone-contextual flavor ──────────────────────────────────────────────────

func TestZoneItemDescription_PotionOfHealingAllZones(t *testing.T) {
	zones := []ZoneID{
		ZoneGoblinWarrens, ZoneCryptValdris, ZoneForestShadows,
		ZoneSunkenTemple, ZoneManorBlackspire, ZoneUnderforge,
		ZoneUnderdark, ZoneFeywildCrossing, ZoneDragonsLair, ZoneAbyssPortal,
	}
	for _, z := range zones {
		desc := zoneItemDescription(z, "Potion of Healing")
		if desc == "" {
			t.Errorf("zone %s has no Potion of Healing flavor", z)
		}
	}
}

func TestZoneItemDescription_NonMatchingItemReturnsEmpty(t *testing.T) {
	if got := zoneItemDescription(ZoneGoblinWarrens, "Random Junk"); got != "" {
		t.Errorf("expected empty for unmapped item, got %q", got)
	}
}

func TestLootFlavorLine_AllTiersPickSomething(t *testing.T) {
	for _, tier := range []LootTier{LootTierCommon, LootTierUncommon, LootTierRare, LootTierEpic, LootTierLegendary} {
		if got := lootFlavorLine(tier); got == "" {
			t.Errorf("tier %s produced empty flavor line", tier)
		}
	}
}
