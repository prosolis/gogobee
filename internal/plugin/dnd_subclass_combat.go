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
	case SubclassAbjuration:
		// L5 Arcane Ward: HP buffer = 2× Mage level. 5e fluffs this as
		// recharged when casting an abjuration spell of L1+; we approximate
		// "always-on" by refilling at the start of every combat — the player
		// can refresh by simply casting any abjuration spell pre-combat
		// (mage_armor, shield_of_faith, etc.), which our combat already
		// handles via pending_cast. L7 Improved Abjuration: + proficiency
		// bonus to ward HP (5e: + prof to Counterspell/Dispel checks; we
		// fold the static piece into the durability buff since Counterspell
		// is reaction-deferred to Phase 11).
		if c.Level >= 5 {
			ward := 2 * c.Level
			if c.Level >= 7 {
				ward += proficiencyBonus(c.Level)
			}
			mods.ArcaneWardHP = ward
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
// Bumps Exhaustion if Berserker rage fired and applies Grim Harvest healing
// if a Necromancy Mage's queued spell delivered the killing blow. Called
// from combat_bridge after SimulateCombat returns.
//
// raged is captured before combat (mods.BerserkerRage at combat-start time);
// result + mods are read here to detect the spell-kill and look up the
// stashed slot level / damage type.
func persistDnDPostCombatSubclass(c *DnDCharacter, raged bool, result CombatResult, mods CombatModifiers) error {
	if c == nil {
		return nil
	}
	dirty := false
	if raged {
		c.Exhaustion++
		dirty = true
	}
	if heal := grimHarvestHeal(c, result, mods); heal > 0 {
		c.HPCurrent = min(c.HPMax, c.HPCurrent+heal)
		dirty = true
	}
	if !dirty {
		return nil
	}
	return SaveDnDCharacter(c)
}

// grimHarvestHeal returns the HP to restore when a Necromancy Mage's queued
// spell delivered the killing blow. 5e: heal 2× spell level on kill (3× if
// the spell is necrotic). Slot=0 = nothing was stashed → no heal. Returns 0
// if the player lost or the killing blow wasn't the pre-combat spell.
func grimHarvestHeal(c *DnDCharacter, result CombatResult, mods CombatModifiers) int {
	if c == nil || c.Class != ClassMage || c.Subclass != SubclassNecromancy || c.Level < 5 {
		return 0
	}
	if !result.PlayerWon || mods.GrimHarvestSlot <= 0 {
		return 0
	}
	// The pre-combat spell killed the enemy iff the spell_cast event itself
	// dropped EnemyHP to 0. If a later round-event finished the kill, no heal.
	for _, ev := range result.Events {
		if ev.Action == "spell_cast" {
			if ev.EnemyHP > 0 {
				return 0
			}
			break
		}
	}
	heal := 2 * mods.GrimHarvestSlot
	if mods.GrimHarvestNecrotic {
		heal = 3 * mods.GrimHarvestSlot
	}
	return heal
}
