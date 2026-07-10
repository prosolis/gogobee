package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"maunium.net/go/mautrix/id"
)

// runZoneCombatRoster auto-resolves one zone encounter for a whole roster.
//
// It is the seam P6e opens: before it, only elite and boss doorways seated a
// party (through the turn engine), and every other fight in a zone — exploration
// rooms, patrol encounters, harvest interrupts — resolved as the leader alone.
// On a 38-room expedition that meant a party of three fought together twice and
// the leader soloed the other ~35, which is why P7 measured zero member deaths
// and zero TPKs across 240 seats: the members were never in the fight.
//
// Roster order is the seating order, and roster[0] is the leader. Callers get
// back the seats that actually sat down, aligned index-for-index with
// res.Seats, so the character-scoped close-out (loot, death) can fan out and the
// run-scoped one (threat, kill record) can stay with the leader.
//
// A member who is down sits the encounter out. A leader who is down is a
// caller-side error — the run is theirs and the walk should not have reached
// here.
//
// Effects applied here are exactly the character-scoped ones runZoneCombat
// always applied, now per seat: HP, XP, achievements, the subclass persist, the
// heal items actually burned, and Misty's condition repair. Loot, threat, kill
// records and death remain the caller's, because only the caller knows the room.
func (p *AdventurePlugin) runZoneCombatRoster(
	roster []id.UserID,
	monster DnDMonsterTemplate,
	tier int,
	phases []CombatPhase,
	dmMood int,
) (PartyCombatResult, []id.UserID, error) {
	if len(roster) == 0 {
		return PartyCombatResult{}, nil, fmt.Errorf("runZoneCombatRoster: empty roster")
	}
	if phases == nil {
		phases = dungeonCombatPhases
	}

	type seatBuild struct {
		uid     id.UserID
		dndChar *DnDCharacter
		advChar *AdventureCharacter
		equip   map[EquipmentSlot]*AdvEquipment
		mods    CombatModifiers
	}

	var (
		players []Combatant
		builds  []seatBuild
		seated  []id.UserID
		enemy   Combatant
	)

	for i, uid := range roster {
		leader := i == 0

		// The leader's failures are the encounter's failures; a member's only
		// cost them their seat.
		bail := func(err error) (PartyCombatResult, []id.UserID, error) {
			return PartyCombatResult{}, nil, err
		}

		advChar, err := loadAdvCharacter(uid)
		if err != nil || advChar == nil {
			if leader {
				return bail(fmt.Errorf("load adv character: %w", err))
			}
			slog.Warn("zone: party member left out of auto-resolved fight", "user", uid, "err", err)
			continue
		}
		equip, err := loadAdvEquipment(uid)
		if err != nil {
			if leader {
				return bail(fmt.Errorf("load equipment: %w", err))
			}
			slog.Warn("zone: party member left out of auto-resolved fight", "user", uid, "err", err)
			continue
		}

		// A downed member sits it out. They own no close-out claim — and the
		// check runs before the arm, so sitting out doesn't cost them the
		// ability they had readied.
		if !leader {
			if hp, _ := dndHPSnapshot(uid); hp <= 0 {
				continue
			}
		}

		// Auto-resolve builds and fights in one breath, so this seat can arm and
		// apply together. The turn-based path has to split the two — see
		// consumeArmedAbility.
		armed := ""
		if c, cerr := LoadDnDCharacter(uid); cerr == nil && c != nil {
			trySimAutoArm(c)
			armed = consumeArmedAbility(c)
		}
		player, e, dndChar, err := p.buildZoneCombatants(uid, monster, tier, dmMood, armed)
		if err != nil {
			if leader {
				return bail(err)
			}
			slog.Warn("zone: party member left out of auto-resolved fight", "user", uid, "err", err)
			continue
		}
		if leader {
			enemy = e
		}

		// Pre-combat one-shots that the turn-based path does NOT share: a queued
		// spell and the panic-heal consumable trigger. Both mutate this seat's
		// Combatant once, before the fight runs. The queued spell can also debuff
		// the shared enemy, so every seat's cast lands on the one stat block.
		applyPendingCast(uid, dndChar, &player.Stats, &player.Mods, &enemy.Stats)
		setupAutoHealFromInventory(p.loadConsumableInventory(uid), &player.Mods)

		players = append(players, player)
		builds = append(builds, seatBuild{
			uid: uid, dndChar: dndChar, advChar: advChar, equip: equip, mods: player.Mods,
		})
		seated = append(seated, uid)
	}

	res := simulateParty(players, enemy, phases)
	dumpCombatEventsIfDebug(
		fmt.Sprintf("zone:%s vs %s (%d seat(s))", monster.ID, players[0].Name, len(players)),
		res.Seats[0])

	for i, b := range builds {
		seatRes := res.Seats[i]

		// Remove the actual heal items consumed during combat (one inventory item
		// per heal_item event this seat fired). Cheapest-tier first.
		consumeFiredHealingItems(b.uid, countHealEventsFired(seatRes))

		// Misty condition repair (post-combat, same 20% chance as the arena and
		// encounter paths in combat_bridge.go). Gourmet food keeps her gear in
		// shape on long expeditions, not just in the arena.
		if b.advChar.MistyBuffExpires != nil && time.Now().UTC().Before(*b.advChar.MistyBuffExpires) {
			if rand.Float64() < 0.20 {
				npcRepairMostDamaged(b.uid, b.equip, 5)
			}
		}

		persistDnDHPAfterCombat(b.uid, seatRes.PlayerEndHP)
		p.postCombatBookkeeping(b.uid, b.dndChar, b.mods.BerserkerRage, seatRes, b.mods)

		if xp := zoneCombatXP(seatRes, monster.CR, tier); xp > 0 {
			if _, err := p.grantDnDXP(b.uid, xp); err != nil {
				slog.Error("dnd: grantDnDXP zone", "user", b.uid, "err", err)
			}
		}
	}

	return res, seated, nil
}

// zoneCombatRoster is the roster a zone encounter seats for this walker: their
// expedition's party if they lead one, otherwise just them. Downed members are
// filtered by runZoneCombatRoster, not here — this answers "who is on the
// expedition", not "who can stand up".
//
// It must not touch getActiveZoneRun: that lookup carries the §4.3 idle reap,
// and the callers are mid-walk on the run it would reap. fightRoster observes
// the same rule, for the same reason.
func zoneCombatRoster(walker id.UserID) []id.UserID { return fightRoster(walker) }

// closeOutZoneWin hands loot to every seat still standing and the hospital to
// every seat that isn't. It returns the leader's own loot line — the only one
// that belongs in the leader's room narration — and who went down.
//
// The one asymmetry is deliberate. A *solo* player can win at 0 HP: a retaliate
// aura kills the swinger on the same blow that drops the enemy, and
// resolvePlayerAttack returns before enemyDown is consumed. Today that player is
// not marked dead, and P6e is not the place to change it — it would move the
// sim's outcome labels. A *party* marks its downed seats dead on a win, which is
// exactly what the turn-engine close-out has always done (finishPartyWin).
func (p *AdventurePlugin) closeOutZoneWin(
	res PartyCombatResult,
	seated []id.UserID,
	zone ZoneDefinition,
	monster DnDMonsterTemplate,
	isBoss, elite bool,
	deathSource string,
) (leaderDrop string, downed []id.UserID) {
	party := len(seated) > 1
	for i, uid := range seated {
		if res.Seats[i].PlayerEndHP > 0 {
			drop := p.dropZoneLoot(uid, zone.ID, monster, isBoss, elite)
			if i == 0 {
				leaderDrop = drop
			}
			continue
		}
		// Down when the enemy fell. No loot — they never saw it drop.
		downed = append(downed, uid)
		if party {
			markAdventureDead(uid, deathSource, zone.Display)
		}
	}
	return leaderDrop, downed
}

// closeOutZoneLoss marks every seat the fight actually killed. A loss is not the
// same as a death: the engine's timeout is a retreat ("ran out the clock"), and
// it leaves HP where it was. So death is read per seat off HP, never off the
// fight's terminal status — which for one seat is exactly the old
// `if !result.TimedOut` rule, since a solo player at 0 HP has already ended the
// fight.
func closeOutZoneLoss(res PartyCombatResult, seated []id.UserID, zone ZoneDefinition, deathSource string) (killed []id.UserID) {
	for i, uid := range seated {
		if res.Seats[i].PlayerEndHP <= 0 {
			markAdventureDead(uid, deathSource, zone.Display)
			killed = append(killed, uid)
		}
	}
	return killed
}

// partyCasualtyLine names the members a fight put on the floor, for the leader's
// narration. Empty for a solo walker, and empty when everyone walked away.
func partyCasualtyLine(downed []id.UserID) string {
	if len(downed) == 0 {
		return ""
	}
	names := make([]string, 0, len(downed))
	for _, uid := range downed {
		name, _ := loadDisplayName(uid)
		if name == "" {
			name = uid.Localpart()
		}
		names = append(names, "**"+name+"**")
	}
	return "💀 " + joinNames(names) + " went down."
}
