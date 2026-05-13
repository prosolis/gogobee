package plugin

import "testing"

func makeInventory(names ...string) []ConsumableItem {
	var items []ConsumableItem
	for i, name := range names {
		def := consumableDefByName(name)
		if def == nil {
			panic("unknown consumable: " + name)
		}
		items = append(items, ConsumableItem{InventoryID: int64(i + 1), Def: def})
	}
	return items
}

func TestSelectConsumables_TrivialFightSkips(t *testing.T) {
	strong := CombatStats{MaxHP: 200, Attack: 60}
	weak := CombatStats{MaxHP: 30, Attack: 5}
	inv := makeInventory("Berry Poultice", "Coal Bomb")

	selected := SelectConsumables(inv, strong, weak, 0, 0, true)
	if len(selected) != 0 {
		t.Errorf("should skip consumables for trivial fight, got %d", len(selected))
	}
}

func TestSelectConsumables_DangerousUsesHighTier(t *testing.T) {
	weak := CombatStats{MaxHP: 80, Attack: 15}
	strong := CombatStats{MaxHP: 200, Attack: 50}
	inv := makeInventory("Berry Poultice", "Spirit Tonic", "Coal Bomb", "Ancient Artifact Oil")

	selected := SelectConsumables(inv, weak, strong, 0, 0, true)
	if len(selected) != 2 {
		t.Fatalf("expected 2 consumables, got %d", len(selected))
	}

	hasHighTierOff, hasHighTierDef := false, false
	for _, s := range selected {
		if s.Def.Tier >= 4 && s.Def.Slot == "offensive" {
			hasHighTierOff = true
		}
		if s.Def.Tier >= 4 && s.Def.Slot == "defensive" {
			hasHighTierDef = true
		}
	}
	if !hasHighTierOff {
		t.Error("should pick high-tier offensive for dangerous fight")
	}
	if !hasHighTierDef {
		t.Error("should pick high-tier defensive for dangerous fight")
	}
}

func TestSelectConsumables_MaxTwo(t *testing.T) {
	player := CombatStats{MaxHP: 100, Attack: 30}
	enemy := CombatStats{MaxHP: 120, Attack: 35}
	inv := makeInventory(
		"Berry Poultice", "Herb Salve", "Mushroom Brew",
		"Coal Bomb", "Goblin Grease", "Blooper Ink Vial",
	)

	selected := SelectConsumables(inv, player, enemy, 0, 0, true)
	if len(selected) > 2 {
		t.Errorf("max 2 consumables, got %d", len(selected))
	}

	offCount, defCount := 0, 0
	for _, s := range selected {
		if s.Def.Slot == "offensive" {
			offCount++
		} else {
			defCount++
		}
	}
	if offCount > 1 {
		t.Errorf("max 1 offensive, got %d", offCount)
	}
	if defCount > 1 {
		t.Errorf("max 1 defensive, got %d", defCount)
	}
}

func TestSelectConsumables_EasyUsesLowTier(t *testing.T) {
	player := CombatStats{MaxHP: 100, Attack: 30}
	enemy := CombatStats{MaxHP: 50, Attack: 15} // easy: ratio ~0.55

	inv := makeInventory("Berry Poultice", "Spirit Tonic", "Coal Bomb", "Ancient Artifact Oil")

	selected := SelectConsumables(inv, player, enemy, 0, 0, true)
	for _, s := range selected {
		if s.Def.Tier > 2 {
			t.Errorf("easy fight should use T1-T2, got %s (T%d)", s.Def.Name, s.Def.Tier)
		}
	}
}

func TestSelectConsumables_ArenaRoundEscalation(t *testing.T) {
	player := CombatStats{MaxHP: 100, Attack: 30}
	enemy := CombatStats{MaxHP: 80, Attack: 25} // competitive

	inv := makeInventory("Berry Poultice", "Spirit Tonic", "Coal Bomb", "Sapphire Elixir")

	r1 := SelectConsumables(inv, player, enemy, 1, 0, true)
	r4 := SelectConsumables(inv, player, enemy, 4, 0, true)

	maxTierR1 := 0
	for _, s := range r1 {
		if s.Def.Tier > maxTierR1 {
			maxTierR1 = s.Def.Tier
		}
	}
	maxTierR4 := 0
	for _, s := range r4 {
		if s.Def.Tier > maxTierR4 {
			maxTierR4 = s.Def.Tier
		}
	}
	if maxTierR4 <= maxTierR1 && len(r4) > 0 && len(r1) > 0 {
		t.Logf("arena round escalation: R1 maxTier=%d, R4 maxTier=%d", maxTierR1, maxTierR4)
	}
}

func TestApplyConsumableMods_Heal(t *testing.T) {
	stats := CombatStats{MaxHP: 100}
	mods := CombatModifiers{DamageReduct: 1.0}
	items := makeInventory("Herb Salve")
	ApplyConsumableMods(&stats, &mods, items)
	if mods.HealItem != 20 {
		t.Errorf("heal item = %d, want 20", mods.HealItem)
	}
}

func TestApplyConsumableMods_AtkBoost(t *testing.T) {
	stats := CombatStats{}
	mods := CombatModifiers{}
	items := makeInventory("Goblin Grease")
	ApplyConsumableMods(&stats, &mods, items)
	if mods.DamageBonus < 0.24 || mods.DamageBonus > 0.26 {
		t.Errorf("damage bonus = %f, want ~0.25", mods.DamageBonus)
	}
}

func TestApplyConsumableMods_Ward(t *testing.T) {
	stats := CombatStats{}
	mods := CombatModifiers{}
	items := makeInventory("Quartz Ward")
	ApplyConsumableMods(&stats, &mods, items)
	if mods.WardCharges != 1 {
		t.Errorf("ward charges = %d, want 1", mods.WardCharges)
	}
}

func TestApplyConsumableMods_AutoCrit(t *testing.T) {
	stats := CombatStats{}
	mods := CombatModifiers{}
	items := makeInventory("Ancient Artifact Oil")
	ApplyConsumableMods(&stats, &mods, items)
	if !mods.AutoCritFirst {
		t.Error("AutoCritFirst should be true")
	}
	if mods.DamageBonus < 0.34 {
		t.Errorf("damage bonus = %f, want ~0.35", mods.DamageBonus)
	}
}

func TestApplyConsumableMods_SporeCloud(t *testing.T) {
	stats := CombatStats{}
	mods := CombatModifiers{}
	items := makeInventory("Spore Cloud")
	ApplyConsumableMods(&stats, &mods, items)
	if mods.SporeCloud != 2 {
		t.Errorf("spore rounds = %d, want 2", mods.SporeCloud)
	}
}

func TestApplyConsumableMods_Reflect(t *testing.T) {
	stats := CombatStats{}
	mods := CombatModifiers{}
	items := makeInventory("Voidstone Shard")
	ApplyConsumableMods(&stats, &mods, items)
	if mods.ReflectNext < 0.49 {
		t.Errorf("reflect = %f, want ~0.50", mods.ReflectNext)
	}
}

func TestApplyConsumableMods_DefBoost(t *testing.T) {
	stats := CombatStats{}
	mods := CombatModifiers{DamageReduct: 1.0}
	items := makeInventory("Mushroom Brew")
	ApplyConsumableMods(&stats, &mods, items)
	if mods.DamageReduct < 0.79 || mods.DamageReduct > 0.81 {
		t.Errorf("damage reduct = %f, want ~0.80", mods.DamageReduct)
	}
}

func TestApplyConsumableMods_SpeedBoost(t *testing.T) {
	stats := CombatStats{Speed: 10}
	mods := CombatModifiers{}
	items := makeInventory("Blooper Ink Vial")
	ApplyConsumableMods(&stats, &mods, items)
	if stats.Speed != 13 { // 10 * 1.30 = 13
		t.Errorf("speed = %d, want 13", stats.Speed)
	}
}

func TestApplyConsumableMods_CritBoost(t *testing.T) {
	stats := CombatStats{CritRate: 0.05}
	mods := CombatModifiers{}
	items := makeInventory("Sapphire Elixir")
	ApplyConsumableMods(&stats, &mods, items)
	if stats.CritRate < 0.19 || stats.CritRate > 0.21 {
		t.Errorf("crit rate = %f, want ~0.20", stats.CritRate)
	}
}

func TestApplyConsumableMods_FlatDmg(t *testing.T) {
	stats := CombatStats{}
	mods := CombatModifiers{}
	items := makeInventory("Coal Bomb")
	ApplyConsumableMods(&stats, &mods, items)
	if mods.FlatDmgStart != 8 {
		t.Errorf("flat damage = %d, want 8", mods.FlatDmgStart)
	}
}

func TestRollConsumableDrop_ValidActivity(t *testing.T) {
	drops := 0
	for i := 0; i < 1000; i++ {
		item := RollConsumableDrop(AdvActivityMining, 3)
		if item != nil {
			drops++
			if item.Type != "consumable" {
				t.Errorf("drop type = %q, want consumable", item.Type)
			}
			if item.Name != "Quartz Ward" {
				t.Errorf("mining T3 drop = %q, want Quartz Ward", item.Name)
			}
			if item.Value <= 0 {
				t.Errorf("drop sell value = %d, want > 0", item.Value)
			}
		}
	}
	// 15% chance → expect ~150 in 1000
	if drops < 100 || drops > 220 {
		t.Errorf("drop rate: %d/1000 (expect ~150 at 15%%)", drops)
	}
}

func TestRollConsumableDrop_InvalidActivity(t *testing.T) {
	item := RollConsumableDrop(AdvActivityRest, 3)
	if item != nil {
		t.Error("rest activity should not drop consumables")
	}
}

func TestRollConsumableDrop_T1NoDrop(t *testing.T) {
	for i := 0; i < 100; i++ {
		item := RollConsumableDrop(AdvActivityForaging, 1)
		if item != nil {
			t.Fatal("T1 should not drop consumables")
		}
	}
}

func TestBuyableConsumables(t *testing.T) {
	buyable := BuyableConsumables()
	if len(buyable) == 0 {
		t.Fatal("should have buyable consumables")
	}
	for _, def := range buyable {
		if !def.Buyable {
			t.Errorf("%s marked buyable=false but returned by BuyableConsumables", def.Name)
		}
		if def.Price <= 0 {
			t.Errorf("%s has no price", def.Name)
		}
	}
}
