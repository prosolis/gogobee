package plugin

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

// ── Balance Constants ───────────────────────────────────────────────────────
//
// Phase 1 values. Adjust during balance pass. Co-op should feel meaningfully
// harder than solo and reward groups for showing up.

type CoopFundingTier string

const (
	CoopFundNone       CoopFundingTier = "none"
	CoopFundMinimal    CoopFundingTier = "minimal"
	CoopFundStandard   CoopFundingTier = "standard"
	CoopFundAggressive CoopFundingTier = "aggressive"
	CoopFundAllIn      CoopFundingTier = "all_in"
)

type coopFundingDef struct {
	cost     int
	modifier int // % added to per-floor success roll
	label    string
}

var coopFundingTable = map[CoopFundingTier]coopFundingDef{
	CoopFundNone:       {0, -10, "None"},
	CoopFundMinimal:    {500, 0, "Minimal"},
	CoopFundStandard:   {1500, 8, "Standard"},
	CoopFundAggressive: {4000, 18, "Aggressive"},
	CoopFundAllIn:      {10000, 30, "All In"},
}

// fundingTierOrder for menu display.
var coopFundingOrder = []CoopFundingTier{
	CoopFundMinimal, CoopFundStandard, CoopFundAggressive, CoopFundAllIn,
}

type coopTierDef struct {
	totalDays      int
	baseFailurePct int     // base % failure per floor at zero modifier
	rewardBase     int     // base reward (gold) before split
	difficulty     string  // label
	minLevel       int     // newbie liability threshold
	lootMult       float64 // for display only
}

// Reward bases tuned via Monte Carlo (coop_dungeon_balance_test.go) so the
// optimal funding strategy walks up tiers: T1-T2 Minimal/Standard, T3 Standard,
// T4 Standard or Aggressive, T5 Aggressive only. All-In stays -EV at every
// tier — it's the boost lever for liability parties, not the optimal play.
var coopTierTable = map[int]coopTierDef{
	1: {2, 25, 22500, "Moderate", 5, 2.0},
	2: {3, 32, 40000, "Hard", 12, 3.5},
	3: {4, 40, 72500, "Very Hard", 20, 5.5},
	4: {5, 48, 120000, "Brutal", 30, 8.0},
	5: {7, 58, 600000, "Murderous", 40, 12.0},
}

const (
	coopMinPartySize  = 2
	coopMaxPartySize  = 4
	coopInviteWindow  = 24 * time.Hour
	coopLiabilityCap  = 8 // liability players' funding modifier capped at +8% regardless of tier
	coopAdventureRake = 0.05
)

// ── Command Dispatch ────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleCoopCmd(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "coop"))
	lower := strings.ToLower(args)

	switch {
	case args == "" || lower == "help":
		return p.SendDM(ctx.Sender, coopHelpText)
	case lower == "list":
		return p.handleCoopList(ctx)
	case lower == "status":
		return p.handleCoopStatus(ctx)
	case lower == "stats":
		return p.SendDM(ctx.Sender, renderCoopStats())
	case strings.HasPrefix(lower, "start "):
		return p.handleCoopStart(ctx, strings.TrimSpace(args[6:]))
	case strings.HasPrefix(lower, "join"):
		return p.handleCoopJoin(ctx, strings.TrimSpace(strings.TrimPrefix(args, "join")))
	case strings.HasPrefix(lower, "fund "):
		return p.handleCoopFund(ctx, strings.TrimSpace(args[5:]))
	case lower == "cancel":
		return p.handleCoopCancel(ctx)
	case strings.HasPrefix(lower, "vote "):
		return p.handleCoopVote(ctx, strings.TrimSpace(args[5:]))
	case strings.HasPrefix(lower, "bet "):
		return p.handleCoopBet(ctx, strings.TrimSpace(args[4:]))
	case strings.HasPrefix(lower, "gift "):
		return p.handleCoopGift(ctx, strings.TrimSpace(args[5:]))
	case strings.HasPrefix(lower, "giftvote "):
		return p.handleCoopGiftVote(ctx, strings.TrimSpace(args[9:]))
	case strings.HasPrefix(lower, "admgift "):
		return p.handleCoopAdmGift(ctx, strings.TrimSpace(args[8:]))
	}

	return p.SendDM(ctx.Sender, "Unknown coop command. Try `!coop help`.")
}

const coopHelpText = `**Co-op Dungeon Commands**

` + "`!coop list`" + ` — Show open invites
` + "`!coop start <tier>`" + ` — Open an invite for a co-op dungeon (tier 1-5)
` + "`!coop join [<run_id>]`" + ` — Join an open invite (defaults to most recent)
` + "`!coop fund <tier>`" + ` — Set today's funding (none/minimal/standard/aggressive/all_in)
` + "`!coop vote <A|B|C>`" + ` — Vote on the day's floor event
` + "`!coop bet <amount> <success|failure>`" + ` — Spectator bet (parimutuel, 10% rake)
` + "`!coop gift <run_id> <basket|mimic>`" + ` — Send a gift (1 harvest action)
` + "`!coop giftvote <id> <open|leave>`" + ` — Party votes whether to open a gift
` + "`!coop status`" + ` — Show your current run
` + "`!coop stats`" + ` — Aggregate stats: outcomes, gold flow, betting, gifts, TwinBee helpfulness
` + "`!coop cancel`" + ` — Cancel an open invite (leader only, before lock)

**How it runs:**
- 24h invite window. Run locks automatically when window closes.
- 2-4 players. 2 minimum or invite cancels.
- Locking the party consumes the day's combat action for every member.
- Each subsequent day, every player picks a funding tier. Funding modifies
  the party's per-floor success chance.
- Inactive players (no funding decision) auto-play at None (-10%).
- Wipes refund the day's combat action only. Funding is gone.
- On success, the reward is split evenly. Tax applies.`

// ── Handlers ────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleCoopStart(ctx MessageContext, tierArg string) error {
	tier, err := strconv.Atoi(tierArg)
	if err != nil || tier < 1 || tier > 5 {
		return p.SendDM(ctx.Sender, "Tier must be 1-5. Example: `!coop start 3`.")
	}

	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}
	if !char.Alive {
		return p.SendDM(ctx.Sender, "You're dead. Co-op dungeons require a living adventurer.")
	}
	if !char.CanDoCombat(false) {
		return p.SendDM(ctx.Sender, "You've already used your combat action today. Try tomorrow.")
	}

	// One active run per player.
	if existing, _ := loadCoopRunForUser(ctx.Sender); existing != nil {
		return p.SendDM(ctx.Sender, fmt.Sprintf("You're already in a co-op run (#%d, %s). Use `!coop status`.", existing.ID, existing.Status))
	}

	def := coopTierTable[tier]
	run, err := createCoopRun(tier, ctx.Sender, def.totalDays, def.baseFailurePct)
	if err != nil {
		slog.Error("coop: create run", "err", err)
		return p.SendDM(ctx.Sender, "Couldn't open an invite. Try again.")
	}
	if err := addCoopMember(run.ID, ctx.Sender, 0, char.CombatLevel < def.minLevel); err != nil {
		slog.Error("coop: add leader", "err", err)
		return p.SendDM(ctx.Sender, "Couldn't add you to the run.")
	}

	gr := gamesRoom()
	if gr != "" {
		postID, err := p.SendMessageID(gr, renderCoopInvite(run, []CoopMember{{UserID: ctx.Sender, TurnOrder: 0}}, char.DisplayName))
		if err == nil {
			_ = setCoopRunInvitePostID(run.ID, postID)
			_ = p.PinEvent(gr, postID)
		}
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("⚔️ Tier %d co-op invite opened (#%d). Locks in 24h. Recruit your party.", tier, run.ID))
}

func (p *AdventurePlugin) handleCoopJoin(ctx MessageContext, runIDArg string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}
	if !char.Alive {
		return p.SendDM(ctx.Sender, "You're dead. The dungeon does not accept corpses.")
	}
	if !char.CanDoCombat(false) {
		return p.SendDM(ctx.Sender, "You've already used your combat action today. Try tomorrow.")
	}
	if existing, _ := loadCoopRunForUser(ctx.Sender); existing != nil {
		return p.SendDM(ctx.Sender, fmt.Sprintf("You're already in run #%d.", existing.ID))
	}

	var run *CoopRun
	if runIDArg == "" {
		run, err = loadMostRecentOpenCoopRun()
	} else {
		id, perr := strconv.Atoi(runIDArg)
		if perr != nil {
			return p.SendDM(ctx.Sender, "Run ID must be a number. Try `!coop join 7` or `!coop join`.")
		}
		run, err = loadCoopRun(id)
	}
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "No open invite found. Use `!coop list` to see open runs.")
	}
	if run.Status != "open" {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Run #%d is %s. Can't join.", run.ID, run.Status))
	}

	members, err := loadCoopMembers(run.ID)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load the party.")
	}
	if len(members) >= coopMaxPartySize {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Run #%d is full (%d/%d).", run.ID, len(members), coopMaxPartySize))
	}

	def := coopTierTable[run.Tier]
	liability := char.CombatLevel < def.minLevel
	if err := addCoopMember(run.ID, ctx.Sender, len(members), liability); err != nil {
		slog.Error("coop: join", "err", err)
		return p.SendDM(ctx.Sender, "Couldn't join.")
	}

	gr := gamesRoom()
	if gr != "" {
		warn := ""
		if liability {
			warn = " ⚠️ liability"
		}
		_ = p.SendMessage(gr, fmt.Sprintf("⚔️ %s joined Tier %d Co-op #%d (%d/%d)%s.",
			char.DisplayName, run.Tier, run.ID, len(members)+1, coopMaxPartySize, warn))
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("Joined run #%d. Wait for the lock; the run starts then.", run.ID))
}

func (p *AdventurePlugin) handleCoopList(ctx MessageContext) error {
	runs, err := loadOpenCoopRuns()
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load the list.")
	}
	if len(runs) == 0 {
		return p.SendDM(ctx.Sender, "No open invites right now. Start one with `!coop start <tier>`.")
	}
	var sb strings.Builder
	sb.WriteString("**Open Co-op Invites**\n\n")
	for _, r := range runs {
		members, _ := loadCoopMembers(r.ID)
		closesIn := time.Until(r.CreatedAt.Add(coopInviteWindow)).Truncate(time.Minute)
		sb.WriteString(fmt.Sprintf("  #%d  T%d (%s)  %d/%d  closes in %s\n",
			r.ID, r.Tier, coopTierTable[r.Tier].difficulty, len(members), coopMaxPartySize, closesIn))
	}
	sb.WriteString("\nJoin with `!coop join <id>` or just `!coop join` for the most recent.")
	return p.SendDM(ctx.Sender, sb.String())
}

func (p *AdventurePlugin) handleCoopStatus(ctx MessageContext) error {
	run, err := loadCoopRunForUser(ctx.Sender)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "You're not in a co-op run. Use `!coop list` or `!coop start <tier>`.")
	}
	members, _ := loadCoopMembers(run.ID)
	return p.SendDM(ctx.Sender, renderCoopStatus(p, run, members))
}

func (p *AdventurePlugin) handleCoopFund(ctx MessageContext, tierArg string) error {
	tier := parseCoopFundingTier(tierArg)
	if tier == "" {
		return p.SendDM(ctx.Sender, "Funding must be one of: none, minimal, standard, aggressive, all_in.")
	}

	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	run, err := loadCoopRunForUser(ctx.Sender)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "You're not in an active co-op run.")
	}
	if run.Status != "active" {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Run #%d is %s — funding decisions don't apply.", run.ID, run.Status))
	}

	day := run.CurrentDay
	if day < 1 {
		return p.SendDM(ctx.Sender, "The run hasn't started yet. Wait for the lock tick.")
	}

	member, err := loadCoopMember(run.ID, ctx.Sender)
	if err != nil || member == nil {
		return p.SendDM(ctx.Sender, "You're not on the party roster for this run.")
	}
	if _, alreadySet := member.DailyFunding[day]; alreadySet {
		return p.SendDM(ctx.Sender, fmt.Sprintf("You've already locked in funding for day %d. Decisions are final until the daily tick.", day))
	}

	cost := coopFundingTable[tier].cost
	if cost > 0 {
		balance := p.euro.GetBalance(ctx.Sender)
		if balance < float64(cost) {
			return p.SendDM(ctx.Sender, fmt.Sprintf("That tier costs €%d. You have €%.0f.", cost, balance))
		}
		if !p.euro.Debit(ctx.Sender, float64(cost), "coop_funding") {
			return p.SendDM(ctx.Sender, "Payment failed.")
		}
	}

	member.DailyFunding[day] = tier
	member.TotalContributed += cost
	if err := saveCoopMemberFunding(member); err != nil {
		// Refund and surface
		if cost > 0 {
			p.euro.Credit(ctx.Sender, float64(cost), "coop_funding_refund")
		}
		return p.SendDM(ctx.Sender, "Couldn't save your decision. Gold refunded if any was charged.")
	}
	if cost > 0 {
		if err := addCoopRunPool(run.ID, cost); err != nil {
			slog.Error("coop: pool update", "err", err)
		}
	}

	mod := coopFundingTable[tier].modifier
	return p.SendDM(ctx.Sender, fmt.Sprintf("Funding for day %d locked: %s (€%d, %+d%%). Tomorrow's tick resolves the floor.",
		day, coopFundingTable[tier].label, cost, mod))
}

func (p *AdventurePlugin) handleCoopCancel(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	run, err := loadCoopRunForUser(ctx.Sender)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "No co-op run to cancel.")
	}
	if run.LeaderID != ctx.Sender {
		return p.SendDM(ctx.Sender, "Only the party leader can cancel.")
	}
	if run.Status != "open" {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Run #%d already %s — too late.", run.ID, run.Status))
	}
	if err := setCoopRunStatus(run.ID, "cancelled"); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't cancel.")
	}
	gr := gamesRoom()
	if gr != "" {
		if run.InvitePostID != "" {
			_ = p.UnpinEvent(gr, run.InvitePostID)
		}
		_ = p.SendMessage(gr, fmt.Sprintf("⚔️ Tier %d Co-op #%d cancelled by the leader.", run.Tier, run.ID))
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("Run #%d cancelled.", run.ID))
}

// handleCoopAdmGift is an admin-only debug path that creates a gift on
// behalf of the caller (or a specified sender) without the harvest-action
// or party-membership checks. Useful for testing the gift mechanic without
// recruiting a non-party spectator.
//
// Usage: !coop admgift <run_id> <basket|mimic> [sender_user_id]
func (p *AdventurePlugin) handleCoopAdmGift(ctx MessageContext, args string) error {
	if !p.IsAdmin(ctx.Sender) {
		return nil
	}
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return p.SendDM(ctx.Sender, "Usage: `!coop admgift <run_id> <basket|mimic> [sender_user_id]`. Admin-only — bypasses party-member and harvest checks.")
	}
	runID, err := strconv.Atoi(parts[0])
	if err != nil {
		return p.SendDM(ctx.Sender, "Run ID must be a number.")
	}
	giftType := strings.ToLower(parts[1])
	if giftType != coopGiftBasket && giftType != coopGiftMimic {
		return p.SendDM(ctx.Sender, "Gift type must be `basket` or `mimic`.")
	}
	sender := ctx.Sender
	if len(parts) >= 3 {
		sender = id.UserID(parts[2])
	}

	run, err := loadCoopRun(runID)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "Run not found.")
	}
	if run.Status != "active" {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Run #%d is %s — gifts only land during active runs.", runID, run.Status))
	}

	giftID, err := createCoopGift(runID, sender, run.CurrentDay, giftType)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't create gift.")
	}

	gr := gamesRoom()
	if gr != "" {
		gift, _ := loadCoopGift(giftID)
		members, _ := loadCoopMembers(runID)
		if gift != nil {
			postID, perr := p.SendMessageID(gr, renderCoopGiftPost(p, run, gift, members))
			if perr == nil {
				_ = saveCoopGiftPostID(giftID, postID)
			}
		}
	}

	return p.SendDM(ctx.Sender, fmt.Sprintf("📦 Admin gift dispatched: %s from %s to run #%d (day %d). Gift id #%d. No harvest action consumed.",
		giftType, sender, runID, run.CurrentDay, giftID))
}

func (p *AdventurePlugin) handleCoopVote(ctx MessageContext, voteArg string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	run, err := loadCoopRunForUser(ctx.Sender)
	if err != nil || run == nil {
		return p.SendDM(ctx.Sender, "You're not in an active co-op run.")
	}
	if run.Status != "active" {
		return p.SendDM(ctx.Sender, "No active floor event to vote on.")
	}
	event, err := loadCoopEvent(run.ID, run.CurrentDay)
	if err != nil || event == nil {
		return p.SendDM(ctx.Sender, "No floor event found for the current day.")
	}
	if event.Outcome != "" {
		return p.SendDM(ctx.Sender, "Voting closed for this floor.")
	}
	label, ok := validateCoopVote(event.Category, event.EventIndex, voteArg)
	if !ok {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Vote one of: %s.",
			strings.Join(coopEventOptionLabels(event.Category, event.EventIndex), ", ")))
	}

	if event.Votes == nil {
		event.Votes = map[id.UserID]string{}
	}
	event.Votes[ctx.Sender] = label
	if err := saveCoopEventVotes(event); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save your vote.")
	}

	// Update the game-room post in place to show the running tally.
	gr := gamesRoom()
	if gr != "" && event.PostEventID != "" {
		members, _ := loadCoopMembers(run.ID)
		_ = p.EditMessage(gr, event.PostEventID, renderCoopEventPost(run, members, event))
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("Vote recorded: %s.", label))
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func parseCoopFundingTier(s string) CoopFundingTier {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	switch s {
	case "none", "skip", "pass":
		return CoopFundNone
	case "minimal", "min":
		return CoopFundMinimal
	case "standard", "std":
		return CoopFundStandard
	case "aggressive", "agg":
		return CoopFundAggressive
	case "all_in", "allin", "all":
		return CoopFundAllIn
	}
	return ""
}

// effectiveModifier returns a member's per-day funding modifier, respecting the
// liability cap.
func coopMemberModifier(m *CoopMember, day int) (CoopFundingTier, int) {
	tier, ok := m.DailyFunding[day]
	if !ok {
		tier = CoopFundNone
	}
	mod := coopFundingTable[tier].modifier
	if m.IsLiability && mod > coopLiabilityCap {
		mod = coopLiabilityCap
	}
	return tier, mod
}

// coopLevelBonus returns the per-floor success bonus from a player's combat
// level relative to the tier's minimum. Levels above the floor count up
// (capped); levels below count down. Encourages tier-appropriate parties.
func coopLevelBonus(combatLevel, tierMinLevel int) int {
	delta := combatLevel - tierMinLevel
	bonus := delta * 4 / 10 // 0.4× scale, integer math
	if bonus > 8 {
		bonus = 8
	}
	if bonus < -8 {
		bonus = -8
	}
	return bonus
}

// coopPetBonus returns the per-floor success bonus from a player's pet. Worth
// roughly a quarter of an additional baseline party member at pet level 10.
// Returns 0 if the player has no pet.
func coopPetBonus(char *AdventureCharacter) int {
	if !char.HasPet() || char.PetLevel <= 0 {
		return 0
	}
	bonus := char.PetLevel * 25 / 100 // 0.25× scale
	if bonus > 2 {
		bonus = 2
	}
	return bonus
}
