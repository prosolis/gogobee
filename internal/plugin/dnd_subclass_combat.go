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
//
// Comment convention: `// internal note (not user-facing)` marks block
// comments documenting tuning history (Phase 2/3 balance notes, scaling
// rationale, etc.). They are engineering context and must NOT be lifted
// into Description / Flavor strings by codegen.

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
		// Phase 10 SUB3b-i — L10 Spell Resistance: 5e gives advantage on
		// saves vs. spells *and* resistance to spell damage. Our 1v1 enemies
		// rarely force spell saves, so the "advantage" half is largely inert;
		// we collapse the effect to a flat 8% incoming-damage reduction
		// (DamageReduct is multiplicative, so 0.92 = ~8% off everything,
		// which is close to "resistance against the spell-damage subset").
		if c.Level >= 10 {
			mods.DamageReduct *= 0.92
		}
		// Phase 10 SUB3b-i — L15 Spell Reflection: 5e lets a successful
		// counterspell redirect the spell back at the caster. We don't model
		// counterspell (reaction primitive deferred to Phase 11), so we ride
		// the ReflectNext channel — the first incoming hit reflects 30% of
		// its damage back. Closest in-engine analogue to "the magic hits the
		// caster instead".
		if c.Level >= 15 {
			mods.ReflectNext += 0.30
		}
	case SubclassNecromancy:
		// Phase 10 SUB3b-i — L15 Improved Undead Thralls. 5e: undead thralls
		// gain +CON mod to HP and attack damage. We don't model an Animate
		// Dead summoning system (SUB2b skipped Undead Thralls for the same
		// reason), but the L15 capstone is too central to skip — proxy a
		// permanent skeletal minion via the PetAttack channel. CON-mod scales
		// dmg, mirroring the 5e formula. L10 Command Undead's combat surface
		// is folded into grimHarvestHeal (deeper authority over death = more
		// life harvested per spell-kill); see dnd_subclass_combat.go below.
		if c.Level >= 15 {
			conMod := abilityModifier(c.CON)
			if conMod < 0 {
				conMod = 0
			}
			thrallDmg := 6 + conMod
			if mods.PetAttackProc < 0.30 {
				mods.PetAttackProc = 0.30
			}
			if mods.PetAttackDmg < thrallDmg {
				mods.PetAttackDmg = thrallDmg
			}
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
	case SubclassLifeDomain:
		// Phase 10 SUB3c — Life Domain L10 Divine Strike. 5e: +1d8 radiant
		// damage on a weapon hit, once per turn (+2d8 at L14). We collapse
		// the once-per-turn cadence to "every weapon hit" since our 1v1 model
		// has no turn-boundary primitive; avg 1d8 = 4, 2d8 = 9 at L14+.
		// L5 Disciple of Life rides lifeDomainHealBonus and L15 Supreme
		// Healing rides lifeDomainSupremeHealing — both out-of-combat heal
		// hooks, so they don't surface here.
		if c.Level >= 14 {
			mods.DivineStrikePerHit += 9
		} else if c.Level >= 10 {
			mods.DivineStrikePerHit += 4
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
		// Phase 10 SUB3c — War Domain L10 Divine Strike: +1d8 weapon-type
		// damage on every weapon hit (+2d8 at L14). Same channel as Life
		// Domain's radiant version — engine doesn't track damage types.
		if c.Level >= 14 {
			mods.DivineStrikePerHit += 9
		} else if c.Level >= 10 {
			mods.DivineStrikePerHit += 4
		}
		// Phase 10 SUB3c — L15 Avatar of Battle: 5e gives resistance to
		// bludgeoning/piercing/slashing from non-magical weapons. Most enemy
		// damage in our model is non-magical physical, so resistance lands
		// on the bulk of incoming damage. We collapse to a flat 20% incoming
		// reduction — softer than full 50% physical resistance to account
		// for magical/elemental hits the resistance wouldn't catch.
		if c.Level >= 15 {
			mods.DamageReduct *= 0.80
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
		// Phase 10 SUB3c — Trickery Domain L10 Divine Strike: +1d8 poison
		// (+2d8 at L14). Same engine channel as the other two domains.
		if c.Level >= 14 {
			mods.DivineStrikePerHit += 9
		} else if c.Level >= 10 {
			mods.DivineStrikePerHit += 4
		}
		// Phase 10 SUB3c — L15 Improved Duplicity: 5e spawns up to 4
		// duplicates and grants advantage to allies adjacent to one. No
		// allies in 1v1, so the literal effect is inert. We proxy the
		// permanent-illusion fantasy passively: +1 round of SporeCloud
		// (duplicates flicker around the foe creating extra confusion on
		// top of Cloak of Shadows) and a small flanking damage bump.
		if c.Level >= 15 {
			mods.SporeCloud++
			mods.DamageBonus += 0.05
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
		// Phase 10 SUB3d — Hunter L10 Multiattack (Volley/Whirlwind). Both 5e
		// options are AoE; in 1v1 there's only one target, so we collapse the
		// extra-target throughput to "extra hit per round on the one foe" — a
		// flat +15% damage bump (close to a half-extra-attack's value, soft
		// on the upside since AoE wouldn't all land on a single target).
		if c.Level >= 10 {
			mods.DamageBonus += 0.15
		}
		// Phase 10 SUB3d — Hunter L15 Superior Hunter's Defense. 5e picks one
		// of Evasion / Stand Against the Tide / Uncanny Dodge. Uncanny Dodge
		// (halve damage from one attack/turn) is the most generally-applicable
		// — proxy as a permanent ~15% incoming damage reduction.
		if c.Level >= 15 {
			mods.DamageReduct *= 0.85
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
		// Phase 10 SUB3d — Beast Master L10 Share Spells. 5e: any spell the
		// Ranger casts on themselves can also affect the pet within 30 ft.
		// In practice this means the pet rides the Ranger's self-buffs
		// (Hunter's Mark damage rider, Pass without Trace, etc.). We fold
		// that into the pet channel: bump per-hit damage and proc rate to
		// reflect the buffed pet, plus a small DamageBonus rider matching
		// the player's own buffed swings sharing.
		if c.Level >= 10 {
			if mods.PetAttackProc < 0.45 {
				mods.PetAttackProc = 0.45
			}
			if mods.PetAttackDmg < 6+c.Level/2 {
				mods.PetAttackDmg = 6 + c.Level/2
			}
			mods.DamageBonus += 0.05
		}
		// Phase 10 SUB3d — Beast Master L15 Superior Bond. 5e: pet immune to
		// charm/fright, returns at 1 HP after a long rest if killed. We don't
		// simulate pet death, so we proxy the "always-there partner" fantasy
		// as a further pet-throughput bump and a small DamageReduct (pet
		// body-blocks more reliably).
		if c.Level >= 15 {
			if mods.PetAttackProc < 0.55 {
				mods.PetAttackProc = 0.55
			}
			extra := 8 + c.Level/2
			if mods.PetAttackDmg < extra {
				mods.PetAttackDmg = extra
			}
			mods.DamageReduct *= 0.92
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
		// Phase 10 SUB3d — Gloom Stalker L10 Stalker's Flurry. 5e: re-roll a
		// missed attack once per turn. AssassinateAdvantage already grants
		// "best of two" on the opener (L7 Stalker's Flurry proxy), so the
		// L10 expansion to "every turn" rides as steady throughput — a flat
		// +12% damage bump representing recovered misses across the fight.
		if c.Level >= 10 {
			mods.DamageBonus += 0.12
		}
		// Phase 10 SUB3d — Gloom Stalker L15 Shadowy Dodge. 5e: Reaction —
		// when attacked, impose disadvantage on that attack. We don't model
		// reactions, so collapse the "one attack per round at disadvantage"
		// to a permanent ~10% incoming damage reduction.
		if c.Level >= 15 {
			mods.DamageReduct *= 0.90
		}

	// ── Open5e caster scaffold — Druid / Bard / Sorcerer / Warlock /
	// Paladin subclasses. Passive-only, consistent with the majority of the
	// original fifteen (Champion, Thief, the Mage and Ranger subclasses):
	// no new resource pools or active abilities. Each L5/7/10/15 tier rides
	// an existing CombatModifiers channel — per-case notes name the 5e
	// ability each proxy stands in for. DamageReduct is initialized to 1.0
	// by DerivePlayerStats, so the `*=` reductions compose cleanly.
	case SubclassCircleLand:
		// L5 Natural Recovery: a measured self-heal — rides HealItem (fires
		// once at <50% HP). L7 Land's Stride: +2 Speed (initiative). L10
		// Nature's Ward: resistance to a slice of elemental/poison damage,
		// collapsed to a flat 8% incoming reduction. L15 Nature's Sanctuary:
		// foes hesitate to close — 1 round of SporeCloud — plus a further 10%.
		if c.Level >= 5 {
			mods.HealItem += 5
		}
		if c.Level >= 7 {
			stats.Speed += 2
		}
		if c.Level >= 10 {
			mods.DamageReduct *= 0.92
		}
		if c.Level >= 15 {
			mods.SporeCloud++
			mods.DamageReduct *= 0.90
		}
	case SubclassCircleMoon:
		// L5 Combat Wild Shape: a bestial form's HP pool — an ArcaneWardHP
		// buffer scaling with level — plus the form's heavier hits (+10%
		// damage). L7 Primal Strike: beast attacks bite harder (+5%). L10
		// Elemental Wild Shape: an elemental form's opening slam — a
		// FlatDmgStart burst. L15 Archdruid: the form barely tires — a flat
		// 15% incoming reduction.
		if c.Level >= 5 {
			if buf := 3 * c.Level; mods.ArcaneWardHP < buf {
				mods.ArcaneWardHP = buf
			}
			mods.DamageBonus += 0.10
		}
		if c.Level >= 7 {
			mods.DamageBonus += 0.05
		}
		if c.Level >= 10 {
			mods.FlatDmgStart += 8
		}
		if c.Level >= 15 {
			mods.DamageReduct *= 0.85
		}
	case SubclassCircleShepherd:
		// L5 Spirit Totem (Bear): a warding spirit grants a temp-HP buffer.
		// L7 Mighty Summoner: a conjured beast joins the fight — rides the
		// pet channel. L10 Guardian Spirit: the spirits mend you — HealItem.
		// L15 Faithful Summons: the conjured ally is tougher and strikes
		// more often.
		if c.Level >= 5 {
			if buf := 2 * c.Level; mods.ArcaneWardHP < buf {
				mods.ArcaneWardHP = buf
			}
		}
		if c.Level >= 7 {
			if mods.PetAttackProc < 0.25 {
				mods.PetAttackProc = 0.25
			}
			if dmg := 3 + c.Level/2; mods.PetAttackDmg < dmg {
				mods.PetAttackDmg = dmg
			}
		}
		if c.Level >= 10 {
			mods.HealItem += c.Level / 2
		}
		if c.Level >= 15 {
			if mods.PetAttackProc < 0.45 {
				mods.PetAttackProc = 0.45
			}
			if dmg := 8 + c.Level/2; mods.PetAttackDmg < dmg {
				mods.PetAttackDmg = dmg
			}
		}
	case SubclassCollegeLore:
		// L5 Cutting Words: a barbed quip throws the foe off — 2 rounds of
		// SporeCloud miss chance. L7 Additional Magical Secrets: borrowed
		// versatility, a flat +1 to attack rolls. L10 Peerless Skill: a
		// self-aimed Bardic Inspiration die on the opener (FirstAttackBonus).
		// L15: deeper Magical Secrets keep paying out — +10% damage.
		if c.Level >= 5 {
			mods.SporeCloud += 2
		}
		if c.Level >= 7 {
			stats.AttackBonus++
		}
		if c.Level >= 10 {
			mods.FirstAttackBonus += 5
		}
		if c.Level >= 15 {
			mods.DamageBonus += 0.10
		}
	case SubclassCollegeValor:
		// L5 Combat Inspiration: songs that sharpen the blade (+10% damage).
		// L7 Extra Attack: a real second swing (5e literal Extra Attack).
		// Class-identity audit (2026-05-16) — previously proxied as +1
		// AttackBonus + 10% damage; now uses ExtraAttacks. L10 Battle
		// Magic: a bonus-action strike after a spell — a FlatDmgStart
		// burst. L15: +1 AC, late-game shield half of Combat Inspiration.
		if c.Level >= 5 {
			mods.DamageBonus += 0.10
		}
		if c.Level >= 7 {
			mods.ExtraAttacks += 1
		}
		if c.Level >= 10 {
			mods.FlatDmgStart += 6
		}
		if c.Level >= 15 {
			stats.AC++
		}
	case SubclassCollegeGlamour:
		// L5 Mantle of Inspiration: a fey-glamour temp-HP buffer. L7 Mantle
		// of Majesty: the foe is briefly enthralled — skips its first
		// attack. L10 Enthralling Performance: lingering fascination — 2
		// rounds of SporeCloud miss chance. L15 Unbreakable Majesty: foes
		// struggle to bring themselves to strike you — a flat 15% reduction.
		if c.Level >= 5 {
			if buf := 2 * c.Level; mods.ArcaneWardHP < buf {
				mods.ArcaneWardHP = buf
			}
		}
		if c.Level >= 7 {
			mods.SpellEnemySkipFirst = true
		}
		if c.Level >= 10 {
			mods.SporeCloud += 2
		}
		if c.Level >= 15 {
			mods.DamageReduct *= 0.85
		}
	case SubclassDraconicBloodline:
		// L5 Draconic Resilience: scaled hide — +1 AC and an HP buffer. L7
		// Elemental Affinity: CHA folded into spell damage — +10%. L10
		// Dragon Wings: mobility and a tougher frame — an 8% reduction and a
		// small damage bump. L15 Draconic Presence: a frightful aura — 2
		// rounds of SporeCloud miss chance.
		// Phase 2 class-balance: L5 was defense-only; gave it a small bite
		// (10% damage) so the L5 chassis isn't pure attrition vs T4 monsters.
		if c.Level >= 5 {
			stats.AC++
			if mods.ArcaneWardHP < c.Level {
				mods.ArcaneWardHP = c.Level
			}
			mods.DamageBonus += 0.10
		}
		if c.Level >= 7 {
			mods.DamageBonus += 0.10
		}
		if c.Level >= 10 {
			mods.DamageReduct *= 0.92
			mods.DamageBonus += 0.05
		}
		if c.Level >= 15 {
			mods.SporeCloud += 2
		}
	case SubclassWildMagic:
		// L5 Tides of Chaos: fortune favors the opener — advantage on the
		// first attack. L7 Bend Luck: nudge a roll your way —
		// FirstAttackBonus. L10 Controlled Chaos: a surge you actually aimed
		// — a FlatDmgStart burst. L15 Spell Bombardment: dice that keep
		// exploding — +15% damage.
		// Phase 2 class-balance: bare advantage-on-opener left this subclass
		// the weakest L5 caster pick (0.065 at T4 pre-tune vs Champion 0.92).
		// Added a 10% damage rider — Wild Magic surges as flavor, ~+1d6 of
		// chaos damage per round in mechanics.
		if c.Level >= 5 {
			mods.AssassinateAdvantage = true
			mods.DamageBonus += 0.10
		}
		if c.Level >= 7 {
			mods.FirstAttackBonus += 3
		}
		if c.Level >= 10 {
			mods.FlatDmgStart += 10
		}
		if c.Level >= 15 {
			mods.DamageBonus += 0.15
		}
	case SubclassStormSorcery:
		// L5 Tempestuous Magic: the storm moves first — +1 initiative and
		// +2 Speed. L7 Heart of the Storm: a thunderclap as you cast — a
		// level-scaled FlatDmgStart burst. L10 Storm Guide: you ride the
		// weather — a small 5% reduction and 1 round of SporeCloud. L15
		// Storm's Fury: incoming blows are answered with lightning —
		// ReflectNext on the first hit.
		// Phase 2 class-balance: init/speed alone left this subclass at 0.035
		// T4 pre-tune. Pull a small slice of the L7 thunderclap forward so
		// the L5 chassis has actual damage output.
		if c.Level >= 5 {
			mods.InitiativeBias += 1
			stats.Speed += 2
			mods.FlatDmgStart += 4 + c.Level/2
		}
		if c.Level >= 7 {
			mods.FlatDmgStart += 5 + c.Level/2
		}
		if c.Level >= 10 {
			mods.DamageReduct *= 0.95
			mods.SporeCloud++
		}
		if c.Level >= 15 {
			mods.ReflectNext += 0.30
		}
	case SubclassFiendPatron:
		// L5 Dark One's Blessing: temp HP as foes fall — modeled as an
		// up-front CHA-scaled buffer. L7 Dark One's Own Luck: a nudged roll
		// — FirstAttackBonus. L10 Fiendish Resilience: chosen resistance — a
		// flat 10% incoming reduction. L15 Hurl Through Hell: a foe sent
		// briefly to the lower planes — a heavy FlatDmgStart burst.
		if c.Level >= 5 {
			chaMod := abilityModifier(c.CHA)
			if chaMod < 0 {
				chaMod = 0
			}
			if buf := c.Level + chaMod; mods.ArcaneWardHP < buf {
				mods.ArcaneWardHP = buf
			}
		}
		if c.Level >= 7 {
			mods.FirstAttackBonus += 3
		}
		if c.Level >= 10 {
			mods.DamageReduct *= 0.90
		}
		if c.Level >= 15 {
			mods.FlatDmgStart += 12
		}
	case SubclassArchfeyPatron:
		// L5 Fey Presence: the foe is briefly charmed/frightened — skips its
		// first attack. L7 Misty Escape: you blink out of harm's way — 2
		// rounds of SporeCloud miss chance. L10 Beguiling Defenses: hostile
		// magic rebounds — ReflectNext on the first hit. L15 Dark Delirium:
		// the foe is lost in illusion — more SporeCloud and a small edge.
		if c.Level >= 5 {
			mods.SpellEnemySkipFirst = true
		}
		if c.Level >= 7 {
			mods.SporeCloud += 2
		}
		if c.Level >= 10 {
			mods.ReflectNext += 0.25
		}
		if c.Level >= 15 {
			mods.SporeCloud += 2
			mods.DamageBonus += 0.05
		}
	case SubclassGreatOldOne:
		// L5 Entropic Ward: a missed attack tilts the next exchange — a
		// small reduction and 1 round of SporeCloud. L7 Thought Shield:
		// psychic resilience — a flat 8% incoming reduction. L10 Create
		// Thrall: a dominated mind fights beside you — the pet channel. L15:
		// deeper psychic dominion keeps the thrall in the fight and sharpens
		// your own assault (+10% damage).
		// Phase 2 class-balance: L5 was defense-only (0.130 T4 pre-tune).
		// Added a small bite so the L5 chassis can press the advantage when
		// the foe misses.
		if c.Level >= 5 {
			mods.DamageReduct *= 0.95
			mods.SporeCloud++
			mods.DamageBonus += 0.10
		}
		if c.Level >= 7 {
			mods.DamageReduct *= 0.92
		}
		if c.Level >= 10 {
			if mods.PetAttackProc < 0.30 {
				mods.PetAttackProc = 0.30
			}
			if dmg := 4 + c.Level/2; mods.PetAttackDmg < dmg {
				mods.PetAttackDmg = dmg
			}
		}
		if c.Level >= 15 {
			mods.DamageBonus += 0.10
		}
	case SubclassOathDevotion:
		// L5 Sacred Weapon: a blade lit with the oath — +1 attack and
		// radiant damage on every hit (DivineStrikePerHit, the channel the
		// Cleric domains use). L7 Aura of Devotion: a small warding aura —
		// 5% reduction. L10 Aura of Courage/Purity: the aura hardens — a
		// further 8% reduction. L15 Holy Nimbus: a radiant aura that sears
		// the foe on engagement — a FlatDmgStart burst.
		if c.Level >= 5 {
			stats.AttackBonus++
			mods.DivineStrikePerHit += 4
		}
		if c.Level >= 7 {
			mods.DamageReduct *= 0.95
		}
		if c.Level >= 10 {
			mods.DamageReduct *= 0.92
		}
		if c.Level >= 15 {
			mods.FlatDmgStart += 10
		}
	case SubclassOathVengeance:
		// L5 Vow of Enmity: advantage against the marked foe plus the focus
		// to press it — advantage on the opener and +10% damage. L7
		// Relentless Avenger: you keep pace with a fleeing target — +2 Speed
		// and a small damage bump. L10 Soul of Vengeance: an extra strike at
		// the marked foe — +10% damage. L15: the hunt never lets up —
		// a FlatDmgStart opener.
		if c.Level >= 5 {
			mods.AssassinateAdvantage = true
			mods.DamageBonus += 0.10
		}
		if c.Level >= 7 {
			stats.Speed += 2
			mods.DamageBonus += 0.05
		}
		if c.Level >= 10 {
			mods.DamageBonus += 0.10
		}
		if c.Level >= 15 {
			mods.FlatDmgStart += 6
		}
	case SubclassOathAncients:
		// L5 Nature's Wrath: the green and growing things slow the foe — a
		// small reduction. L7 Aura of Warding: resistance to spell damage —
		// a further 8% reduction. L10 Undying Sentinel: you refuse to fall —
		// survive one otherwise-lethal hit (DeathSave). L15 Elder Champion:
		// the old light blazes up — +1 attack and +10% damage.
		if c.Level >= 5 {
			mods.DamageReduct *= 0.95
		}
		if c.Level >= 7 {
			mods.DamageReduct *= 0.92
		}
		if c.Level >= 10 {
			mods.DeathSave = true
		}
		if c.Level >= 15 {
			stats.AttackBonus++
			mods.DamageBonus += 0.10
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
	// Three more maneuvers using existing CombatModifiers fields. The
	// remaining four spec maneuvers (Pushing, Goading, Riposte, Commander's
	// Strike) need ally-targeting / reaction mechanics that the one-shot
	// engine doesn't model — they're omitted rather than approximated
	// inaccurately.
	dndActiveAbilities["disarming_attack"] = DnDAbility{
		ID:          "disarming_attack",
		Name:        "Disarming Attack",
		Class:       ClassFighter,
		Subclass:    SubclassBattleMaster,
		Resource:    "superiority",
		Description: "Knock the enemy's weapon aside: their damage is reduced for the rest of the fight (consumes 1 superiority die).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			// 25% incoming-damage cut models a fighter who is now
			// swinging an improvised weapon at -d4-ish. DamageReduct
			// is multiplicative (1.0 neutral, lower = less damage),
			// so multiply down.
			mods.DamageReduct *= 0.75
		},
	}
	dndActiveAbilities["menacing_attack"] = DnDAbility{
		ID:          "menacing_attack",
		Name:        "Menacing Attack",
		Class:       ClassFighter,
		Subclass:    SubclassBattleMaster,
		Resource:    "superiority",
		Description: "Snarl as you swing — the enemy hesitates and skips its first attack (consumes 1 superiority die).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			mods.SpellEnemySkipFirst = true
		},
	}
	dndActiveAbilities["parry"] = DnDAbility{
		ID:          "parry",
		Name:        "Parry",
		Class:       ClassFighter,
		Subclass:    SubclassBattleMaster,
		Resource:    "superiority",
		Description: "Set yourself for incoming blows — physical damage taken is sharply reduced (consumes 1 superiority die).",
		Apply: func(c *DnDCharacter, mods *CombatModifiers) {
			// 40% reduction; multiplicative (see disarming_attack note).
			mods.DamageReduct *= 0.60
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

// lifeDomainSupremeHealing reports whether a Life Domain Cleric has reached
// L15 Supreme Healing — when true, healing-spell dice should resolve at max
// face value instead of being rolled. resolveHealOutOfCombat consults this to
// decide whether to roll or take max for each die.
func lifeDomainSupremeHealing(c *DnDCharacter) bool {
	return c != nil && c.Class == ClassCleric &&
		c.Subclass == SubclassLifeDomain && c.Level >= 15
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
	// Phase 10 SUB3b-i — L10 Command Undead. The 5e ability is "take control
	// of any undead", which has no surface in our 1v1 combat (no allies to
	// hand summons over to). We re-fluff it here as deeper authority over
	// death amplifying the harvest itself: multipliers tick up by 1 at L10
	// (2× → 3× non-necrotic, 3× → 4× necrotic).
	mult := 2
	if mods.GrimHarvestNecrotic {
		mult = 3
	}
	if c.Level >= 10 {
		mult++
	}
	return mult * mods.GrimHarvestSlot
}
