package plugin

import "testing"

func TestMapLegacySlot(t *testing.T) {
	cases := []struct {
		legacy EquipmentSlot
		want   DnDSlot
	}{
		{SlotWeapon, DnDSlotMainHand},
		{SlotArmor, DnDSlotChest},
		{SlotHelmet, DnDSlotHead},
		{SlotBoots, DnDSlotFeet},
		{SlotTool, DnDSlotOffHand},
	}
	for _, c := range cases {
		if got := mapLegacySlot(c.legacy); got != c.want {
			t.Errorf("mapLegacySlot(%s) = %s, want %s", c.legacy, got, c.want)
		}
	}
}

func TestInferRarity(t *testing.T) {
	cases := []struct {
		tier       int
		masterwork bool
		arenaTier  int
		want       DnDRarity
	}{
		{1, false, 0, RarityCommon},
		{2, false, 0, RarityCommon},
		{3, false, 0, RarityUncommon},
		{4, false, 0, RarityUncommon},
		{5, false, 0, RarityRare},
		{6, false, 0, RarityRare},
		{7, false, 0, RarityEpic},
		{9, false, 0, RarityLegendary},
		// Masterwork bumps to Rare/Epic
		{2, true, 0, RarityRare},
		{6, true, 0, RarityEpic},
		// Arena set
		{3, false, 1, RarityUncommon},
		{3, false, 3, RarityRare},
		{3, false, 4, RarityEpic},
		{3, false, 5, RarityLegendary},
	}
	for _, c := range cases {
		got := inferRarity(c.tier, c.masterwork, c.arenaTier)
		if got != c.want {
			t.Errorf("inferRarity(t=%d mw=%v arena=%d) = %s, want %s",
				c.tier, c.masterwork, c.arenaTier, got, c.want)
		}
	}
}

func TestRarityIcon(t *testing.T) {
	for _, r := range []DnDRarity{RarityCommon, RarityUncommon, RarityRare, RarityEpic, RarityLegendary} {
		if rarityIcon(r) == "" {
			t.Errorf("rarityIcon(%s) returned empty string", r)
		}
	}
	if rarityIcon("Bogus") != "" {
		t.Error("rarityIcon for unknown rarity should be empty")
	}
}

func TestEquipmentRarity_NilSafety(t *testing.T) {
	if got := equipmentRarity(nil); got != RarityCommon {
		t.Errorf("equipmentRarity(nil) = %s, want Common", got)
	}
}

func TestDnDSlotOrderComplete(t *testing.T) {
	// Every D&D slot is in the order list exactly once.
	seen := map[DnDSlot]bool{}
	for _, s := range dndSlotOrder {
		if seen[s] {
			t.Errorf("dndSlotOrder has duplicate %s", s)
		}
		seen[s] = true
	}
	expected := []DnDSlot{
		DnDSlotHead, DnDSlotChest, DnDSlotCloak, DnDSlotLegs, DnDSlotHands, DnDSlotFeet,
		DnDSlotMainHand, DnDSlotOffHand,
		DnDSlotRing1, DnDSlotRing2, DnDSlotAmulet,
	}
	if len(seen) != len(expected) {
		t.Errorf("dndSlotOrder size = %d, want %d", len(seen), len(expected))
	}
}
