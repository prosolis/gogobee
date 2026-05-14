package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

// grantCombatAchievements checks combat results for achievement-worthy moments.
func (p *AdventurePlugin) grantCombatAchievements(userID id.UserID, result CombatResult) {
	if p.achievements == nil {
		return
	}
	if result.PlayerWon && result.NearDeath {
		p.achievements.GrantAchievement(userID, "combat_near_death")
	}
	if result.SniperKilled {
		p.achievements.GrantAchievement(userID, "combat_sniper_kill")
	}
	if result.MistyHealed {
		p.achievements.GrantAchievement(userID, "combat_misty_clutch")
	}
	for _, ev := range result.Events {
		if ev.Action == "pet_whiff" {
			p.achievements.GrantAchievement(userID, "combat_pet_save")
			break
		}
	}
	if checkDeathSaveEvent(result.Events) {
		p.achievements.GrantAchievement(userID, "combat_death_save")
	}
	for _, ev := range result.Events {
		if ev.Action == "use_consumable" {
			p.achievements.GrantAchievement(userID, "combat_consumable_used")
			break
		}
	}
}

// runDungeonCombat executes dungeon combat using the new simulation engine.
func (p *AdventurePlugin) runDungeonCombat(
	userID id.UserID,
	char *AdventureCharacter,
	equip map[EquipmentSlot]*AdvEquipment,
	loc *AdvLocation,
	bonuses *AdvBonusSummary,
	inPenaltyZone bool,
) CombatResult {
	chatLvl := p.chatLevel(userID)
	hasGrudge := char.GrudgeLocation == loc.Name

	playerStats, playerMods := DerivePlayerStats(char, equip, bonuses, chatLvl, char.CurrentStreak, hasGrudge)

	// Penalty zone Attack/Defense reduction (mirrors +5% death, -15% success
	// from old system). The MaxHP penalty is applied below, after
	// applyDnDPlayerLayer reseats MaxHP onto the dnd HP scale.
	if inPenaltyZone {
		playerStats.Attack = int(float64(playerStats.Attack) * 0.85)
		playerStats.Defense = int(float64(playerStats.Defense) * 0.85)
	}

	// Load consumables from inventory and auto-select
	consumables := p.loadConsumableInventory(userID)
	enemyStats, enemyMods := DeriveDungeonMonsterStats(loc)

	// All combat is D&D. Auto-migrate if needed.
	dndChar, _, err := ensureDnDCharacterForCombat(userID, char)
	if err != nil {
		slog.Error("dnd: ensureDnDCharacterForCombat (dungeon) failed", "user", userID, "err", err)
		return CombatResult{}
	}
	applyDnDPlayerLayer(&playerStats, dndChar)
	applyDnDEquipmentLayer(&playerStats, dndChar, equip)
	applyDnDHPScaling(&playerStats, dndChar)
	if inPenaltyZone {
		playerStats.MaxHP = int(float64(playerStats.MaxHP) * 0.90)
		if playerStats.StartHP > 0 {
			playerStats.StartHP = int(float64(playerStats.StartHP) * 0.90)
		}
	}
	applyClassPassives(&playerStats, &playerMods, dndChar)
	applyRacePassives(&playerStats, &playerMods, dndChar)
	applySubclassPassives(&playerStats, &playerMods, dndChar)
	if firedName, fired := applyArmedAbility(dndChar, &playerMods); fired {
		slog.Info("dnd: armed ability fired", "user", userID, "ability", firedName)
	}
	applyDnDDungeonMonsterLayer(&enemyStats, loc.Tier)
	applyPendingCast(userID, dndChar, &playerStats, &playerMods, &enemyStats)

	// Auto-consumable: panic heal only (see dnd_boss_consumables.go).
	setupAutoHealFromInventory(consumables, &playerMods)

	// Dungeon monsters T3+ can have abilities
	var ability *MonsterAbility
	switch {
	case loc.Tier >= 5:
		ability = &MonsterAbility{Name: "Abyssal Wrath", Phase: "any", ProcChance: 0.30, Effect: "enrage"}
	case loc.Tier == 4:
		ability = &MonsterAbility{Name: "Dark Curse", Phase: "clash", ProcChance: 0.25, Effect: "poison"}
	case loc.Tier == 3:
		ability = &MonsterAbility{Name: "Stone Skin", Phase: "opening", ProcChance: 0.20, Effect: "armor_break"}
	}

	displayName, _ := loadDisplayName(userID)
	player := Combatant{
		Name:     displayName,
		Stats:    playerStats,
		Mods:     playerMods,
		IsPlayer: true,
	}
	enemy := Combatant{
		Name:    loc.Denizens,
		Stats:   enemyStats,
		Mods:    enemyMods,
		Ability: ability,
	}

	result := SimulateCombat(player, enemy, dungeonCombatPhases)
	dumpCombatEventsIfDebug(fmt.Sprintf("dungeon:%s vs %s", loc.Name, displayName), result)

	// Remove the actual heal items consumed during combat. No more
	// burning Coal Bombs / Ink Vials on chump fights — those stay put
	// until a player-driven use command lands.
	consumeFiredHealingItems(userID, countHealEventsFired(result))

	p.grantCombatAchievements(userID, result)

	persistDnDHPAfterCombat(userID, result.PlayerEndHP)
	if err := persistDnDPostCombatSubclass(dndChar, playerMods.BerserkerRage, result, playerMods); err != nil {
		slog.Error("dnd: post-combat subclass persist (dungeon)", "user", userID, "err", err)
	}

	if xp := dungeonCombatXP(result, loc.Tier); xp > 0 {
		if _, err := p.grantDnDXP(userID, xp); err != nil {
			slog.Error("dnd: grantDnDXP dungeon", "user", userID, "err", err)
		}
	}
	_ = dndChar

	return result
}

// loadCombatBonuses computes the AdvBonusSummary for combat stat derivation.
func (p *AdventurePlugin) loadCombatBonuses(userID id.UserID, char *AdventureCharacter) *AdvBonusSummary {
	treasures, _ := loadAdvTreasureBonuses(userID)
	buffs, _ := loadAdvActiveBuffs(userID)
	hasGrudge := char.GrudgeLocation != ""
	return computeAdvBonuses(treasures, buffs, char.CurrentStreak, hasGrudge)
}

// loadConsumableInventory scans inventory for items matching consumable definitions.
// If the player has sufficient foraging level, auto-crafts consumables from ingredients first.
func (p *AdventurePlugin) loadConsumableInventory(userID id.UserID) []ConsumableItem {
	items, err := loadAdvInventory(userID)
	if err != nil {
		return nil
	}

	// Auto-craft if player has foraging level 10+
	char, err := loadAdvCharacter(userID)
	if err == nil && char.ForagingSkill >= 10 {
		craftResults, updatedItems := autoCraftConsumables(userID, items, char.ForagingSkill)
		if len(craftResults) > 0 {
			items = updatedItems
			for _, cr := range craftResults {
				if cr.Success {
					slog.Info("crafting: auto-crafted", "user", userID, "item", cr.Recipe.Result)
					if p.achievements != nil {
						p.achievements.GrantAchievement(userID, "adv_first_craft")
						if cr.Recipe.Tier >= 5 {
							p.achievements.GrantAchievement(userID, "adv_craft_t5")
						}
					}
				} else {
					slog.Info("crafting: failed", "user", userID, "item", cr.Recipe.Result)
				}
			}
			// Store results for narrative rendering
			p.storeCraftResults(userID, craftResults)
		}
	}

	var consumables []ConsumableItem
	for _, item := range items {
		if item.Type != "consumable" {
			continue
		}
		def := consumableDefByName(item.Name)
		if def == nil {
			continue
		}
		consumables = append(consumables, ConsumableItem{
			InventoryID: item.ID,
			Def:         def,
		})
	}
	return consumables
}

func (p *AdventurePlugin) storeCraftResults(userID id.UserID, results []CraftResult) {
	p.craftResults.Store(string(userID), results)
}

func (p *AdventurePlugin) popCraftResults(userID id.UserID) []CraftResult {
	val, ok := p.craftResults.LoadAndDelete(string(userID))
	if !ok {
		return nil
	}
	return val.([]CraftResult)
}

// prependCraftNarrative adds crafting results to the first combat phase message.
func (p *AdventurePlugin) prependCraftNarrative(userID id.UserID, messages []string) []string {
	results := p.popCraftResults(userID)
	if len(results) == 0 || len(messages) == 0 {
		return messages
	}

	var lines []string
	for _, cr := range results {
		if cr.Success {
			lines = append(lines, fmt.Sprintf("🧪 _Crafted **%s** from foraged ingredients._", cr.Recipe.Result))
		} else {
			lines = append(lines, fmt.Sprintf("💨 _Failed to craft %s — ingredients lost._", cr.Recipe.Result))
		}
	}

	prefix := strings.Join(lines, "\n") + "\n\n"
	messages[0] = prefix + messages[0]
	return messages
}

// sendCombatMessages sends phase messages with delays, then a final message.
// Runs in a goroutine so it doesn't block the message handler.
// Returns a channel that is closed when all messages have been sent.
func (p *AdventurePlugin) sendCombatMessages(userID id.UserID, phaseMessages []string, finalMessage string) <-chan struct{} {
	return p.sendCombatMessagesWithDelay(userID, phaseMessages, finalMessage, 5, 4) // 5–8s
}

// sendZoneCombatMessages is the zone/expedition variant — same staging as
// arena, but tighter pacing (2–3s) so dungeon advances feel snappier.
func (p *AdventurePlugin) sendZoneCombatMessages(userID id.UserID, phaseMessages []string, finalMessage string) <-chan struct{} {
	return p.sendCombatMessagesWithDelay(userID, phaseMessages, finalMessage, 2, 2) // 2–3s
}

// sendCombatMessagesWithDelay is the shared streamer. base/jitter define
// the per-message delay window in seconds: delay = base + rand.IntN(jitter).
func (p *AdventurePlugin) sendCombatMessagesWithDelay(userID id.UserID, phaseMessages []string, finalMessage string, base, jitter int) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, msg := range phaseMessages {
			_ = p.SendDM(userID, msg)
			delay := base
			if jitter > 0 {
				delay += rand.IntN(jitter)
			}
			time.Sleep(time.Duration(delay) * time.Second)
		}
		_ = p.SendDM(userID, finalMessage)
	}()
	return done
}

// XPBonusParams holds inputs for the shared XP bonus pipeline.
type XPBonusParams struct {
	BaseXP        int
	NearDeath     bool
	BonusMult     float64 // from AdvBonusSummary.XPMultiplier (percentage, e.g. 15 = +15%)
	Ironclad      bool
	OverlevelMult float64 // 1.0 = no penalty
}

// XPResult holds the final XP and a human-readable breakdown of bonuses applied.
type XPResult struct {
	Total     int
	Breakdown string // e.g. "+15% near-death, +5% ironclad" — empty if no bonuses
}

// applyXPBonuses applies near-death, bonus multiplier, ironclad, and overlevel
// penalties to a base XP value. Used by both dungeon and adventure paths.
func applyXPBonuses(p XPBonusParams) XPResult {
	xp := p.BaseXP
	var parts []string
	if p.NearDeath {
		xp = int(float64(xp) * 1.15)
		parts = append(parts, "+15% near-death")
	}
	if p.BonusMult != 0 {
		xp = int(float64(xp) * (1 + p.BonusMult/100))
		parts = append(parts, fmt.Sprintf("+%.0f%% bonus", p.BonusMult))
	}
	if p.Ironclad {
		xp = int(float64(xp) * 1.05)
		parts = append(parts, "+5% ironclad")
	}
	if p.OverlevelMult < 1.0 {
		xp = max(1, int(float64(xp)*p.OverlevelMult))
		penalty := int((1.0 - p.OverlevelMult) * 100)
		parts = append(parts, fmt.Sprintf("-%d%% overlevel", penalty))
	}
	breakdown := ""
	if len(parts) > 0 {
		breakdown = strings.Join(parts, ", ")
	}
	return XPResult{Total: xp, Breakdown: breakdown}
}

// DeathTransitionParams holds inputs for the shared death state machine.
type DeathTransitionParams struct {
	Char           *AdventureCharacter
	Equip          map[EquipmentSlot]*AdvEquipment
	Pet            PetState // pet state for ditch-recovery roll; zero value disables
	ChatLevel      int
	Location       string // set as GrudgeLocation; empty = don't set
	Source         string // death source: "adventure" | "arena" — recorded on Kill()
	DeathLocation  string // human-readable death location for the daily report; falls back to Location
	AllowPardon    bool   // chat level pardon (adventure only)
	AllowSovereign bool   // probability-band Sovereign reprieve (non-engine path)
	EngineSaved    bool   // combat engine used Sovereign death save
}

// DeathTransitionResult describes what happened during the death transition.
type DeathTransitionResult struct {
	Pardoned     bool
	Reprieved    bool // Sovereign reprieve (non-engine)
	PetRecovered bool
	Died         bool
}

// transitionDeath runs the shared death state machine: pardon → engine cooldown →
// Sovereign reprieve → kill + pet ditch recovery.
func transitionDeath(p DeathTransitionParams) DeathTransitionResult {
	var r DeathTransitionResult

	if p.AllowPardon && p.ChatLevel >= 20 && p.Char.PardonAvailable() && rand.Float64() < 0.33 {
		r.Pardoned = true
		now := time.Now().UTC()
		p.Char.LastPardonUsed = &now
		if p.Location != "" {
			p.Char.GrudgeLocation = p.Location
		}
		return r
	}

	if p.EngineSaved {
		now := time.Now().UTC()
		p.Char.DeathReprieveLast = &now
	}

	if p.AllowSovereign && advEquippedArenaSets(p.Equip)["sovereign"] && p.Char.DeathReprieveAvailable() {
		r.Reprieved = true
		now := time.Now().UTC()
		p.Char.DeathReprieveLast = &now
		if p.Location != "" {
			p.Char.GrudgeLocation = p.Location
		}
		for _, slot := range allSlots {
			if eq, ok := p.Equip[slot]; ok {
				eq.Condition = 1
			}
		}
		return r
	}

	deathLoc := p.DeathLocation
	if deathLoc == "" {
		deathLoc = p.Location
	}
	p.Char.Kill(p.Source, deathLoc)
	if p.Location != "" {
		p.Char.GrudgeLocation = p.Location
	}
	r.Died = true

	if petRollDitchRecovery(p.Pet) && p.Char.DeadUntil != nil {
		reduced := time.Now().UTC().Add(petDitchRecoveryTime(p.Pet.Level))
		p.Char.DeadUntil = &reduced
		r.PetRecovered = true
	}

	return r
}

// checkDeathSaveEvent scans combat events for a Sovereign death save.
func checkDeathSaveEvent(events []CombatEvent) bool {
	for _, ev := range events {
		if ev.Action == "death_save" {
			return true
		}
	}
	return false
}

// injectConsumableEvents prepends use_consumable events so the narrative shows which items were consumed.
// If items were available but skipped (trivial threat), a skip event is injected instead.
//
// Uses PlayerEntryHP / EnemyEntryHP — the actual HP each side entered combat
// with — so the pre-combat HP line reflects wounded carry-over honestly. Was
// using PlayerStartHP (= MaxHP), which made wounded entries display "47/47"
// and made any HP loss in opening look like the player took silent damage.
func injectConsumableEvents(result CombatResult, selected []ConsumableItem, inventorySize int) CombatResult {
	entryHP := result.PlayerEntryHP
	if entryHP == 0 {
		entryHP = result.PlayerStartHP
	}
	enemyEntryHP := result.EnemyEntryHP
	if enemyEntryHP == 0 {
		enemyEntryHP = result.EnemyStartHP
	}
	if len(selected) == 0 {
		if inventorySize > 0 {
			result.Events = append([]CombatEvent{{
				Round: 0, Phase: "pre_combat", Actor: "consumable", Action: "consumable_skip",
				PlayerHP: entryHP, EnemyHP: enemyEntryHP,
			}}, result.Events...)
		}
		return result
	}
	var events []CombatEvent
	for _, c := range selected {
		events = append(events, CombatEvent{
			Round: 0, Phase: "pre_combat", Actor: "consumable", Action: "use_consumable",
			PlayerHP: entryHP, EnemyHP: enemyEntryHP,
			Desc: c.Def.Name,
		})
	}
	result.Events = append(events, result.Events...)
	return result
}

// combatResultToOutcome maps a CombatResult to an AdvOutcomeType for dungeon integration.
func combatResultToOutcome(result CombatResult) AdvOutcomeType {
	if !result.PlayerWon {
		return AdvOutcomeDeath
	}
	// Won with >85% HP remaining → exceptional
	remainingPct := float64(result.PlayerEndHP) / float64(max(1, result.PlayerStartHP))
	if remainingPct > 0.85 {
		return AdvOutcomeExceptional
	}
	return AdvOutcomeSuccess
}

// combatDegradation derives equipment damage from actual combat events.
func combatDegradation(result CombatResult, equip map[EquipmentSlot]*AdvEquipment) map[EquipmentSlot]int {
	damage := make(map[EquipmentSlot]int)

	if !result.PlayerWon {
		// Death: heavy degradation
		for _, slot := range allSlots {
			damage[slot] = 20
		}
		damage[SlotWeapon] = 30
		damage[SlotArmor] = 30
		return applyDegradationModifiers(damage, equip)
	}

	// Derive from events: each enemy hit on player → armor/helmet wear
	// Each player hit → weapon wear, environmental → boots
	for _, ev := range result.Events {
		switch {
		case ev.Actor == "enemy" && (ev.Action == "hit" || ev.Action == "crit"):
			damage[SlotArmor] += 1 + rand.IntN(3)
			damage[SlotHelmet] += 1 + rand.IntN(2)
		case ev.Actor == "enemy" && ev.Action == "cleave":
			damage[SlotArmor] += 2 + rand.IntN(3)
		case ev.Actor == "enemy" && (ev.Action == "bonus_damage" || ev.Action == "aoe" || ev.Action == "execute"):
			damage[SlotArmor] += 1 + rand.IntN(2)
		case ev.Actor == "enemy" && ev.Action == "lifesteal":
			damage[SlotArmor] += 1 + rand.IntN(2)
		case ev.Actor == "player" && (ev.Action == "hit" || ev.Action == "crit"):
			damage[SlotWeapon] += 1
		case ev.Actor == "environment":
			damage[SlotBoots] += 1 + rand.IntN(2)
		case ev.Action == "armor_break":
			damage[SlotArmor] += 3
		}
	}

	return applyDegradationModifiers(damage, equip)
}

func applyDegradationModifiers(damage map[EquipmentSlot]int, equip map[EquipmentSlot]*AdvEquipment) map[EquipmentSlot]int {
	tempered := advEquippedArenaSets(equip)["tempered"]
	for slot, dmg := range damage {
		eq, ok := equip[slot]
		if !ok {
			continue
		}
		if tempered {
			dmg = int(float64(dmg) * 0.75)
		}
		if eq.ActionsUsed >= 20 {
			dmg = int(float64(dmg) * 0.8)
		}
		if dmg < 0 {
			dmg = 0
		}
		damage[slot] = dmg
		eq.Condition -= dmg
		if eq.Condition < 0 {
			eq.Condition = 0
		}
	}
	return damage
}

// resolveDungeonAction runs a dungeon encounter through the combat engine
// and returns an AdvActionResult compatible with the existing adventure flow.
func (p *AdventurePlugin) resolveDungeonAction(
	char *AdventureCharacter,
	equip map[EquipmentSlot]*AdvEquipment,
	loc *AdvLocation,
	bonuses *AdvBonusSummary,
	inPenaltyZone bool,
) *AdvActionResult {
	result := &AdvActionResult{
		Location: loc,
		XPSkill:  "combat",
	}

	combat := p.runDungeonCombat(char.UserID, char, equip, loc, bonuses, inPenaltyZone)
	result.CombatLog = &combat
	result.Outcome = combatResultToOutcome(combat)

	// Misty condition repair (post-combat, same as arena)
	now := time.Now().UTC()
	if char.MistyBuffExpires != nil && now.Before(*char.MistyBuffExpires) {
		if rand.Float64() < 0.20 {
			npcRepairMostDamaged(char.UserID, equip, 5)
		}
	}
	result.NearDeath = combat.NearDeath

	// Overlevel penalty
	skillLevel := advEffectiveSkill(char, loc.Activity, bonuses)
	overlevelMult := advOverlevelMultiplier(skillLevel, loc)

	// Loot on success/exceptional
	if result.Outcome == AdvOutcomeSuccess || result.Outcome == AdvOutcomeExceptional {
		result.LootItems = generateAdvLoot(loc, result.Outcome == AdvOutcomeExceptional, bonuses.LootQuality)
		if overlevelMult < 1.0 {
			for i := range result.LootItems {
				result.LootItems[i].Value = max(1, int64(float64(result.LootItems[i].Value)*overlevelMult))
			}
		}
		for _, item := range result.LootItems {
			result.TotalLootValue += item.Value
		}
	}

	// XP calculation
	xp := advXPForOutcome(loc.Activity, loc.Tier, result.Outcome)
	if result.Outcome == AdvOutcomeSuccess || result.Outcome == AdvOutcomeExceptional {
		xp = advXPTable[loc.Activity][loc.Tier].Success
		if result.Outcome == AdvOutcomeExceptional {
			xp = advXPTable[loc.Activity][loc.Tier].Exceptional
		}
	}
	xpResult := applyXPBonuses(XPBonusParams{
		BaseXP:        xp,
		NearDeath:     result.NearDeath,
		BonusMult:     bonuses.XPMultiplier,
		Ironclad:      advEquippedArenaSets(equip)["ironclad"],
		OverlevelMult: overlevelMult,
	})
	result.XPGained = xpResult.Total
	result.XPBreakdown = xpResult.Breakdown

	// Equipment degradation from combat events
	result.EquipDamage = combatDegradation(combat, equip)
	result.EquipBroken = advCheckBrokenEquipment(equip)

	// Increment actions_used for equipment mastery
	for _, eq := range equip {
		eq.ActionsUsed++
	}

	return result
}

