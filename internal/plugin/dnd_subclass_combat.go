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
	case SubclassBattleMaster:
		// L7 Know Your Enemy: 5e gives factual info about the target after
		// 1 min observation. We don't model "observation time" — proxy the
		// tactical edge as a flat +1 to attack rolls from L7 onward.
		if c.Level >= 7 {
			stats.AttackBonus++
		}
	case SubclassAssassin:
		// L5 Assassinate: advantage on the opening strike + bonus damage
		// stacked on top of the Rogue's existing Sneak Attack auto-crit
		// (proxy for "crits vs. surprised targets"). Bonus scales with
		// level; L7 Impostor — flavored as deeper study of the mark — adds
		// a small surprise-damage bump.
		if c.Level >= 5 {
			mods.AssassinateAdvantage = true
			bonus := c.Level
			if c.Level >= 7 {
				bonus += 3
			}
			mods.AssassinateBonusDmg = bonus
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

	// Phase 10 SUB2a-ii — Battle Master superiority-die maneuvers. Each
	// consumes one die from the new "superiority" resource pool (4/long-rest
	// at L5; long-rest refresh in our model — 5e refreshes on short rest).
	// We don't model 5e's interactive "before/after the roll" choice; a die
	// commits at !arm time and fires on the next combat.
	//
	// Three non-reaction maneuvers covered:
	//   - Precision Attack → +d8 (≈+4 flat) on first attack roll
	//   - Tripping Attack  → enemy skips its first attack (reuses Phase 9
	//     SpellEnemySkipFirst)
	//   - Rally            → temp-HP buffer at <50% (reuses HealItem; self
	//     buff since one-shot combat has no ally-target UI)
	dndActiveAbilities["precision_attack"] = DnDAbility{
		ID:          "precision_attack",
		Name:        "Precision Attack",
		Class:       ClassFighter,
		Subclass:    SubclassBattleMaster,
		Resource:    "superiority",
		Description: "Add +d8 (≈+4) to your first attack roll this combat (consumes 1 superiority die).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			mods.FirstAttackBonus += 4
		},
	}
	dndActiveAbilities["trip_attack"] = DnDAbility{
		ID:          "trip_attack",
		Name:        "Tripping Attack",
		Class:       ClassFighter,
		Subclass:    SubclassBattleMaster,
		Resource:    "superiority",
		Description: "Trip the enemy on engagement: they skip their first attack (consumes 1 superiority die).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			mods.SpellEnemySkipFirst = true
		},
	}
	dndActiveAbilities["rally"] = DnDAbility{
		ID:          "rally",
		Name:        "Rally",
		Class:       ClassFighter,
		Subclass:    SubclassBattleMaster,
		Resource:    "superiority",
		Description: "Steel yourself for the fight: heal at low HP for d8 + CHA mod (consumes 1 superiority die).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			mods.HealItem += 4 + abilityModifier(c.CHA)
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
