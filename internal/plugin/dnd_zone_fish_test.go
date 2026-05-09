package plugin

import (
	"math/rand/v2"
	"testing"
)

// ── §6.1 Fishing zone gate ─────────────────────────────────────────────────

func TestIsFishingZone_AllowList(t *testing.T) {
	allowed := map[ZoneID]bool{
		ZoneForestShadows:   true,
		ZoneSunkenTemple:    true,
		ZoneUnderdark:       true,
		ZoneFeywildCrossing: true,
	}
	all := []ZoneID{
		ZoneGoblinWarrens, ZoneCryptValdris, ZoneForestShadows,
		ZoneSunkenTemple, ZoneManorBlackspire, ZoneUnderforge,
		ZoneUnderdark, ZoneFeywildCrossing, ZoneDragonsLair, ZoneAbyssPortal,
	}
	for _, z := range all {
		if got, want := isFishingZone(z), allowed[z]; got != want {
			t.Errorf("isFishingZone(%s) = %v, want %v", z, got, want)
		}
	}
}

// ── §6.2 fishing skill bonus folds in legacy rank ──────────────────────────

func TestFishingSkillBonus_Bands(t *testing.T) {
	cases := []struct {
		skill int
		want  int
	}{
		{0, 0}, {1, 0}, {4, 0}, {5, 1}, {9, 1}, {10, 2}, {15, 2}, {99, 2},
	}
	for _, c := range cases {
		if got := fishingSkillBonus(c.skill); got != c.want {
			t.Errorf("fishingSkillBonus(%d) = %d, want %d", c.skill, got, c.want)
		}
	}
}

// ── §6.2 Ranger +20% rare-catch upgrade ────────────────────────────────────

func TestRangerRareCatchUpgrade_OnlyAppliesToFish(t *testing.T) {
	// Non-fish action: never upgrade, even for rangers.
	rng := rand.New(rand.NewPCG(1, 1))
	for i := 0; i < 100; i++ {
		got := rangerRareCatchUpgrade(ClassRanger, HarvestForage, OutcomeStandard, rng)
		if got != OutcomeStandard {
			t.Fatalf("non-fish action upgraded: %s", got)
		}
	}
	// Non-ranger fish: never upgrade.
	rng = rand.New(rand.NewPCG(2, 2))
	for i := 0; i < 100; i++ {
		got := rangerRareCatchUpgrade(ClassFighter, HarvestFish, OutcomeStandard, rng)
		if got != OutcomeStandard {
			t.Fatalf("non-ranger upgraded: %s", got)
		}
	}
	// Ranger fish + Rich/Nothing: passes through unchanged.
	rng = rand.New(rand.NewPCG(3, 3))
	for i := 0; i < 100; i++ {
		if got := rangerRareCatchUpgrade(ClassRanger, HarvestFish, OutcomeRich, rng); got != OutcomeRich {
			t.Fatalf("Rich changed: %s", got)
		}
		if got := rangerRareCatchUpgrade(ClassRanger, HarvestFish, OutcomeNothing, rng); got != OutcomeNothing {
			t.Fatalf("Nothing changed: %s", got)
		}
	}
}

func TestRangerRareCatchUpgrade_RoughRate(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 7))
	const trials = 4000
	upgrades := 0
	for i := 0; i < trials; i++ {
		out := rangerRareCatchUpgrade(ClassRanger, HarvestFish, OutcomeStandard, rng)
		if out == OutcomeRich {
			upgrades++
		}
	}
	rate := float64(upgrades) / float64(trials)
	if rate < 0.15 || rate > 0.25 {
		t.Errorf("upgrade rate %.3f outside expected ~0.20 ±0.05", rate)
	}
}

// ── §6.1 Feywild Time Distortion narration hook ─────────────────────────────

func TestFeywildFishDistortion_OnlyFiresInFeywildOnFish(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 11))
	for i := 0; i < 200; i++ {
		if got := feywildFishDistortion(ZoneForestShadows, HarvestFish, rng); got != "" {
			t.Fatalf("non-feywild fired: %q", got)
		}
		if got := feywildFishDistortion(ZoneFeywildCrossing, HarvestForage, rng); got != "" {
			t.Fatalf("non-fish action fired: %q", got)
		}
	}
}

// ── §3 fish entries present in registry per §6.1 zones ─────────────────────

func TestZoneFishRegistry_Coverage(t *testing.T) {
	want := map[ZoneID][]string{
		ZoneForestShadows:   {"shadow_trout", "gloomfin", "phantom_carp"},
		ZoneSunkenTemple:    {"black_pearl", "blind_cave_fish", "merrow_eel", "dareth_lanternfish", "aboleth_spawn"},
		ZoneUnderdark:       {"blind_albino_bass", "glowing_cave_eel", "underdark_shark", "eyeless_king"},
		ZoneFeywildCrossing: {"moonlit_minnow", "dreaming_pike", "timeworn_koi"},
	}
	for zid, ids := range want {
		seen := map[string]bool{}
		for _, r := range ZoneResourcesByAction(zid, HarvestFish) {
			seen[r.ID] = true
		}
		for _, id := range ids {
			if !seen[id] {
				t.Errorf("zone %s missing fish entry %s", zid, id)
			}
		}
	}
	// And: zones not on the §6.1 list should have no fish entries.
	noFish := []ZoneID{
		ZoneGoblinWarrens, ZoneCryptValdris, ZoneManorBlackspire,
		ZoneUnderforge, ZoneDragonsLair, ZoneAbyssPortal,
	}
	for _, z := range noFish {
		if got := ZoneResourcesByAction(z, HarvestFish); len(got) != 0 {
			t.Errorf("zone %s has %d fish entries, want 0", z, len(got))
		}
	}
}

// ── §5 zone-contextual item flavor matrix ──────────────────────────────────

func TestZoneItemDescription_LootMatrixCoverage(t *testing.T) {
	// Every zone's loot table entry name should resolve to either a
	// flavor line or the Potion of Healing exemplar — but the specific
	// invariant we want is: every zone has *some* per-item flavor populated
	// beyond just Potion of Healing.
	zones := []ZoneID{
		ZoneGoblinWarrens, ZoneCryptValdris, ZoneForestShadows,
		ZoneSunkenTemple, ZoneManorBlackspire, ZoneUnderforge,
		ZoneUnderdark, ZoneFeywildCrossing, ZoneDragonsLair, ZoneAbyssPortal,
	}
	for _, z := range zones {
		zm, ok := zoneItemFlavor[z]
		if !ok || len(zm) == 0 {
			t.Errorf("zone %s: no item flavor matrix populated", z)
			continue
		}
		// Every flavor entry should resolve via the public path.
		for name, want := range zm {
			if got := zoneItemDescription(z, name); got != want {
				t.Errorf("zone %s item %q: zoneItemDescription mismatch", z, name)
			}
		}
	}
}

func TestZoneItemDescription_MissingItemReturnsEmpty(t *testing.T) {
	if got := zoneItemDescription(ZoneGoblinWarrens, "Nonexistent Item"); got != "" {
		t.Errorf("got %q, want empty for unknown item", got)
	}
}

// ── Fish entries respect §8.1 sell-value bands ──────────────────────────────

func TestZoneFish_SellValueBands(t *testing.T) {
	bands := map[DnDRarity][2]int{
		RarityCommon:    {3, 20},
		RarityUncommon:  {25, 70},
		RarityRare:      {100, 350},
		RarityVeryRare:  {500, 1500},
		RarityLegendary: {3000, 10000},
	}
	zones := []ZoneID{
		ZoneForestShadows, ZoneSunkenTemple, ZoneUnderdark, ZoneFeywildCrossing,
	}
	for _, z := range zones {
		for _, r := range ZoneResourcesByAction(z, HarvestFish) {
			band, ok := bands[r.Rarity]
			if !ok {
				t.Errorf("fish %s has unhandled rarity %s", r.ID, r.Rarity)
				continue
			}
			if r.SellValue < band[0] || r.SellValue > band[1] {
				t.Errorf("fish %s sell %d outside band %v for %s", r.ID, r.SellValue, band, r.Rarity)
			}
		}
	}
}
