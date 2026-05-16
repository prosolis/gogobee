package plugin

// Shared combat resolution primitives.
//
// These are the building blocks both the auto-resolve engine (SimulateCombat)
// and the upcoming turn-based engine call into, so a single attack resolves
// identically regardless of which loop drives it. Everything here is a
// behavior-preserving extraction from combat_engine.go — the combat
// characterization golden file pins that no event stream shifted.

// effectiveAttackBonus returns the d20 attack bonus for a combatant, applying
// the appendix §8 non-proficient-weapon penalty (-4) when the combatant wields
// a D&D weapon without proficiency. Monsters and weaponless combatants have no
// Weapon set, so they fall through to the raw bonus.
func effectiveAttackBonus(s CombatStats) int {
	ab := s.AttackBonus
	if s.Weapon != nil && !s.WeaponProficient {
		ab -= 4
	}
	return ab
}

// attackCritFloor returns the lowest d20 face that counts as a critical hit.
// Default 20; Champion Improved Critical (and later tiers) lower it via
// CritThreshold. A zero or out-of-range CritThreshold means "use default".
func attackCritFloor(m CombatModifiers) int {
	if m.CritThreshold > 0 && m.CritThreshold < 20 {
		return m.CritThreshold
	}
	return 20
}

// attackConnects decides whether a d20 attack lands. roll is the raw d20 face
// (post-reroll), total is roll+bonus, ac is the defender's armor class, and
// critFloor is the crit threshold from attackCritFloor. A raw 1 always misses
// (fumble); a roll at/above critFloor always hits; otherwise total must meet AC.
func attackConnects(roll, total, ac, critFloor int) bool {
	if roll == 1 {
		return false
	}
	if roll >= critFloor {
		return true
	}
	return total >= ac
}

// applyPlayerHitDamageMods runs the post-hit player damage stack on a connected
// attack: crit doubling (natural or AutoCritFirst consumable), Orc Rage, the
// Berserker rage flat+multiplicative bumps, Cleric Divine Strike, and the
// Assassinate first-hit bonus. It consumes the relevant one-shot state on st
// (autoCrit, pendingRageAttack, assassinateBonusUsed) and returns the final
// damage plus the event action/desc labels. dmg is clamped to >=1.
func applyPlayerHitDamageMods(st *combatState, player *Combatant, dmg int, isCritRoll, blocked bool) (int, string, string) {
	isCrit := isCritRoll
	autoCritFired := false
	if !isCritRoll && st.autoCrit {
		isCrit = true
		autoCritFired = true
		st.autoCrit = false
	} else if st.autoCrit && isCritRoll {
		// Natural crit consumes the auto-crit charge but doesn't get the
		// "passive fired" flavor — the dice already explain it.
		st.autoCrit = false
	}
	if isCrit {
		// Crit: double damage. (5e rolls extra dice; we double total to
		// match the engine's pre-Phase-8 crit semantics.)
		dmg *= 2
	}
	// Orc Rage: +50% damage on this attack, then consume.
	if st.pendingRageAttack {
		dmg = int(float64(dmg) * 1.5)
		st.pendingRageAttack = false
	}
	// Phase 10 SUB2a — Berserker rage: +flat per hit, plus Frenzy
	// multiplicative bump that approximates the bonus-attack-per-turn we
	// can't model in one-shot combat.
	if player.Mods.BerserkerRage {
		if player.Mods.RageMeleeDmg > 0 {
			dmg += player.Mods.RageMeleeDmg
		}
		if player.Mods.FrenzyDmgBonus > 0 {
			dmg = int(float64(dmg) * (1 + player.Mods.FrenzyDmgBonus))
		}
	}
	// Phase 10 SUB3c — Divine Strike. Flat per-hit bonus on weapon hits only
	// (5e specs "weapon hit"; no Weapon means we're on the legacy/non-weapon
	// damage path and Divine Strike doesn't apply). Lands every hit because
	// our 1v1 model has no concept of "once per turn" turn boundaries.
	if player.Mods.DivineStrikePerHit > 0 && player.Stats.Weapon != nil {
		dmg += player.Mods.DivineStrikePerHit
	}
	// Class-identity audit (2026-05-16) — Rogue Sneak Attack. Add Nd6 on
	// every player hit. Same "once per turn = every hit" convention as
	// Divine Strike. Rogues swing once per round so this naturally lands
	// once/round at the cadence 5e expects. The bonus rides through the
	// crit doubling above (already applied to dmg) so a sneak-attack crit
	// reads correctly: doubled weapon dmg, then +Nd6 unaffected by the
	// crit. (5e doubles the sneak attack dice on crit too — we under-
	// shoot by ~Nd6 on the rare crit, which is acceptable for now.)
	if player.Mods.SneakAttackDie > 0 {
		for i := 0; i < player.Mods.SneakAttackDie; i++ {
			dmg += 1 + st.roll(6)
		}
	}
	// Class-identity audit (2026-05-16) — Ranger Hunter's Mark. +Nd6 on
	// every player hit. The implicit "you marked this enemy on round 0"
	// model: in 1v1, the only foe is always the marked one.
	if player.Mods.HuntersMarkDie > 0 {
		for i := 0; i < player.Mods.HuntersMarkDie; i++ {
			dmg += 1 + st.roll(6)
		}
	}
	// Phase 10 SUB2a-ii — Assassin Death Strike proxy: bonus damage on the
	// first hit only. Stacks on top of the Rogue's Sneak Attack auto-crit
	// (which already doubled the base damage above) — the bonus itself is
	// applied AFTER the crit doubling so the math is "double base + flat
	// surprise damage". Consumed on first hit regardless of crit status.
	if player.Mods.AssassinateBonusDmg > 0 && !st.assassinateBonusUsed {
		dmg += player.Mods.AssassinateBonusDmg
		st.assassinateBonusUsed = true
	}
	dmg = max(1, dmg)

	action := "hit"
	desc := ""
	if isCrit {
		action = "crit"
		if autoCritFired {
			desc = "auto_crit"
		}
	} else if blocked {
		action = "block"
	}
	return dmg, action, desc
}
