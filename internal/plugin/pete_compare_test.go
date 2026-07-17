package plugin

import (
	"testing"

	"gogobee/internal/peteclient"
)

// weapon/ring/wondrous helpers build a MagicItem the codified effect formula
// (magicItemEffectFor) can score without touching the DB.
func mkItem(kind MagicItemKind, rarity DnDRarity, slot DnDSlot) MagicItem {
	return MagicItem{ID: string(kind) + "_" + string(rarity), Kind: kind, Rarity: rarity, Slot: slot}
}

// TestMagicItemDeltasDirection pins the "which way is better" call for each stat,
// including DamageReductMult where LOWER is the improvement.
func TestMagicItemDeltasDirection(t *testing.T) {
	neutral := magicItemEffect{DamageReductMult: 1.0}

	t.Run("more damage is better", func(t *testing.T) {
		d := magicItemDeltas(magicItemEffect{DamageBonus: 0.15, DamageReductMult: 1.0}, magicItemEffect{DamageBonus: 0.10, DamageReductMult: 1.0})
		if len(d) != 1 || d[0].Label != "damage" || !d[0].Better || d[0].Text != "+5% damage" {
			t.Fatalf("got %+v", d)
		}
	})

	t.Run("less damage taken is better", func(t *testing.T) {
		// cand mult 0.90 (blocks 10%) vs worn neutral 1.0 (blocks nothing).
		d := magicItemDeltas(magicItemEffect{DamageReductMult: 0.90}, neutral)
		if len(d) != 1 || d[0].Label != "defense" || !d[0].Better || d[0].Text != "-10% damage taken" {
			t.Fatalf("got %+v", d)
		}
	})

	t.Run("weaker armor reads as worse", func(t *testing.T) {
		// cand blocks less than worn: mult rises, damage taken goes up.
		d := magicItemDeltas(magicItemEffect{DamageReductMult: 0.96}, magicItemEffect{DamageReductMult: 0.90})
		if len(d) != 1 || d[0].Better || d[0].Text != "+6% damage taken" {
			t.Fatalf("got %+v", d)
		}
	})

	t.Run("hp and opening damage", func(t *testing.T) {
		d := magicItemDeltas(
			magicItemEffect{DamageReductMult: 1.0, MaxHP: 10, FlatDmgStart: 3},
			magicItemEffect{DamageReductMult: 1.0, MaxHP: 6, FlatDmgStart: 5},
		)
		byLabel := map[string]itemDelta{}
		for _, x := range d {
			byLabel[x.Label] = itemDelta{x.Better, x.Text}
		}
		if v := byLabel["hp"]; v.text != "+4 HP" || !v.better {
			t.Errorf("hp: %+v", v)
		}
		if v := byLabel["opening"]; v.text != "-2 opening damage" || v.better {
			t.Errorf("opening: %+v", v)
		}
	})

	t.Run("sub-percent change is dropped", func(t *testing.T) {
		// 0.004 fraction = 0.4% → rounds to 0% → not a visible delta.
		if d := magicItemDeltas(
			magicItemEffect{DamageBonus: 0.104, DamageReductMult: 1.0},
			magicItemEffect{DamageBonus: 0.100, DamageReductMult: 1.0},
		); len(d) != 0 {
			t.Fatalf("expected no visible delta, got %+v", d)
		}
	})
}

type itemDelta struct {
	better bool
	text   string
}

// TestCompareVerdict pins strict-dominance classification and the overrides.
func TestCompareVerdict(t *testing.T) {
	gain := []peteclient.ItemDelta{{Better: true}}
	loss := []peteclient.ItemDelta{{Better: false}}
	mixed := []peteclient.ItemDelta{{Better: true}, {Better: false}}

	cases := []struct {
		name        string
		deltas      []peteclient.ItemDelta
		empty, inrt bool
		want        string
	}{
		{"all gains", gain, false, false, "upgrade"},
		{"all losses", loss, false, false, "downgrade"},
		{"mixed", mixed, false, false, "sidegrade"},
		{"no change", nil, false, false, "same"},
		{"empty slot", gain, true, false, "new"},
		{"inert overrides upgrade", gain, false, true, "inert"},
		{"inert overrides new", gain, true, true, "inert"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := compareVerdict(c.deltas, c.empty, c.inrt); got != c.want {
				t.Errorf("compareVerdict(%s) = %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestMagicItemCompareIntegration drives the whole builder against equipped maps.
func TestMagicItemCompareIntegration(t *testing.T) {
	rareWpn := mkItem(MagicItemWeapon, RarityRare, DnDSlotMainHand)    // +15% damage
	uncWpn := mkItem(MagicItemWeapon, RarityUncommon, DnDSlotMainHand) // +10% damage

	t.Run("upgrade over a weaker worn weapon", func(t *testing.T) {
		eq := map[DnDSlot]EquippedMagicItem{DnDSlotMainHand: {Slot: DnDSlotMainHand, Item: uncWpn}}
		c := magicItemCompare(rareWpn, 0, eq)
		if c.Verdict != "upgrade" || c.VsName != uncWpn.Name || c.VsSlot != "main_hand" {
			t.Fatalf("got %+v", c)
		}
	})

	t.Run("downgrade under a stronger worn weapon", func(t *testing.T) {
		eq := map[DnDSlot]EquippedMagicItem{DnDSlotMainHand: {Slot: DnDSlotMainHand, Item: rareWpn}}
		if c := magicItemCompare(uncWpn, 0, eq); c.Verdict != "downgrade" {
			t.Fatalf("got %+v", c)
		}
	})

	t.Run("same as an identical worn weapon", func(t *testing.T) {
		eq := map[DnDSlot]EquippedMagicItem{DnDSlotMainHand: {Slot: DnDSlotMainHand, Item: rareWpn}}
		c := magicItemCompare(rareWpn, 0, eq)
		if c.Verdict != "same" || len(c.Deltas) != 0 {
			t.Fatalf("got %+v", c)
		}
	})

	t.Run("new into an empty slot names no worn item", func(t *testing.T) {
		c := magicItemCompare(rareWpn, 0, map[DnDSlot]EquippedMagicItem{})
		if c.Verdict != "new" || c.VsName != "" || len(c.Deltas) == 0 {
			t.Fatalf("got %+v", c)
		}
	})

	t.Run("attunement item with no free bond is inert", func(t *testing.T) {
		ring := mkItem(MagicItemRing, RarityRare, DnDSlotRing1)
		ring.Attunement = true
		// Three bonds spent elsewhere; the ring slot is empty. Wearing it does nothing.
		eq := map[DnDSlot]EquippedMagicItem{
			DnDSlotChest:  {Slot: DnDSlotChest, Item: mkItem(MagicItemArmor, RarityRare, DnDSlotChest), Attuned: true},
			DnDSlotAmulet: {Slot: DnDSlotAmulet, Item: mkItem(MagicItemWondrous, RarityRare, DnDSlotAmulet), Attuned: true},
			DnDSlotCloak:  {Slot: DnDSlotCloak, Item: mkItem(MagicItemWondrous, RarityRare, DnDSlotCloak), Attuned: true},
		}
		if c := magicItemCompare(ring, 0, eq); c.Verdict != "inert" {
			t.Fatalf("got %+v", c)
		}
	})

	t.Run("evicting the worn occupant frees its bond, so not inert", func(t *testing.T) {
		ring := mkItem(MagicItemRing, RarityRare, DnDSlotRing1)
		ring.Attunement = true
		wornRing := mkItem(MagicItemRing, RarityUncommon, DnDSlotRing1)
		wornRing.Attunement = true
		// Three bonds spent — but one of them is the ring the candidate would replace.
		eq := map[DnDSlot]EquippedMagicItem{
			DnDSlotRing1:  {Slot: DnDSlotRing1, Item: wornRing, Attuned: true},
			DnDSlotChest:  {Slot: DnDSlotChest, Item: mkItem(MagicItemArmor, RarityRare, DnDSlotChest), Attuned: true},
			DnDSlotAmulet: {Slot: DnDSlotAmulet, Item: mkItem(MagicItemWondrous, RarityRare, DnDSlotAmulet), Attuned: true},
		}
		if c := magicItemCompare(ring, 0, eq); c.Verdict == "inert" {
			t.Fatalf("bond freed by eviction, should not be inert: %+v", c)
		}
	})
}
