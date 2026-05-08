package plugin

import (
	"log/slog"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase 2 — D&D combat layer hookup.
//
// Phase 2 strategy: keep legacy HP/damage/dodge-rate scaling intact. The D&D
// layer adds AC + d20-vs-AC hit resolution on top. This preserves the
// existing balance while making combat read as D&D for opted-in players.
//
// HP rescaling and condition-system overhaul are deferred to later phases.

// ── Tunable constants ────────────────────────────────────────────────────────

// Class-derived "weapon proficiency" bonus baked into AttackBonus.
// Fighter is a martial class; Mage uses spell-attack baseline.
var dndClassWeaponBonus = map[DnDClass]int{
	ClassFighter: 2,
	ClassRanger:  1,
	ClassRogue:   1,
	ClassCleric:  0,
	ClassMage:    0,
}

// Monster AC formulas. Tuned so a typical L1 player (+5 attack bonus) hits
// roughly 70-80% at low tiers and 50-60% at high tiers. Adjust after live
// data lands.
const (
	dndArenaACBase     = 10
	dndArenaACPerThreat = 0.25 // ThreatLevel 0..30 → AC bump 0..7
	dndDungeonACBase   = 9
	// Dungeon AC scales linearly with tier: T1=10, T5=14
)

// Monster attack bonus. Counters player AC growth. T1 monster +5, T5 +9.
const (
	dndArenaAtkBase    = 4
	dndArenaAtkPerThreat = 0.30
	dndDungeonAtkBase  = 4
	// T1 dungeon +5, T5 +9
)

// ── Player layer ─────────────────────────────────────────────────────────────

// proficiencyBonus implements ceil(level/4) + 1, matching D&D5e (L1-4 = +2,
// L5-8 = +3, L9-12 = +4, ...).
func proficiencyBonus(level int) int {
	if level < 1 {
		level = 1
	}
	return (level-1)/4 + 2
}

// classAttackStatMod returns the ability modifier the class uses for its
// primary attack roll. Fighters use STR (melee), Rogue/Ranger use DEX, Mage
// uses INT (spell attack), Cleric uses WIS.
func classAttackStatMod(c *DnDCharacter) int {
	switch c.Class {
	case ClassFighter:
		return abilityModifier(c.STR)
	case ClassRogue, ClassRanger:
		return abilityModifier(c.DEX)
	case ClassMage:
		return abilityModifier(c.INT)
	case ClassCleric:
		return abilityModifier(c.WIS)
	}
	return abilityModifier(c.STR)
}

// dndPlayerAttackBonus = primary stat mod + proficiency + class weapon bonus.
func dndPlayerAttackBonus(c *DnDCharacter) int {
	return classAttackStatMod(c) + proficiencyBonus(c.Level) + dndClassWeaponBonus[c.Class]
}

// applyDnDPlayerLayer sets AC and AttackBonus on a stat block from the
// player's D&D character. Called after DerivePlayerStats has populated
// HP/Attack/Defense/etc. Phase 8: also wires equipped weapon/armor profiles
// into CombatStats so the d20 attack path uses real weapon dice and AC
// computation per gogobee_equipment_appendix.md.
func applyDnDPlayerLayer(stats *CombatStats, c *DnDCharacter) {
	stats.AC = c.ArmorClass
	stats.AttackBonus = dndPlayerAttackBonus(c)
}

// applyDnDEquipmentLayer populates the Phase 8 equipment-driven fields on
// CombatStats from synthesized profiles of the player's legacy gear. Also
// overrides stats.AC with the equipment-derived computation when an armor
// or shield is equipped.
//
// `equip` is the legacy adventure_equipment map; the synthesizer infers a
// D&D weapon/armor profile from slot+tier+name without requiring a loot
// migration. Helmets, boots, and non-shield tools have no D&D profile yet
// and don't affect this layer.
func applyDnDEquipmentLayer(stats *CombatStats, c *DnDCharacter, equip map[EquipmentSlot]*AdvEquipment) {
	if c == nil {
		return
	}
	// Weapon synthesis.
	weapon := synthesizeWeaponProfile(equip[SlotWeapon])
	if weapon != nil {
		stats.Weapon = weapon
		// Pick STR vs DEX modifier per finesse rule and class default.
		stats.AbilityModForDamage = pickWeaponAbilityMod(weapon, c)
		stats.WeaponProficient = dndClassWeaponProficiency(c.Class, weapon)
		// Magic bonus on the weapon also stacks onto AttackBonus.
		stats.AttackBonus += weapon.MagicBonus
		// Two-handed when the weapon has the property AND no shield is held.
		hasShield := synthesizeShield(equip[SlotTool]) != nil
		if weapon.HasProperty(PropTwoHanded) || (weapon.HasProperty(PropVersatile) && !hasShield) {
			stats.TwoHandedMode = true
		}
	}

	// Armor + shield → AC override per appendix.
	armor := synthesizeArmorProfile(equip[SlotArmor])
	shield := synthesizeShield(equip[SlotTool])
	// Two-handed weapons forbid shields per appendix §5.4.
	if weapon != nil && weapon.HasProperty(PropTwoHanded) {
		shield = nil
	}
	dexMod := abilityModifier(c.DEX)
	// Heavy armor STR-requirement penalty: -2 to DEX-based rolls if STR < req.
	// In our model, AC is the prominent DEX-derived value; honoring this
	// strictly would penalize attack rolls (DEX-attacking finesse/ranged) too.
	// For the AC computation we just clamp the dex bonus to 0 anyway when
	// armor is heavy (MaxDEXBonus=0), so the STR check has no AC effect —
	// it would only matter to attack rolls. Skip the penalty for now;
	// document it as a future refinement.
	if armor != nil || shield != nil {
		stats.AC = computeArmorAC(armor, shield, dexMod)
	}
}

// pickWeaponAbilityMod returns the ability modifier added to weapon damage:
// STR for melee unless the weapon is finesse/ranged and DEX is higher (then DEX).
func pickWeaponAbilityMod(w *WeaponProfile, c *DnDCharacter) int {
	str, dex := abilityModifier(c.STR), abilityModifier(c.DEX)
	switch w.Category {
	case WeaponCatSimpleRanged, WeaponCatMartialRanged:
		return dex
	}
	// Melee: finesse picks the better of STR/DEX.
	if w.HasProperty(PropFinesse) {
		if dex > str {
			return dex
		}
	}
	return str
}

// ── Monster layer ────────────────────────────────────────────────────────────

func applyDnDArenaMonsterLayer(stats *CombatStats, threatLevel int) {
	stats.AC = dndArenaACBase + int(float64(threatLevel)*dndArenaACPerThreat)
	stats.AttackBonus = dndArenaAtkBase + int(float64(threatLevel)*dndArenaAtkPerThreat)
}

func applyDnDDungeonMonsterLayer(stats *CombatStats, tier int) {
	stats.AC = dndDungeonACBase + tier
	stats.AttackBonus = dndDungeonAtkBase + tier
}

// ── Auto-migration on first combat ───────────────────────────────────────────

// classStatPriority returns the standard-array values {15,14,13,12,10,8}
// assigned to STR/DEX/CON/INT/WIS/CHA in the order the class cares about.
// Used by ensureDnDCharacterForCombat — players can rebuild later via !setup
// (which is allowed without respec cooldown when auto_migrated=1).
func classStatPriority(class DnDClass) [6]int {
	// Returned array is in STR, DEX, CON, INT, WIS, CHA order.
	switch class {
	case ClassFighter:
		return [6]int{15, 13, 14, 8, 12, 10} // STR, CON, DEX prioritized
	case ClassRogue:
		return [6]int{8, 15, 13, 14, 10, 12} // DEX, INT, CON
	case ClassMage:
		return [6]int{8, 12, 13, 15, 14, 10} // INT, WIS, CON
	case ClassCleric:
		return [6]int{12, 10, 13, 8, 15, 14} // WIS, CHA, CON
	case ClassRanger:
		return [6]int{12, 15, 13, 10, 14, 8} // DEX, WIS, CON
	}
	return [6]int{15, 14, 13, 12, 10, 8} // fallback: martial-ish
}

// ensureCharForDnDCmd is the helper non-combat D&D command handlers should
// use when they want auto-migration semantics PLUS the legacy-player
// onboarding DM. Combat paths in combat_bridge.go use ensureDnDCharacterForCombat
// directly because they already control the freshMigrate hook.
func (p *AdventurePlugin) ensureCharForDnDCmd(userID id.UserID, char *AdventureCharacter) (*DnDCharacter, error) {
	c, fresh, err := ensureDnDCharacterForCombat(userID, char)
	if err != nil {
		return nil, err
	}
	if fresh {
		p.maybeSendDnDOnboarding(userID, char, c)
	}
	return c, nil
}

// ensureDnDCharacterForCombat returns a usable D&D character for combat. If
// the player already has a confirmed sheet, it's returned. Otherwise an
// auto-migrated character is created using:
//
//   - Race/class inferred from archetypes (Human Fighter fallback)
//   - Class-tuned standard array assignment
//   - Racial modifiers applied
//   - dnd_level seeded from the legacy combat_level
//   - HP/AC computed from class + ability scores + level
//
// auto_migrated=1 is set on the row so !setup can freely overwrite it
// without consuming the !respec cooldown.
//
// Returns (character, freshlyMigrated, err). freshlyMigrated is true only
// on the first call that creates the row — used by callers to fire the
// one-shot onboarding DM for legacy players.
func ensureDnDCharacterForCombat(userID id.UserID, char *AdventureCharacter) (*DnDCharacter, bool, error) {
	existing, err := LoadDnDCharacter(userID)
	if err != nil {
		return nil, false, err
	}
	if existing != nil && !existing.PendingSetup {
		return existing, false, nil
	}
	// existing == nil OR pending_setup=1. In the pending case we leave the
	// player's draft alone and overlay a temporary auto-migrated working
	// character — the draft survives untouched in the DB, but the fight
	// they're trying to start gets a usable sheet *for this fight only*.
	// (We don't write — pending_setup=1 stays so they can finish !setup.)
	if existing != nil && existing.PendingSetup {
		return autoBuildCharacter(userID, char), false, nil
	}

	// Fresh auto-migration. Build, save, return.
	c := autoBuildCharacter(userID, char)
	c.AutoMigrated = true
	c.PendingSetup = false
	if err := SaveDnDCharacter(c); err != nil {
		return nil, false, err
	}
	_ = initResources(userID, c.Class)
	// Phase 9: caster auto-migrants get a starter spell list + slot pool so
	// !cast/!spells work the moment they land. Idempotent.
	_ = ensureSpellsForCharacter(c)
	slog.Info("dnd: auto-migrated character", "user", userID,
		"race", c.Race, "class", c.Class, "level", c.Level)
	return c, true, nil
}

// autoBuildCharacter constructs a complete DnDCharacter from archetype
// inference + the player's adventure state. Does not save.
func autoBuildCharacter(userID id.UserID, char *AdventureCharacter) *DnDCharacter {
	sug := inferDnDFromArchetypes(userID)
	scores := classStatPriority(sug.Class)
	scores = applyRaceMods(sug.Race, scores)

	level := 1
	if char != nil {
		level = dndLevelFromCombatLevel(char.CombatLevel)
	}

	c := &DnDCharacter{
		UserID: userID,
		Race:   sug.Race,
		Class:  sug.Class,
		Level:  level,
		STR:    scores[0], DEX: scores[1], CON: scores[2],
		INT:   scores[3], WIS: scores[4], CHA: scores[5],
		PendingSetup: false,
		AutoMigrated: true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	conMod := abilityModifier(c.CON)
	dexMod := abilityModifier(c.DEX)
	c.HPMax = computeMaxHP(c.Class, conMod, c.Level)
	c.HPCurrent = c.HPMax
	c.ArmorClass = computeAC(c.Class, dexMod)
	return c
}

// ── Roll summary line ────────────────────────────────────────────────────────

// dndRollSummaryLine scans a CombatResult's events for d20 rolls and returns
// a one-liner summarizing the player's accuracy. Returns "" if no D&D rolls
// are present (legacy combat, or a fight that resolved on consumables alone).
//
// Format: "🎲 d20 — 5/8 hit (2 crits, 1 fumble). Best: nat 20."
//
// Surfaced post-combat by arena and dungeon callers; keeps the d20 system
// visible without modifying combat_narrative.go's flavor pools.
func dndRollSummaryLine(result CombatResult) string {
	hits, misses, crits, fumbles := 0, 0, 0, 0
	bestRoll, bestSeen := 0, false
	for _, ev := range result.Events {
		if ev.Actor != "player" || ev.Roll == 0 {
			continue
		}
		switch ev.Action {
		case "hit", "block":
			hits++
		case "crit":
			hits++
			crits++
		case "miss":
			misses++
			if ev.Desc == "fumble" {
				fumbles++
			}
		default:
			continue
		}
		if !bestSeen || ev.Roll > bestRoll {
			bestRoll = ev.Roll
			bestSeen = true
		}
	}
	total := hits + misses
	if total == 0 {
		return ""
	}

	var b []byte
	b = append(b, []byte("🎲 d20 — ")...)
	b = appendInt(b, hits)
	b = append(b, '/')
	b = appendInt(b, total)
	b = append(b, []byte(" hit")...)

	notes := []string{}
	if crits > 0 {
		notes = append(notes, formatN(crits, "crit"))
	}
	if fumbles > 0 {
		notes = append(notes, formatN(fumbles, "fumble"))
	}
	if len(notes) > 0 {
		b = append(b, []byte(" (")...)
		for i, n := range notes {
			if i > 0 {
				b = append(b, []byte(", ")...)
			}
			b = append(b, []byte(n)...)
		}
		b = append(b, ')')
	}
	if bestSeen {
		b = append(b, []byte(". Best: ")...)
		if bestRoll == 20 {
			b = append(b, []byte("nat 20!")...)
		} else {
			b = appendInt(b, bestRoll)
			b = append(b, '.')
		}
	} else {
		b = append(b, '.')
	}
	// Append narrative for nat 20 / nat 1 occurrences. One line per fight
	// regardless of how many rolled — avoids spam.
	sawNat20, sawNat1 := bestRoll == 20, fumbles > 0
	if sawNat20 {
		if line := dndNat20Line(); line != "" {
			b = append(b, []byte("\n_")...)
			b = append(b, []byte(line)...)
			b = append(b, '_')
		}
	}
	if sawNat1 && !sawNat20 {
		if line := dndNat1Line(); line != "" {
			b = append(b, []byte("\n_")...)
			b = append(b, []byte(line)...)
			b = append(b, '_')
		}
	}
	return string(b)
}

func appendInt(b []byte, n int) []byte {
	if n == 0 {
		return append(b, '0')
	}
	if n < 0 {
		b = append(b, '-')
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return append(b, digits...)
}

func formatN(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	out := []byte{}
	out = appendInt(out, n)
	out = append(out, ' ')
	out = append(out, []byte(word+"s")...)
	return string(out)
}

// ── Combat HP scaling (rest teeth) ───────────────────────────────────────────

// dndWoundFloor — when scaling combat MaxHP from sheet HP%, never reduce
// below this fraction of the legacy max. Prevents one-shot deaths when the
// sheet is at 0 HP.
const dndWoundFloor = 0.25

// applyDnDHPScaling scales playerStats.MaxHP based on the player's current
// dnd_character HP fraction. A fully-rested player fights at full legacy HP;
// a wounded player fights at reduced HP, with a floor at dndWoundFloor.
//
// This is what makes !rest mechanically meaningful — without it, the rest
// system is purely cosmetic.
func applyDnDHPScaling(stats *CombatStats, c *DnDCharacter) {
	if c == nil || c.HPMax <= 0 {
		return
	}
	pct := float64(c.HPCurrent) / float64(c.HPMax)
	if pct >= 1.0 {
		return
	}
	if pct < dndWoundFloor {
		pct = dndWoundFloor
	}
	scaled := int(float64(stats.MaxHP) * pct)
	if scaled < 1 {
		scaled = 1
	}
	// Defensive: pct is clamped above to [floor, 1.0], so scaled should
	// always be ≤ original MaxHP. Belt-and-suspenders in case the floor
	// or pct math drifts in a future refactor.
	if scaled > stats.MaxHP {
		scaled = stats.MaxHP
	}
	stats.MaxHP = scaled
}

// ── HP persistence ───────────────────────────────────────────────────────────

// persistDnDHPAfterCombat updates dnd_character.hp_current to reflect wounds
// from a fight, scaled to the D&D HP scale (since combat uses legacy HP).
//
// We compute the % of legacy HP the player ended with and apply the same %
// to dnd_character.hp_max. This keeps the player's "displayed health" in
// the sheet honest even though the combat engine itself uses legacy HP.
//
// No-op if the player has no dnd_character row or hasn't completed setup.
func persistDnDHPAfterCombat(userID id.UserID, legacyStartHP, legacyEndHP int) {
	c, err := LoadDnDCharacter(userID)
	if err != nil || c == nil || c.PendingSetup {
		return
	}
	if legacyStartHP <= 0 || c.HPMax <= 0 {
		return
	}

	pct := float64(legacyEndHP) / float64(legacyStartHP)
	if pct < 0 {
		pct = 0
	} else if pct > 1 {
		pct = 1
	}

	newHP := int(float64(c.HPMax) * pct)
	if newHP < 0 {
		newHP = 0
	}
	if newHP > c.HPMax {
		newHP = c.HPMax
	}

	if _, err := db.Get().Exec(
		`UPDATE dnd_character SET hp_current = ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?`,
		newHP, string(userID),
	); err != nil {
		slog.Error("dnd: persist hp after combat", "user", userID, "err", err)
	}
}
