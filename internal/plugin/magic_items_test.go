package plugin

import "testing"

// TestMagicItemRegistryPopulated guards the generated SRD dump against an empty
// or structurally broken regeneration.
func TestMagicItemRegistryPopulated(t *testing.T) {
	if len(magicItemRegistry) < 200 {
		t.Fatalf("magic item registry too small: got %d, want >= 200", len(magicItemRegistry))
	}
	for id, mi := range magicItemRegistry {
		if mi.ID != id {
			t.Errorf("registry key %q != item ID %q", id, mi.ID)
		}
		if mi.Name == "" {
			t.Errorf("%s: empty Name", id)
		}
		if mi.Kind == "" {
			t.Errorf("%s: empty Kind", id)
		}
		if mi.Rarity == "" {
			t.Errorf("%s: empty Rarity", id)
		}
		if mi.Value <= 0 {
			t.Errorf("%s: non-positive Value %d", id, mi.Value)
		}
	}
}

// TestMagicItemSpotCheck pins a few classifier outputs so a regeneration that
// changes the heuristics gets caught.
func TestMagicItemSpotCheck(t *testing.T) {
	armor, ok := magicItemRegistry["adamantine_armor"]
	if !ok {
		t.Fatal("adamantine_armor missing from registry")
	}
	if armor.Kind != MagicItemArmor || armor.Rarity != RarityUncommon ||
		armor.Slot != DnDSlotChest || armor.Attunement {
		t.Errorf("adamantine_armor classified wrong: %+v", armor)
	}

	amulet, ok := magicItemRegistry["amulet_of_health"]
	if !ok {
		t.Fatal("amulet_of_health missing from registry")
	}
	if amulet.Kind != MagicItemWondrous || amulet.Rarity != RarityRare ||
		amulet.Slot != DnDSlotAmulet || !amulet.Attunement {
		t.Errorf("amulet_of_health classified wrong: %+v", amulet)
	}
}

// TestMagicItemOverlayWins verifies the hand-authored overlay layers on top of
// the generated dump and wins on ID collision. It exercises the merge with a
// throwaway item so the assertion holds even while magicItemOverlay is empty.
func TestMagicItemOverlayWins(t *testing.T) {
	saved := magicItemOverlay
	defer func() {
		magicItemOverlay = saved
		magicItemRegistry = buildMagicItemRegistry()
	}()

	magicItemOverlay = []MagicItem{
		{ID: "adamantine_armor", Name: "Hand-Tuned Plate", Kind: MagicItemArmor,
			Rarity: RarityLegendary, Value: 9999},
	}
	magicItemRegistry = buildMagicItemRegistry()

	got := magicItemRegistry["adamantine_armor"]
	if got.Name != "Hand-Tuned Plate" || got.Rarity != RarityLegendary {
		t.Errorf("overlay did not win on ID collision: %+v", got)
	}
}
