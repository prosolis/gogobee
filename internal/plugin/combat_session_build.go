package plugin

import (
	"fmt"
	"log/slog"

	"maunium.net/go/mautrix/id"
)

// Phase 13 — Combatant reconstruction for turn-based fights.
//
// The auto-resolve path (runZoneCombat) builds a player/enemy Combatant pair,
// runs SimulateCombat, and discards them. The turn-based path needs the same
// pair, but rebuilt fresh on every player command from persisted session
// state — so the build is extracted here as buildZoneCombatants, and
// combatantsForSession wraps it for the resume case.
//
// HP is deliberately NOT carried by the Combatant pair: the turn engine reads
// live HP from the CombatSession row (sess.PlayerHP / sess.EnemyHP), so the
// rebuilt Combatants only need correct Stats/Mods/Ability. Re-deriving from
// current character state each round is safe because zone equipment, streaks,
// and bonuses don't change mid-fight.
//
// applyPendingCast and the auto-heal consumable setup are intentionally left
// out of the shared builder — those are one-shot mutations that runZoneCombat
// applies once before SimulateCombat. Re-applying them on every turn-based
// round would double-consume. Mid-fight !cast / !consume buffs are instead
// persisted on the CombatSession and folded back in by combatantsForSession
// via applySessionBuffs.

// buildZoneCombatants derives the player/enemy Combatant pair for a zone
// encounter: the player's full D&D layer (stats + class/race/subclass passives
// + armed ability) and the monster's bestiary stat block with tier scaling and
// the DM-mood combat tilt folded in. The returned dndChar is handed back so
// callers can run post-combat subclass persistence without reloading it.
//
// armed is the ability id already consumed for this fight (consumeArmedAbility,
// or armAbilityForFight's return on the auto-resolve paths); "" means none. The
// build re-applies it rather than consuming, because the turn-based path calls
// this once per player command and a consuming build would spend the ability on
// round 1 and drop it for the rest of the fight.
func (p *AdventurePlugin) buildZoneCombatants(
	userID id.UserID,
	monster DnDMonsterTemplate,
	tier int,
	dmMood int,
	armed string,
) (player Combatant, enemy Combatant, dndChar *DnDCharacter, err error) {
	tilt := dmMoodCombatTilt(dmMood)
	char, err := loadAdvCharacter(userID)
	if err != nil || char == nil {
		return Combatant{}, Combatant{}, nil, fmt.Errorf("load adv character: %w", err)
	}
	equip, err := loadAdvEquipment(userID)
	if err != nil {
		return Combatant{}, Combatant{}, nil, fmt.Errorf("load equipment: %w", err)
	}
	bonuses := p.loadCombatBonuses(userID, char)
	chatLvl := p.chatLevel(userID)

	playerStats, playerMods := DerivePlayerStats(char, equip, bonuses, chatLvl, char.CurrentStreak, false)

	dndChar, _, err = ensureDnDCharacterForCombat(userID, char)
	if err != nil {
		return Combatant{}, Combatant{}, nil, fmt.Errorf("ensure dnd character: %w", err)
	}
	applyDnDPlayerLayer(&playerStats, dndChar)
	applyDnDEquipmentLayer(&playerStats, dndChar, equip)
	applyDnDHPScaling(&playerStats, dndChar)
	applyClassPassives(&playerStats, &playerMods, dndChar)
	applyRacePassives(&playerStats, &playerMods, dndChar)
	applySubclassPassives(&playerStats, &playerMods, dndChar)
	applyMagicItemEffects(&playerStats, &playerMods, userID)
	applyAbilityByID(dndChar, armed, &playerMods)

	enemyStats, enemyMods := monster.toCombatStats()
	// Tier scaling mirrors runZoneCombat: only raise AC/AttackBonus to a
	// tier floor — boss/elite stat blocks already encode their challenge, so
	// we never double-scale a block that's already above the floor.
	if tier > 1 {
		floorAC := dndDungeonACBase + tier
		if enemyStats.AC < floorAC {
			enemyStats.AC = floorAC
		}
		floorAB := dndDungeonAtkBase + tier
		if enemyStats.AttackBonus < floorAB {
			enemyStats.AttackBonus = floorAB
		}
	}

	// DM mood tilts: monster Attack delta + player initiative bias.
	enemyStats.Attack += tilt.EnemyAttackDelta
	if enemyStats.Attack < 1 {
		enemyStats.Attack = 1
	}
	playerMods.InitiativeBias += tilt.InitiativeBias

	displayName, _ := loadDisplayName(userID)
	player = Combatant{
		Name:     displayName,
		Stats:    playerStats,
		Mods:     playerMods,
		IsPlayer: true,
	}
	enemy = Combatant{
		Name:    monster.Name,
		Stats:   enemyStats,
		Mods:    enemyMods,
		Ability: monster.Ability,
	}
	return player, enemy, dndChar, nil
}

// combatantsForSession reconstructs the Combatant pair for an in-flight solo
// CombatSession — the shape every caller wanted before parties existed. It is
// partyCombatantsForSession reduced to seat 0, and it errors rather than
// silently dropping the rest of a party's roster on the floor.
func (p *AdventurePlugin) combatantsForSession(sess *CombatSession) (player Combatant, enemy Combatant, err error) {
	if sess.IsParty() {
		return Combatant{}, Combatant{}, fmt.Errorf(
			"combat session %s seats %d players; use partyCombatantsForSession", sess.SessionID, sess.RosterSize())
	}
	players, e, err := p.partyCombatantsForSession(sess)
	if err != nil {
		return Combatant{}, Combatant{}, err
	}
	return *players[0], *e, nil
}

// partyCombatantsForSession reconstructs the seated roster and the enemy for an
// in-flight CombatSession. It reloads the zone run for the DM mood + tier and
// looks the enemy up in the bestiary by the session's EnemyID. The combatants
// carry no HP — the turn engine reads that from the session row and the
// participant rows.
//
// Every seat is rebuilt from *its own* player: their sheet, their equipment,
// their passives, their magic items. That is N times the per-round DB chatter
// combatantsForSession already had (project_combat_session_cache_deferred), and
// a party of 3 pays it three times over. Solo — every fight that has ever run,
// and the whole balance corpus — still issues exactly one build.
func (p *AdventurePlugin) partyCombatantsForSession(sess *CombatSession) ([]*Combatant, *Combatant, error) {
	run, rerr := getZoneRun(sess.RunID)
	if rerr != nil {
		return nil, nil, fmt.Errorf("load zone run: %w", rerr)
	}
	if run == nil {
		return nil, nil, fmt.Errorf("combat session %s: zone run %s not found", sess.SessionID, sess.RunID)
	}
	monster, ok := dndBestiary[sess.EnemyID]
	if !ok {
		return nil, nil, fmt.Errorf("combat session %s: enemy %q not in bestiary", sess.SessionID, sess.EnemyID)
	}
	zone := zoneOrFallback(run.ZoneID)

	seats := sess.SeatUserIDs()
	players := make([]*Combatant, len(seats))
	var enemy Combatant
	for seat, uid := range seats {
		// The ability this seat armed was consumed once, at fight start, and its
		// id parked on their statuses. Re-applying it here — not re-consuming —
		// is what makes a rage last the whole fight instead of one round.
		st := sess.actorStatusesForSeat(seat)

		// The hired companion has no sheet to load — by design, because a
		// player_meta row for him is the thing that would turn him into a real
		// character everywhere. Synthesize his seat instead. This branch is
		// load-bearing: buildZoneCombatants would fail on him, and one
		// unbuildable seat fails the whole rebuild, which is what every human's
		// !attack in the fight depends on.
		if isCompanionUser(uid) {
			class, level := companionLoadoutForRun(sess.RunID)
			player, en, _ := p.companionCombatant(class, level, monster, int(zone.Tier), run.DMMood)
			applySessionBuffs(&player, st)
			players[seat] = &player
			if seat == 0 {
				// Unreachable today (seat 0 is always the leader), but if it ever
				// isn't, the enemy still has to come from somewhere.
				enemy = en
				if sess.IsParty() {
					enemy.Stats.MaxHP = scaledEnemyMaxHP(enemy.Stats.MaxHP, sess.RosterSize())
				}
			}
			continue
		}

		player, e, _, err := p.buildZoneCombatants(id.UserID(uid), monster, int(zone.Tier), run.DMMood, st.ArmedAbility)
		if err != nil {
			return nil, nil, fmt.Errorf("seat %d (%s): %w", seat, uid, err)
		}
		// Fold any fight-scoped buffs this seat's mid-fight !cast / !consume
		// layered on back onto their freshly-rebuilt combatant. The depleting
		// one-shots (ward/spore/…) live on their persisted statuses and flow
		// through the turn engine's resume/commit cycle, so only the persistent
		// stat deltas are applied here — and only that seat's own.
		applySessionBuffs(&player, st)
		players[seat] = &player
		if seat == 0 {
			// The enemy build reads only (monster, tier, dmMood): every seat
			// rebuilds the identical stat block, so seat 0's copy is the fight's.
			// Only the *player* half of the build varies by seat.
			enemy = e
			// Party-only enemy HP bump, re-derived each turn from the template so
			// it never compounds. Matches the scalar startPartyCombatSession used
			// for the initial persist; solo (roster 1) scales by 1.0.
			if sess.IsParty() {
				enemy.Stats.MaxHP = scaledEnemyMaxHP(enemy.Stats.MaxHP, sess.RosterSize())
			}
		}
	}
	return players, &enemy, nil
}

// seatFightStartMods re-derives the modifiers a finished fight's close-out still
// needs: the Berserker's rage flag, which decides whether the character owes a
// point of exhaustion, and the Necromancy Mage's Grim Harvest stash.
//
// It re-applies the seat's armed ability to an empty mod set rather than
// rebuilding the whole combatant: no Apply writes to the character (they read
// level and HP and write only mods), so this is pure, and the passive/equipment
// layers a full build would add are not read by any post-combat hook.
//
// The Grim Harvest pair is not re-derived but read back off the seat's
// statuses, where castActionForSeat parked it: the spell that stashed it was
// cast mid-fight, not at fight start, and nothing in the character sheet still
// remembers which one it was.
func seatFightStartMods(sess *CombatSession, seat int, c *DnDCharacter) CombatModifiers {
	var mods CombatModifiers
	st := sess.actorStatusesForSeat(seat)
	applyAbilityByID(c, st.ArmedAbility, &mods)
	mods.GrimHarvestSlot = st.GrimHarvestSlot
	mods.GrimHarvestNecrotic = st.GrimHarvestNecrotic
	return mods
}

// seatCombatResult reconstructs, for one seat of a finished turn-based fight,
// the slice of CombatResult that the post-combat hooks actually read. The turn
// engine never builds a CombatResult — it persists a session and a shared event
// log — so the close-out has to assemble one.
//
// MistyHealed is read back off the seat's event log rather than off a flag: the
// turn engine's CombatResult is a scratch value it discards between steps, and
// the log is the only thing that survives to the close-out. This is how
// combat_pet_save has always been detected.
//
// SniperKilled stays false. Arina's proc is a pre-combat one-shot, not a
// round-end effect, so seatEndOfRound does not carry it and the turn engine
// still has no place to fire it — combat_sniper_kill remains unreachable from a
// manual kill.
//
// NearDeath mirrors the auto-resolve engine's win threshold (below 15% of max);
// its loss-side meaning is unused here, since the only hook that reads it gates
// on PlayerWon.
func seatCombatResult(sess *CombatSession, seat int) CombatResult {
	hp, hpMax := sess.seatHP(seat), sess.seatHPMax(seat)
	won := sess.Status == CombatStatusWon
	events := eventsForSeat(sess.TurnLog, seat)
	return CombatResult{
		PlayerWon:   won,
		Events:      events,
		PlayerEndHP: hp,
		EnemyEndHP:  sess.EnemyHP,
		TotalRounds: sess.Round,
		NearDeath:   won && hp > 0 && float64(hp) < float64(max(1, hpMax))*0.15,
		MistyHealed: hasAction(events, "misty_heal"),
	}
}

// hasAction reports whether the event log holds at least one event with the
// given Action. The one-line "did this happen in the fight?" scan the close-out
// uses to read a proc's outcome off the log rather than a flag.
func hasAction(events []CombatEvent, action string) bool {
	for _, e := range events {
		if e.Action == action {
			return true
		}
	}
	return false
}

// postCombatBookkeepingForSeat is postCombatBookkeeping for one seat of a
// terminal turn-based session — the bridge between what the turn engine
// persisted and what the shared close-out expects.
func (p *AdventurePlugin) postCombatBookkeepingForSeat(sess *CombatSession, seat int) {
	uid := id.UserID(sess.seatUserID(seat))
	// Loaded after the caller's persistDnDHPAfterCombat, so a Grim-Harvest-style
	// hook that heals off HPCurrent reads the fight's real ending HP.
	c, err := LoadDnDCharacter(uid)
	if err != nil || c == nil {
		slog.Error("combat: post-combat bookkeeping skipped, no sheet", "user", uid, "err", err)
		return
	}
	mods := seatFightStartMods(sess, seat, c)
	p.postCombatBookkeeping(uid, c, mods.BerserkerRage, seatCombatResult(sess, seat), mods)
}

// applySessionBuffs re-derives the persistent stat effect of every mid-fight
// buff onto one rebuilt character. The buffs are stored as accumulated deltas
// (diffTurnBuff produced them against that character's state at cast time), so
// re-applying them to a deterministic rebuild reproduces the same totals every
// round without double-counting. It takes ActorStatuses rather than the whole
// session because buffs are per-caster: each seat folds in only its own.
func applySessionBuffs(player *Combatant, s ActorStatuses) {
	player.Stats.AC += s.BuffACBonus
	player.Stats.AttackBonus += s.BuffAtkBonus
	player.Stats.Speed += s.BuffSpeedBonus
	player.Stats.CritRate += s.BuffCritRate
	player.Mods.DamageBonus += s.BuffDamageBonus
	player.Mods.PetAttackProc += s.BuffPetProc
	player.Mods.PetAttackDmg += s.BuffPetDmg
	player.Mods.SpiritWeaponProc += s.BuffSpiritProc
	player.Mods.SpiritWeaponDmg += s.BuffSpiritDmg
	if s.BuffDamageReductMul > 0 {
		player.Mods.DamageReduct *= s.BuffDamageReductMul
	}
}
