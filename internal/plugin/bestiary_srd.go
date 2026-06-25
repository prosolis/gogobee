package plugin

// SRD-shaped stat blocks for the turn-based elite/boss engine.
//
// The auto-resolve path (SimulateCombat) keeps the simplified one-attack
// DnDMonsterTemplate shape — its Attack value is a gameplay-tuned average that
// bakes a creature's whole action into a single number, and the 2026-05-10
// rebalance is calibrated against it. Do NOT feed these profiles into
// auto-resolve.
//
// The turn-based path can afford canonical SRD multiattack *structure*: the
// player issues a command per round, so a creature making three attack rolls a
// turn is a tactical problem the player can answer (heal, control, flee) rather
// than an un-survivable spike.
//
// Balance basis (2026-05-14 tuning pass): raw SRD damage is calibrated for a
// 4-PC party and one-shots gogobee's single player — slice 1 shipped it
// canonical and it was un-survivable across every tier (marilith ~91/round,
// even Grol ~20/round vs a level-1 mage). So each profile keeps its SRD
// *shape* — attack count, names, per-attack to-hit, and the damage split
// between attacks — but the per-attack Damage values are scaled so the
// profile's round total lands at roughly 1.3x the creature's tuned
// auto-resolve Attack stat (dndBestiary). That 1.3x is deliberate: the
// 2026-05-10 rebalance calibrated the single-attack Attack stat as the
// gameplay floor, and turn-based fights should hit a little harder than
// auto-resolve to make the player's per-round agency (heal / control / flee)
// matter — but multiattack already raises hit reliability vs high-AC builds,
// and monster abilities now fire as riders on top, so the headroom over
// auto-resolve stays modest. Damage flows through the same calcDamage penalty
// formula as the Attack stat, so the two are directly comparable in scale.
//
// File is intentionally NOT dnd_-prefixed (see memory: no new dnd_* identifiers).

// SRDAttack is one attack roll within a creature's multiattack action: a d20
// to-hit and an average damage value for that single strike.
type SRDAttack struct {
	Name        string // "Longsword", "Bite", "Claw" — surfaces in the round log
	AttackBonus int    // d20 to-hit modifier; 0 falls back to the template AttackBonus
	Damage      int    // average damage for this single attack
}

// SRDProfile is a creature's full multiattack action — one SRDAttack per attack
// roll it makes on its turn. A creature with no profile registered makes a
// single attack using its DnDMonsterTemplate Attack / AttackBonus.
type SRDProfile struct {
	Attacks []SRDAttack
}

// srdProfiles is the turn-based multiattack registry, keyed by bestiary ID.
// Only elites and bosses that reach the turn-based engine need an entry; every
// other creature falls through to the single-attack template path. Per-attack
// Damage is tuned (not raw SRD) — see the balance-basis note above; the
// // -comment after each profile records the tuned auto-resolve Attack stat the
// total was scaled against.
var srdProfiles = map[string]SRDProfile{
	// ── Bosses ───────────────────────────────────────────────────────────
	"boss_grol_unbroken": {Attacks: []SRDAttack{ // Attack 6 → ~8
		{Name: "Morningstar", AttackBonus: 5, Damage: 5},
		{Name: "Javelin", AttackBonus: 5, Damage: 3},
	}},
	"boss_valdris_unburied": {Attacks: []SRDAttack{ // Attack 8 → ~10
		{Name: "Paralyzing Touch", AttackBonus: 7, Damage: 6},
		{Name: "Necrotic Bolt", AttackBonus: 7, Damage: 4},
	}},
	"boss_hollow_king": {Attacks: []SRDAttack{ // Attack 10 → ~13
		{Name: "Root Slam", AttackBonus: 7, Damage: 7},
		{Name: "Root Slam", AttackBonus: 7, Damage: 6},
	}},
	"boss_dreaming_aboleth": {Attacks: []SRDAttack{ // Attack 16 → ~21
		{Name: "Tentacle", AttackBonus: 9, Damage: 7},
		{Name: "Tentacle", AttackBonus: 9, Damage: 7},
		{Name: "Tentacle", AttackBonus: 9, Damage: 7},
	}},
	"boss_aldric_blackspire": {Attacks: []SRDAttack{ // Attack 21 → ~27
		{Name: "Unarmed Strike", AttackBonus: 9, Damage: 14},
		{Name: "Bite", AttackBonus: 9, Damage: 13},
	}},
	"boss_emberlord_thyrak": {Attacks: []SRDAttack{ // Attack 14 → ~18
		{Name: "Molten Maul", AttackBonus: 8, Damage: 9},
		{Name: "Molten Maul", AttackBonus: 8, Damage: 9},
	}},
	"boss_ilvaras_xunyl": {Attacks: []SRDAttack{ // Attack 19 → ~25
		{Name: "Scourge", AttackBonus: 9, Damage: 13},
		{Name: "Scourge", AttackBonus: 9, Damage: 12},
	}},
	"boss_thornmother": {Attacks: []SRDAttack{ // D8-f #2: 2→3 lashes, ~23→~36 (faceroll → band)
		{Name: "Thorned Lash", AttackBonus: 9, Damage: 13},
		{Name: "Thorned Lash", AttackBonus: 9, Damage: 12},
		{Name: "Thorned Lash", AttackBonus: 9, Damage: 11},
	}},
	"boss_infernax": {Attacks: []SRDAttack{ // D11 T5 lift: 49 → ~42 (nerf; was an impossible wall at L15-16)
		{Name: "Bite", AttackBonus: 11, Damage: 16},
		{Name: "Claw", AttackBonus: 11, Damage: 13},
		{Name: "Claw", AttackBonus: 11, Damage: 13},
	}},
	"boss_belaxath": {Attacks: []SRDAttack{ // D11 T5 lift: 40 → ~41 (buff; was a leader faceroll)
		{Name: "Longsword", AttackBonus: 11, Damage: 24},
		{Name: "Whip", AttackBonus: 11, Damage: 17},
	}},

	// ── Multiattack elites ───────────────────────────────────────────────
	"hobgoblin_warchief": {Attacks: []SRDAttack{ // Attack 4 → ~5
		{Name: "Longsword", AttackBonus: 5, Damage: 3},
		{Name: "Longsword", AttackBonus: 5, Damage: 2},
	}},
	"bandit_captain": {Attacks: []SRDAttack{ // Attack 4 → ~6
		{Name: "Scimitar", AttackBonus: 5, Damage: 2},
		{Name: "Scimitar", AttackBonus: 5, Damage: 2},
		{Name: "Dagger", AttackBonus: 5, Damage: 2},
	}},
	"owlbear": {Attacks: []SRDAttack{ // Attack 6 → ~8
		{Name: "Beak", AttackBonus: 7, Damage: 3},
		{Name: "Claws", AttackBonus: 7, Damage: 5},
	}},
	"merrow": {Attacks: []SRDAttack{ // Attack 4 → ~5
		{Name: "Bite", AttackBonus: 5, Damage: 3},
		{Name: "Claws", AttackBonus: 5, Damage: 2},
	}},
	"helmed_horror": {Attacks: []SRDAttack{ // Attack 7 → ~9
		{Name: "Longsword", AttackBonus: 6, Damage: 5},
		{Name: "Longsword", AttackBonus: 6, Damage: 4},
	}},
	"drow_elite_warrior": {Attacks: []SRDAttack{ // Attack 8 → ~10
		{Name: "Shortsword", AttackBonus: 7, Damage: 4},
		{Name: "Shortsword", AttackBonus: 7, Damage: 3},
		{Name: "Hand Crossbow", AttackBonus: 7, Damage: 3},
	}},
	"salamander": {Attacks: []SRDAttack{ // Attack 8 → ~10
		{Name: "Spear", AttackBonus: 7, Damage: 5},
		{Name: "Tail", AttackBonus: 7, Damage: 5},
	}},
	"guard_drake": {Attacks: []SRDAttack{ // Attack 4 → ~5
		{Name: "Bite", AttackBonus: 5, Damage: 3},
		{Name: "Claws", AttackBonus: 5, Damage: 2},
	}},
	"young_red_dragon": {Attacks: []SRDAttack{ // Attack 16 → ~20
		{Name: "Bite", AttackBonus: 10, Damage: 8},
		{Name: "Claw", AttackBonus: 10, Damage: 6},
		{Name: "Claw", AttackBonus: 10, Damage: 6},
	}},
	"quickling": {Attacks: []SRDAttack{ // Attack 2 → ~3
		{Name: "Dagger", AttackBonus: 5, Damage: 1},
		{Name: "Dagger", AttackBonus: 5, Damage: 1},
		{Name: "Dagger", AttackBonus: 5, Damage: 1},
	}},
	"vrock": {Attacks: []SRDAttack{ // Attack 10 → ~13
		{Name: "Beak", AttackBonus: 6, Damage: 5},
		{Name: "Talons", AttackBonus: 6, Damage: 8},
	}},
	"hezrou": {Attacks: []SRDAttack{ // Attack 13 → ~17
		{Name: "Bite", AttackBonus: 7, Damage: 7},
		{Name: "Claw", AttackBonus: 7, Damage: 5},
		{Name: "Claw", AttackBonus: 7, Damage: 5},
	}},
	"nalfeshnee": {Attacks: []SRDAttack{ // Attack 21 → ~28
		{Name: "Bite", AttackBonus: 10, Damage: 14},
		{Name: "Claw", AttackBonus: 10, Damage: 7},
		{Name: "Claw", AttackBonus: 10, Damage: 7},
	}},
	"marilith": {Attacks: []SRDAttack{ // Attack 26 → ~34
		{Name: "Longsword", AttackBonus: 11, Damage: 5},
		{Name: "Longsword", AttackBonus: 11, Damage: 5},
		{Name: "Longsword", AttackBonus: 11, Damage: 5},
		{Name: "Longsword", AttackBonus: 11, Damage: 5},
		{Name: "Longsword", AttackBonus: 11, Damage: 5},
		{Name: "Longsword", AttackBonus: 11, Damage: 5},
		{Name: "Tail", AttackBonus: 11, Damage: 4},
	}},
	"vampire_spawn": {Attacks: []SRDAttack{ // Attack 8 → ~10
		{Name: "Unarmed Strike", AttackBonus: 6, Damage: 6},
		{Name: "Bite", AttackBonus: 6, Damage: 4},
	}},

	// ── Feywild elites (D8-f #2) ─────────────────────────────────────────
	// Feywild martials facerolled at 97–100% even after an HP/AC raise: the
	// roster was single-attack and low-damage, so tankier monsters just took
	// longer to kill. Multiattack is the lever that actually pulls leaders
	// into band (mirrors underdark's drow_elite). Casters trail (by design).
	"fomorian": {Attacks: []SRDAttack{ // Attack 13 → ~21 (2 big fists)
		{Name: "Greatclub", AttackBonus: 9, Damage: 11},
		{Name: "Greatclub", AttackBonus: 9, Damage: 10},
	}},
	"night_hag": {Attacks: []SRDAttack{ // Attack 8 → ~12 (claw flurry)
		{Name: "Claws", AttackBonus: 7, Damage: 6},
		{Name: "Claws", AttackBonus: 7, Damage: 6},
	}},
	"green_hag": {Attacks: []SRDAttack{ // Attack 6 → ~10 (claw flurry)
		{Name: "Claws", AttackBonus: 6, Damage: 5},
		{Name: "Claws", AttackBonus: 6, Damage: 5},
	}},
}

// enemyAttackProfile returns the attack rolls a creature makes on its turn.
// Registered elites/bosses use their SRD multiattack profile; everyone else
// falls back to a single attack derived from the template stats. A zero
// per-attack AttackBonus falls back to the (tier-scaled) template bonus.
func enemyAttackProfile(enemyID string, stats CombatStats) []SRDAttack {
	prof, ok := srdProfiles[enemyID]
	if !ok || len(prof.Attacks) == 0 {
		return []SRDAttack{{AttackBonus: stats.AttackBonus, Damage: stats.Attack}}
	}
	out := make([]SRDAttack, len(prof.Attacks))
	for i, a := range prof.Attacks {
		if a.AttackBonus == 0 {
			a.AttackBonus = stats.AttackBonus
		}
		out[i] = a
	}
	return out
}
