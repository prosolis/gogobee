package plugin

import (
	"strings"
	"testing"
)

// N1/A2 — the masterwork catalog is keyed to gathering activities, so the
// zone-combat seam had nothing to look up. masterworkDefForZone bridges it.

func TestMasterworkDefForZone_CoversEveryTier(t *testing.T) {
	for tier := 1; tier <= 5; tier++ {
		if def := masterworkDefForZone(tier); def == nil {
			t.Errorf("tier %d has no masterwork def", tier)
		} else if def.Tier != tier {
			t.Errorf("tier %d returned a tier-%d def", tier, def.Tier)
		}
	}
}

// A dungeon drop must be able to land in any of the three slots — otherwise
// zone play would only ever upgrade one piece of gear.
func TestMasterworkDefForZone_RollsAllSlots(t *testing.T) {
	seen := map[EquipmentSlot]bool{}
	for i := 0; i < 300; i++ {
		if def := masterworkDefForZone(3); def != nil {
			seen[def.Slot] = true
		}
	}
	for _, slot := range []EquipmentSlot{SlotWeapon, SlotArmor, SlotBoots} {
		if !seen[slot] {
			t.Errorf("slot %v never rolled in 300 draws", slot)
		}
	}
}

// The pre-existing lookup must keep working for the gathering activities.
func TestMasterworkDefFor_GatheringUnchanged(t *testing.T) {
	for _, act := range masterworkSlotActivities {
		for tier := 1; tier <= 5; tier++ {
			def := masterworkDefFor(act, tier)
			if def == nil {
				t.Fatalf("%s tier %d lost its def", act, tier)
			}
			if def.Activity != act {
				t.Errorf("%s tier %d returned activity %s", act, tier, def.Activity)
			}
		}
	}
}

// Flavor follows the place, not the catalog line. A crypt boss must not
// narrate a pickaxe striking ore.
func TestMasterworkDropFlavorText_DungeonHasItsOwnVoice(t *testing.T) {
	gatheringWords := []string{"pickaxe", "fishing", "foraging", "vein", "ore", "branch"}
	for tier := 1; tier <= 5; tier++ {
		text := masterworkDropFlavorText(AdvActivityDungeon, tier)
		if text == "" {
			t.Fatalf("dungeon tier %d has no flavor", tier)
		}
		lower := strings.ToLower(text)
		for _, w := range gatheringWords {
			if strings.Contains(lower, w) {
				t.Errorf("dungeon tier %d flavor leaks gathering word %q", tier, w)
			}
		}
	}
}
