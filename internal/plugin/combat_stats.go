package plugin

import (
	"math/rand/v2"
	"time"
)

// DerivePlayerStats converts game-layer objects into the combat engine's stat model.
func DerivePlayerStats(
	char *AdventureCharacter,
	equip map[EquipmentSlot]*AdvEquipment,
	bonuses *AdvBonusSummary,
	chatLevel int,
	streak int,
	hasGrudge bool,
) (CombatStats, CombatModifiers) {
	arenaSets := advEquippedArenaSets(equip)

	// Stat baselines. CombatLevel is dead — player power keys off gear here
	// and dnd_level / ability scores via applyDnDPlayerLayer afterwards.
	// MaxHP is computed below only so the armor% / arena% / housing% bonus
	// formulas have something to scale against; the base is subtracted out
	// when we capture stats.HPBonus, so it never reaches combat.
	const legacyBase = 50
	stats := CombatStats{
		MaxHP:     legacyBase,
		Attack:    5 + bonuses.CombatBonus,
		Defense:   3 + bonuses.CombatBonus/2,
		Speed:     5,
		CritRate:  0.03,
		DodgeRate: 0.02,
		BlockRate: 0.02,
	}

	// Equipment contributions
	for _, slot := range allSlots {
		eq, ok := equip[slot]
		if !ok || eq == nil {
			continue
		}
		eTier := advEffectiveTier(eq)
		cond := 0.3 + 0.7*(float64(eq.Condition)/100.0) // smooth degradation curve
		effective := eTier * cond

		switch slot {
		case SlotWeapon:
			stats.Attack += int(effective * 2)
			stats.CritRate += eTier * 0.005
		case SlotArmor:
			stats.Defense += int(effective * 1.5)
			stats.MaxHP += int(float64(stats.MaxHP) * eTier * 0.03)
		case SlotHelmet:
			stats.Defense += int(effective * 0.5)
			stats.DodgeRate += eTier * 0.01
		case SlotBoots:
			stats.Speed += int(effective)
			stats.DodgeRate += eTier * 0.005
		case SlotTool:
			stats.Attack += int(effective * 0.5)
			stats.BlockRate += eTier * 0.01
		}
	}

	// Arena set bonuses
	if arenaSets["champions"] {
		stats.MaxHP = int(float64(stats.MaxHP) * 1.10)
		stats.Attack = int(float64(stats.Attack) * 1.10)
		stats.Defense = int(float64(stats.Defense) * 1.10)
		stats.Speed = int(float64(stats.Speed) * 1.10)
	}
	if arenaSets["bloodied"] {
		stats.CritRate += 0.03
	}
	if arenaSets["ironclad"] {
		stats.MaxHP = int(float64(stats.MaxHP) * 1.05)
	}
	// Sovereign: handled via DeathSave modifier
	// Tempered: handled post-combat in degradation

	// Housing HP bonus
	stats.MaxHP += int(float64(stats.MaxHP) * char.HouseHPBonus())

	// Capture the equipment/arena/housing HP delta as fight-only headroom.
	// applyDnDPlayerLayer adds this to c.HPMax to form combat MaxHP — gear
	// power preserved, dnd_character.hp_current is the canonical wound store
	// with no scale conversion. The legacy 50+CL*2 base is intentionally
	// dropped; monster damage is tuned to the dnd HP scale.
	stats.HPBonus = stats.MaxHP - legacyBase

	// Streak bonuses
	switch {
	case streak >= 30:
		stats.Attack = int(float64(stats.Attack) * 1.20)
		stats.Defense = int(float64(stats.Defense) * 1.15)
	case streak >= 14:
		stats.Attack = int(float64(stats.Attack) * 1.15)
		stats.Defense = int(float64(stats.Defense) * 1.10)
	case streak >= 7:
		stats.Attack = int(float64(stats.Attack) * 1.10)
		stats.Defense = int(float64(stats.Defense) * 1.05)
	case streak >= 3:
		stats.Attack = int(float64(stats.Attack) * 1.05)
	}

	// Grudge bonus
	if hasGrudge {
		stats.Attack = int(float64(stats.Attack) * 1.10)
	}

	// Treasure bonuses mapped to stats
	if bonuses.DeathModifier < 0 {
		stats.Defense += int(-bonuses.DeathModifier * 2)
	}
	if bonuses.SuccessBonus > 0 {
		stats.Attack += int(bonuses.SuccessBonus * 0.5)
	}
	if bonuses.ExceptionalBonus > 0 {
		stats.CritRate += bonuses.ExceptionalBonus / 100.0
	}

	// Chat level perks
	chatTier := chatLevel / 10
	if chatTier > 5 {
		chatTier = 5
	}
	stats.CritRate += float64(chatTier) * 0.005

	// Cap rates
	if stats.CritRate > 0.50 {
		stats.CritRate = 0.50
	}
	if stats.DodgeRate > 0.40 {
		stats.DodgeRate = 0.40
	}
	if stats.BlockRate > 0.40 {
		stats.BlockRate = 0.40
	}

	// Modifiers
	mods := CombatModifiers{
		DamageBonus:  0,
		DamageReduct: 1.0,
	}

	// Streak damage reduction
	switch {
	case streak >= 30:
		mods.DamageReduct = 0.95
	case streak >= 14:
		mods.DamageReduct = 0.97
	}

	// Sovereign death save
	if arenaSets["sovereign"] {
		if char.DeathReprieveLast == nil || time.Since(*char.DeathReprieveLast) >= 168*time.Hour {
			mods.DeathSave = true
		}
	}

	// Pet modifiers. Two pets each contribute at half weight — the combat mods
	// are an average over the active pets, so a pair reads as roughly one full
	// pet (flavor-forward, not a stat spike). A lone pet averages over one
	// element, so x/1==x and 0.0+x==x reproduce the former values exactly,
	// keeping the single-pet combat golden byte-identical. The engines roll one
	// attack/deflect/whiff off these mods per round regardless of pet count, so
	// the RNG draw order is unchanged too.
	pets := make([]struct{ level, armor int }, 0, 2)
	if char.HasPet() {
		pets = append(pets, struct{ level, armor int }{char.PetLevel, char.PetArmorTier})
	}
	if char.HasPet2() {
		pets = append(pets, struct{ level, armor int }{char.Pet2Level, char.Pet2ArmorTier})
	}
	if n := len(pets); n > 0 {
		var atk, defl, whiff float64
		levelSum := 0
		for _, pt := range pets {
			atk += petAttackChance(pt.level)
			defl += petDeflectChance(pt.level, pt.armor)
			whiff += 0.01 + float64(pt.level)*0.005
			levelSum += pt.level
		}
		mods.PetAttackProc = atk / float64(n)
		mods.PetDeflectProc = defl / float64(n)
		mods.PetWhiffProc = whiff / float64(n)
		mods.PetAttackDmg = 3 + (levelSum*2+n)/(2*n) // 3 + rounded average level
	}
	if char.PetMorningDefense {
		mods.DamageReduct *= 0.95 // 5% less damage
	}

	// NPC debuffs
	now := time.Now().UTC()
	if char.MistyDebuffExpires != nil && now.Before(*char.MistyDebuffExpires) {
		mods.CrowdRevengeProc = 0.20
		mods.CrowdRevengeDmg = 3 + rand.IntN(6) // 3-8 damage
	}

	// NPC buffs
	if char.MistyBuffExpires != nil && now.Before(*char.MistyBuffExpires) {
		mods.MistyHealProc = 0.20
		mods.MistyHealAmt = 8 + char.CombatLevel/5 + rand.IntN(5)
	}
	if char.ArinaBuffExpires != nil && now.Before(*char.ArinaBuffExpires) {
		mods.SniperKillProc = 0.08
	}

	return stats, mods
}

// DeriveArenaMonsterStats converts an ArenaMonster to combat engine stats.
// Arena monsters face players with high combat level and arena-tier gear,
// so stats must scale hard with ThreatLevel.
func DeriveArenaMonsterStats(monster *ArenaMonster) (CombatStats, CombatModifiers) {
	tl := float64(monster.ThreatLevel)
	bl := monster.BaseLethality

	stats := CombatStats{
		MaxHP:     40 + int(tl*4+bl*60),
		Attack:    8 + int(bl*40) + int(tl*0.8),
		Defense:   3 + int(tl*0.5+bl*10),
		Speed:     5 + int(tl*0.3),
		CritRate:  bl * 0.20,
		DodgeRate: 0.02 + tl*0.003,
		BlockRate: 0.01 + bl*0.03,
	}
	mods := CombatModifiers{DamageBonus: 0, DamageReduct: 1.0}
	return stats, mods
}

// DeriveDungeonMonsterStats synthesizes monster stats from a dungeon location.
// Tuned to the dnd HP scale (post HP-unification): well-equipped players at
// the location's MinLevel should win the vast majority of fights, with deaths
// coming from bad luck (crits, hazards, initiative). Old quadratic Attack
// formula assumed legacy ~100 HP players; the new linear scaling matches the
// ~12–80 HP range of dnd-scale fighters.
func DeriveDungeonMonsterStats(loc *AdvLocation) (CombatStats, CombatModifiers) {
	t := float64(loc.Tier)
	death := loc.BaseDeathPct // 8 to 60

	stats := CombatStats{
		MaxHP:     12 + int(t*7+death*0.3),      // T1≈22, T5≈65 — sized so dnd-scale players can finish a kill within the phase budget
		Attack:    int(t*1.5) + int(death*0.04), // T1=1, T5=9 — tuned to dnd HP pools (players have 13–80 HP, not 100+)
		Defense:   2 + int(t*1.2),
		Speed:     4 + int(t*1.5),
		CritRate:  0.03 + death*0.003,
		DodgeRate: 0.02 + t*0.008,
		BlockRate: 0.01 + t*0.005,
	}
	mods := CombatModifiers{DamageBonus: 0, DamageReduct: 1.0}
	return stats, mods
}
