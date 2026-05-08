package plugin

// Phase 10 SUB2a — subclass combat hooks.
//
// applySubclassPassives layers subclass-driven flags onto CombatModifiers
// the same way applyClassPassives does. Called from combat_bridge after
// applyClassPassives so subclass effects can stack on top of (or override)
// class baseline.
//
// Subclass active abilities (currently just Berserker's rage) are
// registered via init() into dndActiveAbilities below, gated by
// DnDAbility.Subclass.

func applySubclassPassives(stats *CombatStats, mods *CombatModifiers, c *DnDCharacter) {
	if c == nil || c.Subclass == "" {
		return
	}
	switch c.Subclass {
	case SubclassChampion:
		// L5 Improved Critical: crit on nat 19+. SUB3 Superior Critical
		// (L15) will lower this to 18.
		if c.Level >= 5 {
			mods.CritThreshold = 19
		}
	}
}

func init() {
	// Berserker rage — active ability that piggybacks on the Fighter
	// stamina pool. 5e specs 3 uses/long-rest as a separate pool, but
	// stamina is already sized at 3 for Fighter and the design's
	// "approachability" constraint preferred reusing the existing pool
	// over introducing per-subclass resource types in SUB2a.
	//
	// While armed and fired:
	//   - +2 flat damage per hit (RageMeleeDmg)
	//   - incoming weapon damage halved (PhysicalResistRage)
	//   - +50% damage multiplier for Frenzy approximation
	//     (one-shot combat can't model "1 bonus attack per turn" literally)
	//
	// Post-combat exhaustion increment lives in
	// persistDnDPostCombatSubclass — Frenzy specs +1 exhaustion after rage
	// ends, and in our model rage spans the whole one-shot combat so we
	// always tick exhaustion when rage fired.
	dndActiveAbilities["rage"] = DnDAbility{
		ID:          "rage",
		Name:        "Rage",
		Class:       ClassFighter,
		Subclass:    SubclassBerserker,
		Resource:    "stamina",
		Description: "Enter rage for the next combat: +2 damage per hit, halve incoming weapon damage, Frenzy bonus attack approximation. +1 exhaustion after.",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			mods.BerserkerRage = true
			mods.RageMeleeDmg = 2
			mods.PhysicalResistRage = true
			mods.FrenzyDmgBonus = 0.5
		},
	}
}

// persistDnDPostCombatSubclass handles end-of-combat subclass bookkeeping.
// Currently: increment Exhaustion if Berserker rage fired. Called from
// combat_bridge after the combat result is computed.
//
// raged is computed as (mods.BerserkerRage at combat-start time) — pass it
// through from the caller where the mods value is still in scope.
func persistDnDPostCombatSubclass(c *DnDCharacter, raged bool) error {
	if c == nil || !raged {
		return nil
	}
	c.Exhaustion++
	return SaveDnDCharacter(c)
}
