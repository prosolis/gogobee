# GogoBee — Equipment Appendix
> **Companion to:** `gogobee_dnd_design_doc.md`  
> **Version:** 1.0  
> **Source:** D&D 5e SRD, adapted for GogoBee bot mechanics

---

## ⚠️ Implementation Notes

- All damage dice and stat values are **starting baselines**. Balance tuning is a human responsibility.
- Weapon/armor names may be reskinned for flavor (e.g., "TwinBee Aegis" instead of "Shield") but base mechanics must match this spec.
- Class proficiency restrictions from the main design doc apply here. An unproficient player may equip an item but incurs **-4 to attack rolls** and cannot add stat modifiers to damage.
- All gold (gp) costs map 1:1 to GogoBee **coins** unless the economy team decides otherwise.
- Item IDs follow the format: `wpn_<name>` for weapons, `arm_<name>` for armor.

---

## 1. Weapon Properties Glossary

| Property | Effect |
|---|---|
| **Ammunition** | Requires ammo (arrows, bolts, etc.) to fire. Recover half after combat. |
| **Finesse** | Player may use STR or DEX modifier for attack and damage (choose one, apply consistently). |
| **Heavy** | Small creatures (Halfling) have disadvantage on attack rolls with this weapon. |
| **Light** | Can be dual-wielded. Bonus action attack with off-hand (no stat modifier to damage). |
| **Loading** | Can only fire once per turn regardless of extra attacks. |
| **Range** | Listed as normal/long range. Attacks beyond normal range have disadvantage. |
| **Reach** | +5 ft attack range. Relevant if zone/range mechanics are ever added. |
| **Thrown** | Can be thrown as a ranged attack using STR modifier. |
| **Two-Handed** | Requires both hands. Cannot use a shield simultaneously. |
| **Versatile** | Can be wielded one-handed (lower damage) or two-handed (higher damage). |
| **Special** | See individual entry for unique rules. |

---

## 2. Simple Weapons

Simple weapons are proficient for **all classes**.

### 2.1 Simple Melee Weapons

| Weapon | Cost | Damage | Damage Type | Weight | Properties |
|---|---|---|---|---|---|
| Club | 1 gp | 1d4 | Bludgeoning | 2 lb | Light |
| Dagger | 2 gp | 1d4 | Piercing | 1 lb | Finesse, Light, Thrown (20/60) |
| Greatclub | 2 gp | 1d8 | Bludgeoning | 10 lb | Two-Handed |
| Handaxe | 5 gp | 1d6 | Slashing | 2 lb | Light, Thrown (20/60) |
| Javelin | 5 gp | 1d6 | Piercing | 2 lb | Thrown (30/120) |
| Light Hammer | 2 gp | 1d4 | Bludgeoning | 2 lb | Light, Thrown (20/60) |
| Mace | 5 gp | 1d6 | Bludgeoning | 4 lb | — |
| Quarterstaff | 2 gp | 1d6 / 1d8 | Bludgeoning | 4 lb | Versatile |
| Sickle | 1 gp | 1d4 | Slashing | 2 lb | Light |
| Spear | 1 gp | 1d6 / 1d8 | Piercing | 3 lb | Thrown (20/60), Versatile |

### 2.2 Simple Ranged Weapons

| Weapon | Cost | Damage | Damage Type | Weight | Properties |
|---|---|---|---|---|---|
| Crossbow, Light | 25 gp | 1d8 | Piercing | 5 lb | Ammunition (80/320), Loading, Two-Handed |
| Dart | 5 cp | 1d4 | Piercing | 0.25 lb | Finesse, Thrown (20/60) |
| Shortbow | 25 gp | 1d6 | Piercing | 2 lb | Ammunition (80/320), Two-Handed |
| Sling | 1 sp | 1d4 | Bludgeoning | — | Ammunition (30/120) |

---

## 3. Martial Weapons

Martial weapons require class proficiency. See main design doc Section 3.2 for class proficiency tables.

### 3.1 Martial Melee Weapons

| Weapon | Cost | Damage | Damage Type | Weight | Properties |
|---|---|---|---|---|---|
| Battleaxe | 10 gp | 1d8 / 1d10 | Slashing | 4 lb | Versatile |
| Flail | 10 gp | 1d8 | Bludgeoning | 2 lb | — |
| Glaive | 20 gp | 1d10 | Slashing | 6 lb | Heavy, Reach, Two-Handed |
| Greataxe | 30 gp | 1d12 | Slashing | 7 lb | Heavy, Two-Handed |
| Greatsword | 50 gp | 2d6 | Slashing | 6 lb | Heavy, Two-Handed |
| Halberd | 20 gp | 1d10 | Slashing | 6 lb | Heavy, Reach, Two-Handed |
| Lance | 10 gp | 1d12 | Piercing | 6 lb | Reach, Special (disadvantage vs. adjacent) |
| Longsword | 15 gp | 1d8 / 1d10 | Slashing | 3 lb | Versatile |
| Maul | 10 gp | 2d6 | Bludgeoning | 10 lb | Heavy, Two-Handed |
| Morningstar | 15 gp | 1d8 | Piercing | 4 lb | — |
| Pike | 5 gp | 1d10 | Piercing | 18 lb | Heavy, Reach, Two-Handed |
| Rapier | 25 gp | 1d8 | Piercing | 2 lb | Finesse |
| Scimitar | 25 gp | 1d6 | Slashing | 3 lb | Finesse, Light |
| Shortsword | 10 gp | 1d6 | Piercing | 2 lb | Finesse, Light |
| Trident | 5 gp | 1d6 / 1d8 | Piercing | 4 lb | Thrown (20/60), Versatile |
| War Pick | 5 gp | 1d8 | Piercing | 2 lb | — |
| Warhammer | 15 gp | 1d8 / 1d10 | Bludgeoning | 2 lb | Versatile |
| Whip | 2 gp | 1d4 | Slashing | 3 lb | Finesse, Reach |

### 3.2 Martial Ranged Weapons

| Weapon | Cost | Damage | Damage Type | Weight | Properties |
|---|---|---|---|---|---|
| Crossbow, Hand | 75 gp | 1d6 | Piercing | 3 lb | Ammunition (30/120), Light, Loading |
| Crossbow, Heavy | 50 gp | 1d10 | Piercing | 18 lb | Ammunition (100/400), Heavy, Loading, Two-Handed |
| Longbow | 50 gp | 1d8 | Piercing | 2 lb | Ammunition (150/600), Heavy, Two-Handed |
| Net | 1 gp | — | — | 3 lb | Special: Restrained condition on hit (STR DC 10 to escape). Large or smaller targets only. |

---

## 4. Ammunition

Ammo is tracked as a consumable. Players recover **half their spent ammo** after each combat (round down).

| Ammunition | Cost (20 units) | Used By |
|---|---|---|
| Arrows | 1 gp | Shortbow, Longbow |
| Bolts | 1 gp | Crossbow (light, heavy, hand) |
| Sling Bullets | 4 cp | Sling |
| Blowgun Needles | 1 gp | Blowgun |
| Darts | 5 cp each | Darts (see Simple Ranged) |

---

## 5. Armor

### 5.1 Light Armor

Proficiency: Rogue, Ranger, Cleric (and Fighter by inheritance).

| Armor | Cost | AC | STR Req. | Stealth | Weight |
|---|---|---|---|---|---|
| Padded | 5 gp | 11 + DEX mod | — | Disadvantage | 8 lb |
| Leather | 10 gp | 11 + DEX mod | — | — | 10 lb |
| Studded Leather | 45 gp | 12 + DEX mod | — | — | 13 lb |

### 5.2 Medium Armor

Proficiency: Fighter, Ranger, Cleric.  
DEX bonus to AC capped at **+2** for all medium armor.

| Armor | Cost | AC | STR Req. | Stealth | Weight |
|---|---|---|---|---|---|
| Hide | 10 gp | 12 + DEX mod (max +2) | — | — | 12 lb |
| Chain Shirt | 50 gp | 13 + DEX mod (max +2) | — | — | 20 lb |
| Scale Mail | 50 gp | 14 + DEX mod (max +2) | — | Disadvantage | 45 lb |
| Breastplate | 400 gp | 14 + DEX mod (max +2) | — | — | 20 lb |
| Half Plate | 750 gp | 15 + DEX mod (max +2) | — | Disadvantage | 40 lb |

### 5.3 Heavy Armor

Proficiency: Fighter only.  
Heavy armor grants **no DEX bonus** to AC. STR requirements must be met or movement is reduced (flavor only in bot context — apply -2 to DEX-based rolls as penalty).

| Armor | Cost | AC | STR Req. | Stealth | Weight |
|---|---|---|---|---|---|
| Ring Mail | 30 gp | 14 | — | Disadvantage | 40 lb |
| Chain Mail | 75 gp | 16 | STR 13 | Disadvantage | 55 lb |
| Splint | 200 gp | 17 | STR 15 | Disadvantage | 60 lb |
| Plate | 1,500 gp | 18 | STR 15 | Disadvantage | 65 lb |

### 5.4 Shield

| Item | Cost | AC Bonus | Proficiency | Weight |
|---|---|---|---|---|
| Shield | 10 gp | +2 | Fighter, Ranger, Cleric | 6 lb |

A shield occupies the **off-hand slot**. Cannot be used with Two-Handed weapons.

---

## 6. Adventuring Gear & Consumables

These are available at Thom Krooke's shop or as dungeon loot.

### 6.1 Potions

| Item | Cost | Effect | Notes |
|---|---|---|---|
| Potion of Healing | 50 gp | Restore 2d4+2 HP | Bonus action to drink in combat |
| Potion of Greater Healing | 100 gp | Restore 4d4+4 HP | Bonus action to drink in combat |
| Potion of Superior Healing | 500 gp | Restore 8d4+8 HP | Rare drop only; not in shop |
| Antitoxin | 50 gp | Advantage on CON saves vs. poison for 1 hour / 3 combats | — |
| Potion of Speed | 250 gp | +1 attack per turn for 1 minute / 1 combat | Rare drop only |
| Potion of Resistance | 300 gp | Resistance to one damage type for 1 hour | Type specified on item |

### 6.2 Utility Items

| Item | Cost | Effect |
|---|---|---|
| Rope, Hemp (50 ft) | 1 gp | Enables Athletics checks for climbing/binding |
| Torch | 1 cp | Removes darkness penalty in underground zones (future) |
| Thieves' Tools | 25 gp | Required for Rogue lockpicking checks |
| Herbalism Kit | 5 gp | Required for crafting basic potions (future) |
| Spell Component Pouch | 25 gp | Required for Mage spells with material components |
| Holy Symbol | 5 gp | Required for Cleric divine abilities |
| Arcane Focus (Staff) | 5 gp | Alternative to Spell Component Pouch for Mage |
| Arrows (20) | 1 gp | Ammunition |
| Bolts (20) | 1 gp | Ammunition |
| Quiver | 1 gp | Holds 20 arrows or bolts |

---

## 7. Magic Weapons (Base Templates)

Magic weapons drop from dungeon encounters and high-CR monsters. They use the mundane weapon as a base, then add a bonus and optionally a magical property. Attunement required for +2 and above.

### 7.1 Bonus Weapon Tiers

| Tier | Attack Bonus | Damage Bonus | Rarity | Attunement |
|---|---|---|---|---|
| +1 | +1 to attack rolls | +1 to damage | Uncommon | No |
| +2 | +2 to attack rolls | +2 to damage | Rare | Yes |
| +3 | +3 to attack rolls | +3 to damage | Very Rare | Yes |

### 7.2 Named Magic Weapons (Starter Set)

These are specific named items with unique properties. Each requires attunement.

| Item | Base | Rarity | Bonus | Special Property |
|---|---|---|---|---|
| **Flame Tongue** | Longsword | Rare | +1 | On hit: +2d6 fire damage. Command word activates flame (light source). |
| **Sword of Wounding** | Shortsword | Rare | +1 | On hit: target cannot regain HP until start of their next turn. |
| **Frost Brand** | Greatsword | Very Rare | +1 | On hit: +1d6 cold damage. Cold resistance while attuned. |
| **Dagger of Venom** | Dagger | Rare | +1 | Once per day: coat blade with poison. Next hit: +2d10 poison, CON DC 15 or Poisoned. |
| **Bow of Speed** | Longbow | Very Rare | +2 | Ignore Loading property on any crossbow. +1 attack per turn. |
| **Mace of Smiting** | Mace | Rare | +1 | On crit vs. construct: +2d6 bonus damage. |
| **Staff of the Adder** | Quarterstaff | Uncommon | +1 | Cleric/Ranger only. Bonus action: animate as snake (1d4 piercing, poison on bite CON DC 11). |
| **Rapier of Puncturing** | Rapier | Uncommon | +1 | On crit: target makes CON DC 13 or loses 2d4 HP at start of their next 3 turns (bleed). |

---

## 8. Magic Armor (Base Templates)

### 8.1 Bonus Armor Tiers

| Tier | AC Bonus | Rarity | Attunement |
|---|---|---|---|
| +1 | +1 to AC | Rare | No |
| +2 | +2 to AC | Very Rare | Yes |
| +3 | +3 to AC | Legendary | Yes |

### 8.2 Named Magic Armor (Starter Set)

| Item | Base | Rarity | Special Property |
|---|---|---|---|
| **Armor of Resistance** | Any | Rare | Resistance to one damage type (specified on item). Attunement. |
| **Demon Armor** | Plate | Very Rare | +1 AC. Unarmed strikes deal 1d8 + STR slashing. Cursed: cannot be removed without *Remove Curse* spell. |
| **Dragon Scale Mail** | Scale Mail | Very Rare | +1 AC. Resistance to one dragon damage type. Advantage on saves vs. dragons. |
| **Elven Chain** | Chain Shirt | Rare | Treated as light armor for proficiency. No Stealth disadvantage. |
| **Mithral Armor** | Medium or Heavy | Uncommon | No STR requirement. No Stealth disadvantage. No attunement. |
| **Adamantine Armor** | Medium or Heavy | Uncommon | Any critical hit against wearer becomes a normal hit instead. |
| **Spellguard Shield** | Shield | Very Rare | Advantage on saves vs. spells. Magic missiles cannot hit wearer. |
| **Cloak of Protection** | — (Amulet slot) | Uncommon | +1 to AC and all saving throws. Attunement. |

---

## 9. Class-Weapon Affinity Matrix

Summary of which weapon categories each class can use with full proficiency:

| Weapon Category | Fighter | Rogue | Mage | Cleric | Ranger |
|---|---|---|---|---|---|
| Simple Melee | ✅ | ✅ | ✅ | ✅ | ✅ |
| Simple Ranged | ✅ | ✅ | ✅ | ✅ | ✅ |
| Martial Melee | ✅ | ⚠️ Shortsword, Rapier, Scimitar, Longsword only | ❌ | ❌ | ✅ |
| Martial Ranged | ✅ | ⚠️ Hand crossbow only | ❌ | ❌ | ✅ |
| Heavy Armor | ✅ | ❌ | ❌ | ❌ | ❌ |
| Medium Armor | ✅ | ❌ | ❌ | ✅ | ✅ |
| Light Armor | ✅ | ✅ | ❌ | ✅ | ✅ |
| Shield | ✅ | ❌ | ❌ | ✅ | ✅ |

---

## 10. Carry Weight (Optional Mechanic)

If inventory weight is implemented, use these rules:

- **Carry Capacity:** STR score × 15 lbs
- **Encumbered** (> STR × 5 lbs): -10 to movement speed (flavor; -1 to DEX rolls)
- **Heavily Encumbered** (> STR × 10 lbs): -20 to movement; disadvantage on STR/DEX/CON checks
- **Over capacity:** Cannot move. Combat actions at disadvantage.

Recommended: implement carry weight as a soft warning only in v1. Hard enforcement deferred to future patch.

---

## 11. Go Data Structures — Equipment Extension

```go
// WeaponProfile extends the base Item struct for weapons.
type WeaponProfile struct {
    ItemID       string   `json:"item_id"`
    DamageDie    string   `json:"damage_die"`    // e.g. "1d8", "2d6"
    DamageType   string   `json:"damage_type"`   // slashing, piercing, bludgeoning
    Properties   []string `json:"properties"`    // finesse, light, thrown, etc.
    RangeNormal  int      `json:"range_normal"`  // 0 if melee
    RangeLong    int      `json:"range_long"`    // 0 if melee
    VersatileDie string   `json:"versatile_die"` // e.g. "1d10"; empty if not versatile
    MagicBonus   int      `json:"magic_bonus"`   // 0 for mundane
    MagicProp    string   `json:"magic_prop"`    // empty for mundane
}

// ArmorProfile extends the base Item struct for armor.
type ArmorProfile struct {
    ItemID       string `json:"item_id"`
    ArmorType    string `json:"armor_type"`    // light, medium, heavy, shield
    BaseAC       int    `json:"base_ac"`
    MaxDEXBonus  int    `json:"max_dex_bonus"` // -1 = unlimited (light), 2 = medium, 0 = heavy
    STRRequire   int    `json:"str_require"`   // 0 if no requirement
    StealthDisad bool   `json:"stealth_disad"`
    MagicBonus   int    `json:"magic_bonus"`
    MagicProp    string `json:"magic_prop"`
}

// AmmoStack tracks a player's ammunition supply.
type AmmoStack struct {
    PlayerID string `json:"player_id"`
    Type     string `json:"ammo_type"` // arrows, bolts, sling_bullets, etc.
    Quantity int    `json:"quantity"`
}

// ACResult holds the resolved AC for a player at combat time.
// Computed from equipped armor + DEX modifier + magic bonuses.
type ACResult struct {
    Base       int `json:"base"`
    DEXApplied int `json:"dex_applied"`
    MagicBonus int `json:"magic_bonus"`
    ShieldBonus int `json:"shield_bonus"`
    Total      int `json:"total"`
}

// ComputeAC resolves a player's AC from their equipped gear and stats.
func ComputeAC(armor *ArmorProfile, shield *ArmorProfile, dexMod int) ACResult {
    ac := ACResult{}
    if armor == nil {
        // Unarmored: 10 + DEX
        ac.Base = 10
        ac.DEXApplied = dexMod
    } else {
        ac.Base = armor.BaseAC + armor.MagicBonus
        switch armor.MaxDEXBonus {
        case -1: // light: full DEX
            ac.DEXApplied = dexMod
        case 2: // medium: cap at +2
            if dexMod > 2 { ac.DEXApplied = 2 } else { ac.DEXApplied = dexMod }
        case 0: // heavy: no DEX
            ac.DEXApplied = 0
        }
    }
    if shield != nil {
        ac.ShieldBonus = 2 + shield.MagicBonus
    }
    ac.Total = ac.Base + ac.DEXApplied + ac.ShieldBonus
    return ac
}
```

---

*End of Equipment Appendix. Reference alongside `gogobee_dnd_design_doc.md` in all Claude Code sessions.*
