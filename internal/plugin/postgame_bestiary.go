package plugin

// Tier 6 "Mythic" post-game bestiary (Phase P2) — 15 bespoke elites + 5
// signature bosses. Layer 1 only: everything here is expressible with the
// current engine (single MonsterAbility, HP/AC/Attack/BlockRate/FireAttacker).
// The bespoke per-boss mechanics in gogobee_postgame_zones_plan.md are Layer 2
// (later hooks) — the zones are shippable and fun on these stat blocks alone.
//
// These are opening bids, calibrated off post-D11 Belaxath (CR19, HP300,
// AC20, Atk31). Every T6 boss opens noticeably above that; the P7 sim sweep
// (n=750, L18/L20, control arm) decides the final numbers. IsElite is flagged
// on the zone roster (ZoneEnemy), not here.
//
// Attack = average-damage convention; AttackBonus = d20 to-hit modifier.

var _ = func() bool {
	tier6 := map[string]DnDMonsterTemplate{
		// ── Zone 1 — The Ossuary Ascendant ──────────────────────────────
		"grave_cardinal": {
			ID: "grave_cardinal", Name: "Grave Cardinal",
			CR: 17, HP: 230, AC: 17, Attack: 28, AttackBonus: 10, Speed: 12,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Last Rites", Phase: "any", ProcChance: 0.45, Effect: "debuff"},
			XPValue:   18000,
			Notes:     "Ossuary elite. Fused clergy skeletons; censer of grave-smoke pre-blesses your corpse (accumulating AC drain).",
		},
		"reliquary_knight": {
			ID: "reliquary_knight", Name: "Reliquary Knight",
			CR: 18, HP: 230, AC: 20, Attack: 30, AttackBonus: 10, Speed: 10,
			BlockRate: 0.35,
			Ability:   &MonsterAbility{Name: "Coffin Guard", Phase: "any", ProcChance: 0.50, Effect: "block"},
			XPValue:   20000,
			Notes:     "Ossuary elite. Empty donor-bone armor; a coffin lid for a shield. The tanky wall of the tier.",
		},
		"the_chorister": {
			ID: "the_chorister", Name: "The Chorister",
			CR: 17, HP: 190, AC: 16, Attack: 27, AttackBonus: 10, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Dirge", Phase: "opening", ProcChance: 0.45, Effect: "stun"},
			XPValue:   18000,
			Notes:     "Ossuary elite. Nine banshee throats in one column of veils; sings your own name in rounds.",
		},

		// ── Zone 2 — The First Hoard ─────────────────────────────────────
		"coinborn_simulacrum": {
			ID: "coinborn_simulacrum", Name: "Coinborn Simulacrum",
			CR: 18, HP: 210, AC: 18, Attack: 22, AttackBonus: 10, Speed: 12,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Mirror Mint", Phase: "opening", ProcChance: 0.50, Effect: "debuff"},
			XPValue:   20000,
			Notes:     "First Hoard elite. The hoard's antibody: a person-shaped cascade of coins that mints a copy of your weapon. (Layer 2: Attack derived from player weapon bonus.)",
		},
		"wyrm_sworn_executor": {
			ID: "wyrm_sworn_executor", Name: "Wyrm-Sworn Executor",
			CR: 19, HP: 240, AC: 19, Attack: 24, AttackBonus: 11, Speed: 12,
			BlockRate:    0.15,
			Ability:      &MonsterAbility{Name: "Scale Debt", Phase: "decisive", ProcChance: 0.30, Effect: "aoe_fire"},
			XPValue:      22000,
			Notes:        "First Hoard elite. Dragonborn cultist end-state; half his body is ember and it is eating the rest.",
			FireAttacker: true,
		},
		"choir_drake_matriarch": {
			ID: "choir_drake_matriarch", Name: "Choir Drake Matriarch",
			CR: 17, HP: 190, AC: 17, Attack: 21, AttackBonus: 10, Speed: 10,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Resonance", Phase: "any", ProcChance: 0.30, Effect: "stun"},
			XPValue:   18000,
			Notes:     "First Hoard elite. A wingless drake that never left the cradle; resonance notes that crack stone.",
		},

		// ── Zone 3 — The Unplace ─────────────────────────────────────────
		"the_unnumbered": {
			ID: "the_unnumbered", Name: "The Unnumbered",
			CR: 18, HP: 250, AC: 16, Attack: 18, AttackBonus: 10, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Too Many Arms", Phase: "any", ProcChance: 0.28, Effect: "aoe"},
			XPValue:   20000,
			Notes:     "Unplace elite. Several demons in one silhouette; the count changes with the angle. (Layer 2: extra attack on rounds the player missed.)",
		},
		"angleworn_horror": {
			ID: "angleworn_horror", Name: "Angleworn Horror",
			CR: 17, HP: 180, AC: 18, Attack: 24, AttackBonus: 11, Speed: 14,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Oblique", Phase: "any", ProcChance: 0.50, Effect: "evade"},
			XPValue:   18000,
			Notes:     "Unplace elite. Hunts through corners; has been attacking you since three rooms ago. (Layer 2: pre-applied damage tick on entry.)",
		},
		"echo_of_belaxath": {
			ID: "echo_of_belaxath", Name: "Echo of Belaxath",
			CR: 19, HP: 220, AC: 18, Attack: 23, AttackBonus: 11, Speed: 14,
			BlockRate:    0.15,
			Ability:      &MonsterAbility{Name: "Rerun", Phase: "decisive", ProcChance: 0.25, Effect: "aoe"},
			XPValue:      22000,
			Notes:        "Unplace elite. A Balor outline filled with static; a memory the Unplace keeps replaying. Drops nothing — pure toll.",
			FireAttacker: true,
		},

		// ── Zone 4 — The Drowned Star ────────────────────────────────────
		"choir_of_static": {
			ID: "choir_of_static", Name: "Choir of Static",
			CR: 17, HP: 170, AC: 19, Attack: 28, AttackBonus: 10, Speed: 14,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Mimic Shoal", Phase: "any", ProcChance: 0.50, Effect: "evade"},
			XPValue:   18000,
			Notes:     "Drowned Star elite. Thousands of hair-thin fish in the shape of the last thing that frightened them: you.",
		},
		"lantern_warden": {
			ID: "lantern_warden", Name: "Lantern Warden",
			CR: 18, HP: 230, AC: 19, Attack: 31, AttackBonus: 10, Speed: 12,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "False Light", Phase: "opening", ProcChance: 0.45, Effect: "stun"},
			XPValue:   20000,
			Notes:     "Drowned Star elite. An angler-knight; its lure is a stolen fragment of Seraphel's halo.",
		},
		"star_gorged_leviathanling": {
			ID: "star_gorged_leviathanling", Name: "Star-Gorged Leviathanling",
			CR: 19, HP: 290, AC: 16, Attack: 33, AttackBonus: 11, Speed: 12,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Radiant Gout", Phase: "decisive", ProcChance: 0.45, Effect: "aoe"},
			XPValue:   22000,
			Notes:     "Drowned Star elite. Ate a piece of the star as a hatchling and never stopped glowing; its silhouette shows through its own flesh.",
		},

		// ── Zone 5 — The Last Meridian ───────────────────────────────────
		"the_hour_thief": {
			ID: "the_hour_thief", Name: "The Hour Thief",
			CR: 17, HP: 180, AC: 18, Attack: 22, AttackBonus: 10, Speed: 18,
			BlockRate: 0.10,
			Ability:   &MonsterAbility{Name: "Pickpocket the Present", Phase: "any", ProcChance: 0.45, Effect: "stun"},
			XPValue:   18000,
			Notes:     "Meridian elite. A stooped construct with a satchel full of ticking. (Layer 2: each proc permanently +2 its Speed this fight.)",
		},
		"pendulum_colossus": {
			ID: "pendulum_colossus", Name: "Pendulum Colossus",
			CR: 19, HP: 280, AC: 17, Attack: 30, AttackBonus: 11, Speed: 10,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Full Swing", Phase: "decisive", ProcChance: 0.50, Effect: "aoe"},
			XPValue:   22000,
			Notes:     "Meridian elite. The cathedral's original pendulum, given legs; massive telegraphed hits. (Layer 2: only procs on even rounds — learnable.)",
		},
		"wardens_in_waiting": {
			ID: "wardens_in_waiting", Name: "Wardens-in-Waiting",
			CR: 18, HP: 260, AC: 19, Attack: 24, AttackBonus: 10, Speed: 10,
			BlockRate: 0.30,
			Ability:   &MonsterAbility{Name: "Changing of the Guard", Phase: "any", ProcChance: 0.50, Effect: "block"},
			XPValue:   20000,
			Notes:     "Meridian elite. Twin funerary constructs sharing one soul on a strict shift. (Layer 2: while one acts the other blocks — alternating BlockRate.)",
		},

		// ── Signature bosses ─────────────────────────────────────────────
		"boss_valdris_ascendant": {
			ID: "boss_valdris_ascendant", Name: "Valdris, At Last",
			CR: 26, HP: 550, AC: 21, Attack: 52, AttackBonus: 12, Speed: 14,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Apotheosis Nova", Phase: "decisive", ProcChance: 0.40, Effect: "aoe"},
			XPValue:   90000,
			Notes:     "Ossuary Ascendant boss. A true lich, ascended one vertebra at a time. Phase 2 below 50% HP. (Layer 2: Phylactery Verses — three secret nodes each strip a Legendary Resistance / rebirth proc.)",
		},
		"boss_aurvandryx": {
			ID: "boss_aurvandryx", Name: "Aurvandryx, the Ember Before Fire",
			CR: 28, HP: 470, AC: 22, Attack: 45, AttackBonus: 12, Speed: 16,
			BlockRate:    0.20,
			Ability:      &MonsterAbility{Name: "First Flame", Phase: "decisive", ProcChance: 0.45, Effect: "aoe_fire"},
			XPValue:      120000,
			Notes:        "First Hoard boss. The progenitor wyrm whose shed scales became every dragon. Fire that predates fire resistance. Phase 2 below 40% HP. (Layer 2: Greed Tax — Attack scales with loot picked up this run.)",
			FireAttacker: true,
		},
		"boss_seamstress": {
			ID: "boss_seamstress", Name: "The Seamstress",
			CR: 27, HP: 385, AC: 21, Attack: 39, AttackBonus: 12, Speed: 14,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Needle Rain", Phase: "decisive", ProcChance: 0.40, Effect: "aoe"},
			XPValue:   100000,
			Notes:     "Unplace boss. A corrupted celestial sewing herself into the tear; half of her is on the other side. Phase 2 below 35% HP. (Layer 2: Inversion Stitch — healing and damage swap direction on her in telegraphed pulses.)",
		},
		"boss_seraphel": {
			ID: "boss_seraphel", Name: "Seraphel, the Light That Sank",
			CR: 27, HP: 560, AC: 21, Attack: 54, AttackBonus: 12, Speed: 14,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Sanctified Undertow", Phase: "decisive", ProcChance: 0.40, Effect: "aoe"},
			XPValue:   100000,
			Notes:     "Drowned Star boss. An angel who rode a dying star into the trench and kept it alive ten thousand years. Phase 2 below 50% HP. (Layer 2: Two Hearts — a second ~150 HP Star-Heart pool; mercy path is a hidden loot bias.)",
		},
		"boss_custodian": {
			ID: "boss_custodian", Name: "The Custodian of the Last Hour",
			CR: 27, HP: 500, AC: 21, Attack: 48, AttackBonus: 12, Speed: 14,
			BlockRate: 0.15,
			Ability:   &MonsterAbility{Name: "Chime", Phase: "decisive", ProcChance: 0.45, Effect: "aoe"},
			XPValue:   100000,
			Notes:     "Last Meridian boss. A titanic clock-golem decommissioning the hours; it apologizes before each swing. Phase 2 below 45% HP. (Layer 2: Amendment — rewinds to its round-3 HP snapshot once; soft midnight timer past round 20.)",
		},
	}
	for id, m := range tier6 {
		dndBestiary[id] = m
	}
	return true
}()
