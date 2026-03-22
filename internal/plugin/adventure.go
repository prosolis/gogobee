package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// ── Plugin ───────────────────────────────────────────────────────────────────

type AdventurePlugin struct {
	Base
	euro        *EuroPlugin
	mu          sync.Mutex
	dmToPlayer  map[id.RoomID]id.UserID
	pending     sync.Map // userID string -> *advPendingInteraction
	userLocks   sync.Map // userID string -> *sync.Mutex
	morningHour int
	summaryHour int
}

// advUserLock returns a per-user mutex to prevent concurrent action resolution.
func (p *AdventurePlugin) advUserLock(userID id.UserID) *sync.Mutex {
	val, _ := p.userLocks.LoadOrStore(string(userID), &sync.Mutex{})
	return val.(*sync.Mutex)
}

type advPendingInteraction struct {
	Type      string // "treasure_discard"
	Data      interface{}
	ExpiresAt time.Time
}

type advPendingTreasureDiscard struct {
	NewTreasure *AdvTreasureDef
	Existing    []AdvTreasureDef
}

func NewAdventurePlugin(client *mautrix.Client, euro *EuroPlugin) *AdventurePlugin {
	return &AdventurePlugin{
		Base:        NewBase(client),
		euro:        euro,
		dmToPlayer:  make(map[id.RoomID]id.UserID),
		morningHour: envInt("ADVENTURE_MORNING_HOUR", 8),
		summaryHour: envInt("ADVENTURE_SUMMARY_HOUR", 20),
	}
}

func (p *AdventurePlugin) Name() string { return "adventure" }

func (p *AdventurePlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "adventure", Description: "Daily adventure game — dungeon, mine, forage, or rest", Usage: "!adventure", Category: "Games"},
	}
}

func (p *AdventurePlugin) Init() error {
	// Rehydrate DM room mappings for existing characters
	chars, err := loadAllAdvCharacters()
	if err != nil {
		slog.Warn("adventure: no characters to rehydrate", "err", err)
	} else {
		for _, c := range chars {
			p.registerDMRoom(c.UserID)
		}
		slog.Info("adventure: rehydrated DM rooms", "count", len(chars))
	}

	// Start schedulers
	go p.morningTicker()
	go p.summaryTicker()
	go p.midnightTicker()

	return nil
}

func (p *AdventurePlugin) OnReaction(_ ReactionContext) error { return nil }

// ── Message Dispatch ─────────────────────────────────────────────────────────

func (p *AdventurePlugin) OnMessage(ctx MessageContext) error {
	// 1. Check if this is a DM reply from a registered player
	p.mu.Lock()
	playerID, isDM := p.dmToPlayer[ctx.RoomID]
	p.mu.Unlock()

	if isDM && playerID == ctx.Sender {
		return p.handleDMReply(ctx)
	}

	// 2. Command dispatch
	if !p.IsCommand(ctx.Body, "adventure") {
		return nil
	}

	args := strings.TrimSpace(p.GetArgs(ctx.Body, "adventure"))
	lower := strings.ToLower(args)

	switch {
	case args == "" || lower == "menu":
		return p.handleMenu(ctx)
	case lower == "status":
		return p.handleStatus(ctx)
	case strings.HasPrefix(lower, "sell "):
		return p.handleSellCmd(ctx, strings.TrimSpace(args[5:]))
	case lower == "shop":
		return p.handleShopCmd(ctx)
	case strings.HasPrefix(lower, "buy "):
		return p.handleBuyCmd(ctx, strings.TrimSpace(args[4:]))
	case lower == "inventory" || lower == "inv":
		return p.handleInventoryCmd(ctx)
	case lower == "leaderboard" || lower == "lb":
		return p.handleLeaderboard(ctx)
	case strings.HasPrefix(lower, "revive "):
		return p.handleAdminRevive(ctx, strings.TrimSpace(args[7:]))
	case lower == "summary":
		return p.handleAdminSummary(ctx)
	}

	return nil
}

// ── Command Handlers ─────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleMenu(ctx MessageContext) error {
	char, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character.")
	}

	if !char.Alive {
		text := renderAdvDeathStatusDM(char)
		return p.SendDM(ctx.Sender, text)
	}

	if char.ActionTakenToday {
		return p.SendDM(ctx.Sender, "You've already taken your action today. Tomorrow awaits. Try to survive it.")
	}

	treasures, _ := loadAdvTreasureBonuses(char.UserID)
	buffs, _ := loadAdvActiveBuffs(char.UserID)
	bonuses := computeAdvBonuses(treasures, buffs, char.CurrentStreak, false)
	balance := p.euro.GetBalance(char.UserID)

	text := renderAdvMorningDM(char, equip, balance, bonuses)
	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleStatus(ctx MessageContext) error {
	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No adventurer found. Type `!adventure` to create one.")
	}

	equip, _ := loadAdvEquipment(ctx.Sender)
	items, _ := loadAdvInventory(ctx.Sender)
	treasures, _ := loadAdvTreasureBonuses(ctx.Sender)
	balance := p.euro.GetBalance(ctx.Sender)

	text := renderAdvCharacterSheet(char, equip, items, treasures, balance)
	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleShopCmd(ctx MessageContext) error {
	_, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character.")
	}

	balance := p.euro.GetBalance(ctx.Sender)
	text := advShopListings(equip, balance)
	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleBuyCmd(ctx MessageContext, itemName string) error {
	_, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character.")
	}

	slot, def, found := advFindShopItem(itemName)
	if !found {
		return p.SendDM(ctx.Sender, fmt.Sprintf("No item matching '%s' found in the shop. Type `!adventure shop` to see what's available.", itemName))
	}

	result := p.advBuyEquipment(ctx.Sender, slot, def, equip)
	return p.SendDM(ctx.Sender, result)
}

func (p *AdventurePlugin) handleSellCmd(ctx MessageContext, args string) error {
	p.ensureCharacter(ctx.Sender)

	var result string
	if strings.ToLower(args) == "all" {
		result = p.advSellAll(ctx.Sender)
	} else {
		result = p.advSellItem(ctx.Sender, args)
	}
	return p.SendDM(ctx.Sender, result)
}

func (p *AdventurePlugin) handleInventoryCmd(ctx MessageContext) error {
	p.ensureCharacter(ctx.Sender)
	text := advInventoryDisplay(ctx.Sender)
	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleLeaderboard(ctx MessageContext) error {
	chars, err := loadAllAdvCharacters()
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load leaderboard.")
	}
	text := renderAdvLeaderboard(chars)
	return p.SendReply(ctx.RoomID, ctx.EventID, text)
}

func (p *AdventurePlugin) handleAdminRevive(ctx MessageContext, target string) error {
	if !p.IsAdmin(ctx.Sender) {
		return nil
	}

	// Resolve user
	targetID, found := p.ResolveUser(target, ctx.RoomID)
	if !found {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Could not find that user.")
	}

	char, err := loadAdvCharacter(targetID)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "That user has no adventurer.")
	}

	if char.Alive {
		return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("%s is already alive.", char.DisplayName))
	}

	char.Alive = true
	char.DeadUntil = nil
	if err := saveAdvCharacter(char); err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to revive.")
	}

	p.SendDM(targetID, renderAdvRespawnDM(char))
	return p.SendReply(ctx.RoomID, ctx.EventID, fmt.Sprintf("✅ %s has been revived.", char.DisplayName))
}

func (p *AdventurePlugin) handleAdminSummary(ctx MessageContext) error {
	if !p.IsAdmin(ctx.Sender) {
		return nil
	}
	go p.postDailySummary()
	return p.SendReply(ctx.RoomID, ctx.EventID, "Daily summary will be posted shortly.")
}

// ── DM Reply Handling ────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleDMReply(ctx MessageContext) error {
	body := strings.TrimSpace(ctx.Body)

	// Skip if it looks like a command for another plugin
	if strings.HasPrefix(body, "!") && !strings.HasPrefix(strings.ToLower(body), "!adventure") {
		return nil
	}

	// Strip !adventure prefix if present
	if strings.HasPrefix(strings.ToLower(body), "!adventure") {
		// Re-dispatch as command
		return p.OnMessage(ctx)
	}

	// Check for pending interaction first
	if val, ok := p.pending.Load(string(ctx.Sender)); ok {
		interaction := val.(*advPendingInteraction)
		if time.Now().Before(interaction.ExpiresAt) {
			return p.resolvePendingInteraction(ctx, interaction)
		}
		p.pending.Delete(string(ctx.Sender))
		p.SendDM(ctx.Sender, "Your previous prompt expired. Moving on.")
	}

	// Parse as activity choice
	return p.parseAndResolveChoice(ctx, body)
}

func (p *AdventurePlugin) resolvePendingInteraction(ctx MessageContext, interaction *advPendingInteraction) error {
	p.pending.Delete(string(ctx.Sender))

	switch interaction.Type {
	case "treasure_discard":
		return p.handleTreasureDiscard(ctx, interaction)
	}
	return nil
}

func (p *AdventurePlugin) handleTreasureDiscard(ctx MessageContext, interaction *advPendingInteraction) error {
	data := interaction.Data.(*advPendingTreasureDiscard)
	body := strings.TrimSpace(strings.ToLower(ctx.Body))

	if body == "keep" {
		return p.SendDM(ctx.Sender, fmt.Sprintf("You left the %s behind. It will stay where you found it, judging you, forever.", data.NewTreasure.Name))
	}

	choice, err := strconv.Atoi(body)
	if err != nil || choice < 1 || choice > len(data.Existing) {
		return p.SendDM(ctx.Sender, "Reply with 1, 2, or 3 to discard, or `keep` to leave the new treasure behind.")
	}

	// Discard the chosen treasure
	discarded := data.Existing[choice-1]
	if err := advDiscardTreasure(ctx.Sender, discarded.Key); err != nil {
		return p.SendDM(ctx.Sender, "Failed to discard treasure. Try again.")
	}

	// Save the new treasure
	if err := advSaveTreasure(ctx.Sender, data.NewTreasure); err != nil {
		return p.SendDM(ctx.Sender, "Failed to save new treasure. Something went wrong.")
	}

	return p.SendDM(ctx.Sender, fmt.Sprintf("You discarded **%s** and kept **%s**.\n\n_%s_",
		discarded.Name, data.NewTreasure.Name, data.NewTreasure.InventoryDesc))
}

// ── Activity Choice Parsing ──────────────────────────────────────────────────

func (p *AdventurePlugin) parseAndResolveChoice(ctx MessageContext, body string) error {
	// Acquire per-user lock to prevent double actions from concurrent DM replies
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return nil // not a registered player
	}

	if !char.Alive {
		return p.SendDM(ctx.Sender, renderAdvDeathStatusDM(char))
	}

	if char.ActionTakenToday {
		return p.SendDM(ctx.Sender, "You've already taken your action today. Rest now. Try again tomorrow.")
	}

	lower := strings.ToLower(body)

	// Parse "5" or "rest"
	if lower == "5" || lower == "rest" {
		return p.resolveRest(ctx, char)
	}

	// Parse "4" or "shop"
	if lower == "4" || lower == "shop" {
		equip, _ := loadAdvEquipment(ctx.Sender)
		balance := p.euro.GetBalance(ctx.Sender)
		return p.SendDM(ctx.Sender, advShopListings(equip, balance))
	}

	// Parse activity + location
	activity, loc := p.parseActivityLocation(lower, char)
	if loc == nil {
		return p.SendDM(ctx.Sender, "I didn't understand that. Reply with a number and location, e.g: `1 Soggy Cellar`, or just `1` for the first available.")
	}

	return p.resolveActivity(ctx, char, activity, loc)
}

func (p *AdventurePlugin) parseActivityLocation(input string, char *AdventureCharacter) (AdvActivityType, *AdvLocation) {
	parts := strings.SplitN(input, " ", 2)
	if len(parts) == 0 {
		return "", nil
	}

	first := parts[0]
	rest := ""
	if len(parts) > 1 {
		rest = strings.TrimSpace(parts[1])
	}

	var activity AdvActivityType

	// Parse activity from number or word
	switch first {
	case "1", "dungeon", "d":
		activity = AdvActivityDungeon
	case "2", "mine", "m":
		activity = AdvActivityMining
	case "3", "forage", "f", "forest":
		activity = AdvActivityForaging
	default:
		// Try matching location name directly
		for _, act := range []AdvActivityType{AdvActivityDungeon, AdvActivityMining, AdvActivityForaging} {
			if loc := findAdvLocation(act, input); loc != nil {
				return act, loc
			}
		}
		return "", nil
	}

	// If no location specified, pick first eligible
	if rest == "" {
		equip, _ := loadAdvEquipment(char.UserID)
		treasures, _ := loadAdvTreasureBonuses(char.UserID)
		buffs, _ := loadAdvActiveBuffs(char.UserID)
		bonuses := computeAdvBonuses(treasures, buffs, char.CurrentStreak, false)
		eligible := advEligibleLocations(char, equip, activity, bonuses)
		if len(eligible) == 0 {
			return activity, nil
		}
		return activity, eligible[0].Location
	}

	// Try to parse tier number
	if tier, err := strconv.Atoi(rest); err == nil {
		loc := findAdvLocationByTier(activity, tier)
		if loc != nil {
			return activity, loc
		}
	}

	// Fuzzy match location name
	loc := findAdvLocation(activity, rest)
	return activity, loc
}

// ── Activity Resolution ──────────────────────────────────────────────────────

func (p *AdventurePlugin) resolveActivity(ctx MessageContext, char *AdventureCharacter, activity AdvActivityType, loc *AdvLocation) error {
	equip, err := loadAdvEquipment(char.UserID)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your equipment.")
	}

	treasures, _ := loadAdvTreasureBonuses(char.UserID)
	buffs, _ := loadAdvActiveBuffs(char.UserID)

	// Check grudge
	hasGrudge := char.GrudgeLocation == loc.Name
	bonuses := computeAdvBonuses(treasures, buffs, char.CurrentStreak, hasGrudge)

	// Check eligibility
	eligible, inPenaltyZone := advIsEligible(char, equip, loc, bonuses)
	if !eligible {
		return p.SendDM(ctx.Sender, fmt.Sprintf("You are not eligible for %s. Your level or equipment tier is too low.", loc.Name))
	}

	// Resolve the action
	result := resolveAdvAction(char, equip, loc, bonuses, inPenaltyZone)

	// Select flavor text
	result.FlavorText, result.FlavorKey = p.selectFlavorText(char, result)

	// Apply XP
	switch result.XPSkill {
	case "combat":
		char.CombatXP += result.XPGained
	case "mining":
		char.MiningXP += result.XPGained
	case "foraging":
		char.ForagingXP += result.XPGained
	}

	// Check level up
	result.LeveledUp, result.NewLevel = checkAdvLevelUp(char, result.XPSkill)

	// Handle death
	if result.Outcome == AdvOutcomeDeath {
		char.Alive = false
		deadUntil := time.Now().UTC().Add(24 * time.Hour)
		char.DeadUntil = &deadUntil
		char.GrudgeLocation = loc.Name
	} else if hasGrudge && (result.Outcome == AdvOutcomeSuccess || result.Outcome == AdvOutcomeExceptional) {
		// Clear grudge on successful return
		char.GrudgeLocation = ""
	}

	// Add loot to inventory
	for _, item := range result.LootItems {
		_ = addAdvInventoryItem(char.UserID, item)
	}

	// Party bonus: check if someone else visited the same location today
	if result.Outcome == AdvOutcomeSuccess || result.Outcome == AdvOutcomeExceptional {
		if advCheckPartyBonus(char.UserID, loc.Name) {
			// Apply party bonus: +10% loot value
			partyBonus := int64(float64(result.TotalLootValue) * 0.10)
			if partyBonus > 0 {
				result.TotalLootValue += partyBonus
				// Credit the bonus directly
				p.euro.Credit(char.UserID, float64(partyBonus), "adventure_party_bonus")
			}
		}
	}

	// Mark action taken
	char.ActionTakenToday = true

	// Update streak info
	result.StreakBonus = char.CurrentStreak

	// Save character
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("adventure: failed to save character", "user", char.UserID, "err", err)
		return p.SendDM(ctx.Sender, "Something went wrong saving your progress. Your action was not recorded. Try again.")
	}

	// Save equipment changes
	for _, slot := range allSlots {
		if eq, ok := equip[slot]; ok {
			if err := saveAdvEquipment(char.UserID, eq); err != nil {
				slog.Error("adventure: failed to save equipment", "user", char.UserID, "slot", slot, "err", err)
			}
		}
	}

	// Log activity
	logAdvActivity(char.UserID, string(activity), loc.Name, string(result.Outcome),
		result.TotalLootValue, result.XPGained, result.FlavorKey)

	// Send resolution DM
	text := renderAdvResolutionDM(result, char)
	if err := p.SendDM(ctx.Sender, text); err != nil {
		slog.Error("adventure: failed to send resolution DM", "user", ctx.Sender, "err", err)
	}

	// Check for treasure drop
	if result.Outcome == AdvOutcomeSuccess || result.Outcome == AdvOutcomeExceptional {
		p.checkTreasureDrop(ctx.Sender, char, loc)
	}

	return nil
}

func (p *AdventurePlugin) resolveRest(ctx MessageContext, char *AdventureCharacter) error {
	char.ActionTakenToday = true
	if err := saveAdvCharacter(char); err != nil {
		return p.SendDM(ctx.Sender, "Failed to save. Even resting is broken.")
	}

	logAdvActivity(char.UserID, string(AdvActivityRest), "", "rest", 0, 0, "")

	return p.SendDM(ctx.Sender, fmt.Sprintf(
		"%s, you chose rest. No loot. No XP. No death.\n\n"+
			"You sat in your hovel and stared at the wall and achieved absolutely nothing. "+
			"Tomorrow awaits. It will probably be the same.",
		char.DisplayName))
}

// ── Treasure Drop Check ─────────────────────────────────────────────────────

func (p *AdventurePlugin) checkTreasureDrop(userID id.UserID, char *AdventureCharacter, loc *AdvLocation) {
	drop := rollAdvTreasureDrop(loc.Tier, userID)
	if drop == nil {
		return
	}

	// Check treasure count
	count, err := advCountTreasures(userID)
	if err != nil {
		return
	}

	if count < advMaxTreasures {
		// Directly save
		if err := advSaveTreasure(userID, drop.Def); err != nil {
			slog.Error("adventure: failed to save treasure", "user", userID, "err", err)
			return
		}

		// Send discovery flavor
		p.sendTreasureDiscoveryDM(userID, char, drop.Def, loc)
		return
	}

	// At cap — prompt for discard
	existing, err := advUserTreasures(userID)
	if err != nil || len(existing) == 0 {
		return
	}

	// Set pending interaction
	p.pending.Store(string(userID), &advPendingInteraction{
		Type: "treasure_discard",
		Data: &advPendingTreasureDiscard{
			NewTreasure: drop.Def,
			Existing:    existing,
		},
		ExpiresAt: time.Now().Add(5 * time.Minute),
	})

	text := renderAdvTreasureDiscardPrompt(drop.Def, existing)
	p.SendDM(userID, text)
}

func (p *AdventurePlugin) sendTreasureDiscoveryDM(userID id.UserID, char *AdventureCharacter, def *AdvTreasureDef, loc *AdvLocation) {
	// Pick from discovery pool
	pool, ok := TreasureDiscovery[def.Tier]
	if !ok || len(pool) == 0 {
		return
	}

	text := pool[rand.IntN(len(pool))]
	text = advSubstituteFlavor(text, map[string]string{
		"{treasure_name}": def.Name,
		"{bonus_desc}":    def.InventoryDesc,
		"{location}":      loc.Name,
	})

	p.SendDM(userID, text)

	// Room announcement for tier 5 or special items
	if def.RoomAnnounce != "" {
		gr := gamesRoom()
		if gr != "" {
			announce := advSubstituteFlavor(def.RoomAnnounce, map[string]string{
				"{name}":     char.DisplayName,
				"{location}": loc.Name,
			})
			p.SendMessage(id.RoomID(gr), announce)
		}
	}
}

// ── Flavor Text Selection ────────────────────────────────────────────────────

func (p *AdventurePlugin) selectFlavorText(char *AdventureCharacter, result *AdvActionResult) (string, string) {
	loc := result.Location
	vars := map[string]string{
		"{name}":     char.DisplayName,
		"{location}": loc.Name,
		"{value}":    fmt.Sprintf("%d", result.TotalLootValue),
		"{xp}":       fmt.Sprintf("%d", result.XPGained),
	}

	// Add item names
	if len(result.LootItems) > 0 {
		names := make([]string, len(result.LootItems))
		for i, item := range result.LootItems {
			names[i] = item.Name
		}
		vars["{item}"] = joinAdvItems(names)
		vars["{ore}"] = names[0]
		vars["{item_2}"] = ""
		if len(names) > 1 {
			vars["{item_2}"] = names[1]
		}
	} else {
		vars["{item}"] = ""
		vars["{ore}"] = ""
	}

	// Equipment names for flavor
	equip, _ := loadAdvEquipment(char.UserID)
	if eq, ok := equip[SlotTool]; ok {
		vars["{tool}"] = eq.Name
	}
	if eq, ok := equip[SlotArmor]; ok {
		vars["{armor}"] = eq.Name
	}

	var pool []string
	category := fmt.Sprintf("%s_%s", loc.Activity, result.Outcome)

	switch loc.Activity {
	case AdvActivityDungeon:
		switch result.Outcome {
		case AdvOutcomeDeath:
			if tierPool, ok := DungeonDeath[loc.Tier]; ok {
				pool = tierPool
			}
		case AdvOutcomeEmpty:
			if tierPool, ok := DungeonEmpty[loc.Tier]; ok {
				pool = tierPool
			}
		case AdvOutcomeSuccess:
			if tierPool, ok := DungeonSuccess[loc.Tier]; ok {
				pool = tierPool
			}
		case AdvOutcomeExceptional:
			if tierPool, ok := DungeonExceptional[loc.Tier]; ok {
				pool = tierPool
			}
		}

	case AdvActivityMining:
		switch result.Outcome {
		case AdvOutcomeDeath:
			if tierPool, ok := MiningDeath[loc.Tier]; ok {
				pool = tierPool
			}
		case AdvOutcomeCaveIn:
			pool = MiningCaveIn
		case AdvOutcomeEmpty:
			if tierPool, ok := MiningEmpty[loc.Tier]; ok {
				pool = tierPool
			}
		case AdvOutcomeSuccess, AdvOutcomeExceptional:
			if tierPool, ok := MiningSuccess[loc.Tier]; ok {
				pool = tierPool
			}
		}

	case AdvActivityForaging:
		switch result.Outcome {
		case AdvOutcomeDeath:
			pool = ForagingDeath
		case AdvOutcomeHornets:
			pool = ForagingHornets
		case AdvOutcomeBear:
			pool = ForagingBear
		case AdvOutcomeRiver:
			pool = ForagingRiver
		case AdvOutcomeSuccess, AdvOutcomeExceptional:
			if tierPool, ok := ForagingGoodHaul[loc.Tier]; ok {
				pool = tierPool
			}
		}
	}

	if len(pool) == 0 {
		return fmt.Sprintf("You went to %s. Things happened.", loc.Name), ""
	}

	text, idx := advPickFlavor(pool, char.UserID, category)
	key := fmt.Sprintf("%s_%d", category, idx)
	return advSubstituteFlavor(text, vars), key
}

// ── Character Ensurance ──────────────────────────────────────────────────────

func (p *AdventurePlugin) ensureCharacter(userID id.UserID) (*AdventureCharacter, map[EquipmentSlot]*AdvEquipment, error) {
	char, err := loadAdvCharacter(userID)
	if err != nil {
		// Auto-create
		displayName := p.displayName(userID)
		if err := createAdvCharacter(userID, displayName); err != nil {
			return nil, nil, err
		}
		char, err = loadAdvCharacter(userID)
		if err != nil {
			return nil, nil, err
		}

		// Register DM room
		p.registerDMRoom(userID)

		// Send onboarding
		text := renderAdvOnboardingDM(char)
		p.SendDM(userID, text)
	}

	equip, err := loadAdvEquipment(userID)
	if err != nil {
		return char, nil, err
	}

	return char, equip, nil
}

func (p *AdventurePlugin) displayName(userID id.UserID) string {
	resp, err := p.Client.GetDisplayName(context.Background(), userID)
	if err != nil || resp.DisplayName == "" {
		// Fallback to localpart
		s := string(userID)
		if idx := strings.Index(s, ":"); idx > 0 {
			s = s[1:idx]
		}
		return s
	}
	return resp.DisplayName
}
