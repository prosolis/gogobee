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
