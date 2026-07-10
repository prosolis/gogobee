package plugin

import (
	"math/rand/v2"
	"strconv"
	"strings"
)

// Phase 8 — equipment profile registry per gogobee_equipment_appendix.md.
//
// Catalogs every weapon and armor entry from the appendix as data tables,
// plus a synthesis layer that maps legacy AdvEquipment rows (which only
// know slot + tier + name) onto a sensible D&D profile. This gives us
// real weapon damage dice and armor AC computation in combat without
// requiring a loot-side rewrite.

// ── Weapon profiles ──────────────────────────────────────────────────────────

// WeaponCategory groups weapons for class-proficiency lookup.
type WeaponCategory int

const (
	WeaponCatSimpleMelee WeaponCategory = iota
	WeaponCatSimpleRanged
	WeaponCatMartialMelee
	WeaponCatMartialRanged
)

// WeaponProperty enumerates the weapon properties from appendix §1.
type WeaponProperty string

const (
	PropFinesse    WeaponProperty = "finesse"
	PropLight      WeaponProperty = "light"
	PropHeavy      WeaponProperty = "heavy"
	PropTwoHanded  WeaponProperty = "two_handed"
	PropVersatile  WeaponProperty = "versatile"
	PropThrown     WeaponProperty = "thrown"
	PropAmmunition WeaponProperty = "ammunition"
	PropLoading    WeaponProperty = "loading"
	PropReach      WeaponProperty = "reach"
)

// WeaponProfile mirrors the spec's struct. DamageDie is parsed from "1d8",
// "2d6", "1d10" etc. into Count/Sides on construction.
type WeaponProfile struct {
	ID          string
	Name        string
	Category    WeaponCategory
	DamageCount int    // dice count
	DamageSides int    // die sides
	DamageType  string // slashing, piercing, bludgeoning
	Properties  []WeaponProperty
	VersaCount  int // versatile two-handed dice (0 if not versatile)
	VersaSides  int
	MagicBonus  int    // 0 for mundane
	MagicProp   string // empty for mundane
	NamedItem   bool   // true for §7.2 named magic weapons
}

// HasProperty reports whether the weapon has the given property.
func (w *WeaponProfile) HasProperty(p WeaponProperty) bool {
	for _, q := range w.Properties {
		if q == p {
			return true
		}
	}
	return false
}

// dndWeaponRegistry — every weapon from appendix §2 and §3.
var dndWeaponRegistry = []WeaponProfile{
	// §2.1 Simple Melee
	{ID: "wpn_club", Name: "Club", Category: WeaponCatSimpleMelee, DamageCount: 1, DamageSides: 4, DamageType: "bludgeoning", Properties: []WeaponProperty{PropLight}},
	{ID: "wpn_dagger", Name: "Dagger", Category: WeaponCatSimpleMelee, DamageCount: 1, DamageSides: 4, DamageType: "piercing", Properties: []WeaponProperty{PropFinesse, PropLight, PropThrown}},
	{ID: "wpn_greatclub", Name: "Greatclub", Category: WeaponCatSimpleMelee, DamageCount: 1, DamageSides: 8, DamageType: "bludgeoning", Properties: []WeaponProperty{PropTwoHanded}},
	{ID: "wpn_handaxe", Name: "Handaxe", Category: WeaponCatSimpleMelee, DamageCount: 1, DamageSides: 6, DamageType: "slashing", Properties: []WeaponProperty{PropLight, PropThrown}},
	{ID: "wpn_javelin", Name: "Javelin", Category: WeaponCatSimpleMelee, DamageCount: 1, DamageSides: 6, DamageType: "piercing", Properties: []WeaponProperty{PropThrown}},
	{ID: "wpn_light_hammer", Name: "Light Hammer", Category: WeaponCatSimpleMelee, DamageCount: 1, DamageSides: 4, DamageType: "bludgeoning", Properties: []WeaponProperty{PropLight, PropThrown}},
	{ID: "wpn_mace", Name: "Mace", Category: WeaponCatSimpleMelee, DamageCount: 1, DamageSides: 6, DamageType: "bludgeoning"},
	{ID: "wpn_quarterstaff", Name: "Quarterstaff", Category: WeaponCatSimpleMelee, DamageCount: 1, DamageSides: 6, DamageType: "bludgeoning", Properties: []WeaponProperty{PropVersatile}, VersaCount: 1, VersaSides: 8},
	{ID: "wpn_sickle", Name: "Sickle", Category: WeaponCatSimpleMelee, DamageCount: 1, DamageSides: 4, DamageType: "slashing", Properties: []WeaponProperty{PropLight}},
	{ID: "wpn_spear", Name: "Spear", Category: WeaponCatSimpleMelee, DamageCount: 1, DamageSides: 6, DamageType: "piercing", Properties: []WeaponProperty{PropThrown, PropVersatile}, VersaCount: 1, VersaSides: 8},

	// §2.2 Simple Ranged
	{ID: "wpn_crossbow_light", Name: "Light Crossbow", Category: WeaponCatSimpleRanged, DamageCount: 1, DamageSides: 8, DamageType: "piercing", Properties: []WeaponProperty{PropAmmunition, PropLoading, PropTwoHanded}},
	{ID: "wpn_dart", Name: "Dart", Category: WeaponCatSimpleRanged, DamageCount: 1, DamageSides: 4, DamageType: "piercing", Properties: []WeaponProperty{PropFinesse, PropThrown}},
	{ID: "wpn_shortbow", Name: "Shortbow", Category: WeaponCatSimpleRanged, DamageCount: 1, DamageSides: 6, DamageType: "piercing", Properties: []WeaponProperty{PropAmmunition, PropTwoHanded}},
	{ID: "wpn_sling", Name: "Sling", Category: WeaponCatSimpleRanged, DamageCount: 1, DamageSides: 4, DamageType: "bludgeoning", Properties: []WeaponProperty{PropAmmunition}},

	// §3.1 Martial Melee
	{ID: "wpn_battleaxe", Name: "Battleaxe", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 8, DamageType: "slashing", Properties: []WeaponProperty{PropVersatile}, VersaCount: 1, VersaSides: 10},
	{ID: "wpn_flail", Name: "Flail", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 8, DamageType: "bludgeoning"},
	{ID: "wpn_glaive", Name: "Glaive", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 10, DamageType: "slashing", Properties: []WeaponProperty{PropHeavy, PropReach, PropTwoHanded}},
	{ID: "wpn_greataxe", Name: "Greataxe", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 12, DamageType: "slashing", Properties: []WeaponProperty{PropHeavy, PropTwoHanded}},
	{ID: "wpn_greatsword", Name: "Greatsword", Category: WeaponCatMartialMelee, DamageCount: 2, DamageSides: 6, DamageType: "slashing", Properties: []WeaponProperty{PropHeavy, PropTwoHanded}},
	{ID: "wpn_halberd", Name: "Halberd", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 10, DamageType: "slashing", Properties: []WeaponProperty{PropHeavy, PropReach, PropTwoHanded}},
	{ID: "wpn_lance", Name: "Lance", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 12, DamageType: "piercing", Properties: []WeaponProperty{PropReach}},
	{ID: "wpn_longsword", Name: "Longsword", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 8, DamageType: "slashing", Properties: []WeaponProperty{PropVersatile}, VersaCount: 1, VersaSides: 10},
	{ID: "wpn_maul", Name: "Maul", Category: WeaponCatMartialMelee, DamageCount: 2, DamageSides: 6, DamageType: "bludgeoning", Properties: []WeaponProperty{PropHeavy, PropTwoHanded}},
	{ID: "wpn_morningstar", Name: "Morningstar", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 8, DamageType: "piercing"},
	{ID: "wpn_pike", Name: "Pike", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 10, DamageType: "piercing", Properties: []WeaponProperty{PropHeavy, PropReach, PropTwoHanded}},
	{ID: "wpn_rapier", Name: "Rapier", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 8, DamageType: "piercing", Properties: []WeaponProperty{PropFinesse}},
	{ID: "wpn_scimitar", Name: "Scimitar", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 6, DamageType: "slashing", Properties: []WeaponProperty{PropFinesse, PropLight}},
	{ID: "wpn_shortsword", Name: "Shortsword", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 6, DamageType: "piercing", Properties: []WeaponProperty{PropFinesse, PropLight}},
	{ID: "wpn_trident", Name: "Trident", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 6, DamageType: "piercing", Properties: []WeaponProperty{PropThrown, PropVersatile}, VersaCount: 1, VersaSides: 8},
	{ID: "wpn_war_pick", Name: "War Pick", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 8, DamageType: "piercing"},
	{ID: "wpn_warhammer", Name: "Warhammer", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 8, DamageType: "bludgeoning", Properties: []WeaponProperty{PropVersatile}, VersaCount: 1, VersaSides: 10},
	{ID: "wpn_whip", Name: "Whip", Category: WeaponCatMartialMelee, DamageCount: 1, DamageSides: 4, DamageType: "slashing", Properties: []WeaponProperty{PropFinesse, PropReach}},

	// §3.2 Martial Ranged
	{ID: "wpn_crossbow_hand", Name: "Hand Crossbow", Category: WeaponCatMartialRanged, DamageCount: 1, DamageSides: 6, DamageType: "piercing", Properties: []WeaponProperty{PropAmmunition, PropLight, PropLoading}},
	{ID: "wpn_crossbow_heavy", Name: "Heavy Crossbow", Category: WeaponCatMartialRanged, DamageCount: 1, DamageSides: 10, DamageType: "piercing", Properties: []WeaponProperty{PropAmmunition, PropHeavy, PropLoading, PropTwoHanded}},
	{ID: "wpn_longbow", Name: "Longbow", Category: WeaponCatMartialRanged, DamageCount: 1, DamageSides: 8, DamageType: "piercing", Properties: []WeaponProperty{PropAmmunition, PropHeavy, PropTwoHanded}},
}

// weaponByID returns the WeaponProfile for an ID, or nil.
func weaponByID(id string) *WeaponProfile {
	for i := range dndWeaponRegistry {
		if dndWeaponRegistry[i].ID == id {
			return &dndWeaponRegistry[i]
		}
	}
	return nil
}

// ── Armor profiles ───────────────────────────────────────────────────────────

// ArmorType identifies the proficiency category an armor falls under.
type ArmorType int

const (
	ArmorTypeLight ArmorType = iota
	ArmorTypeMedium
	ArmorTypeHeavy
	ArmorTypeShield
)

// ArmorProfile is the spec's struct. MaxDEXBonus convention:
//
//	-1 = unlimited (light armor takes full DEX mod)
//	 2 = medium (cap at +2)
//	 0 = heavy (no DEX bonus)
//	 0 for shields (shields don't add DEX)
type ArmorProfile struct {
	ID           string
	Name         string
	Type         ArmorType
	BaseAC       int // 0 for shields (shields use ShieldBonus)
	ShieldBonus  int // +2 base for shields, 0 for armor
	MaxDEXBonus  int
	STRRequire   int
	StealthDisad bool
	MagicBonus   int
	MagicProp    string
}

var dndArmorRegistry = []ArmorProfile{
	// §5.1 Light
	{ID: "arm_padded", Name: "Padded", Type: ArmorTypeLight, BaseAC: 11, MaxDEXBonus: -1, StealthDisad: true},
	{ID: "arm_leather", Name: "Leather", Type: ArmorTypeLight, BaseAC: 11, MaxDEXBonus: -1},
	{ID: "arm_studded", Name: "Studded Leather", Type: ArmorTypeLight, BaseAC: 12, MaxDEXBonus: -1},
	// §5.2 Medium
	{ID: "arm_hide", Name: "Hide", Type: ArmorTypeMedium, BaseAC: 12, MaxDEXBonus: 2},
	{ID: "arm_chain_shirt", Name: "Chain Shirt", Type: ArmorTypeMedium, BaseAC: 13, MaxDEXBonus: 2},
	{ID: "arm_scale_mail", Name: "Scale Mail", Type: ArmorTypeMedium, BaseAC: 14, MaxDEXBonus: 2, StealthDisad: true},
	{ID: "arm_breastplate", Name: "Breastplate", Type: ArmorTypeMedium, BaseAC: 14, MaxDEXBonus: 2},
	{ID: "arm_half_plate", Name: "Half Plate", Type: ArmorTypeMedium, BaseAC: 15, MaxDEXBonus: 2, StealthDisad: true},
	// §5.3 Heavy
	{ID: "arm_ring_mail", Name: "Ring Mail", Type: ArmorTypeHeavy, BaseAC: 14, MaxDEXBonus: 0, StealthDisad: true},
	{ID: "arm_chain_mail", Name: "Chain Mail", Type: ArmorTypeHeavy, BaseAC: 16, MaxDEXBonus: 0, STRRequire: 13, StealthDisad: true},
	{ID: "arm_splint", Name: "Splint", Type: ArmorTypeHeavy, BaseAC: 17, MaxDEXBonus: 0, STRRequire: 15, StealthDisad: true},
	{ID: "arm_plate", Name: "Plate", Type: ArmorTypeHeavy, BaseAC: 18, MaxDEXBonus: 0, STRRequire: 15, StealthDisad: true},
	// §5.4 Shield
	{ID: "arm_shield", Name: "Shield", Type: ArmorTypeShield, BaseAC: 0, ShieldBonus: 2, MaxDEXBonus: 0},
}

func armorByID(id string) *ArmorProfile {
	for i := range dndArmorRegistry {
		if dndArmorRegistry[i].ID == id {
			return &dndArmorRegistry[i]
		}
	}
	return nil
}

// ── Class proficiency matrix (appendix §9) ──────────────────────────────────

// dndClassWeaponProficiency reports whether a class is proficient with a
// weapon. Rogue's restricted martial list (Shortsword, Rapier, Scimitar,
// Longsword, Hand crossbow) is hard-coded.
func dndClassWeaponProficiency(class DnDClass, w *WeaponProfile) bool {
	if w == nil {
		return true
	}
	// All classes are proficient with simple weapons.
	if w.Category == WeaponCatSimpleMelee || w.Category == WeaponCatSimpleRanged {
		return true
	}
	switch class {
	case ClassFighter, ClassRanger:
		return true // proficient with all martial
	case ClassRogue:
		// Restricted martial list per appendix §9.
		switch w.ID {
		case "wpn_shortsword", "wpn_rapier", "wpn_scimitar", "wpn_longsword", "wpn_crossbow_hand":
			return true
		}
		return false
	case ClassMage, ClassCleric:
		// Mage: daggers + staves only. Cleric: simple only. Both already
		// covered by the simple-weapon early-exit. Here we're in martial.
		return false
	}
	return false
}

// dndClassArmorProficiency returns whether the class is proficient with the
// given armor type. Per appendix §9 + main design doc §3.2.
func dndClassArmorProficiency(class DnDClass, a *ArmorProfile) bool {
	if a == nil {
		return true
	}
	switch class {
	case ClassFighter:
		return true // Fighter wears anything
	case ClassRanger, ClassCleric:
		return a.Type == ArmorTypeLight || a.Type == ArmorTypeMedium || a.Type == ArmorTypeShield
	case ClassRogue:
		return a.Type == ArmorTypeLight
	case ClassMage:
		return false // Mage proficient with no armor
	}
	return false
}

// ── Damage dice rolling ─────────────────────────────────────────────────────

// rollWeaponDamage rolls the weapon's damage dice and adds the ability mod
// + magic bonus. If twoHanded is true and the weapon is versatile, rolls
// the larger versatile die instead. Returns the unmodified dice total too
// for the crit doubling math.
func rollWeaponDamage(rng *rand.Rand, w *WeaponProfile, abilityMod int, twoHanded bool) (total, dice int) {
	count, sides := w.DamageCount, w.DamageSides
	if twoHanded && w.HasProperty(PropVersatile) && w.VersaCount > 0 {
		count, sides = w.VersaCount, w.VersaSides
	}
	for i := 0; i < count; i++ {
		dice += 1 + rngIntN(rng, sides)
	}
	total = dice + abilityMod + w.MagicBonus
	if total < 1 {
		total = 1
	}
	return
}

// avgWeaponDamage returns the expected (mean) damage for a weapon given the
// ability mod. Used by tests and balance checks. Versatile two-handed mode
// not considered here — tests pick the form they want.
func avgWeaponDamage(w *WeaponProfile, abilityMod int) float64 {
	count, sides := float64(w.DamageCount), float64(w.DamageSides)
	avg := count*(sides+1)/2 + float64(abilityMod) + float64(w.MagicBonus)
	if avg < 1 {
		avg = 1
	}
	return avg
}

// ── AC computation per appendix ──────────────────────────────────────────────

// computeArmorAC implements the appendix's ComputeAC helper. Returns the
// total AC; set armor=nil for unarmored, shield=nil for no shield.
func computeArmorAC(armor, shield *ArmorProfile, dexMod int) int {
	base := 10
	dexApplied := dexMod
	magicBonus := 0
	if armor != nil {
		base = armor.BaseAC
		magicBonus = armor.MagicBonus
		switch armor.MaxDEXBonus {
		case -1:
			dexApplied = dexMod
		case 2:
			if dexMod > 2 {
				dexApplied = 2
			} else {
				dexApplied = dexMod
			}
		case 0:
			dexApplied = 0
		}
	}
	shieldBonus := 0
	if shield != nil {
		shieldBonus = shield.ShieldBonus + shield.MagicBonus
	}
	return base + dexApplied + magicBonus + shieldBonus
}

// ── Legacy gear synthesis ────────────────────────────────────────────────────
//
// Existing AdvEquipment rows have name, tier, slot, masterwork, arena_tier
// — but no D&D weapon ID. We synthesize a sensible profile from these fields
// so combat gets real weapon dice and armor AC without a loot rewrite.
//
// Legacy → D&D mapping per slot:
//   Weapon:
//     Tier 1   → Club / Dagger by name keyword
//     Tier 2   → Mace / Quarterstaff
//     Tier 3   → Longsword (versatile)
//     Tier 4   → Battleaxe (versatile, two-handed mode)
//     Tier 5   → Greatsword (2d6)
//     Tier 6+  → Greatsword + magic_bonus = (tier - 5)
//   Armor:
//     Tier 1   → Padded
//     Tier 2   → Leather
//     Tier 3   → Chain Shirt (medium)
//     Tier 4   → Scale Mail (medium)
//     Tier 5   → Half Plate (medium)
//     Tier 6+  → Plate (heavy) + magic_bonus = (tier - 5)
//   Tool slot may include shields by name; otherwise no AC contribution.
//
// This is intentionally generous — we want existing high-tier players to
// see a damage upgrade, not a downgrade, on the rebrand to D&D dice.

// synthesizeWeaponProfile inspects a legacy AdvEquipment row and returns a
// best-fit WeaponProfile. Returns nil if the slot isn't a weapon.
func synthesizeWeaponProfile(eq *AdvEquipment) *WeaponProfile {
	if eq == nil || eq.Slot != SlotWeapon {
		return nil
	}
	tier := eq.Tier
	if eq.Masterwork && tier < 5 {
		tier = 5 // Masterwork promotes to top-tier base
	}
	nameLower := strings.ToLower(eq.Name)

	var base *WeaponProfile
	switch {
	case tier <= 1:
		if strings.Contains(nameLower, "dagger") || strings.Contains(nameLower, "knife") {
			base = weaponByID("wpn_dagger")
		} else {
			base = weaponByID("wpn_club")
		}
	case tier == 2:
		if strings.Contains(nameLower, "staff") || strings.Contains(nameLower, "wand") {
			base = weaponByID("wpn_quarterstaff")
		} else if strings.Contains(nameLower, "axe") {
			base = weaponByID("wpn_handaxe")
		} else {
			base = weaponByID("wpn_mace")
		}
	case tier == 3:
		if strings.Contains(nameLower, "bow") {
			base = weaponByID("wpn_shortbow")
		} else if strings.Contains(nameLower, "axe") {
			base = weaponByID("wpn_battleaxe")
		} else {
			base = weaponByID("wpn_longsword")
		}
	case tier == 4:
		if strings.Contains(nameLower, "bow") {
			base = weaponByID("wpn_longbow")
		} else if strings.Contains(nameLower, "axe") {
			base = weaponByID("wpn_battleaxe")
		} else if strings.Contains(nameLower, "hammer") {
			base = weaponByID("wpn_warhammer")
		} else {
			base = weaponByID("wpn_longsword")
		}
	default: // tier 5+
		if strings.Contains(nameLower, "bow") {
			base = weaponByID("wpn_longbow")
		} else if strings.Contains(nameLower, "axe") {
			base = weaponByID("wpn_greataxe")
		} else if strings.Contains(nameLower, "hammer") || strings.Contains(nameLower, "maul") {
			base = weaponByID("wpn_maul")
		} else {
			base = weaponByID("wpn_greatsword")
		}
	}
	if base == nil {
		// Should never happen — registry has all the IDs above. Fallback club.
		base = weaponByID("wpn_club")
	}
	// Copy so we can mutate magic bonus without polluting the registry.
	out := *base
	if eq.Tier > 5 {
		out.MagicBonus = eq.Tier - 5 // T6 = +1, T7 = +2, T8 = +3
		if out.MagicBonus > 3 {
			out.MagicBonus = 3
		}
	}
	if eq.Masterwork && out.MagicBonus < 1 {
		out.MagicBonus = 1
	}
	return &out
}

// synthesizeArmorProfile inspects a legacy AdvEquipment row and returns a
// best-fit ArmorProfile for the chest (armor) slot. Returns nil for other slots.
func synthesizeArmorProfile(eq *AdvEquipment) *ArmorProfile {
	if eq == nil || eq.Slot != SlotArmor {
		return nil
	}
	tier := eq.Tier
	if eq.Masterwork && tier < 5 {
		tier = 5
	}
	var base *ArmorProfile
	switch {
	case tier <= 1:
		base = armorByID("arm_padded")
	case tier == 2:
		base = armorByID("arm_leather")
	case tier == 3:
		base = armorByID("arm_chain_shirt")
	case tier == 4:
		base = armorByID("arm_scale_mail")
	case tier == 5:
		base = armorByID("arm_half_plate")
	default:
		base = armorByID("arm_plate")
	}
	if base == nil {
		base = armorByID("arm_padded")
	}
	out := *base
	if eq.Tier > 5 {
		out.MagicBonus = eq.Tier - 5
		if out.MagicBonus > 3 {
			out.MagicBonus = 3
		}
	}
	if eq.Masterwork && out.MagicBonus < 1 {
		out.MagicBonus = 1
	}
	return &out
}

// synthesizeShield — tool slot can hold shields when name matches.
// Returns nil if the equipped tool isn't a shield.
func synthesizeShield(eq *AdvEquipment) *ArmorProfile {
	if eq == nil || eq.Slot != SlotTool {
		return nil
	}
	if !strings.Contains(strings.ToLower(eq.Name), "shield") {
		return nil
	}
	base := armorByID("arm_shield")
	if base == nil {
		return nil
	}
	out := *base
	if eq.Tier > 5 {
		out.MagicBonus = eq.Tier - 5
		if out.MagicBonus > 3 {
			out.MagicBonus = 3
		}
	}
	if eq.Masterwork && out.MagicBonus < 1 {
		out.MagicBonus = 1
	}
	return &out
}

// ── Misc helpers ────────────────────────────────────────────────────────────

// parseDamageDie parses strings like "1d8" or "2d6" into (count, sides).
// Used by tests; the registry above pre-parses these at compile time.
func parseDamageDie(s string) (count, sides int, ok bool) {
	parts := strings.SplitN(strings.ToLower(strings.TrimSpace(s)), "d", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	count, err := strconv.Atoi(parts[0])
	if err != nil || count < 1 {
		return 0, 0, false
	}
	sides, err = strconv.Atoi(parts[1])
	if err != nil || sides < 2 {
		return 0, 0, false
	}
	return count, sides, true
}
