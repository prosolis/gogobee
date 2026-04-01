package plugin

import (
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
	euro         *EuroPlugin
	achievements *AchievementsPlugin
	mu           sync.Mutex
	dmToPlayer     map[id.RoomID]id.UserID
	pending        sync.Map // userID string -> *advPendingInteraction
	userLocks      sync.Map // userID string -> *sync.Mutex
	dmRemindedDate sync.Map // userID string -> "2006-01-02" date string
	dmMenuSentAt   sync.Map // userID string -> time.Time (last time actionable menu was DM'd)
	arenaDeadlines sync.Map // userID string -> time.Time (auto-cashout deadline)
	arenaPending   sync.Map // userID string -> int (pending tier number awaiting confirmation)
	morningHour int
	summaryHour int
}

// advUserLock returns a per-user mutex to prevent concurrent action resolution.
func (p *AdventurePlugin) advUserLock(userID id.UserID) *sync.Mutex {
	val, _ := p.userLocks.LoadOrStore(string(userID), &sync.Mutex{})
	return val.(*sync.Mutex)
}

const advDMResponseWindow = 15 * time.Minute

// advMarkMenuSent records that an actionable adventure menu was DM'd to the user.
// Only bare-number DM replies within this window will be treated as adventure choices.
func (p *AdventurePlugin) advMarkMenuSent(userID id.UserID) {
	p.dmMenuSentAt.Store(string(userID), time.Now())
}

// advIsInResponseWindow returns true if the user was recently sent an actionable menu.
func (p *AdventurePlugin) advIsInResponseWindow(userID id.UserID) bool {
	val, ok := p.dmMenuSentAt.Load(string(userID))
	if !ok {
		return false
	}
	return time.Since(val.(time.Time)) < advDMResponseWindow
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

// SetAchievements wires the achievements plugin after both are initialized.
func (p *AdventurePlugin) SetAchievements(ach *AchievementsPlugin) {
	p.achievements = ach
}

func (p *AdventurePlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "adventure", Description: "Daily adventure game — dungeon, mine, forage, or rest", Usage: "!adventure", Category: "Games"},
		{Name: "arena", Description: "Arena combat — fight through 5 tiers of increasingly deadly monsters", Usage: "!arena", Category: "Games"},
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

	// Always reset daily actions at startup — idempotent (WHERE clause
	// only touches characters whose last_action_date < today). This handles
	// the case where the old buggy code marked the midnight job as completed
	// even though the actual reset failed due to SQLite contention.
	if err := resetAllAdvDailyActions(); err != nil {
		slog.Error("adventure: startup daily reset failed", "err", err)
	}
	// Revive any characters whose DeadUntil has expired
	p.catchUpRespawns(chars)

	// Start schedulers
	go p.morningTicker()
	go p.summaryTicker()
	go p.midnightTicker()
	go p.eventTicker()
	go p.arenaAutoCashoutTicker()

	// Auto-cashout any arena runs left in 'awaiting' from a prior restart
	p.arenaCleanupStaleRuns()

	return nil
}

func (p *AdventurePlugin) catchUpRespawns(chars []AdventureCharacter) {
	now := time.Now().UTC()
	for _, char := range chars {
		if !char.Alive && char.DeadUntil != nil && now.After(*char.DeadUntil) {
			char.Alive = true
			char.DeadUntil = nil
			if err := saveAdvCharacter(&char); err != nil {
				slog.Error("adventure: catch-up revive failed", "user", char.UserID, "err", err)
				continue
			}
			slog.Info("adventure: catch-up revived player", "user", char.UserID)
			text := renderAdvRespawnDM(&char)
			if err := p.SendDM(char.UserID, text); err != nil {
				slog.Error("adventure: catch-up respawn DM failed", "user", char.UserID, "err", err)
			}
		}
	}
}

func (p *AdventurePlugin) OnReaction(_ ReactionContext) error { return nil }

// ── Message Dispatch ─────────────────────────────────────────────────────────

func (p *AdventurePlugin) OnMessage(ctx MessageContext) error {
	// 1. Arena commands (work in rooms and DMs)
	if p.IsCommand(ctx.Body, "arena") {
		return p.dispatchArenaCommand(ctx)
	}

	// 2. Check if this is a DM reply from a registered player
	p.mu.Lock()
	playerID, isDM := p.dmToPlayer[ctx.RoomID]
	p.mu.Unlock()

	if isDM && playerID == ctx.Sender {
		return p.handleDMReply(ctx)
	}

	// 3. Command dispatch
	if !p.IsCommand(ctx.Body, "adventure") {
		return nil
	}

	return p.dispatchCommand(ctx)
}

func (p *AdventurePlugin) dispatchCommand(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "adventure"))
	lower := strings.ToLower(args)

	switch {
	case args == "" || lower == "menu":
		return p.handleMenu(ctx)
	case lower == "status":
		return p.handleStatus(ctx)
	case strings.HasPrefix(lower, "sell "):
		return p.handleSellCmd(ctx, strings.TrimSpace(args[5:]))
	case lower == "shop" || strings.HasPrefix(lower, "shop "):
		return p.handleShopCmd(ctx, strings.TrimSpace(strings.TrimPrefix(lower, "shop")))
	case strings.HasPrefix(lower, "buy "):
		return p.handleBuyCmd(ctx, strings.TrimSpace(args[4:]))
	case lower == "equip":
		return p.handleEquipCmd(ctx)
	case lower == "inventory" || lower == "inv":
		return p.handleInventoryCmd(ctx)
	case lower == "leaderboard" || lower == "lb":
		return p.handleLeaderboard(ctx)
	case strings.HasPrefix(lower, "revive "):
		return p.handleAdminRevive(ctx, strings.TrimSpace(args[7:]))
	case lower == "summary":
		return p.handleAdminSummary(ctx)
	case lower == "respond":
		return p.handleEventRespond(ctx)
	case lower == "help":
		return p.SendDM(ctx.Sender, advHelpText)
	}

	return p.SendDM(ctx.Sender, "Unknown command. Type `!adventure help` to see available commands.")
}

const advHelpText = `**Adventure Commands**

` + "`!adventure`" + ` — Show today's activity menu
` + "`!adventure status`" + ` — View your character sheet
` + "`!adventure shop`" + ` — Browse equipment categories
` + "`!adventure shop <category>`" + ` — View a category (weapon, armor, helmet, boots, tool)
` + "`!adventure buy <item>`" + ` — Buy equipment (e.g. ` + "`buy Enchanted Blade`" + ` or ` + "`buy 4 sword`" + `)
` + "`!adventure equip`" + ` — Equip Masterwork gear from inventory
` + "`!adventure sell <item>`" + ` — Sell an inventory item (or ` + "`sell all`" + `)
` + "`!adventure inventory`" + ` — View your inventory
` + "`!adventure leaderboard`" + ` — View the leaderboard
` + "`!adventure respond`" + ` — Respond to a mid-day event
` + "`!adventure help`" + ` — This message

**Arena:**
` + "`!arena`" + ` — Show arena tier menu
` + "`!arena tier <1-5>`" + ` — Enter a tier
` + "`!arena fight`" + ` — Fight current round
` + "`!arena descend`" + ` — Descend to next tier (keep earnings at risk)
` + "`!arena cashout`" + ` — Take earnings and leave
` + "`!arena status`" + ` — Current run state
` + "`!arena leaderboard`" + ` — Top arena players

**In DM:** Reply with a number (e.g. ` + "`1`" + `) or location name to take your daily action.`

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
		// On holidays, allow second action if not yet taken
		isHol, _ := isHolidayToday()
		if isHol && !char.HolidayActionTaken {
			treasures, _ := loadAdvTreasureBonuses(char.UserID)
			buffs, _ := loadAdvActiveBuffs(char.UserID)
			bonuses := computeAdvBonuses(treasures, buffs, char.CurrentStreak, false)
			text := renderAdvHolidaySecondPrompt(char, equip, bonuses)
			p.advMarkMenuSent(ctx.Sender)
			return p.SendDM(ctx.Sender, text)
		}

		now := time.Now().UTC()
		midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		remaining := midnight.Sub(now)
		hours := int(remaining.Hours())
		minutes := int(remaining.Minutes()) % 60
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You've already taken your action today. Tomorrow awaits. Try to survive it.\n\n"+
				"Next action: 00:00 UTC (%dh %dm from now)\n"+
				"Morning DM: %02d:00 UTC\n\n"+
				"The Arena is always open: `!arena`",
			hours, minutes, p.morningHour))
	}

	treasures, _ := loadAdvTreasureBonuses(char.UserID)
	buffs, _ := loadAdvActiveBuffs(char.UserID)
	bonuses := computeAdvBonuses(treasures, buffs, char.CurrentStreak, false)
	balance := p.euro.GetBalance(char.UserID)

	_, holName := isHolidayToday()
	text := renderAdvMorningDM(char, equip, balance, bonuses, holName)
	p.advMarkMenuSent(ctx.Sender)
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

func (p *AdventurePlugin) handleShopCmd(ctx MessageContext, category string) error {
	_, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character.")
	}

	balance := p.euro.GetBalance(ctx.Sender)

	if category == "" {
		return p.SendDM(ctx.Sender, advShopOverview(equip, balance))
	}

	slot := advParseShopCategory(category)
	if slot == "" {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Unknown category '%s'. Try: weapon, armor, helmet, boots, or tool.", category))
	}

	return p.SendDM(ctx.Sender, advShopCategory(slot, equip, balance))
}

func (p *AdventurePlugin) handleBuyCmd(ctx MessageContext, itemName string) error {
	char, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character.")
	}
	if !char.Alive {
		return p.SendDM(ctx.Sender, "You're dead. Shopping can wait until you've respawned.")
	}

	slot, def, found := advFindShopItem(itemName)
	if !found {
		return p.SendDM(ctx.Sender, fmt.Sprintf("No item matching '%s' found in the shop. Type `!adventure shop` to see what's available.", itemName))
	}

	result := p.advBuyEquipment(ctx.Sender, slot, def, equip)
	return p.SendDM(ctx.Sender, result)
}

func (p *AdventurePlugin) handleSellCmd(ctx MessageContext, args string) error {
	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character.")
	}
	if !char.Alive {
		return p.SendDM(ctx.Sender, "You're dead. No haggling from beyond the grave.")
	}

	var result string
	if strings.ToLower(args) == "all" {
		result = p.advSellAll(ctx.Sender)
	} else {
		result = p.advSellItem(ctx.Sender, args)
	}
	return p.SendDM(ctx.Sender, result)
}

func (p *AdventurePlugin) handleInventoryCmd(ctx MessageContext) error {
	if _, _, err := p.ensureCharacter(ctx.Sender); err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character.")
	}
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
	if p.achievements != nil {
		p.achievements.GrantAchievement(targetID, "adv_revived")
	}
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

	// Strip !adventure prefix if present — dispatch directly to avoid recursion
	if strings.HasPrefix(strings.ToLower(body), "!adventure") {
		return p.dispatchCommand(ctx)
	}

	// Check for pending interaction first (always honored regardless of window)
	if val, ok := p.pending.Load(string(ctx.Sender)); ok {
		interaction := val.(*advPendingInteraction)
		if time.Now().Before(interaction.ExpiresAt) {
			return p.resolvePendingInteraction(ctx, interaction)
		}
		p.pending.Delete(string(ctx.Sender))
		p.SendDM(ctx.Sender, "Your previous prompt expired. Moving on.")
	}

	// Only interpret bare messages as adventure choices if the user was recently
	// shown an actionable menu. This prevents "1" typed during UNO (or any other
	// DM-based game) from triggering adventure responses.
	if !p.advIsInResponseWindow(ctx.Sender) {
		return nil
	}

	// Parse as activity choice
	return p.parseAndResolveChoice(ctx, body)
}

func (p *AdventurePlugin) resolvePendingInteraction(ctx MessageContext, interaction *advPendingInteraction) error {
	p.pending.Delete(string(ctx.Sender))

	switch interaction.Type {
	case "treasure_discard":
		return p.handleTreasureDiscard(ctx, interaction)
	case "masterwork_equip":
		return p.handleMasterworkEquipReply(ctx, interaction)
	case "masterwork_equip_confirm":
		return p.handleMasterworkEquipConfirm(ctx, interaction)
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
		// On holidays, allow second action if not yet taken
		isHol, _ := isHolidayToday()
		if !isHol || char.HolidayActionTaken {
			// Only send the reminder once per day — subsequent DM messages
			// are silently ignored so they can be handled by other plugins (e.g. UNO).
			today := time.Now().UTC().Format("2006-01-02")
			if prev, ok := p.dmRemindedDate.Load(string(ctx.Sender)); ok && prev.(string) == today {
				return nil
			}
			p.dmRemindedDate.Store(string(ctx.Sender), today)

			now := time.Now().UTC()
			midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
			remaining := midnight.Sub(now)
			hours := int(remaining.Hours())
			minutes := int(remaining.Minutes()) % 60
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"You've already taken your action today. Rest now. Try again tomorrow.\n\n"+
					"Next action: 00:00 UTC (%dh %dm from now)\n\n"+
					"The Arena is always open: `!arena`",
				hours, minutes))
		}
		// Fall through for holiday second action
	}

	lower := strings.ToLower(body)

	// Parse "6" or "rest"
	if lower == "6" || lower == "rest" {
		return p.resolveRest(ctx, char)
	}

	// Parse "5" or "shop"
	if lower == "5" || lower == "shop" {
		equip, _ := loadAdvEquipment(ctx.Sender)
		balance := p.euro.GetBalance(ctx.Sender)
		return p.SendDM(ctx.Sender, advShopOverview(equip, balance))
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
	case "4", "fish", "fishing":
		activity = AdvActivityFishing
	default:
		// Try matching location name directly
		for _, act := range []AdvActivityType{AdvActivityDungeon, AdvActivityMining, AdvActivityForaging, AdvActivityFishing} {
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

	// Per-location cooldown: 3 hours after a successful run
	if remaining := advLocationCooldown(char.UserID, loc.Name); remaining > 0 {
		mins := int(remaining.Minutes())
		if mins < 1 {
			return p.SendDM(ctx.Sender, fmt.Sprintf("🕐 %s is still being cleared out. Try again in less than a minute, or pick a different location.", loc.Name))
		}
		h, m := mins/60, mins%60
		if h > 0 {
			return p.SendDM(ctx.Sender, fmt.Sprintf("🕐 %s is still being cleared out. Try again in %dh %dm, or pick a different location.", loc.Name, h, m))
		}
		return p.SendDM(ctx.Sender, fmt.Sprintf("🕐 %s is still being cleared out. Try again in %d minutes, or pick a different location.", loc.Name, m))
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
	case "fishing":
		char.FishingXP += result.XPGained
	}

	// Check level up
	result.LeveledUp, result.NewLevel = checkAdvLevelUp(char, result.XPSkill)

	// Handle death
	deathReprieved := false
	if result.Outcome == AdvOutcomeDeath {
		// Sovereign set: Death's Reprieve — survive lethal outcome
		if advEquippedArenaSets(equip)["sovereign"] && char.DeathReprieveAvailable() {
			deathReprieved = true
			now := time.Now().UTC()
			char.DeathReprieveLast = &now
			char.GrudgeLocation = loc.Name
			// Gear absorbs the blow — all equipment set to 1 condition
			for _, slot := range allSlots {
				if eq, ok := equip[slot]; ok {
					eq.Condition = 1
				}
			}
			// Post room announcement
			nextWindow := now.Add(168 * time.Hour)
			gr := gamesRoom()
			if gr != "" {
				p.SendMessage(gr, renderArenaDeathReprieve(char.DisplayName, loc.Name, nextWindow))
			}
		} else {
			char.Kill()
			char.GrudgeLocation = loc.Name
		}
	} else if hasGrudge && (result.Outcome == AdvOutcomeSuccess || result.Outcome == AdvOutcomeExceptional) {
		// Clear grudge on successful return
		char.GrudgeLocation = ""
	}

	// Add loot to inventory
	for _, item := range result.LootItems {
		_ = addAdvInventoryItem(char.UserID, item)
	}

	// Determine if this is the holiday second action
	isAction2 := char.ActionTakenToday // already taken = this is the second
	isHol, _ := isHolidayToday()

	// Mark action taken and record the date for streak tracking
	if !isAction2 {
		char.ActionTakenToday = true
		char.LastActionDate = time.Now().UTC().Format("2006-01-02")
	}

	// Holiday flags: mark second action done, or mark it done on death during action 1
	if isAction2 {
		char.HolidayActionTaken = true
	} else if isHol && result.Outcome == AdvOutcomeDeath && !deathReprieved {
		char.HolidayActionTaken = true // died on action 1 — no second action
	}

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

	// Log activity BEFORE party bonus check so both visitors can see each other
	logAdvActivity(char.UserID, string(activity), loc.Name, string(result.Outcome),
		result.TotalLootValue, result.XPGained, result.FlavorKey)

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

	// Send resolution DM with closing block
	text := renderAdvResolutionDM(result, char)
	if deathReprieved {
		nextWindow := char.DeathReprieveLast.Add(168 * time.Hour)
		text += fmt.Sprintf("\n\n⚔️ **Death's Reprieve activated.** Your Sovereign gear absorbed the killing blow. "+
			"You survived — barely. All equipment took heavy damage.\n"+
			"Next reprieve window: %s", nextWindow.Format("2006-01-02 15:04 UTC"))
	}
	closing := advClosingBlock(result.Outcome, char.UserID, loc.Name, p.morningHour, p.summaryHour)
	if closing != "" {
		text += "\n" + closing
	}
	if err := p.SendDM(ctx.Sender, text); err != nil {
		slog.Error("adventure: failed to send resolution DM", "user", ctx.Sender, "err", err)
	}

	// Check for treasure drop
	if result.Outcome == AdvOutcomeSuccess || result.Outcome == AdvOutcomeExceptional {
		p.checkTreasureDrop(ctx.Sender, char, loc)
		p.checkMasterworkDrop(ctx.Sender, char, equip, loc, result.Outcome)
	}

	// TODO: holiday achievement hooks

	// Holiday: offer second action if this was action 1 and player survived
	if !isAction2 && isHol && (result.Outcome != AdvOutcomeDeath || deathReprieved) {
		equip2, _ := loadAdvEquipment(char.UserID)
		treasures2, _ := loadAdvTreasureBonuses(char.UserID)
		buffs2, _ := loadAdvActiveBuffs(char.UserID)
		bonuses2 := computeAdvBonuses(treasures2, buffs2, char.CurrentStreak, false)
		prompt := renderAdvHolidaySecondPrompt(char, equip2, bonuses2)
		if err := p.SendDM(ctx.Sender, prompt); err != nil {
			slog.Error("adventure: failed to send holiday second prompt", "user", ctx.Sender, "err", err)
		}
	}

	return nil
}

func (p *AdventurePlugin) resolveRest(ctx MessageContext, char *AdventureCharacter) error {
	isAction2 := char.ActionTakenToday
	isHol, _ := isHolidayToday()

	if !isAction2 {
		char.ActionTakenToday = true
		char.LastActionDate = time.Now().UTC().Format("2006-01-02")
	}
	if isAction2 {
		char.HolidayActionTaken = true
	}

	if err := saveAdvCharacter(char); err != nil {
		return p.SendDM(ctx.Sender, "Failed to save. Even resting is broken.")
	}

	logAdvActivity(char.UserID, string(AdvActivityRest), "", "rest", 0, 0, "")

	// Compute reset countdown
	now := time.Now().UTC()
	midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	remaining := midnight.Sub(now)
	hours := int(remaining.Hours())
	minutes := int(remaining.Minutes()) % 60

	restMsg := fmt.Sprintf(
		"%s, you chose rest. No loot. No XP. No death.\n\n"+
			"You sat in your hovel and stared at the wall and achieved absolutely nothing. "+
			"Tomorrow awaits. It will probably be the same.\n\n"+
			"─────────────────────────────\n"+
			"Next action: 00:00 UTC (%dh %dm from now)\n"+
			"Morning DM: %02d:00 UTC\n"+
			"Evening summary: %02d:00 UTC\n\n"+
			"The Arena is always open: `!arena`",
		char.DisplayName, hours, minutes, p.morningHour, p.summaryHour)

	if err := p.SendDM(ctx.Sender, restMsg); err != nil {
		slog.Error("adventure: failed to send rest DM", "user", ctx.Sender, "err", err)
	}

	// Holiday: offer second action if this was action 1
	if !isAction2 && isHol {
		equip, _ := loadAdvEquipment(char.UserID)
		treasures, _ := loadAdvTreasureBonuses(char.UserID)
		buffs, _ := loadAdvActiveBuffs(char.UserID)
		bonuses := computeAdvBonuses(treasures, buffs, char.CurrentStreak, false)
		prompt := renderAdvHolidaySecondPrompt(char, equip, bonuses)
		if err := p.SendDM(ctx.Sender, prompt); err != nil {
			slog.Error("adventure: failed to send holiday second prompt", "user", ctx.Sender, "err", err)
		}
	}

	return nil
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

	case AdvActivityFishing:
		switch result.Outcome {
		case AdvOutcomeDeath:
			if tierPool, ok := FishingDeath[loc.Tier]; ok {
				pool = tierPool
			}
		case AdvOutcomeEmpty:
			if tierPool, ok := FishingEmpty[loc.Tier]; ok {
				pool = tierPool
			}
		case AdvOutcomeSuccess:
			if tierPool, ok := FishingSuccess[loc.Tier]; ok {
				pool = tierPool
			}
		case AdvOutcomeExceptional:
			if tierPool, ok := FishingExceptional[loc.Tier]; ok {
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
		displayName := p.DisplayName(userID)
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

