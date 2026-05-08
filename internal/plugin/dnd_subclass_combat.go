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
		// L5 Improved Critical: crit on nat 19+. L15 Superior Critical
		// lowers this further to 18.
		if c.Level >= 5 {
			mods.CritThreshold = 19
		}
		// Phase 10 SUB3a-i — L10 Additional Fighting Style. 5e lets the
		// player pick a second style; in 1v1 we can't surface a UI for
		// that, so we collapse to the two most generally useful
		// approximations stacked: Defense (+1 AC) and Dueling (+10%
		// damage when one-handed — we don't track grip state, so this is
		// a flat damage bump regardless).
		if c.Level >= 10 {
			stats.AC++
			mods.DamageBonus += 0.10
		}
		// Phase 10 SUB3a-i — L15 Superior Critical: crit floor drops to 18.
		if c.Level >= 15 {
			mods.CritThreshold = 18
		}
	case SubclassBerserker:
		// L5/L7 Berserker is rage-driven (active ability — see init() below)
		// plus L7 Mindless Rage which is condition immunity, irrelevant in
		// our 1v1 model. Phase 10 SUB3a-i adds the always-on L10/L15 layer.
		//
		// L10 Intimidating Presence: 5e is an Action that frightens one
		// target on a failed WIS save. One-shot combat has no action
		// economy to spend on a non-damage Action, so we proxy as 2 rounds
		// of 15% enemy miss chance via SporeCloud (the same channel
		// fog-cloud / Cloak of Shadows uses).
		// L15 Retaliation: 5e Reaction — when struck in melee, one melee
		// attack back. We don't model reactions, so the average per-fight
		// damage uplift is approximated as a +15% damage multiplier.
		if c.Level >= 10 {
			mods.SporeCloud += 2
		}
		if c.Level >= 15 {
			mods.DamageBonus += 0.15
		}
	case SubclassBattleMaster:
		// L7 Know Your Enemy: 5e gives factual info about the target after
		// 1 min observation. We don't model "observation time" — proxy the
		// tactical edge as a flat +1 to attack rolls from L7 onward.
		if c.Level >= 7 {
			stats.AttackBonus++
		}
	case SubclassThief:
		// Phase 10 SUB3a-iii — Thief L10/L15.
		//
		// L10 Use Magic Device (5e L13: ignore class/race/level requirements
		// on magic items, including spell scrolls and wands). We don't have a
		// scroll/wand item system, so we proxy the fantasy as a small free
		// "magical implement" effect every fight: a pre-combat wand-zap and a
		// flat damage bump representing better tactical use of magic items
		// the Thief is now allowed to wield.
		// L15 Thief's Reflexes (5e L17: take two turns during the first round
		// of combat). Engine has no "extra turn" primitive, so we ride the
		// AssassinateBonusDmg + AssassinateAdvantage channel — the same one
		// Gloom Stalker's Dread Ambusher uses for bonus-opener damage. The
		// flat-damage proxy approximates the value of one extra turn's hit.
		if c.Level >= 10 {
			mods.FlatDmgStart += 6
			mods.DamageBonus += 0.05
		}
		if c.Level >= 15 {
			mods.AssassinateAdvantage = true
			mods.AssassinateBonusDmg += 8
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
	case SubclassWarDomain:
		// L5 War Priest: bonus-action weapon attack, usable WIS-mod times per
		// long rest. One-shot combat can't model an extra discrete attack, so
		// we proxy throughput with a flat +1 attack bonus and a small damage
		// bump. L7 War God's Blessing is a reaction granting an ally +10 to
		// attack — no allies in 1v1 combat, so it's a no-op.
		if c.Level >= 5 {
			stats.AttackBonus++
			mods.DamageBonus += 0.15
		}
	case SubclassTrickeryDomain:
		// L7 Cloak of Shadows: turn invisible until you attack/cast/end of
		// next turn. We proxy the brief defensive window as 2 rounds of 15%
		// enemy miss chance via SporeCloud (same channel a fog-cloud-style
		// consumable would use). L5 Blessing of the Trickster (Stealth
		// advantage on an ally) is non-combat flavor — skipped.
		if c.Level >= 7 {
			mods.SporeCloud += 2
		}
	case SubclassHunter:
		// L5 Hunter's Prey: 5e gives a one-of-three pick (Colossus Slayer:
		// +1d8 vs. damaged foes once per turn / Giant Killer reaction /
		// Horde Breaker second attack). One-shot combat can't model the
		// pick UI, so we collapse to the most generally-applicable flavor
		// — Colossus Slayer — as a flat +10% damage bump (any hit after
		// the opener lands on an already-damaged target).
		// L7 Defensive Tactics: collapse to Multiattack Defense — +1 AC.
		if c.Level >= 5 {
			mods.DamageBonus += 0.10
		}
		if c.Level >= 7 {
			stats.AC++
		}
	case SubclassBeastMaster:
		// L5 Ranger's Companion + Exceptional Training: a baseline combat
		// pet shows up on initiative and pokes the enemy. Use max-style
		// floors — if the player already has a stronger NPC pet active
		// (e.g. a higher-level adventure pet via combat_stats.go), keep
		// theirs. PetAttackDmg scales with level so the companion grows
		// with the Ranger.
		// L7 Bestial Fury: pet attacks twice when commanded. We can't
		// schedule a second swing in the existing pet hook, so we bump
		// the proc rate (extra swings per fight) and per-hit damage.
		if c.Level >= 5 {
			proc := 0.20
			dmg := 2 + c.Level/2
			if c.Level >= 7 {
				proc = 0.35
				dmg += 3
			}
			if mods.PetAttackProc < proc {
				mods.PetAttackProc = proc
			}
			if mods.PetAttackDmg < dmg {
				mods.PetAttackDmg = dmg
			}
		}
	case SubclassGloomStalker:
		// L5 Dread Ambusher: +WIS to initiative and +1 attack with +1d8 in
		// the first round. Initiative rides on Speed in our model; the
		// extra opener damage rides AssassinateBonusDmg (consumed on first
		// hit only — the same channel the Assassin uses).
		// L5 Umbral Sight: the in-darkness stealth advantage is wired in
		// subclassSkillAdvantage; here we just give the combat half.
		// L7 Stalker's Flurry: 5e re-rolls a missed attack. The closest
		// engine primitive is AssassinateAdvantage (best of two on the
		// first roll), which is a slightly tighter expression of the
		// same fantasy ("you don't whiff your opener").
		if c.Level >= 5 {
			stats.Speed += abilityModifier(c.WIS)
			mods.AssassinateBonusDmg += 5 + abilityModifier(c.WIS)
		}
		if c.Level >= 7 {
			mods.AssassinateAdvantage = true
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
		Description: "Add +d8 (≈+4, +5 at L10) to your first attack roll this combat (consumes 1 superiority die).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			// Phase 10 SUB3a-ii — L10 Improved Combat Superiority bumps the
			// die from d8 (avg 4.5) to d10 (avg 5.5).
			bonus := 4
			if c != nil && c.Level >= 10 {
				bonus = 5
			}
			mods.FirstAttackBonus += bonus
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
		Description: "Steel yourself for the fight: heal at low HP for d8 + CHA mod (d10 at L10) (consumes 1 superiority die).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			// Phase 10 SUB3a-ii — L10 Improved Combat Superiority: d8 → d10.
			base := 4
			if c != nil && c.Level >= 10 {
				base = 5
			}
			mods.HealItem += base + abilityModifier(c.CHA)
		},
	}

	// Phase 10 SUB2c — Cleric Channel Divinity. All three Cleric subclasses
	// share the "channel_divinity" resource pool (2/long-rest in our model;
	// 5e refreshes 1/short-rest at L2 and 2/short-rest at L6). One armed
	// expression per fight, mirroring the Battle Master pattern.
	dndActiveAbilities["preserve_life"] = DnDAbility{
		ID:          "preserve_life",
		Name:        "Preserve Life",
		Class:       ClassCleric,
		Subclass:    SubclassLifeDomain,
		Resource:    "channel_divinity",
		Description: "Channel Divinity: pour life force in at low HP — heals 5× Cleric level (consumes 1 channel divinity).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			// 5e caps the heal at half max HP per target; in 1v1 the entire
			// pool lands on the caster. HealItem fires once when player
			// drops below 50%, which matches Preserve Life's "in trouble"
			// fantasy.
			heal := 5 * c.Level
			if c.HPMax > 0 && heal > c.HPMax/2 {
				heal = c.HPMax / 2
			}
			mods.HealItem += heal
		},
	}
	dndActiveAbilities["guided_strike"] = DnDAbility{
		ID:          "guided_strike",
		Name:        "Guided Strike",
		Class:       ClassCleric,
		Subclass:    SubclassWarDomain,
		Resource:    "channel_divinity",
		Description: "Channel Divinity: +10 to your first attack roll this combat (consumes 1 channel divinity).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			mods.FirstAttackBonus += 10
		},
	}
	dndActiveAbilities["invoke_duplicity"] = DnDAbility{
		ID:          "invoke_duplicity",
		Name:        "Invoke Duplicity",
		Class:       ClassCleric,
		Subclass:    SubclassTrickeryDomain,
		Resource:    "channel_divinity",
		Description: "Channel Divinity: an illusory duplicate flanks the foe — advantage on your first attack and +10% damage (consumes 1 channel divinity).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			mods.AssassinateAdvantage = true
			mods.DamageBonus += 0.10
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

// lifeDomainHealBonus returns the extra HP a Life Domain Cleric's healing
// spell restores. Disciple of Life (L5): +(2 + spell level) HP, with cantrips
// counting as level 1 for the formula. Blessed Healer (L7) — "when casting a
// healing spell on an ally, you regain 2 + slot level HP" — has no ally in
// 1v1 play and is intentionally skipped here. Returns 0 for non-Life-Domain
// callers, non-heal spells, or pre-L5 levels.
func lifeDomainHealBonus(c *DnDCharacter, spell SpellDefinition, slotLevel int) int {
	if c == nil || c.Class != ClassCleric || c.Subclass != SubclassLifeDomain {
		return 0
	}
	if c.Level < 5 || spell.Effect != EffectSpellHeal {
		return 0
	}
	lvl := slotLevel
	if lvl < 1 {
		lvl = 1
	}
	return 2 + lvl
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
