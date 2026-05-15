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
func (p *AdventurePlugin) buildZoneCombatants(
	userID id.UserID,
	monster DnDMonsterTemplate,
	tier int,
	dmMood int,
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
	if firedName, fired := applyArmedAbility(dndChar, &playerMods); fired {
		slog.Info("dnd: armed ability fired (turn-based build)", "user", userID, "ability", firedName)
	}

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

// combatantsForSession reconstructs the Combatant pair for an in-flight
// CombatSession. It reloads the zone run for the DM mood + tier and looks the
// enemy up in the bestiary by the session's EnemyID. The pair carries no HP —
// the turn engine reads that from the session row.
func (p *AdventurePlugin) combatantsForSession(sess *CombatSession) (player Combatant, enemy Combatant, err error) {
	run, rerr := getZoneRun(sess.RunID)
	if rerr != nil {
		return Combatant{}, Combatant{}, fmt.Errorf("load zone run: %w", rerr)
	}
	if run == nil {
		return Combatant{}, Combatant{}, fmt.Errorf("combat session %s: zone run %s not found", sess.SessionID, sess.RunID)
	}
	monster, ok := dndBestiary[sess.EnemyID]
	if !ok {
		return Combatant{}, Combatant{}, fmt.Errorf("combat session %s: enemy %q not in bestiary", sess.SessionID, sess.EnemyID)
	}
	zone := zoneOrFallback(run.ZoneID)
	player, enemy, _, err = p.buildZoneCombatants(id.UserID(sess.UserID), monster, int(zone.Tier), run.DMMood)
	if err != nil {
		return player, enemy, err
	}
	// Fold any fight-scoped buffs a mid-fight !cast / !consume layered on back
	// onto the freshly-rebuilt player. The depleting one-shots (ward/spore/…)
	// live on the session's Statuses and flow through the turn engine's
	// resume/commit cycle, so only the persistent stat deltas are applied here.
	applySessionBuffs(&player, sess.Statuses)
	return player, enemy, err
}

// applySessionBuffs re-derives the persistent stat effect of every mid-fight
// buff onto the rebuilt player. The buffs are stored as accumulated deltas
// (diffTurnBuff produced them against the player's state at cast time), so
// re-applying them to a deterministic rebuild reproduces the same totals every
// round without double-counting.
func applySessionBuffs(player *Combatant, s CombatStatuses) {
	player.Stats.AC += s.BuffACBonus
	player.Stats.AttackBonus += s.BuffAtkBonus
	player.Stats.Speed += s.BuffSpeedBonus
	player.Stats.CritRate += s.BuffCritRate
	player.Mods.DamageBonus += s.BuffDamageBonus
	player.Mods.PetAttackProc += s.BuffPetProc
	player.Mods.PetAttackDmg += s.BuffPetDmg
	if s.BuffDamageReductMul > 0 {
		player.Mods.DamageReduct *= s.BuffDamageReductMul
	}
}
