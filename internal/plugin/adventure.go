package plugin

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// ── Plugin ───────────────────────────────────────────────────────────────────

type AdventurePlugin struct {
	Base
	euro         *EuroPlugin
	xp           *XPPlugin
	achievements *AchievementsPlugin
	mu           sync.Mutex
	dmToPlayer     map[id.RoomID]id.UserID
	pending        sync.Map // userID string -> *advPendingInteraction
	userLocks      sync.Map // userID string -> *sync.Mutex
	dmRemindedDate sync.Map // userID string -> "2006-01-02" date string
	dmMenuSentAt   sync.Map // userID string -> time.Time (last time actionable menu was DM'd)
	arenaDeadlines sync.Map // userID string -> time.Time (auto-cashout deadline)
	arenaPending   sync.Map // userID string -> int (pending tier number awaiting confirmation)
	arenaBailCh    sync.Map // userID string -> chan struct{} (bail signal for countdown goroutine)
	shopSessions   sync.Map // userID string -> *advShopSession
	hospitalNudges sync.Map // userID string -> time.Time (when to send nudge)
	craftResults   sync.Map // userID string -> []CraftResult (pending craft results for narrative)
	treasureUndo   sync.Map // userID string -> *advTreasureUndoToken (10-min auto-swap reversal)
	morningHour int
	summaryHour int
}

// advUserLock returns a per-user mutex to prevent concurrent action resolution.
func (p *AdventurePlugin) advUserLock(userID id.UserID) *sync.Mutex {
	val, _ := p.userLocks.LoadOrStore(string(userID), &sync.Mutex{})
	return val.(*sync.Mutex)
}

const advDMResponseWindow = 3 * time.Hour
const advTreasureUndoWindow = 10 * time.Minute

// advTreasureUndoToken tracks a recent auto-swap so the player can undo it
// within the window by replying "undo" in DM. Holds the discarded def so
// the swap can be reversed exactly. AnnounceTimer is the deferred room
// announcement — fired once the undo window closes if the swap stands,
// stopped on undo so reversed swaps don't get publicly announced.
type advTreasureUndoToken struct {
	Discarded     *AdvTreasureDef
	Kept          *AdvTreasureDef
	ExpiresAt     time.Time
	AnnounceTimer *time.Timer
}

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

func NewAdventurePlugin(client *mautrix.Client, euro *EuroPlugin, xp *XPPlugin) *AdventurePlugin {
	return &AdventurePlugin{
		Base:        NewBase(client),
		euro:        euro,
		xp:          xp,
		dmToPlayer:  make(map[id.RoomID]id.UserID),
		morningHour: envInt("ADVENTURE_MORNING_HOUR", 8),
		summaryHour: envInt("ADVENTURE_SUMMARY_HOUR", 20),
	}
}

// chatLevel returns the user's chat level for perk calculations.
func (p *AdventurePlugin) chatLevel(userID id.UserID) int {
	if p.xp == nil {
		return 0
	}
	return p.xp.GetLevel(userID)
}

func (p *AdventurePlugin) Name() string    { return "adventure" }
func (p *AdventurePlugin) Version() string { return "2.5.0" }

// SetAchievements wires the achievements plugin after both are initialized.
func (p *AdventurePlugin) SetAchievements(ach *AchievementsPlugin) {
	p.achievements = ach
}

func (p *AdventurePlugin) Commands() []CommandDef {
	return []CommandDef{
		{Name: "adventure", Description: "Town services menu (expeditions: see !expedition)", Usage: "!adventure", Category: "Games"},
		{Name: "arena", Description: "Arena combat — fight through 5 tiers of increasingly deadly monsters", Usage: "!arena", Category: "Games"},
		{Name: "thom", Description: "Visit Thom Krooke — housing and loans", Usage: "!thom", Category: "Games"},
		{Name: "setup", Description: "Create or continue your Adv 2.0 character (race, class, stats)", Usage: "!setup", Category: "Games"},
		{Name: "sheet", Description: "View your Adv 2.0 character sheet", Usage: "!sheet", Category: "Games"},
		{Name: "abilities", Description: "List your Adv 2.0 class and race abilities", Usage: "!abilities", Category: "Games"},
		{Name: "respec", Description: "Wipe and rebuild your Adv 2.0 character (5000 euros, 7-day cooldown)", Usage: "!respec", Category: "Games"},
		{Name: "subclass", Description: "View or choose your Adv 2.0 subclass (unlocks at L5; change costs 500 euros, 30-day cooldown)", Usage: "!subclass [name|number]", Category: "Games"},
		{Name: "check", Description: "Roll an Adv 2.0 skill check (d20 + ability modifier vs DC)", Usage: "!check <skill> [dc]", Category: "Games"},
		{Name: "rest", Description: "Take an Adv 2.0 rest (`short`: 1h cooldown, partial heal — `long`: 24h, full heal, requires housing or inn)", Usage: "!rest short|long", Category: "Games"},
		{Name: "arm", Description: "Pre-arm an active Adv 2.0 ability for next combat (consumes resource)", Usage: "!arm <ability>", Category: "Games"},
		{Name: "roll", Description: "Roll dice (NdN+M format, e.g. 2d6+3, d20, 4d6-1)", Usage: "!roll <dice>", Category: "Games"},
		{Name: "stats", Description: "Show your Adv 2.0 ability scores and modifiers", Usage: "!stats", Category: "Games"},
		{Name: "level", Description: "Show your Adv 2.0 level and XP progress", Usage: "!level", Category: "Games"},
		{Name: "cast", Description: "Cast an Adv 2.0 spell (queues for next combat, or resolves out-of-combat)", Usage: "!cast <spell> [--upcast N] [--target @user]", Category: "Games"},
		{Name: "spells", Description: "List your known spells, prepared spells, and spell slots", Usage: "!spells [learn <spell>]", Category: "Games"},
		{Name: "prepare", Description: "Cleric: prepare a spell for the day (cap = WIS mod + level)", Usage: "!prepare <spell> | !prepare clear <spell>", Category: "Games"},
		{Name: "forage", Description: "Forage in your current expedition region (WIS / Survival)", Usage: "!forage", Category: "Games"},
		{Name: "mine", Description: "Mine or salvage in your current expedition region (STR / Athletics)", Usage: "!mine", Category: "Games"},
		{Name: "scavenge", Description: "Scavenge in your current expedition region (INT / Investigation)", Usage: "!scavenge", Category: "Games"},
		{Name: "essence", Description: "Harvest magical essence in your current expedition region (INT / Arcana)", Usage: "!essence", Category: "Games"},
		{Name: "commune", Description: "Commune with spiritual energy in your current expedition region (Cleric primary)", Usage: "!commune", Category: "Games"},
		{Name: "fish", Description: "Fish in your current expedition region (DEX / Sleight of Hand; water zones only)", Usage: "!fish", Category: "Games"},
		{Name: "resources", Description: "List harvestable resources in your current expedition region", Usage: "!resources", Category: "Games"},
		{Name: "sell", Description: "Sell harvested materials/fish/items to Thom Krooke (post-expedition; CHA Persuasion DC 17 = +15%)", Usage: "!sell [list|all|<item>]", Category: "Games"},
		{Name: "craft", Description: "Craft a discovered recipe at Thom Krooke (consumes ingredients)", Usage: "!craft [list|<recipe>]", Category: "Games"},
		{Name: "lore", Description: "Dig through Thom Krooke's lore stacks for an undiscovered recipe (INT/Arcana DC 15)", Usage: "!lore", Category: "Games"},
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
	if err := lockCoopCombatActions(); err != nil {
		slog.Error("adventure: startup coop combat lock failed", "err", err)
	}
	// Phase R1 — archive any active dnd_zone_run rows that aren't linked to
	// an active expedition. Idempotent: re-running it does nothing once the
	// orphan set is empty.
	if n, err := archiveOrphanZoneRuns(); err != nil {
		slog.Error("adventure: R1 orphan zone-run archive failed", "err", err)
	} else if n > 0 {
		slog.Info("adventure: R1 archived orphan zone runs", "count", n)
	}
	// Revive any characters whose DeadUntil has expired
	p.catchUpRespawns(chars)

	// Start schedulers
	go p.morningTicker()
	go p.summaryTicker()
	go p.midnightTicker()
	go p.eventTicker()
	// Arena auto-cashout ticker removed — replaced by per-session countdown goroutines
	go p.rivalChallengeTicker()
	go p.robbieTicker()
	go p.hospitalNudgeTicker()
	go p.mortgageTicker()
	go p.coopTicker()
	go p.expeditionBriefingTicker()
	go p.expeditionRecapTicker()

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
	// 0. D&D layer commands (Phase 1 — work in rooms and DMs)
	if p.IsCommand(ctx.Body, "setup") {
		return p.handleDnDSetupCmd(ctx, p.GetArgs(ctx.Body, "setup"))
	}
	if p.IsCommand(ctx.Body, "sheet") {
		return p.handleDnDSheetCmd(ctx)
	}
	if p.IsCommand(ctx.Body, "abilities") {
		return p.handleDnDAbilitiesCmd(ctx)
	}
	if p.IsCommand(ctx.Body, "respec") {
		return p.handleDnDRespecCmd(ctx)
	}
	if p.IsCommand(ctx.Body, "subclass") {
		return p.handleDnDSubclassCmd(ctx, p.GetArgs(ctx.Body, "subclass"))
	}
	if p.IsCommand(ctx.Body, "check") {
		return p.handleDnDCheckCmd(ctx, p.GetArgs(ctx.Body, "check"))
	}
	if p.IsCommand(ctx.Body, "rest") {
		return p.handleDnDRestCmd(ctx, p.GetArgs(ctx.Body, "rest"))
	}
	if p.IsCommand(ctx.Body, "arm") {
		return p.handleDnDArmCmd(ctx, p.GetArgs(ctx.Body, "arm"))
	}
	if p.IsCommand(ctx.Body, "cast") {
		return p.handleDnDCastCmd(ctx, p.GetArgs(ctx.Body, "cast"))
	}
	if p.IsCommand(ctx.Body, "spells") {
		return p.handleDnDSpellsCmd(ctx, p.GetArgs(ctx.Body, "spells"))
	}
	if p.IsCommand(ctx.Body, "prepare") {
		return p.handleDnDPrepareCmd(ctx, p.GetArgs(ctx.Body, "prepare"))
	}
	if p.IsCommand(ctx.Body, "roll") {
		return p.handleDnDRollCmd(ctx, p.GetArgs(ctx.Body, "roll"))
	}
	if p.IsCommand(ctx.Body, "stats") {
		return p.handleDnDStatsCmd(ctx)
	}
	if p.IsCommand(ctx.Body, "level") {
		return p.handleDnDLevelCmd(ctx)
	}
	if p.IsCommand(ctx.Body, "zone") {
		return p.handleDnDZoneCmd(ctx, p.GetArgs(ctx.Body, "zone"))
	}
	if p.IsCommand(ctx.Body, "expedition") {
		return p.handleDnDExpeditionCmd(ctx, p.GetArgs(ctx.Body, "expedition"))
	}
	if p.IsCommand(ctx.Body, "camp") {
		return p.handleCampCmd(ctx, p.GetArgs(ctx.Body, "camp"))
	}
	if p.IsCommand(ctx.Body, "threat") {
		return p.handleThreatCmd(ctx)
	}
	if p.IsCommand(ctx.Body, "region") {
		return p.handleRegionCmd(ctx, p.GetArgs(ctx.Body, "region"))
	}
	if p.IsCommand(ctx.Body, "extract") {
		return p.handleExtractCmd(ctx, p.GetArgs(ctx.Body, "extract"))
	}
	if p.IsCommand(ctx.Body, "resume") {
		return p.handleResumeCmd(ctx, p.GetArgs(ctx.Body, "resume"))
	}
	if p.IsCommand(ctx.Body, "map") {
		return p.handleExpeditionMapCmd(ctx, p.GetArgs(ctx.Body, "map"))
	}
	if p.IsCommand(ctx.Body, "forage") {
		return p.handleHarvestCmd(ctx, HarvestForage)
	}
	if p.IsCommand(ctx.Body, "mine") {
		return p.handleHarvestCmd(ctx, HarvestMine)
	}
	if p.IsCommand(ctx.Body, "scavenge") {
		return p.handleHarvestCmd(ctx, HarvestScavenge)
	}
	if p.IsCommand(ctx.Body, "essence") {
		return p.handleHarvestCmd(ctx, HarvestEssence)
	}
	if p.IsCommand(ctx.Body, "commune") {
		return p.handleHarvestCmd(ctx, HarvestCommune)
	}
	if p.IsCommand(ctx.Body, "fish") {
		return p.handleHarvestCmd(ctx, HarvestFish)
	}
	if p.IsCommand(ctx.Body, "resources") {
		return p.handleResourcesCmd(ctx)
	}
	if p.IsCommand(ctx.Body, "sell") {
		return p.handleResourceSellCmd(ctx, p.GetArgs(ctx.Body, "sell"))
	}
	if p.IsCommand(ctx.Body, "craft") {
		return p.handleCraftCmd(ctx, p.GetArgs(ctx.Body, "craft"))
	}
	if p.IsCommand(ctx.Body, "lore") {
		return p.handleLoreCmd(ctx)
	}

	// 1. Arena commands (work in rooms and DMs)
	if p.IsCommand(ctx.Body, "bail") {
		return p.handleArenaBail(ctx)
	}
	if p.IsCommand(ctx.Body, "arena") {
		return p.dispatchArenaCommand(ctx)
	}

	// 1b. Hospital commands (work in rooms and DMs)
	if p.IsCommand(ctx.Body, "hospital") {
		return p.handleHospitalCmd(ctx)
	}

	// 1c. Co-op dungeon commands (work in rooms and DMs)
	if p.IsCommand(ctx.Body, "coop") {
		return p.handleCoopCmd(ctx)
	}

	// 2. Check if this is a DM reply from a registered player
	p.mu.Lock()
	playerID, isDM := p.dmToPlayer[ctx.RoomID]
	p.mu.Unlock()

	if isDM && playerID == ctx.Sender {
		return p.handleDMReply(ctx)
	}

	// 3. NPC encounter tracking — count room messages from adventure players
	if !isDM && !strings.HasPrefix(ctx.Body, "!") {
		safeGo("npc-track", func() {
			p.npcTrackMessage(ctx.Sender)
		})
	}

	// 4. Thom Krooke commands (work in rooms and DMs)
	if p.IsCommand(ctx.Body, "thom") {
		return p.handleThomCmd(ctx)
	}

	// 5. Command dispatch
	if !p.IsCommand(ctx.Body, "adventure") && !p.IsCommand(ctx.Body, "adv") {
		return nil
	}

	return p.dispatchCommand(ctx)
}

func (p *AdventurePlugin) dispatchCommand(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "adventure"))
	if args == "" && p.IsCommand(ctx.Body, "adv") {
		args = strings.TrimSpace(p.GetArgs(ctx.Body, "adv"))
	}
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
	case lower == "rivals":
		return p.handleRivalsCmd(ctx)
	case lower == "babysit" || strings.HasPrefix(lower, "babysit "):
		return p.handleBabysitCmd(ctx, strings.TrimSpace(strings.TrimPrefix(lower, "babysit")))
	case lower == "blacksmith" || lower == "repair":
		return p.handleBlacksmithCmd(ctx)
	case lower == "repair all":
		return p.handleRepairAllCmd(ctx)
	case strings.HasPrefix(lower, "repair "):
		return p.handleRepairSlotCmd(ctx, strings.TrimSpace(args[7:]))
	case lower == "boost":
		return p.handleBoostCmd(ctx)
	case lower == "recipes":
		return p.handleRecipesCmd(ctx)
	case lower == "mastery":
		return p.handleMasteryCmd(ctx)
	case lower == "treasures" || strings.HasPrefix(lower, "treasures "):
		return p.handleTreasuresCmd(ctx, strings.TrimSpace(strings.TrimPrefix(lower, "treasures")))
	}

	return p.SendDM(ctx.Sender, "Unknown command. Type `!adventure help` to see available commands.")
}

const advHelpText = `**Adventure Commands**

` + "`!adventure`" + ` — Show town services menu (expeditions are run via ` + "`!expedition`" + `)
` + "`!adventure status`" + ` — View your character sheet
` + "`!expedition`" + ` — Adventure: pick a zone, advance through rooms, harvest as you go
` + "`!adventure shop`" + ` — Browse equipment categories
` + "`!adventure shop <category>`" + ` — View a category (weapon, armor, helmet, boots, tool)
` + "`!adventure buy <item>`" + ` — Buy equipment (e.g. ` + "`buy Enchanted Blade`" + ` or ` + "`buy 4 sword`" + `)
` + "`!adventure equip`" + ` — Equip Masterwork gear from inventory
` + "`!adventure sell <item>`" + ` — Sell an inventory item (or ` + "`sell all`" + `)
` + "`!adventure inventory`" + ` — View your inventory
` + "`!adventure leaderboard`" + ` — View the leaderboard
` + "`!adventure respond`" + ` — Respond to a mid-day event
` + "`!adventure rivals`" + ` — View rival duel records
` + "`!adventure babysit`" + ` — Adventurer Babysitting Service
` + "`!adventure babysit auto`" + ` — Toggle auto-babysit (protects streaks on missed days)
` + "`!adventure babysit focus <skill>`" + ` — Pick the skill auto-babysit trains (mining/fishing/foraging)
` + "`!adventure blacksmith`" + ` — Visit the blacksmith (view repair costs)
` + "`!adventure repair all`" + ` — Repair all damaged equipment
` + "`!adventure repair <slot>`" + ` — Repair a specific slot
` + "`!adventure recipes`" + ` — List crafting recipes known at your current Foraging level
` + "`!adventure mastery`" + ` — Per-slot equipment mastery progress and active bonus
` + "`!adventure treasures`" + ` — List your treasures · ` + "`treasures lock`" + ` to refuse swaps
` + "`!coop`" + ` — Co-op dungeons (multi-day party runs). See ` + "`!coop help`" + `.
` + "`!hospital`" + ` — Visit St. Guildmore's Memorial Hospital (same-day revival when dead)
` + "`!thom`" + ` — Visit Thom Krooke (housing and loans)
` + "`!adventure help`" + ` — This message

**Arena:**
` + "`!arena`" + ` — Show arena tier menu
` + "`!arena tier <1-5>`" + ` — Enter a tier
` + "`!arena fight`" + ` — Fight current round
` + "`!arena descend`" + ` — Descend to next tier (keep earnings at risk)
` + "`!arena cashout`" + ` — Take earnings and leave
` + "`!arena status`" + ` — Current run state
` + "`!arena leaderboard`" + ` — Top arena players

**Systems:**
• Foraging level 10+ unlocks auto-crafting consumables from loot ingredients before combat.
• A 5% community tax funds the lottery pot. Arena and hospital apply 10%.

**In DM:** Reply with a number (e.g. ` + "`1`" + `) or location name to take your daily action.`

// ── Command Handlers ─────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleMenu(ctx MessageContext) error {
	char, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character.")
	}

	if !char.Alive {
		// On-demand revive if death timer has expired
		if char.DeadUntil != nil && time.Now().UTC().After(*char.DeadUntil) {
			char.Alive = true
			char.DeadUntil = nil
			if err := saveAdvCharacter(char); err != nil {
				slog.Error("adventure: on-demand revive failed", "user", char.UserID, "err", err)
			} else {
				text := renderAdvRespawnDM(char)
				p.SendDM(ctx.Sender, text)
				// Fall through to show menu
			}
		}
		if !char.Alive {
			text := renderAdvDeathStatusDM(char)
			return p.SendDM(ctx.Sender, text)
		}
	}

	isHol, holName := isHolidayToday()
	if char.AllActionsUsed(isHol) {
		now := time.Now().UTC()
		midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		remaining := midnight.Sub(now)
		hours := int(remaining.Hours())
		minutes := int(remaining.Minutes()) % 60
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You've used all your actions today. Tomorrow awaits. Try to survive it.\n\n"+
				"Next action: 00:00 UTC (%dh %dm from now)\n"+
				"Morning DM: %02d:00 UTC\n\n"+
				"The Arena is always open: `!arena`",
			hours, minutes, p.morningHour))
	}

	treasures, _ := loadAdvTreasureBonuses(char.UserID)
	buffs, _ := loadAdvActiveBuffs(char.UserID)
	bonuses := computeAdvBonuses(treasures, buffs, char.CurrentStreak, false)
	balance := p.euro.GetBalance(char.UserID)

	text := renderAdvMorningDM(char, equip, balance, bonuses, holName)
	p.advMarkMenuSent(ctx.Sender)
	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleStatus(ctx MessageContext) error {
	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "No adventurer found. Type `!adventure` to create one.")
	}

	// On-demand revive if death timer has expired
	if !char.Alive && char.DeadUntil != nil && time.Now().UTC().After(*char.DeadUntil) {
		char.Alive = true
		char.DeadUntil = nil
		if err := saveAdvCharacter(char); err != nil {
			slog.Error("adventure: on-demand revive failed", "user", char.UserID, "err", err)
		} else {
			p.SendDM(ctx.Sender, renderAdvRespawnDM(char))
		}
	}

	equip, err := loadAdvEquipment(ctx.Sender)
	if err != nil {
		slog.Error("adventure: failed to load equipment for status", "user", ctx.Sender, "err", err)
	}
	items, err := loadAdvInventory(ctx.Sender)
	if err != nil {
		slog.Error("adventure: failed to load inventory for status", "user", ctx.Sender, "err", err)
	}
	treasures, err := loadAdvTreasureBonuses(ctx.Sender)
	if err != nil {
		slog.Error("adventure: failed to load treasures for status", "user", ctx.Sender, "err", err)
	}
	balance := p.euro.GetBalance(ctx.Sender)

	text := renderAdvCharacterSheet(char, equip, items, treasures, balance)
	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleShopCmd(ctx MessageContext, args string) error {
	_, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character.")
	}

	balance := p.euro.GetBalance(ctx.Sender)
	showAll := strings.Contains(strings.ToLower(args), "show all")
	category := strings.TrimSpace(strings.Replace(strings.ToLower(args), "show all", "", 1))

	p.shopSessionStart(ctx.Sender)

	if category == "" {
		text := luigiShopGreeting(ctx.Sender, equip, balance, showAll, p.chatLevel(ctx.Sender))
		if disc := p.shopSessionAnnounceDiscount(ctx.Sender); disc != "" {
			text += "\n\n" + disc
		}
		p.pending.Store(string(ctx.Sender), &advPendingInteraction{
			Type:      "shop_category",
			Data:      &advPendingShopCategory{ShowAll: showAll},
			ExpiresAt: time.Now().Add(advDMResponseWindow),
		})
		p.advMarkMenuSent(ctx.Sender)
		p.shopScheduleBrowseNudge(ctx.Sender)
		return p.SendDM(ctx.Sender, text)
	}

	slot := advParseShopCategory(category)
	if slot == "" {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Unknown category '%s'. Try: weapon, armor, helmet, boots, or tool.", category))
	}

	text := luigiCategoryView(ctx.Sender, slot, equip, balance, showAll)
	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "shop_item",
		Data:      &advPendingShopItem{Slot: slot, ShowAll: showAll},
		ExpiresAt: time.Now().Add(advDMResponseWindow),
	})
	p.advMarkMenuSent(ctx.Sender)
	p.shopScheduleBrowseNudge(ctx.Sender)
	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleBuyCmd(ctx MessageContext, itemName string) error {
	char, equip, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character. Try `!adventure` to create one first.")
	}
	if !char.Alive {
		return p.SendDM(ctx.Sender, "You're dead. Shopping can wait until you've respawned.")
	}

	if itemName == "" {
		return p.SendDM(ctx.Sender, "Usage: `!buy <item name>`. Type `!adventure shop` to browse.")
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
		return p.SendReply(ctx.RoomID, ctx.EventID, "Failed to load your character. Try `!adventure` to create one first.")
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
	lower := strings.ToLower(body)
	if strings.HasPrefix(body, "!") && !strings.HasPrefix(lower, "!adventure") && !strings.HasPrefix(lower, "!adv") && !strings.HasPrefix(lower, "!thom") {
		return nil
	}

	// Handle !thom in DMs
	if strings.HasPrefix(lower, "!thom") {
		return p.handleThomCmd(ctx)
	}

	// Strip !adventure / !adv prefix if present — dispatch directly to avoid recursion
	if strings.HasPrefix(lower, "!adventure") || strings.HasPrefix(lower, "!adv") {
		return p.dispatchCommand(ctx)
	}

	// "undo" reverts a recent treasure auto-swap. Checked ahead of the
	// pending-interaction block so it doesn't get consumed by an unrelated
	// pending prompt.
	if lower == "undo" {
		if p.tryTreasureUndo(ctx.Sender) {
			return nil
		}
	}

	// Check for pending interaction first (always honored regardless of window)
	if val, ok := p.pending.Load(string(ctx.Sender)); ok {
		interaction := val.(*advPendingInteraction)
		if time.Now().Before(interaction.ExpiresAt) {
			return p.resolvePendingInteraction(ctx, interaction)
		}
		p.pending.Delete(string(ctx.Sender))
		p.shopSessionEnd(ctx.Sender)
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
	case "rival_rps":
		return p.resolveRivalRPSRound(ctx, interaction)
	case "blacksmith_slot":
		return p.resolveBlacksmithSlotChoice(ctx, interaction)
	case "blacksmith_confirm":
		return p.resolveBlacksmithConfirm(ctx, interaction)
	case "shop_category":
		return p.resolveShopCategoryChoice(ctx, interaction)
	case "shop_item":
		return p.resolveShopItemChoice(ctx, interaction)
	case "shop_supply":
		return p.resolveShopSupplyChoice(ctx, interaction)
	case "shop_confirm":
		return p.resolveShopConfirm(ctx, interaction)
	case "hospital_pay":
		return p.resolveHospitalPay(ctx, interaction)
	case "npc_encounter":
		return p.resolveNPCEncounter(ctx, interaction)
	case "pet_arrival":
		return p.resolvePetArrival(ctx)
	case "pet_type":
		return p.resolvePetType(ctx)
	case "pet_name":
		return p.resolvePetName(ctx)
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
		// On-demand revive if death timer has expired
		if char.DeadUntil != nil && time.Now().UTC().After(*char.DeadUntil) {
			char.Alive = true
			char.DeadUntil = nil
			if err := saveAdvCharacter(char); err != nil {
				slog.Error("adventure: on-demand revive failed", "user", char.UserID, "err", err)
			} else {
				p.SendDM(ctx.Sender, renderAdvRespawnDM(char))
			}
		}
		if !char.Alive {
			return p.SendDM(ctx.Sender, renderAdvDeathStatusDM(char))
		}
	}

	isHol, _ := isHolidayToday()
	if char.AllActionsUsed(isHol) {
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
			"You've used all your actions today. Rest now. Try again tomorrow.\n\n"+
				"Next action: 00:00 UTC (%dh %dm from now)\n\n"+
				"The Arena is always open: `!arena`",
			hours, minutes))
	}

	lower := strings.ToLower(body)

	// Parse "7" or "rest"
	if lower == "7" || lower == "rest" {
		return p.resolveRest(ctx, char)
	}

	// Parse "6" or "blacksmith"
	if lower == "6" || lower == "blacksmith" {
		return p.handleBlacksmithCmd(ctx)
	}

	// Parse "5" or "shop"
	if lower == "5" || lower == "shop" {
		return p.handleShopCmd(ctx, "")
	}

	// Phase R1 — legacy activity loop (1/dungeon, 2/mine, 3/forage, 4/fish + tier-or-name) is
	// deprecated. The new path is the expedition system. Anything that parses as a legacy
	// activity is intercepted here and routed to a deprecation notice; non-matches still
	// fall through to the unknown-command response so other plugins can pick it up.
	if isLegacyActivityInput(lower, char) {
		return p.SendDM(ctx.Sender, renderLegacyActivityDeprecation(char))
	}

	return p.SendDM(ctx.Sender, "I didn't understand that. Type `!adventure help` for commands, or `!expedition` to head into a zone.")
}

// isLegacyActivityInput reports whether `input` looks like a legacy activity-loop
// dispatch (1/dungeon, 2/mine, 3/forage, 4/fish, with or without tier/location).
// Used by the Phase R1 deprecation gate to redirect the player to !expedition
// without swallowing arbitrary unrelated DMs.
func isLegacyActivityInput(input string, char *AdventureCharacter) bool {
	if input == "" {
		return false
	}
	parts := strings.SplitN(input, " ", 2)
	first := parts[0]
	switch first {
	case "1", "2", "3", "4",
		"d", "m", "f",
		"dungeon", "mine", "forage", "forest", "fish", "fishing":
		return true
	}
	for _, act := range []AdvActivityType{AdvActivityDungeon, AdvActivityMining, AdvActivityForaging, AdvActivityFishing} {
		if findAdvLocation(act, input) != nil {
			return true
		}
	}
	return false
}

// renderLegacyActivityDeprecation is the Phase R1 deprecation DM. The standalone
// daily activity loop is gone; players now run expeditions and harvest inside
// rooms instead.
func renderLegacyActivityDeprecation(char *AdventureCharacter) string {
	var sb strings.Builder
	sb.WriteString("⚠️ **The daily adventure loop has retired.**\n\n")
	sb.WriteString("Solo dungeon runs, mining, foraging, and fishing are now part of the **expedition system** — pick a zone, advance through rooms, harvest as you go.\n\n")
	sb.WriteString("Get started:\n")
	sb.WriteString("• `!expedition` — overview & open expeditions\n")
	sb.WriteString("• `!expedition start <zone>` — begin a new run\n")
	sb.WriteString("• `!forage` · `!mine` · `!scavenge` · `!fish` · `!essence` · `!commune` — harvest in cleared rooms\n")
	sb.WriteString("• `!resources` — list nodes in your current room\n\n")
	sb.WriteString("Shop, blacksmith, rest, and Thom Krooke are still here on `!adventure`.")
	return sb.String()
}

func (p *AdventurePlugin) resolveRest(ctx MessageContext, char *AdventureCharacter) error {
	isHol, _ := isHolidayToday()

	// Rest consumes all remaining actions
	combatMax := maxCombatActions
	harvestMax := maxHarvestActions
	if isHol {
		combatMax++
		harvestMax++
	}
	char.CombatActionsUsed = combatMax
	char.HarvestActionsUsed = harvestMax
	char.ActionTakenToday = true
	char.LastActionDate = time.Now().UTC().Format("2006-01-02")

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

	return nil
}

// ── Treasure Drop Check ─────────────────────────────────────────────────────

func (p *AdventurePlugin) checkTreasureDrop(userID id.UserID, char *AdventureCharacter, loc *AdvLocation) {
	drop, roll, rate := rollAdvTreasureDropDetailed(loc.Tier, userID, p.chatLevel(userID))
	if drop == nil {
		// Near-miss feedback: when the roll was within 2× of the drop rate,
		// tell the player they almost got it. Treasure rates are 0.15–1.5%
		// so without this signal the system feels invisible.
		if rate > 0 && roll < rate*2 {
			p.SendDM(userID, fmt.Sprintf("🎁 *Treasure: just missed* — rolled %.2f%% against %.2f%% drop chance.",
				roll*100, rate*100))
		}
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

	// At cap — try the auto-swap path before falling back to the manual
	// discard prompt. The prompt was the wound; most players don't want
	// agency at the rare moment of finding a treasure, they want the swap.
	existing, err := advUserTreasures(userID)
	if err != nil || len(existing) == 0 {
		return
	}

	// Lock honors the player's "I'm done curating" state — refuse the drop
	// instead of churning their set.
	if char.TreasuresLocked {
		p.SendDM(userID, fmt.Sprintf("💎 *Treasure refused* — found **%s** (Tier %d) but your treasures are locked. Unlock with `!adventure treasures unlock` to allow swaps.",
			drop.Def.Name, drop.Def.Tier))
		return
	}

	// Pick the worst replaceable existing treasure as the swap candidate.
	// Irreplaceable treasures (special_* bonuses) are protected — never
	// auto-discarded, even by a higher-tier drop.
	worstIdx := -1
	var worstRank advTreasureRankKey
	for i := range existing {
		ex := existing[i]
		if advTreasureIrreplaceable(&ex) {
			continue
		}
		r := advTreasureRank(&ex)
		if worstIdx < 0 || advTreasureRankBetter(worstRank, r) {
			worstIdx, worstRank = i, r
		}
	}

	newRank := advTreasureRank(drop.Def)

	// All existing are irreplaceable — fall back to the manual prompt so
	// the player consciously chooses what to give up.
	if worstIdx < 0 {
		p.pending.Store(string(userID), &advPendingInteraction{
			Type: "treasure_discard",
			Data: &advPendingTreasureDiscard{
				NewTreasure: drop.Def,
				Existing:    existing,
			},
			ExpiresAt: time.Now().Add(advDMResponseWindow),
		})
		p.SendDM(userID, renderAdvTreasureDiscardPrompt(drop.Def, existing))
		return
	}

	// New treasure is irreplaceable but would force out a replaceable —
	// auto-swap is fine here, the protection rule is about not removing
	// existing irreplaceables.
	if !advTreasureRankBetter(newRank, worstRank) {
		// New is no better than the worst we'd give up. Don't churn —
		// just tell the player about the find passively.
		p.SendDM(userID, fmt.Sprintf("💎 *Found %s (Tier %d)* — left behind: your current set ranks higher.",
			drop.Def.Name, drop.Def.Tier))
		return
	}

	// Swap: discard worst, save new, store undo token.
	discarded := existing[worstIdx]
	if err := advDiscardTreasure(userID, discarded.Key); err != nil {
		slog.Error("adventure: failed to discard for auto-swap", "user", userID, "err", err)
		return
	}
	if err := advSaveTreasure(userID, drop.Def); err != nil {
		slog.Error("adventure: failed to save auto-swapped treasure", "user", userID, "err", err)
		// Best-effort recovery — restore the discarded one.
		_ = advSaveTreasure(userID, &discarded)
		return
	}

	// High-tier story items earn a public moment even via auto-swap, but
	// defer the announce until after the undo window so reversed swaps
	// don't get publicly announced. Capture the data we need on close
	// over so the timer doesn't reach back into mutable state.
	announceChar := *char
	announceDef := *drop.Def
	announceLoc := *loc
	announceTimer := time.AfterFunc(advTreasureUndoWindow, func() {
		// Token is deleted on undo, so only fire when it's still present.
		// The map entry also gets cleared lazily below by future undo
		// attempts; firing once based on Timer presence is enough.
		if _, ok := p.treasureUndo.Load(string(userID)); !ok {
			return
		}
		p.treasureUndo.Delete(string(userID))
		p.announceTreasureToRoom(&announceChar, &announceDef, &announceLoc)
	})

	p.treasureUndo.Store(string(userID), &advTreasureUndoToken{
		Discarded:     &discarded,
		Kept:          drop.Def,
		ExpiresAt:     time.Now().Add(advTreasureUndoWindow),
		AnnounceTimer: announceTimer,
	})

	p.SendDM(userID, fmt.Sprintf(
		"💎 **Found %s** (Tier %d)\nAuto-swapped for **%s** (Tier %d, lowest in your set).\n_Reply `undo` within %d min to revert, or `!adventure treasures` to review._",
		drop.Def.Name, drop.Def.Tier,
		discarded.Name, discarded.Tier,
		int(advTreasureUndoWindow.Minutes())))
}

// tryTreasureUndo reverses a recent auto-swap. Returns true when an undo
// was applied. Stale or missing tokens return false so the caller falls
// through to the regular DM dispatch.
func (p *AdventurePlugin) tryTreasureUndo(userID id.UserID) bool {
	val, ok := p.treasureUndo.Load(string(userID))
	if !ok {
		return false
	}
	tok := val.(*advTreasureUndoToken)
	p.treasureUndo.Delete(string(userID))
	// Cancel the deferred room announcement — a reversed swap shouldn't
	// produce a public "X recovered Y" line.
	if tok.AnnounceTimer != nil {
		tok.AnnounceTimer.Stop()
	}
	if time.Now().After(tok.ExpiresAt) {
		p.SendDM(userID, "⌛ The undo window expired. Your treasure swap stands.")
		return true
	}

	// Reverse: discard the kept one, restore the discarded one.
	if err := advDiscardTreasure(userID, tok.Kept.Key); err != nil {
		slog.Error("adventure: failed to discard kept on undo", "user", userID, "err", err)
		return true
	}
	if err := advSaveTreasure(userID, tok.Discarded); err != nil {
		slog.Error("adventure: failed to restore on undo", "user", userID, "err", err)
		return true
	}
	p.SendDM(userID, fmt.Sprintf("↩️ Reverted. Your **%s** is back, **%s** released.",
		tok.Discarded.Name, tok.Kept.Name))
	return true
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

	p.announceTreasureToRoom(char, def, loc)
}

// announceTreasureToRoom posts the public "X recovered Y from Z" notice for
// treasures that carry a RoomAnnounce string (typically tier 5 / story
// items). Called from both the normal discovery path and the auto-swap
// path so high-tier finds keep their public moment regardless of whether
// they arrived under or at the treasure cap.
func (p *AdventurePlugin) announceTreasureToRoom(char *AdventureCharacter, def *AdvTreasureDef, loc *AdvLocation) {
	if def == nil || def.RoomAnnounce == "" {
		return
	}
	gr := gamesRoom()
	if gr == "" {
		return
	}
	announce := advSubstituteFlavor(def.RoomAnnounce, map[string]string{
		"{name}":     char.DisplayName,
		"{location}": loc.Name,
	})
	p.SendMessage(id.RoomID(gr), announce)
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
		if !errors.Is(err, sql.ErrNoRows) {
			// Query error (e.g. missing column) — do NOT auto-create over existing data
			slog.Error("adventure: loadAdvCharacter failed", "user", userID, "err", err)
			return nil, nil, fmt.Errorf("failed to load character: %w", err)
		}
		// Genuinely new player — auto-create
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

// ── Double XP/Money Boost ───────────────────────────────────────────────────

const advBoostCacheKey = "adv_boost_active"

func advBoostActive() bool {
	return db.CacheGet(advBoostCacheKey, 365*86400) == "1"
}

func advSetBoost(active bool) {
	v := "0"
	if active {
		v = "1"
	}
	db.CacheSet(advBoostCacheKey, v)
}

func advApplyBoost(result *AdvActionResult) {
	if !advBoostActive() {
		return
	}
	result.XPGained *= 2
	result.TotalLootValue = 0
	for i := range result.LootItems {
		result.LootItems[i].Value *= 2
		result.TotalLootValue += result.LootItems[i].Value
	}
}

func (p *AdventurePlugin) handleRecipesCmd(ctx MessageContext) error {
	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}
	return p.SendDM(ctx.Sender, renderRecipesKnown(char.ForagingSkill))
}

func (p *AdventurePlugin) handleTreasuresCmd(ctx MessageContext, arg string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil || char == nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}

	switch arg {
	case "lock":
		char.TreasuresLocked = true
		if err := saveAdvCharacter(char); err != nil {
			slog.Error("adventure: failed to save treasures_locked", "user", ctx.Sender, "err", err)
			return p.SendDM(ctx.Sender, "Something went wrong. Try again.")
		}
		return p.SendDM(ctx.Sender, "🔒 **Treasures locked.** Drops at cap will be refused — no auto-swap. Unlock with `!adventure treasures unlock`.")
	case "unlock":
		char.TreasuresLocked = false
		if err := saveAdvCharacter(char); err != nil {
			slog.Error("adventure: failed to save treasures_locked", "user", ctx.Sender, "err", err)
			return p.SendDM(ctx.Sender, "Something went wrong. Try again.")
		}
		return p.SendDM(ctx.Sender, "🔓 **Treasures unlocked.** Higher-tier drops will auto-swap your lowest replaceable treasure (with 10-min undo).")
	case "":
		treasures, err := advUserTreasures(ctx.Sender)
		if err != nil {
			return p.SendDM(ctx.Sender, "Failed to load your treasures.")
		}
		return p.SendDM(ctx.Sender, renderAdvTreasureList(treasures, char.TreasuresLocked))
	default:
		return p.SendDM(ctx.Sender, "Usage: `!adventure treasures` (list) · `!adventure treasures lock` · `!adventure treasures unlock`")
	}
}

func (p *AdventurePlugin) handleMasteryCmd(ctx MessageContext) error {
	if _, _, err := p.ensureCharacter(ctx.Sender); err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}
	equip, err := loadAdvEquipment(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your equipment.")
	}
	return p.SendDM(ctx.Sender, renderAdvMasteryView(equip))
}

func (p *AdventurePlugin) handleBoostCmd(ctx MessageContext) error {
	if !p.IsAdmin(ctx.Sender) {
		return nil
	}

	if advBoostActive() {
		advSetBoost(false)
		return p.SendReply(ctx.RoomID, ctx.EventID, "⚡ Double XP/money boost **disabled**.")
	}
	advSetBoost(true)
	return p.SendReply(ctx.RoomID, ctx.EventID, "⚡ Double XP/money boost **enabled**! All adventure XP and loot values are doubled.")
}

