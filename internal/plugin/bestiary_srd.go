package plugin

// SRD-shaped stat blocks for the turn-based elite/boss engine.
//
// The auto-resolve path (SimulateCombat) keeps the simplified one-attack
// DnDMonsterTemplate shape — its Attack value is a gameplay-tuned average that
// bakes a creature's whole action into a single number, and the 2026-05-10
// rebalance is calibrated against it. Do NOT feed these profiles into
// auto-resolve.
//
// The turn-based path can afford canonical SRD multiattack: the player issues a
// command per round, so a creature making three attack rolls a turn is a
// tactical problem the player can answer (heal, control, flee) rather than an
// un-survivable spike. Each SRDAttack carries its own to-hit and average
// damage, derived from the creature's SRD attack profile — not the tuned
// auto-resolve Attack stat.
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
// other creature falls through to the single-attack template path. Damage
// values are SRD attack-profile averages, rounded.
var srdProfiles = map[string]SRDProfile{
	// ── Bosses ───────────────────────────────────────────────────────────
	"boss_grol_unbroken": {Attacks: []SRDAttack{
		{Name: "Morningstar", AttackBonus: 5, Damage: 11},
		{Name: "Javelin", AttackBonus: 5, Damage: 9},
	}},
	"boss_valdris_unburied": {Attacks: []SRDAttack{
		{Name: "Paralyzing Touch", AttackBonus: 7, Damage: 12},
		{Name: "Necrotic Bolt", AttackBonus: 7, Damage: 10},
	}},
	"boss_hollow_king": {Attacks: []SRDAttack{
		{Name: "Root Slam", AttackBonus: 7, Damage: 13},
		{Name: "Root Slam", AttackBonus: 7, Damage: 13},
	}},
	"boss_dreaming_aboleth": {Attacks: []SRDAttack{
		{Name: "Tentacle", AttackBonus: 9, Damage: 12},
		{Name: "Tentacle", AttackBonus: 9, Damage: 12},
		{Name: "Tentacle", AttackBonus: 9, Damage: 12},
	}},
	"boss_aldric_blackspire": {Attacks: []SRDAttack{
		{Name: "Unarmed Strike", AttackBonus: 9, Damage: 10},
		{Name: "Bite", AttackBonus: 9, Damage: 9},
	}},
	"boss_emberlord_thyrak": {Attacks: []SRDAttack{
		{Name: "Molten Maul", AttackBonus: 8, Damage: 15},
		{Name: "Molten Maul", AttackBonus: 8, Damage: 15},
	}},
	"boss_ilvaras_xunyl": {Attacks: []SRDAttack{
		{Name: "Scourge", AttackBonus: 9, Damage: 14},
		{Name: "Scourge", AttackBonus: 9, Damage: 14},
	}},
	"boss_thornmother": {Attacks: []SRDAttack{
		{Name: "Thorned Lash", AttackBonus: 8, Damage: 16},
		{Name: "Thorned Lash", AttackBonus: 8, Damage: 16},
	}},
	"boss_infernax": {Attacks: []SRDAttack{
		{Name: "Bite", AttackBonus: 11, Damage: 21},
		{Name: "Claw", AttackBonus: 11, Damage: 17},
		{Name: "Claw", AttackBonus: 11, Damage: 17},
	}},
	"boss_belaxath": {Attacks: []SRDAttack{
		{Name: "Longsword", AttackBonus: 11, Damage: 21},
		{Name: "Whip", AttackBonus: 11, Damage: 15},
	}},

	// ── Multiattack elites ───────────────────────────────────────────────
	"hobgoblin_warchief": {Attacks: []SRDAttack{
		{Name: "Longsword", AttackBonus: 5, Damage: 8},
		{Name: "Longsword", AttackBonus: 5, Damage: 8},
	}},
	"bandit_captain": {Attacks: []SRDAttack{
		{Name: "Scimitar", AttackBonus: 5, Damage: 6},
		{Name: "Scimitar", AttackBonus: 5, Damage: 6},
		{Name: "Dagger", AttackBonus: 5, Damage: 5},
	}},
	"owlbear": {Attacks: []SRDAttack{
		{Name: "Beak", AttackBonus: 7, Damage: 10},
		{Name: "Claws", AttackBonus: 7, Damage: 14},
	}},
	"merrow": {Attacks: []SRDAttack{
		{Name: "Bite", AttackBonus: 5, Damage: 8},
		{Name: "Claws", AttackBonus: 5, Damage: 9},
	}},
	"helmed_horror": {Attacks: []SRDAttack{
		{Name: "Longsword", AttackBonus: 6, Damage: 8},
		{Name: "Longsword", AttackBonus: 6, Damage: 8},
	}},
	"drow_elite_warrior": {Attacks: []SRDAttack{
		{Name: "Shortsword", AttackBonus: 7, Damage: 7},
		{Name: "Shortsword", AttackBonus: 7, Damage: 7},
		{Name: "Hand Crossbow", AttackBonus: 7, Damage: 6},
	}},
	"salamander": {Attacks: []SRDAttack{
		{Name: "Spear", AttackBonus: 7, Damage: 10},
		{Name: "Tail", AttackBonus: 7, Damage: 10},
	}},
	"guard_drake": {Attacks: []SRDAttack{
		{Name: "Bite", AttackBonus: 5, Damage: 5},
		{Name: "Claws", AttackBonus: 5, Damage: 5},
	}},
	"young_red_dragon": {Attacks: []SRDAttack{
		{Name: "Bite", AttackBonus: 10, Damage: 17},
		{Name: "Claw", AttackBonus: 10, Damage: 13},
		{Name: "Claw", AttackBonus: 10, Damage: 13},
	}},
	"quickling": {Attacks: []SRDAttack{
		{Name: "Dagger", AttackBonus: 5, Damage: 7},
		{Name: "Dagger", AttackBonus: 5, Damage: 7},
		{Name: "Dagger", AttackBonus: 5, Damage: 7},
	}},
	"vrock": {Attacks: []SRDAttack{
		{Name: "Beak", AttackBonus: 6, Damage: 10},
		{Name: "Talons", AttackBonus: 6, Damage: 14},
	}},
	"hezrou": {Attacks: []SRDAttack{
		{Name: "Bite", AttackBonus: 7, Damage: 15},
		{Name: "Claw", AttackBonus: 7, Damage: 11},
		{Name: "Claw", AttackBonus: 7, Damage: 11},
	}},
	"nalfeshnee": {Attacks: []SRDAttack{
		{Name: "Bite", AttackBonus: 10, Damage: 32},
		{Name: "Claw", AttackBonus: 10, Damage: 15},
		{Name: "Claw", AttackBonus: 10, Damage: 15},
	}},
	"marilith": {Attacks: []SRDAttack{
		{Name: "Longsword", AttackBonus: 11, Damage: 13},
		{Name: "Longsword", AttackBonus: 11, Damage: 13},
		{Name: "Longsword", AttackBonus: 11, Damage: 13},
		{Name: "Longsword", AttackBonus: 11, Damage: 13},
		{Name: "Longsword", AttackBonus: 11, Damage: 13},
		{Name: "Longsword", AttackBonus: 11, Damage: 13},
		{Name: "Tail", AttackBonus: 11, Damage: 13},
	}},
	"vampire_spawn": {Attacks: []SRDAttack{
		{Name: "Unarmed Strike", AttackBonus: 6, Damage: 8},
		{Name: "Bite", AttackBonus: 6, Damage: 6},
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
