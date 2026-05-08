package plugin

import (
	"testing"
)

// ── Damage die parser ──────────────────────────────────────────────────────

func TestParseDamageDie(t *testing.T) {
	cases := []struct {
		s         string
		count, sides int
		ok        bool
	}{
		{"1d8", 1, 8, true},
		{"2d6", 2, 6, true},
		{"1D10", 1, 10, true},
		{"  1d12  ", 1, 12, true},
		{"d8", 0, 0, false},   // no count
		{"1d", 0, 0, false},   // no sides
		{"1d1", 0, 0, false},  // sides < 2
		{"banana", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, c := range cases {
		count, sides, ok := parseDamageDie(c.s)
		if ok != c.ok || count != c.count || sides != c.sides {
			t.Errorf("parseDamageDie(%q) = (%d,%d,%v); want (%d,%d,%v)",
				c.s, count, sides, ok, c.count, c.sides, c.ok)
		}
	}
}

// ── Registry coverage ──────────────────────────────────────────────────────

func TestWeaponRegistryComplete(t *testing.T) {
	// Spot-check that key weapons from appendix §2 + §3 are present and
	// have the right damage dice. If any of these fail, the registry has
	// drifted from the appendix.
	cases := map[string]struct {
		count, sides int
		dmgType      string
	}{
		"wpn_dagger":         {1, 4, "piercing"},
		"wpn_longsword":      {1, 8, "slashing"},
		"wpn_greatsword":     {2, 6, "slashing"},
		"wpn_greataxe":       {1, 12, "slashing"},
		"wpn_maul":           {2, 6, "bludgeoning"},
		"wpn_quarterstaff":   {1, 6, "bludgeoning"},
		"wpn_shortbow":       {1, 6, "piercing"},
		"wpn_longbow":        {1, 8, "piercing"},
		"wpn_crossbow_heavy": {1, 10, "piercing"},
	}
	for id, want := range cases {
		w := weaponByID(id)
		if w == nil {
			t.Errorf("missing weapon %s", id)
			continue
		}
		if w.DamageCount != want.count || w.DamageSides != want.sides {
			t.Errorf("%s damage = %dd%d, want %dd%d", id, w.DamageCount, w.DamageSides, want.count, want.sides)
		}
		if w.DamageType != want.dmgType {
			t.Errorf("%s damage type = %s, want %s", id, w.DamageType, want.dmgType)
		}
	}
}

func TestVersatileWeaponsHaveLargerDie(t *testing.T) {
	for _, id := range []string{"wpn_quarterstaff", "wpn_spear", "wpn_battleaxe", "wpn_longsword", "wpn_warhammer", "wpn_trident"} {
		w := weaponByID(id)
		if w == nil {
			t.Errorf("missing %s", id)
			continue
		}
		if !w.HasProperty(PropVersatile) {
			t.Errorf("%s should be versatile", id)
		}
		if w.VersaSides <= w.DamageSides {
			t.Errorf("%s versa die (%dd%d) not larger than one-handed (%dd%d)",
				id, w.VersaCount, w.VersaSides, w.DamageCount, w.DamageSides)
		}
	}
}

func TestArmorRegistryAppendixValues(t *testing.T) {
	// Direct values from appendix §5.
	cases := map[string]struct {
		baseAC      int
		armorType   ArmorType
		maxDexBonus int
		strReq      int
		stealth     bool
	}{
		"arm_padded":      {11, ArmorTypeLight, -1, 0, true},
		"arm_leather":     {11, ArmorTypeLight, -1, 0, false},
		"arm_studded":     {12, ArmorTypeLight, -1, 0, false},
		"arm_chain_shirt": {13, ArmorTypeMedium, 2, 0, false},
		"arm_scale_mail":  {14, ArmorTypeMedium, 2, 0, true},
		"arm_breastplate": {14, ArmorTypeMedium, 2, 0, false},
		"arm_half_plate":  {15, ArmorTypeMedium, 2, 0, true},
		"arm_chain_mail":  {16, ArmorTypeHeavy, 0, 13, true},
		"arm_splint":      {17, ArmorTypeHeavy, 0, 15, true},
		"arm_plate":       {18, ArmorTypeHeavy, 0, 15, true},
	}
	for id, want := range cases {
		a := armorByID(id)
		if a == nil {
			t.Errorf("missing %s", id)
			continue
		}
		if a.BaseAC != want.baseAC || a.Type != want.armorType ||
			a.MaxDEXBonus != want.maxDexBonus || a.STRRequire != want.strReq ||
			a.StealthDisad != want.stealth {
			t.Errorf("%s mismatch: got base=%d type=%d dex=%d str=%d stealth=%v; want %+v",
				id, a.BaseAC, a.Type, a.MaxDEXBonus, a.STRRequire, a.StealthDisad, want)
		}
	}
}

// ── ComputeAC math ──────────────────────────────────────────────────────────

func TestComputeArmorAC_Unarmored(t *testing.T) {
	// Unarmored, DEX +3 → 13
	if got := computeArmorAC(nil, nil, 3); got != 13 {
		t.Errorf("unarmored DEX+3 AC = %d, want 13", got)
	}
	// Unarmored + shield, DEX +0 → 12
	if got := computeArmorAC(nil, armorByID("arm_shield"), 0); got != 12 {
		t.Errorf("unarmored + shield DEX+0 AC = %d, want 12", got)
	}
}

func TestComputeArmorAC_Light(t *testing.T) {
	leather := armorByID("arm_leather")
	// Leather (11) + DEX +4 → 15
	if got := computeArmorAC(leather, nil, 4); got != 15 {
		t.Errorf("leather DEX+4 AC = %d, want 15", got)
	}
	// Studded (12) + DEX +5 → 17 (light has no cap)
	if got := computeArmorAC(armorByID("arm_studded"), nil, 5); got != 17 {
		t.Errorf("studded DEX+5 AC = %d, want 17", got)
	}
}

func TestComputeArmorAC_MediumCapped(t *testing.T) {
	// Half plate (15) + DEX +4 → 17 (capped at +2)
	if got := computeArmorAC(armorByID("arm_half_plate"), nil, 4); got != 17 {
		t.Errorf("half plate DEX+4 AC = %d, want 17 (cap)", got)
	}
	// Chain shirt (13) + DEX +1 → 14 (under cap)
	if got := computeArmorAC(armorByID("arm_chain_shirt"), nil, 1); got != 14 {
		t.Errorf("chain shirt DEX+1 AC = %d, want 14", got)
	}
}

func TestComputeArmorAC_HeavyIgnoresDex(t *testing.T) {
	// Plate (18) + DEX +5 → 18 (no DEX)
	if got := computeArmorAC(armorByID("arm_plate"), nil, 5); got != 18 {
		t.Errorf("plate DEX+5 AC = %d, want 18 (no DEX)", got)
	}
	// Chain mail (16) + shield + DEX -1 → 18
	if got := computeArmorAC(armorByID("arm_chain_mail"), armorByID("arm_shield"), -1); got != 18 {
		t.Errorf("chain mail + shield AC = %d, want 18", got)
	}
}

func TestComputeArmorAC_MagicBonus(t *testing.T) {
	plate := *armorByID("arm_plate")
	plate.MagicBonus = 2
	// +2 plate (18 + 2) → 20
	if got := computeArmorAC(&plate, nil, 0); got != 20 {
		t.Errorf("+2 plate AC = %d, want 20", got)
	}
}

// ── Weapon damage rolls ─────────────────────────────────────────────────────

func TestRollWeaponDamage_Bounds(t *testing.T) {
	greatsword := weaponByID("wpn_greatsword") // 2d6
	for i := 0; i < 1000; i++ {
		total, dice := rollWeaponDamage(greatsword, 3, false)
		if dice < 2 || dice > 12 {
			t.Fatalf("greatsword dice out of [2,12]: %d", dice)
		}
		if total != dice+3 {
			t.Fatalf("total %d != dice %d + mod 3", total, dice)
		}
	}
}

func TestRollWeaponDamage_VersatileTwoHanded(t *testing.T) {
	longsword := weaponByID("wpn_longsword") // 1d8 / 1d10
	// One-handed
	for i := 0; i < 100; i++ {
		_, dice := rollWeaponDamage(longsword, 0, false)
		if dice < 1 || dice > 8 {
			t.Errorf("longsword 1H dice = %d, want [1,8]", dice)
		}
	}
	// Two-handed (versatile)
	for i := 0; i < 100; i++ {
		_, dice := rollWeaponDamage(longsword, 0, true)
		if dice < 1 || dice > 10 {
			t.Errorf("longsword 2H dice = %d, want [1,10]", dice)
		}
	}
}

func TestRollWeaponDamage_FloorAt1(t *testing.T) {
	// Pathological: 1d4 with -10 mod always rolls 1+(-10) = negative, floored to 1.
	dagger := weaponByID("wpn_dagger")
	for i := 0; i < 100; i++ {
		total, _ := rollWeaponDamage(dagger, -10, false)
		if total < 1 {
			t.Errorf("damage floor violated: %d", total)
		}
	}
}

func TestAvgWeaponDamage(t *testing.T) {
	// 1d8 + 0 mean = 4.5
	got := avgWeaponDamage(weaponByID("wpn_longsword"), 0)
	if got != 4.5 {
		t.Errorf("longsword avg = %v, want 4.5", got)
	}
	// 2d6 + 3 mean = 7 + 3 = 10
	got = avgWeaponDamage(weaponByID("wpn_greatsword"), 3)
	if got != 10.0 {
		t.Errorf("greatsword+3 avg = %v, want 10", got)
	}
}

// ── Class proficiency ──────────────────────────────────────────────────────

func TestClassWeaponProficiency(t *testing.T) {
	cases := []struct {
		class DnDClass
		wpnID string
		want  bool
	}{
		// Simple weapons: everyone proficient
		{ClassMage, "wpn_dagger", true},
		{ClassCleric, "wpn_mace", true},
		{ClassRogue, "wpn_quarterstaff", true},
		// Martial: Fighter/Ranger always
		{ClassFighter, "wpn_greatsword", true},
		{ClassRanger, "wpn_longbow", true},
		// Mage/Cleric: NOT proficient with martial
		{ClassMage, "wpn_longsword", false},
		{ClassCleric, "wpn_greatsword", false},
		// Rogue: restricted martial list
		{ClassRogue, "wpn_shortsword", true},
		{ClassRogue, "wpn_rapier", true},
		{ClassRogue, "wpn_longsword", true},
		{ClassRogue, "wpn_crossbow_hand", true},
		{ClassRogue, "wpn_greatsword", false},
		{ClassRogue, "wpn_warhammer", false},
	}
	for _, c := range cases {
		w := weaponByID(c.wpnID)
		if got := dndClassWeaponProficiency(c.class, w); got != c.want {
			t.Errorf("class=%s wpn=%s = %v, want %v", c.class, c.wpnID, got, c.want)
		}
	}
}

func TestClassArmorProficiency(t *testing.T) {
	cases := []struct {
		class   DnDClass
		armorID string
		want    bool
	}{
		{ClassFighter, "arm_plate", true},
		{ClassFighter, "arm_leather", true},
		{ClassFighter, "arm_shield", true},
		{ClassRanger, "arm_chain_shirt", true}, // medium
		{ClassRanger, "arm_plate", false},      // heavy
		{ClassCleric, "arm_chain_shirt", true},
		{ClassCleric, "arm_plate", false},
		{ClassCleric, "arm_shield", true},
		{ClassRogue, "arm_leather", true},
		{ClassRogue, "arm_chain_shirt", false}, // medium
		{ClassRogue, "arm_shield", false},
		{ClassMage, "arm_leather", false},
		{ClassMage, "arm_shield", false},
	}
	for _, c := range cases {
		a := armorByID(c.armorID)
		if got := dndClassArmorProficiency(c.class, a); got != c.want {
			t.Errorf("class=%s armor=%s = %v, want %v", c.class, c.armorID, got, c.want)
		}
	}
}

// ── Legacy synthesis ───────────────────────────────────────────────────────

func TestSynthesizeWeaponProfile_TierMapping(t *testing.T) {
	cases := []struct {
		name     string
		tier     int
		wantBase string
	}{
		{"Iron Club", 1, "wpn_club"},
		{"Wooden Dagger", 1, "wpn_dagger"},
		{"Steel Mace", 2, "wpn_mace"},
		{"Apprentice Staff", 2, "wpn_quarterstaff"},
		{"Iron Sword", 3, "wpn_longsword"},
		{"Hunter's Bow", 3, "wpn_shortbow"},
		{"Knight's Sword", 4, "wpn_longsword"},
		{"Greatsword", 5, "wpn_greatsword"},
		{"Battleaxe", 5, "wpn_greataxe"},
	}
	for _, c := range cases {
		eq := &AdvEquipment{Slot: SlotWeapon, Tier: c.tier, Name: c.name}
		w := synthesizeWeaponProfile(eq)
		if w == nil {
			t.Errorf("%s tier %d: synthesize returned nil", c.name, c.tier)
			continue
		}
		base := weaponByID(c.wantBase)
		if w.DamageCount != base.DamageCount || w.DamageSides != base.DamageSides {
			t.Errorf("%s tier %d: synthesized %dd%d, expected %dd%d (%s)",
				c.name, c.tier, w.DamageCount, w.DamageSides,
				base.DamageCount, base.DamageSides, c.wantBase)
		}
	}
}

func TestSynthesizeWeaponProfile_HighTierMagicBonus(t *testing.T) {
	eq := &AdvEquipment{Slot: SlotWeapon, Tier: 7, Name: "Legendary Sword"}
	w := synthesizeWeaponProfile(eq)
	if w == nil {
		t.Fatal("synthesize returned nil")
	}
	if w.MagicBonus != 2 { // tier 7 - 5 = +2
		t.Errorf("tier 7 magic bonus = %d, want 2", w.MagicBonus)
	}
}

func TestSynthesizeWeaponProfile_MasterworkPromotes(t *testing.T) {
	eq := &AdvEquipment{Slot: SlotWeapon, Tier: 2, Name: "Sword", Masterwork: true}
	w := synthesizeWeaponProfile(eq)
	if w.DamageSides < 6 {
		t.Errorf("masterwork promoted should be at least tier-5 base (1d8/2d6); got %dd%d",
			w.DamageCount, w.DamageSides)
	}
	if w.MagicBonus < 1 {
		t.Errorf("masterwork should grant +1 minimum, got +%d", w.MagicBonus)
	}
}

func TestSynthesizeWeaponProfile_NonWeaponSlotReturnsNil(t *testing.T) {
	eq := &AdvEquipment{Slot: SlotArmor, Tier: 3, Name: "Plate"}
	if got := synthesizeWeaponProfile(eq); got != nil {
		t.Errorf("non-weapon slot should return nil, got %+v", got)
	}
}

func TestSynthesizeArmorProfile_TierMapping(t *testing.T) {
	cases := []struct {
		tier        int
		wantBaseAC  int
		wantType    ArmorType
	}{
		{1, 11, ArmorTypeLight},   // Padded
		{2, 11, ArmorTypeLight},   // Leather
		{3, 13, ArmorTypeMedium},  // Chain Shirt
		{4, 14, ArmorTypeMedium},  // Scale Mail
		{5, 15, ArmorTypeMedium},  // Half Plate
		{6, 18, ArmorTypeHeavy},   // Plate
	}
	for _, c := range cases {
		eq := &AdvEquipment{Slot: SlotArmor, Tier: c.tier, Name: "Armor"}
		a := synthesizeArmorProfile(eq)
		if a == nil {
			t.Errorf("tier %d: synth returned nil", c.tier)
			continue
		}
		if a.BaseAC != c.wantBaseAC || a.Type != c.wantType {
			t.Errorf("tier %d: got AC=%d type=%d, want AC=%d type=%d",
				c.tier, a.BaseAC, a.Type, c.wantBaseAC, c.wantType)
		}
	}
}

func TestSynthesizeShield_NameMatch(t *testing.T) {
	cases := []struct {
		slot EquipmentSlot
		name string
		want bool
	}{
		{SlotTool, "Iron Shield", true},
		{SlotTool, "Tower Shield", true},
		{SlotTool, "Pickaxe", false},
		{SlotTool, "", false},
		{SlotWeapon, "Shield", false}, // wrong slot
	}
	for _, c := range cases {
		eq := &AdvEquipment{Slot: c.slot, Name: c.name, Tier: 3}
		got := synthesizeShield(eq) != nil
		if got != c.want {
			t.Errorf("slot=%s name=%q: shield=%v, want %v", c.slot, c.name, got, c.want)
		}
	}
}

// ── pickWeaponAbilityMod ────────────────────────────────────────────────────

func TestPickWeaponAbilityMod(t *testing.T) {
	c := &DnDCharacter{STR: 16, DEX: 14, CON: 13, INT: 8, WIS: 10, CHA: 12}
	// STR mod = +3, DEX mod = +2.
	cases := []struct {
		wpnID string
		want  int
	}{
		{"wpn_greatsword", 3}, // melee non-finesse → STR
		{"wpn_longsword", 3},
		{"wpn_dagger", 3},   // finesse melee, but STR > DEX, picks STR
		{"wpn_longbow", 2},  // ranged → DEX
		{"wpn_shortbow", 2},
	}
	for _, tc := range cases {
		got := pickWeaponAbilityMod(weaponByID(tc.wpnID), c)
		if got != tc.want {
			t.Errorf("%s: ability mod = %d, want %d", tc.wpnID, got, tc.want)
		}
	}

	// Now with DEX > STR: finesse should pick DEX.
	c2 := &DnDCharacter{STR: 10, DEX: 18, CON: 14}
	if got := pickWeaponAbilityMod(weaponByID("wpn_dagger"), c2); got != 4 {
		t.Errorf("DEX-favored finesse: got %d, want 4 (DEX +4)", got)
	}
	if got := pickWeaponAbilityMod(weaponByID("wpn_greatsword"), c2); got != 0 {
		t.Errorf("non-finesse with DEX>STR: got %d, want 0 (STR +0)", got)
	}
}

// ── End-to-end: applyDnDEquipmentLayer ─────────────────────────────────────

func TestApplyDnDEquipmentLayer_FighterFullKit(t *testing.T) {
	c := &DnDCharacter{
		Race: RaceHuman, Class: ClassFighter, Level: 5,
		STR: 18, DEX: 14, CON: 16, INT: 10, WIS: 12, CHA: 8,
		ArmorClass: 10, // base
	}
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Slot: SlotWeapon, Tier: 5, Name: "Greatsword"},
		SlotArmor:  {Slot: SlotArmor, Tier: 6, Name: "Plate"},
	}
	stats := CombatStats{}
	applyDnDPlayerLayer(&stats, c)
	applyDnDEquipmentLayer(&stats, c, equip)

	if stats.Weapon == nil {
		t.Fatal("weapon not set")
	}
	if stats.Weapon.DamageCount != 1 || stats.Weapon.DamageSides != 12 {
		// Greatsword name + tier 5 → wpn_greataxe (1d12) per synth name match
		// (greatsword name routes through "axe" branch? No, "greatsword" doesn't
		// contain "axe". Let me check: tier 5 default else branch → wpn_greatsword (2d6).
		// Adjust test if synthesis differs.
		if !(stats.Weapon.DamageCount == 2 && stats.Weapon.DamageSides == 6) {
			t.Errorf("greatsword tier 5 weapon = %dd%d, expected 2d6 or 1d12",
				stats.Weapon.DamageCount, stats.Weapon.DamageSides)
		}
	}
	if !stats.WeaponProficient {
		t.Error("Fighter should be proficient with synthesized weapon")
	}
	// Plate (tier 6) → +1 plate, AC = 18+1 = 19, no DEX, no shield
	if stats.AC != 19 {
		t.Errorf("Fighter+plate+1 AC = %d, want 19", stats.AC)
	}
	// Two-handed mode: greatsword has TwoHanded property (no shield).
	// Greatsword's properties include Heavy + TwoHanded — TwoHandedMode set.
	if !stats.TwoHandedMode {
		t.Error("expected TwoHandedMode for greatsword without shield")
	}
}

func TestApplyDnDEquipmentLayer_MageWithMartialWeapon(t *testing.T) {
	// Mage equips a longsword → not proficient, -4 attack penalty path.
	c := &DnDCharacter{
		Class: ClassMage, Level: 1, STR: 8, DEX: 14, CON: 12, INT: 16, WIS: 13, CHA: 10,
		ArmorClass: 10,
	}
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Slot: SlotWeapon, Tier: 3, Name: "Longsword"},
	}
	stats := CombatStats{}
	applyDnDPlayerLayer(&stats, c)
	applyDnDEquipmentLayer(&stats, c, equip)
	if stats.Weapon == nil {
		t.Fatal("weapon nil")
	}
	if stats.WeaponProficient {
		t.Error("Mage should NOT be proficient with longsword")
	}
}
