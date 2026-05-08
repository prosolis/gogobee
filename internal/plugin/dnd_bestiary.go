package plugin

// Phase 7 — D&D starter bestiary.
//
// These are named monster definitions ready to slot into encounter content.
// Phase 7 ships them as data only — wiring them into specific dungeons or
// arena rounds is a future content task. The combat engine consumes them
// via the existing DnDMonsterTemplate→CombatStats path (see toCombatStats).

// DnDMonsterTemplate is the bestiary record for a named creature. Mirrors
// v1.0 §8.1's stat block schema, simplified to the subset combat actually
// uses (HP, AC, attack, speed, ability proc, special tags).
type DnDMonsterTemplate struct {
	ID          string
	Name        string
	CR          float32 // challenge rating (display only for now)
	HP          int
	AC          int
	Attack      int  // engine's "Attack" stat (raw damage value)
	AttackBonus int  // d20 to-hit bonus
	Speed       int
	BlockRate   float64
	Ability     *MonsterAbility
	XPValue     int
	Notes       string
}

// dndBestiary is the canonical lookup. Keyed by ID.
var dndBestiary = map[string]DnDMonsterTemplate{
	"goblin": {
		ID: "goblin", Name: "Goblin",
		CR: 0.25, HP: 7, AC: 13, Attack: 6, AttackBonus: 4, Speed: 14,
		BlockRate: 0.05,
		Ability:   &MonsterAbility{Name: "Nimble Escape", Phase: "any", ProcChance: 0.20, Effect: "stun"},
		XPValue:   30,
		Notes:     "Fast and skittish. Escapes pin attempts; will retreat if obviously outmatched.",
	},
	"skeleton": {
		ID: "skeleton", Name: "Skeleton",
		CR: 0.25, HP: 13, AC: 13, Attack: 8, AttackBonus: 4, Speed: 10,
		BlockRate: 0.10,
		// Skeletons are immune to poison; we'd model that with a future
		// CombatModifiers.PoisonImmunity flag. For Phase 7, no ability.
		XPValue: 50,
		Notes:   "Bone-and-rust. Resistant to slashing weapons; vulnerable to bludgeoning.",
	},
	"orc_grunt": {
		ID: "orc_grunt", Name: "Orc Grunt",
		CR: 0.5, HP: 15, AC: 13, Attack: 10, AttackBonus: 5, Speed: 11,
		BlockRate: 0.05,
		Ability:   &MonsterAbility{Name: "Aggressive", Phase: "opening", ProcChance: 0.30, Effect: "enrage"},
		XPValue:   100,
		Notes:     "Charges. Bonus damage on its first hit if it goes first.",
	},
	"troll": {
		ID: "troll", Name: "Troll",
		CR: 5, HP: 84, AC: 15, Attack: 22, AttackBonus: 7, Speed: 8,
		BlockRate: 0.10,
		Ability:   &MonsterAbility{Name: "Regeneration", Phase: "any", ProcChance: 0.50, Effect: "lifesteal"},
		XPValue:   1800,
		Notes:     "Regenerates each round unless burned or acid-burned. Fire/acid damage stops the regen.",
	},
	"wyvern": {
		ID: "wyvern", Name: "Wyvern",
		CR: 6, HP: 110, AC: 13, Attack: 28, AttackBonus: 7, Speed: 16,
		BlockRate: 0.05,
		Ability:   &MonsterAbility{Name: "Poison Sting", Phase: "decisive", ProcChance: 0.60, Effect: "poison"},
		XPValue:   2300,
		Notes:     "Aerial; opens with dive attack. Tail sting injects poison (CON DC 15 in tabletop).",
	},
	"ancient_dragon": {
		ID: "ancient_dragon", Name: "Ancient Dragon",
		CR: 20, HP: 367, AC: 22, Attack: 65, AttackBonus: 14, Speed: 18,
		BlockRate: 0.15,
		Ability:   &MonsterAbility{Name: "Frightful Presence", Phase: "opening", ProcChance: 0.80, Effect: "stun"},
		XPValue:   25000,
		Notes:     "Legendary. Breath weapon (cleave-equivalent), frightful presence on opening, regenerates between phases. Tabletop equivalent has legendary actions; the engine approximates with elevated stats.",
	},
}

// ---- Tier 1 zone roster (Phase 11 D1a) ---------------------------------------
//
// Goblin Warrens + Crypt of Valdris enemy entries. Stat blocks per
// gogobee_dungeon_zones.md §5. AttackBonus and Attack are derived from
// each creature's primary attack profile in 5e (rounded for the engine's
// integer "Attack" stat — engine treats this as average damage).

var _ = func() bool {
	tier1 := map[string]DnDMonsterTemplate{
		"goblin_sneak": {
			ID: "goblin_sneak", Name: "Goblin Sneak",
			CR: 0.25, HP: 7, AC: 13, Attack: 6, AttackBonus: 4, Speed: 14,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Pack Tactics", Phase: "any", ProcChance: 0.25, Effect: "advantage"},
			XPValue:   30,
			Notes:     "Nimble Escape; +1d4 if ally adjacent.",
		},
		"goblin_archer": {
			ID: "goblin_archer", Name: "Goblin Archer",
			CR: 0.25, HP: 7, AC: 13, Attack: 5, AttackBonus: 4, Speed: 14,
			BlockRate: 0.0,
			Ability:   &MonsterAbility{Name: "Scurry", Phase: "any", ProcChance: 0.30, Effect: "evade"},
			XPValue:   30,
			Notes:     "Ranged only. Disengages after attack.",
		},
		"hobgoblin_grunt": {
			ID: "hobgoblin_grunt", Name: "Hobgoblin Grunt",
			CR: 0.5, HP: 11, AC: 18, Attack: 9, AttackBonus: 3, Speed: 11,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Martial Advantage", Phase: "any", ProcChance: 0.30, Effect: "bonus_damage"},
			XPValue:   100,
			Notes:     "Pack-tactics variant. Formation bonus when grouped.",
		},
		"worg": {
			ID: "worg", Name: "Worg",
			CR: 0.5, HP: 26, AC: 13, Attack: 10, AttackBonus: 5, Speed: 17,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Knock Prone", Phase: "any", ProcChance: 0.25, Effect: "stun"},
			XPValue:   100,
			Notes:     "Carries goblin riders. STR DC 13 to avoid prone on hit.",
		},
		"goblin_shaman": {
			ID: "goblin_shaman", Name: "Goblin Shaman",
			CR: 1, HP: 22, AC: 11, Attack: 12, AttackBonus: 4, Speed: 12,
			BlockRate: 0.0,
			Ability:   &MonsterAbility{Name: "Burning Hands", Phase: "opening", ProcChance: 0.50, Effect: "aoe_fire"},
			XPValue:   200,
			Notes:     "Burning Hands 2d6 fire DEX DC 11; Healing Word on allies.",
		},
		"hobgoblin_warchief": {
			ID: "hobgoblin_warchief", Name: "Hobgoblin Warchief",
			CR: 2, HP: 52, AC: 18, Attack: 18, AttackBonus: 5, Speed: 11,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Leadership Aura", Phase: "any", ProcChance: 0.40, Effect: "ally_buff"},
			XPValue:   450,
			Notes:     "Elite. +1d4 to ally attacks; multiattack.",
		},
		"zombie": {
			ID: "zombie", Name: "Zombie",
			CR: 0.25, HP: 22, AC: 10, Attack: 5, AttackBonus: 3, Speed: 6,
			BlockRate: 0.0,
			Ability:   &MonsterAbility{Name: "Undead Fortitude", Phase: "decisive", ProcChance: 0.50, Effect: "survive_at_1"},
			XPValue:   50,
			Notes:     "CON DC 5 + damage dealt or survive at 1 HP. AC bumped 8→10 to satisfy engine min.",
		},
		"shadow": {
			ID: "shadow", Name: "Shadow",
			CR: 0.5, HP: 16, AC: 12, Attack: 9, AttackBonus: 4, Speed: 12,
			BlockRate: 0.0,
			Ability:   &MonsterAbility{Name: "Strength Drain", Phase: "any", ProcChance: 0.35, Effect: "stat_drain"},
			XPValue:   100,
			Notes:     "Hit reduces STR by 1d4 until long rest. Resist non-magical physical.",
		},
		"specter": {
			ID: "specter", Name: "Specter",
			CR: 1, HP: 22, AC: 12, Attack: 10, AttackBonus: 4, Speed: 14,
			BlockRate: 0.0,
			Ability:   &MonsterAbility{Name: "Life Drain", Phase: "any", ProcChance: 0.40, Effect: "lifesteal"},
			XPValue:   200,
			Notes:     "Incorporeal: non-magical weapons deal half damage.",
		},
		"wight": {
			ID: "wight", Name: "Wight",
			CR: 3, HP: 45, AC: 14, Attack: 14, AttackBonus: 4, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Life Drain", Phase: "any", ProcChance: 0.45, Effect: "lifesteal"},
			XPValue:   700,
			Notes:     "Elite. Raises slain humanoids as zombies.",
		},
		"flameskull": {
			ID: "flameskull", Name: "Flameskull",
			CR: 4, HP: 40, AC: 13, Attack: 18, AttackBonus: 5, Speed: 15,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Fireball", Phase: "decisive", ProcChance: 0.55, Effect: "aoe_fire"},
			XPValue:   1100,
			Notes:     "Elite. Fireball 8d6 DEX DC 13. Rejuvenation in 1h unless holy water used.",
		},
		"boss_grol_unbroken": {
			ID: "boss_grol_unbroken", Name: "Grol the Unbroken",
			CR: 3, HP: 65, AC: 17, Attack: 22, AttackBonus: 5, Speed: 12,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Surprise Attack", Phase: "opening", ProcChance: 1.0, Effect: "bonus_damage"},
			XPValue:   700,
			Notes:     "Goblin Warrens boss. Bugbear war-chief.",
		},
		"boss_valdris_unburied": {
			ID: "boss_valdris_unburied", Name: "Valdris the Unburied",
			CR: 5, HP: 97, AC: 17, Attack: 28, AttackBonus: 7, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Corrupting Touch", Phase: "any", ProcChance: 0.50, Effect: "max_hp_drain"},
			XPValue:   1800,
			Notes:     "Crypt of Valdris boss. Lich-aspirant; phase 2 below 50% HP.",
		},
	}
	for id, m := range tier1 {
		dndBestiary[id] = m
	}
	return true
}()

// ---- Tier 2 zone roster (Phase 11 D3a) ---------------------------------------
//
// Forest of Shadows + Sunken Temple of Dar'eth enemy entries. Stat blocks
// per gogobee_dungeon_zones.md §5. Same Attack-as-average-damage convention
// as Tier 1: Attack ≈ avg of primary attack profile, AttackBonus = d20
// to-hit modifier. Elites flagged via the zone roster, not here.

var _ = func() bool {
	tier2 := map[string]DnDMonsterTemplate{
		"dire_wolf": {
			ID: "dire_wolf", Name: "Dire Wolf",
			CR: 1, HP: 37, AC: 14, Attack: 10, AttackBonus: 5, Speed: 16,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Pack Tactics", Phase: "any", ProcChance: 0.30, Effect: "advantage"},
			XPValue:   200,
			Notes:     "Knock Prone on hit (STR DC 13); +1d6 if ally adjacent.",
		},
		"bandit_captain": {
			ID: "bandit_captain", Name: "Bandit Captain",
			CR: 2, HP: 65, AC: 15, Attack: 14, AttackBonus: 5, Speed: 12,
			BlockRate: 0.20,
			Ability:   &MonsterAbility{Name: "Parry", Phase: "any", ProcChance: 0.35, Effect: "block"},
			XPValue:   450,
			Notes:     "Multiattack 3x; reaction Parry adds +3 AC vs one attack.",
		},
		"owlbear": {
			ID: "owlbear", Name: "Owlbear",
			CR: 3, HP: 59, AC: 13, Attack: 16, AttackBonus: 7, Speed: 12,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Grapple", Phase: "any", ProcChance: 0.40, Effect: "stun"},
			XPValue:   700,
			Notes:     "Beak + Claw multiattack. Claw hit triggers grapple attempt.",
		},
		"dryad_corrupted": {
			ID: "dryad_corrupted", Name: "Corrupted Dryad",
			CR: 2, HP: 22, AC: 11, Attack: 11, AttackBonus: 4, Speed: 12,
			BlockRate: 0.0,
			Ability:   &MonsterAbility{Name: "Fey Charm", Phase: "any", ProcChance: 0.40, Effect: "stun"},
			XPValue:   450,
			Notes:     "Barkskin self-buff (+2 AC); WIS DC 14 vs charm or waste a turn.",
		},
		"displacer_beast": {
			ID: "displacer_beast", Name: "Displacer Beast",
			CR: 3, HP: 85, AC: 13, Attack: 14, AttackBonus: 6, Speed: 14,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Displacement", Phase: "opening", ProcChance: 0.50, Effect: "evade"},
			XPValue:   700,
			Notes:     "First hit on player each round has disadvantage; Avoidance trait.",
		},
		"green_hag": {
			ID: "green_hag", Name: "Green Hag",
			CR: 3, HP: 82, AC: 17, Attack: 15, AttackBonus: 5, Speed: 12,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Invisible Passage", Phase: "any", ProcChance: 0.35, Effect: "evade"},
			XPValue:   700,
			Notes:     "Elite. Illusory Appearance; Weakness curse on hit.",
		},
		"kuo_toa": {
			ID: "kuo_toa", Name: "Kuo-toa",
			CR: 0.25, HP: 18, AC: 13, Attack: 5, AttackBonus: 3, Speed: 10,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Sticky Shield", Phase: "any", ProcChance: 0.25, Effect: "block"},
			XPValue:   30,
			Notes:     "Amphibious. Slippery (auto-escape grapple).",
		},
		"kuo_toa_whip": {
			ID: "kuo_toa_whip", Name: "Kuo-toa Whip",
			CR: 1, HP: 65, AC: 11, Attack: 11, AttackBonus: 3, Speed: 10,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Divine Eminence", Phase: "decisive", ProcChance: 0.50, Effect: "bonus_damage"},
			XPValue:   200,
			Notes:     "Pincer Staff grapple on hit; +2d6 psychic on Eminence proc.",
		},
		"water_elemental": {
			ID: "water_elemental", Name: "Water Elemental",
			CR: 5, HP: 114, AC: 14, Attack: 26, AttackBonus: 7, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Whelm", Phase: "any", ProcChance: 0.40, Effect: "stun"},
			XPValue:   1800,
			Notes:     "Elite. Whelm grapples + drowning (CON DC 13). Freeze reaction.",
		},
		"merrow": {
			ID: "merrow", Name: "Merrow",
			CR: 2, HP: 45, AC: 13, Attack: 13, AttackBonus: 5, Speed: 12,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Harpoon Pull", Phase: "opening", ProcChance: 0.40, Effect: "stun"},
			XPValue:   450,
			Notes:     "Amphibious. Harpoon yanks target 15 ft; multiattack.",
		},
		"aboleth_thrall": {
			ID: "aboleth_thrall", Name: "Aboleth Thrall",
			CR: 3, HP: 60, AC: 12, Attack: 14, AttackBonus: 5, Speed: 11,
			BlockRate: 0.0,
			Ability:   &MonsterAbility{Name: "Psychic Bond", Phase: "any", ProcChance: 0.35, Effect: "reveal_action"},
			XPValue:   700,
			Notes:     "WIS DC 14 or telegraph next action. Mucus Cloud aura.",
		},
		"boss_hollow_king": {
			ID: "boss_hollow_king", Name: "The Hollow King",
			CR: 6, HP: 142, AC: 15, Attack: 30, AttackBonus: 7, Speed: 13,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Root Surge", Phase: "any", ProcChance: 0.45, Effect: "stun"},
			XPValue:   2300,
			Notes:     "Forest of Shadows boss. Phase 2 below 40% HP — summons 2 Dire Wolves.",
		},
		"boss_dreaming_aboleth": {
			ID: "boss_dreaming_aboleth", Name: "The Dreaming Aboleth",
			CR: 10, HP: 135, AC: 17, Attack: 36, AttackBonus: 9, Speed: 10,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Enslave", Phase: "any", ProcChance: 0.40, Effect: "stun"},
			XPValue:   5900,
			Notes:     "Sunken Temple boss. Tentacle multiattack; legendary actions; psychic drain.",
		},
	}
	for id, m := range tier2 {
		dndBestiary[id] = m
	}
	return true
}()

// ---- Tier 3 zone roster (Phase 11 D4a) ---------------------------------------
//
// Haunted Manor of Blackspire + The Underforge enemy entries. Stat blocks
// per gogobee_dungeon_zones.md §5. Same Attack-as-average-damage convention
// as prior tiers; AttackBonus = d20 to-hit modifier; elites flagged via
// the zone roster, not here. Shadow + Flameskull reuse Tier 1 entries.

var _ = func() bool {
	tier3 := map[string]DnDMonsterTemplate{
		"poltergeist": {
			ID: "poltergeist", Name: "Poltergeist",
			CR: 2, HP: 22, AC: 12, Attack: 10, AttackBonus: 4, Speed: 12,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Telekinetic Thrust", Phase: "any", ProcChance: 0.40, Effect: "stun"},
			XPValue:   450,
			Notes:     "Invisible: melee attackers have disadvantage. Forceful Slam (bludgeoning).",
		},
		"banshee": {
			ID: "banshee", Name: "Banshee",
			CR: 4, HP: 58, AC: 12, Attack: 14, AttackBonus: 4, Speed: 12,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Wail", Phase: "decisive", ProcChance: 0.20, Effect: "execute"},
			XPValue:   1100,
			Notes:     "Wail (1/combat): WIS DC 13 or drop to 0 HP. Corrupting Touch; undead nature.",
		},
		"wraith": {
			ID: "wraith", Name: "Wraith",
			CR: 5, HP: 67, AC: 13, Attack: 21, AttackBonus: 6, Speed: 16,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Life Drain", Phase: "any", ProcChance: 0.40, Effect: "max_hp_drain"},
			XPValue:   1800,
			Notes:     "Incorporeal movement; sunlight sensitivity; resist non-magical physical.",
		},
		"vampire_spawn": {
			ID: "vampire_spawn", Name: "Vampire Spawn",
			CR: 5, HP: 82, AC: 15, Attack: 16, AttackBonus: 6, Speed: 12,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Bite (Life Drain)", Phase: "any", ProcChance: 0.40, Effect: "self_heal"},
			XPValue:   1800,
			Notes:     "Misty Step; Charm Gaze (WIS DC 13). Bite heals vampire for damage dealt.",
		},
		"revenant": {
			ID: "revenant", Name: "Revenant",
			CR: 5, HP: 136, AC: 13, Attack: 25, AttackBonus: 7, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Vengeful Regeneration", Phase: "any", ProcChance: 0.50, Effect: "regenerate"},
			XPValue:   1800,
			Notes:     "Elite. +10 HP/turn regen; cannot die permanently until goal fulfilled; STR DC 20 grapple.",
		},
		"boss_aldric_blackspire": {
			ID: "boss_aldric_blackspire", Name: "Lord Aldric Blackspire",
			CR: 13, HP: 144, AC: 16, Attack: 38, AttackBonus: 9, Speed: 14,
			BlockRate: 0.20,
			Ability:   &MonsterAbility{Name: "Charm", Phase: "any", ProcChance: 0.40, Effect: "stun"},
			XPValue:   10000,
			Notes:     "Manor Blackspire boss. Mist Form retreat; Children of the Night summon; Legendary Resistance 3/combat.",
		},
		"magmin": {
			ID: "magmin", Name: "Magmin",
			CR: 0.5, HP: 9, AC: 14, Attack: 7, AttackBonus: 3, Speed: 10,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Death Burst", Phase: "decisive", ProcChance: 1.00, Effect: "death_aoe"},
			XPValue:   100,
			Notes:     "Death Burst: 2d6 fire in 10 ft on death. Ignite flammable objects on touch.",
		},
		"azer": {
			ID: "azer", Name: "Azer",
			CR: 2, HP: 39, AC: 17, Attack: 11, AttackBonus: 5, Speed: 12,
			BlockRate: 0.20,
			Ability:   &MonsterAbility{Name: "Fire Aura", Phase: "any", ProcChance: 0.50, Effect: "retaliate"},
			XPValue:   450,
			Notes:     "1d10 fire to melee attackers; immune to fire; weapons deal heated bonus damage.",
		},
		"salamander": {
			ID: "salamander", Name: "Salamander",
			CR: 5, HP: 90, AC: 15, Attack: 22, AttackBonus: 7, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Constrict", Phase: "any", ProcChance: 0.40, Effect: "stun"},
			XPValue:   1800,
			Notes:     "Constrict grapple + 2d6 fire/turn; spear multiattack; fire aura.",
		},
		"fire_elemental": {
			ID: "fire_elemental", Name: "Fire Elemental",
			CR: 5, HP: 102, AC: 13, Attack: 20, AttackBonus: 6, Speed: 14,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Fire Form", Phase: "any", ProcChance: 0.50, Effect: "retaliate"},
			XPValue:   1800,
			Notes:     "1d10 contact fire damage; immune to fire; vulnerable to water/cold.",
		},
		"helmed_horror": {
			ID: "helmed_horror", Name: "Helmed Horror",
			CR: 4, HP: 60, AC: 20, Attack: 16, AttackBonus: 6, Speed: 12,
			BlockRate: 0.25,
			Ability:   &MonsterAbility{Name: "Spell Immunity", Phase: "any", ProcChance: 1.00, Effect: "spell_resist"},
			XPValue:   1100,
			Notes:     "Elite. Immune to 3 specific spells (rolled per instance); multiattack; magic resistance.",
		},
		"boss_emberlord_thyrak": {
			ID: "boss_emberlord_thyrak", Name: "Emberlord Thyrak",
			CR: 9, HP: 178, AC: 18, Attack: 34, AttackBonus: 8, Speed: 11,
			BlockRate: 0.20,
			Ability:   &MonsterAbility{Name: "Forge Breath", Phase: "any", ProcChance: 0.30, Effect: "aoe"},
			XPValue:   5000,
			Notes:     "Underforge boss. Phase 2 below 50% HP — spawns 2 Fire Elementals; breath recharge 4–6.",
		},
	}
	for id, m := range tier3 {
		dndBestiary[id] = m
	}
	return true
}()

// ---- Tier 4-5 zone roster (Phase 11 D5a) -------------------------------------
//
// Underdark + Feywild Crossing (T4) and Dragon's Lair + Abyss Portal (T5)
// enemy entries. Stat blocks per gogobee_dungeon_zones.md §5. Same
// Attack-as-average-damage convention as prior tiers; AttackBonus = d20
// to-hit modifier; elites flagged via the zone roster, not here.
// Feywild reuses Tier 2's green_hag entry.

var _ = func() bool {
	tier45 := map[string]DnDMonsterTemplate{
		// ── Underdark ────────────────────────────────────────────────────
		"drow": {
			ID: "drow", Name: "Drow",
			CR: 0.25, HP: 13, AC: 15, Attack: 5, AttackBonus: 4, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Drow Poison", Phase: "any", ProcChance: 0.30, Effect: "stun"},
			XPValue:   50,
			Notes:     "Fey Ancestry (adv. vs charm); Sunlight Sensitivity; Hand Crossbow poison bolt.",
		},
		"drow_elite_warrior": {
			ID: "drow_elite_warrior", Name: "Drow Elite Warrior",
			CR: 5, HP: 71, AC: 18, Attack: 18, AttackBonus: 7, Speed: 12,
			BlockRate: 0.20,
			Ability:   &MonsterAbility{Name: "Parry", Phase: "any", ProcChance: 0.35, Effect: "block"},
			XPValue:   1800,
			Notes:     "Multiattack 3 hits; Parry reaction; Drow Poison (CON DC 13 unconscious).",
		},
		"drow_mage": {
			ID: "drow_mage", Name: "Drow Mage",
			CR: 7, HP: 45, AC: 12, Attack: 22, AttackBonus: 5, Speed: 12,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Lightning Bolt", Phase: "decisive", ProcChance: 0.45, Effect: "aoe"},
			XPValue:   2900,
			Notes:     "Spells: Lightning Bolt, Fly, Darkness, Faerie Fire. Magic Resistance.",
		},
		"mind_flayer": {
			ID: "mind_flayer", Name: "Mind Flayer",
			CR: 7, HP: 71, AC: 15, Attack: 26, AttackBonus: 7, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Mind Blast", Phase: "opening", ProcChance: 0.55, Effect: "stun"},
			XPValue:   2900,
			Notes:     "Mind Blast AoE psychic INT DC 15 or Stunned; Extract Brain instakills stunned; telepathy.",
		},
		"hook_horror": {
			ID: "hook_horror", Name: "Hook Horror",
			CR: 3, HP: 75, AC: 15, Attack: 14, AttackBonus: 6, Speed: 10,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Pack Tactics", Phase: "any", ProcChance: 0.30, Effect: "advantage"},
			XPValue:   700,
			Notes:     "Echoes of Darkness blindsight 60 ft; grapple claws; pack tactics.",
		},
		"roper": {
			ID: "roper", Name: "Roper",
			CR: 5, HP: 93, AC: 20, Attack: 22, AttackBonus: 7, Speed: 4,
			BlockRate: 0.20,
			Ability:   &MonsterAbility{Name: "Reel", Phase: "any", ProcChance: 0.50, Effect: "stun"},
			XPValue:   1800,
			Notes:     "Elite. 6 tendrils grapple+restrain; Reel pulls 25 ft; False Appearance.",
		},
		"boss_ilvaras_xunyl": {
			ID: "boss_ilvaras_xunyl", Name: "Ilvaras Xunyl, Drow High Priestess",
			CR: 12, HP: 162, AC: 16, Attack: 38, AttackBonus: 9, Speed: 12,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Divine Word", Phase: "decisive", ProcChance: 0.40, Effect: "execute"},
			XPValue:   8400,
			Notes:     "Underdark boss. Legendary Actions; Lolth's Favour 1/combat; phase 2 below 35% HP — Lolth's Avatar overlay.",
		},
		// ── Feywild Crossing ─────────────────────────────────────────────
		"redcap": {
			ID: "redcap", Name: "Redcap",
			CR: 3, HP: 45, AC: 13, Attack: 16, AttackBonus: 6, Speed: 14,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Outsize Strength", Phase: "any", ProcChance: 0.40, Effect: "bonus_damage"},
			XPValue:   700,
			Notes:     "Iron Boots add bludgeoning on hit; loses power without blood-soaked cap.",
		},
		"will_o_wisp": {
			ID: "will_o_wisp", Name: "Will-o'-Wisp",
			CR: 2, HP: 22, AC: 19, Attack: 9, AttackBonus: 4, Speed: 14,
			BlockRate: 0.0,
			Ability:   &MonsterAbility{Name: "Consume Life", Phase: "any", ProcChance: 0.40, Effect: "lifesteal"},
			XPValue:   450,
			Notes:     "Variable Illumination; Ephemeral (+13 vs OAs); incorporeal.",
		},
		"quickling": {
			ID: "quickling", Name: "Quickling",
			CR: 1, HP: 10, AC: 16, Attack: 8, AttackBonus: 5, Speed: 18,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Blur", Phase: "any", ProcChance: 0.50, Effect: "evade"},
			XPValue:   200,
			Notes:     "Attacks at disadvantage vs Quickling; 3 attacks/turn; Evasion.",
		},
		"night_hag": {
			ID: "night_hag", Name: "Night Hag",
			CR: 5, HP: 112, AC: 17, Attack: 21, AttackBonus: 7, Speed: 12,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Etherealness", Phase: "any", ProcChance: 0.30, Effect: "evade"},
			XPValue:   1800,
			Notes:     "Etherealness; Dream Haunting (no long rest benefit); Heartstone.",
		},
		"fomorian": {
			ID: "fomorian", Name: "Fomorian",
			CR: 8, HP: 149, AC: 14, Attack: 30, AttackBonus: 9, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Evil Eye", Phase: "any", ProcChance: 0.45, Effect: "stun"},
			XPValue:   3900,
			Notes:     "Elite. Evil Eye WIS DC 14 (Frightened/Poisoned/Stunned — GM choice); stomp AoE.",
		},
		"boss_thornmother": {
			ID: "boss_thornmother", Name: "The Thornmother",
			CR: 11, HP: 187, AC: 17, Attack: 32, AttackBonus: 8, Speed: 12,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Beguiling Bargain", Phase: "opening", ProcChance: 0.50, Effect: "stun"},
			XPValue:   7200,
			Notes:     "Feywild Crossing boss. Coven Magic; Shapechange; phase 2 below 30% HP — True Form, +4d6 psychic, summons 2 Night Hags.",
		},
		// ── Dragon's Lair ────────────────────────────────────────────────
		"kobold": {
			ID: "kobold", Name: "Kobold",
			CR: 0.125, HP: 5, AC: 12, Attack: 4, AttackBonus: 4, Speed: 12,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Pack Tactics", Phase: "any", ProcChance: 0.40, Effect: "advantage"},
			XPValue:   25,
			Notes:     "Sunlight Sensitivity; Trap expertise; pack tactics.",
		},
		"guard_drake": {
			ID: "guard_drake", Name: "Guard Drake",
			CR: 2, HP: 52, AC: 14, Attack: 12, AttackBonus: 5, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Draconic Loyalty", Phase: "any", ProcChance: 1.00, Effect: "fear_immune"},
			XPValue:   450,
			Notes:     "Multiattack; immune to fear near dragons.",
		},
		"kobold_scale_sorcerer": {
			ID: "kobold_scale_sorcerer", Name: "Kobold Scale Sorcerer",
			CR: 1, HP: 27, AC: 15, Attack: 11, AttackBonus: 4, Speed: 12,
			BlockRate: 0.05,
			Ability:   &MonsterAbility{Name: "Chromatic Orb", Phase: "decisive", ProcChance: 0.50, Effect: "bonus_damage"},
			XPValue:   200,
			Notes:     "Spells: Chromatic Orb, Mage Armor, Shield. Draconic Sorcery.",
		},
		"dragonborn_cultist": {
			ID: "dragonborn_cultist", Name: "Dragonborn Cultist",
			CR: 4, HP: 71, AC: 16, Attack: 17, AttackBonus: 6, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Draconic Gift", Phase: "opening", ProcChance: 0.50, Effect: "aoe"},
			XPValue:   1100,
			Notes:     "Breath weapon 2d6 (DC 13); Fanatical Devotion (immune to fear).",
		},
		"young_red_dragon": {
			ID: "young_red_dragon", Name: "Young Red Dragon",
			CR: 10, HP: 178, AC: 18, Attack: 36, AttackBonus: 10, Speed: 16,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Fire Breath", Phase: "any", ProcChance: 0.30, Effect: "aoe_fire"},
			XPValue:   5900,
			Notes:     "Elite. Fire Breath 16d6 DEX DC 21; Multiattack; Frightful Presence WIS DC 16.",
		},
		"boss_infernax": {
			ID: "boss_infernax", Name: "Infernax the Undying",
			CR: 24, HP: 546, AC: 22, Attack: 65, AttackBonus: 14, Speed: 18,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Frightful Presence", Phase: "opening", ProcChance: 0.80, Effect: "stun"},
			XPValue:   62000,
			Notes:     "Dragon's Lair boss. Ancient Red Dragon. Legendary Actions; Lair Actions; phase 2 below 50% HP — fire ignores resistance.",
		},
		// ── Abyss Portal ─────────────────────────────────────────────────
		"quasit": {
			ID: "quasit", Name: "Quasit",
			CR: 1, HP: 7, AC: 13, Attack: 6, AttackBonus: 4, Speed: 14,
			BlockRate: 0.0,
			Ability:   &MonsterAbility{Name: "Invisibility", Phase: "any", ProcChance: 0.40, Effect: "evade"},
			XPValue:   200,
			Notes:     "Scare WIS DC 10 Frightened 1 min; poison claws.",
		},
		"vrock": {
			ID: "vrock", Name: "Vrock",
			CR: 6, HP: 104, AC: 15, Attack: 25, AttackBonus: 6, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Stunning Screech", Phase: "opening", ProcChance: 0.50, Effect: "stun"},
			XPValue:   2300,
			Notes:     "Multiattack; Spores CON DC 14 (5d10 poison/24h); Screech CON DC 14 Stunned.",
		},
		"hezrou": {
			ID: "hezrou", Name: "Hezrou",
			CR: 8, HP: 136, AC: 16, Attack: 30, AttackBonus: 7, Speed: 13,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Stench", Phase: "any", ProcChance: 0.50, Effect: "debuff"},
			XPValue:   3900,
			Notes:     "Stench CON DC 14 Poisoned (disadvantage); multiattack.",
		},
		"nalfeshnee": {
			ID: "nalfeshnee", Name: "Nalfeshnee",
			CR: 13, HP: 184, AC: 18, Attack: 42, AttackBonus: 10, Speed: 12,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Horror Nimbus", Phase: "opening", ProcChance: 0.55, Effect: "stun"},
			XPValue:   10000,
			Notes:     "Multiattack 3 hits; Horror Nimbus AoE WIS DC 15 Frightened; Magic Resistance.",
		},
		"marilith": {
			ID: "marilith", Name: "Marilith",
			CR: 16, HP: 189, AC: 18, Attack: 50, AttackBonus: 11, Speed: 12,
			BlockRate: 0.25,
			Ability:   &MonsterAbility{Name: "Reactive Parry", Phase: "any", ProcChance: 0.50, Effect: "block"},
			XPValue:   15000,
			Notes:     "Elite. 7 attacks/turn; Parry reaction; Reactive (extra reaction); Magic Resistance.",
		},
		"boss_belaxath": {
			ID: "boss_belaxath", Name: "Belaxath the Undivided",
			CR: 19, HP: 262, AC: 19, Attack: 58, AttackBonus: 12, Speed: 14,
			BlockRate: 0.20,
			Ability:   &MonsterAbility{Name: "Lightning Discharge", Phase: "decisive", ProcChance: 0.40, Effect: "aoe"},
			XPValue:   22000,
			Notes:     "Abyss Portal boss. Balor. Fire Aura; Death Throes on death; Demonic Resilience; Legendary Resistance 3/combat; phase 2 below 40% HP — Huge size, advantage on attacks.",
		},
	}
	for id, m := range tier45 {
		dndBestiary[id] = m
	}
	return true
}()

// dndBestiaryByCR returns templates whose CR is at or below the given cap.
// Useful for procedurally selecting a monster appropriate to player level.
func dndBestiaryByCR(maxCR float32) []DnDMonsterTemplate {
	var out []DnDMonsterTemplate
	for _, m := range dndBestiary {
		if m.CR <= maxCR {
			out = append(out, m)
		}
	}
	return out
}

// toCombatStats converts a bestiary entry into the engine's CombatStats +
// CombatModifiers shape. Future encounter content can use this to spawn
// named monsters in the existing combat pipeline.
func (m DnDMonsterTemplate) toCombatStats() (CombatStats, CombatModifiers) {
	stats := CombatStats{
		MaxHP:       m.HP,
		Attack:      m.Attack,
		Defense:     0,
		Speed:       m.Speed,
		BlockRate:   m.BlockRate,
		AC:          m.AC,
		AttackBonus: m.AttackBonus,
	}
	mods := CombatModifiers{DamageReduct: 1.0}
	return stats, mods
}
