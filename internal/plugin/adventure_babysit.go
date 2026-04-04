package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// ── Pricing ─────────────────────────────────────────────────────────────────

func babysitDailyCost(combatLevel int) int {
	return 100 + (combatLevel * 20)
}

// ── Weakest Skill ───────────────────────────────────────────────────────────

func babysitWeakestSkill(char *AdventureCharacter) string {
	skills := []struct {
		name  string
		level int
	}{
		{"mining", char.MiningSkill},
		{"fishing", char.FishingSkill},
		{"foraging", char.ForagingSkill},
	}
	minLevel := skills[0].level
	for _, s := range skills[1:] {
		if s.level < minLevel {
			minLevel = s.level
		}
	}
	// Collect ties
	var tied []string
	for _, s := range skills {
		if s.level == minLevel {
			tied = append(tied, s.name)
		}
	}
	return tied[rand.IntN(len(tied))]
}

// skillToActivity maps a skill name to its activity type.
func skillToActivity(skill string) AdvActivityType {
	switch skill {
	case "mining":
		return AdvActivityMining
	case "fishing":
		return AdvActivityFishing
	case "foraging":
		return AdvActivityForaging
	}
	return AdvActivityMining
}

// ── Command Handlers ────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleBabysitCmd(ctx MessageContext, args string) error {
	lower := strings.ToLower(strings.TrimSpace(args))

	switch {
	case lower == "status":
		return p.handleBabysitStatus(ctx)
	case lower == "cancel":
		return p.handleBabysitCancel(ctx)
	case lower == "week":
		return p.handleBabysitPurchase(ctx, 7)
	case lower == "month":
		return p.handleBabysitPurchase(ctx, 30)
	default:
		return p.SendDM(ctx.Sender, "🍼 **Adventurer Babysitting Service**\n\n"+
			"`!adventure babysit week` — 7 days of service\n"+
			"`!adventure babysit month` — 30 days of service\n"+
			"`!adventure babysit status` — check service status\n"+
			"`!adventure babysit cancel` — cancel early (no refund)")
	}
}

func (p *AdventurePlugin) handleBabysitPurchase(ctx MessageContext, days int) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "No adventurer found. Type `!adventure` to create one.")
	}

	if char.BabysitActive {
		return p.SendDM(ctx.Sender, "🍼 The babysitter is already here. They're not leaving until the job is done.")
	}

	if !char.Alive {
		return p.SendDM(ctx.Sender, "Your adventurer is dead. The babysitter does not work with corpses.")
	}

	daily := babysitDailyCost(char.CombatLevel)
	totalCost := daily * days
	balance := p.euro.GetBalance(char.UserID)
	if balance < float64(totalCost) {
		return p.SendDM(ctx.Sender, fmt.Sprintf("🍼 The babysitting service costs €%d for %d days. You have €%.0f. The service has standards. Not many, but some.", totalCost, days, balance))
	}

	// Debit gold
	if !p.euro.Debit(char.UserID, float64(totalCost), "babysit_purchase") {
		return p.SendDM(ctx.Sender, "Payment failed. The babysitter looked at your wallet and walked away.")
	}

	// Set babysit fields
	skill := babysitWeakestSkill(char)
	expires := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
	char.BabysitActive = true
	char.BabysitExpiresAt = &expires
	char.BabysitSkillFocus = skill

	if err := saveAdvCharacter(char); err != nil {
		slog.Error("babysit: failed to save character", "user", char.UserID, "err", err)
		// Refund
		p.euro.Credit(char.UserID, float64(totalCost), "babysit_refund")
		return p.SendDM(ctx.Sender, "Something went wrong activating the service. Your gold has been refunded.")
	}

	confirm := pickBabysitFlavor(babysitConfirmLines)
	durLabel := "1 week"
	if days == 30 {
		durLabel = "1 month"
	}

	text := fmt.Sprintf("🍼 **Adventurer Babysitting Service — Activated**\n\n"+
		"Duration: %s (%d days)\n"+
		"Cost: €%d\n"+
		"Focus: %s (currently level %d)\n"+
		"Rival duels: declined on your behalf\n\n"+
		"Daily DMs are suspended until the service ends.\n\n"+
		"_%s_", durLabel, days, totalCost, titleCase(skill), babysitSkillLevel(char, skill), confirm)

	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleBabysitStatus(ctx MessageContext) error {
	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "No adventurer found.")
	}

	if !char.BabysitActive {
		return p.SendDM(ctx.Sender, "🍼 No active babysitting service. Use `!adventure babysit week` or `!adventure babysit month` to start.")
	}

	remaining := "unknown"
	if char.BabysitExpiresAt != nil {
		days := int(time.Until(*char.BabysitExpiresAt).Hours() / 24)
		if days < 1 {
			remaining = "less than a day"
		} else {
			remaining = fmt.Sprintf("%d days", days)
		}
	}

	// Load log stats
	logs, err := loadBabysitLogs(char.UserID)
	if err != nil {
		slog.Error("babysit: failed to load logs", "user", char.UserID, "err", err)
	}
	totalGold, totalXP, itemsClaimed, rivalsRefused := babysitLogStats(logs)

	text := fmt.Sprintf("🍼 **Babysitting Service — Status**\n\n"+
		"Time remaining: %s\n"+
		"Skill focus: %s\n"+
		"Days completed: %d\n"+
		"Gold earned: €%d\n"+
		"XP gained: %d\n"+
		"Items claimed by babysitter: %d\n"+
		"Rivals declined: %d",
		remaining, titleCase(char.BabysitSkillFocus), len(logs), totalGold, totalXP, itemsClaimed, rivalsRefused)

	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleBabysitCancel(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "No adventurer found.")
	}

	if !char.BabysitActive {
		return p.SendDM(ctx.Sender, "🍼 There's nothing to cancel. The babysitter isn't here.")
	}

	// Compile partial summary
	logs, err := loadBabysitLogs(char.UserID)
	if err != nil {
		slog.Error("babysit: failed to load logs", "user", char.UserID, "err", err)
	}
	summary := renderBabysitSummary(char, logs)

	// Clear babysit state
	char.BabysitActive = false
	char.BabysitExpiresAt = nil
	char.BabysitSkillFocus = ""
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("babysit: failed to save character on cancel", "user", char.UserID, "err", err)
	}

	// Clear logs
	clearBabysitLogs(char.UserID)

	return p.SendDM(ctx.Sender, "🍼 Service cancelled. No refund. The babysitter was already there.\n\n"+summary)
}

// ── Daily Auto-Resolution ───────────────────────────────────────────────────

func (p *AdventurePlugin) runBabysitDaily(char *AdventureCharacter) {
	activity := skillToActivity(char.BabysitSkillFocus)

	equip, err := loadAdvEquipment(char.UserID)
	if err != nil {
		slog.Error("babysit: failed to load equipment", "user", char.UserID, "err", err)
		return
	}

	// Pick highest-tier eligible location for the focus skill
	bonuses := &AdvBonusSummary{}
	eligible := advEligibleLocations(char, equip, activity, bonuses)
	if len(eligible) == 0 {
		slog.Warn("babysit: no eligible locations", "user", char.UserID, "skill", char.BabysitSkillFocus)
		return
	}
	// Pick the last one (highest tier since they're returned in order)
	loc := eligible[len(eligible)-1].Location
	inPenalty := eligible[len(eligible)-1].InPenaltyZone

	// Resolve action
	result := resolveAdvAction(char, equip, loc, bonuses, inPenalty)

	// Babysitter never lets the adventurer die — reroll death to empty
	if result.Outcome == AdvOutcomeDeath {
		result.Outcome = AdvOutcomeEmpty
		result.LootItems = nil
		result.TotalLootValue = 0
		result.EquipDamage = nil
		result.EquipBroken = nil
	}

	// Apply XP
	switch result.XPSkill {
	case "mining":
		char.MiningXP += result.XPGained
	case "foraging":
		char.ForagingXP += result.XPGained
	case "fishing":
		char.FishingXP += result.XPGained
	}
	checkAdvLevelUp(char, result.XPSkill)

	// Credit gold to player
	if result.TotalLootValue > 0 {
		p.euro.Credit(char.UserID, float64(result.TotalLootValue), "babysit_haul")
	}

	// Items are claimed by the babysitter (not added to player inventory)
	var itemNames []string
	for _, item := range result.LootItems {
		itemNames = append(itemNames, item.Name)
	}

	// No treasure drops during babysitting
	result.TreasureFound = nil

	// Mark action taken
	char.ActionTakenToday = true
	char.LastActionDate = time.Now().UTC().Format("2006-01-02")

	if err := saveAdvCharacter(char); err != nil {
		slog.Error("babysit: failed to save character after daily", "user", char.UserID, "err", err)
	}

	// Log to babysit table
	itemsJSON := ""
	if len(itemNames) > 0 {
		itemsJSON = strings.Join(itemNames, ", ")
	}
	logBabysitActivity(char.UserID, string(activity), string(result.Outcome),
		int(result.TotalLootValue), result.XPGained, itemsJSON)
}

// ── Expiry Check ────────────────────────────────────────────────────────────

func (p *AdventurePlugin) checkBabysitExpiry(chars []AdventureCharacter) {
	now := time.Now().UTC()
	for _, char := range chars {
		if !char.BabysitActive {
			continue
		}
		if char.BabysitExpiresAt == nil || now.Before(*char.BabysitExpiresAt) {
			continue
		}

		// Service expired — compile summary and send DM
		logs, err := loadBabysitLogs(char.UserID)
		if err != nil {
			slog.Error("babysit: failed to load logs", "user", char.UserID, "err", err)
		}
		summary := renderBabysitSummary(&char, logs)

		char.BabysitActive = false
		char.BabysitExpiresAt = nil
		char.BabysitSkillFocus = ""
		if err := saveAdvCharacter(&char); err != nil {
			slog.Error("babysit: failed to save character on expiry", "user", char.UserID, "err", err)
			continue
		}

		clearBabysitLogs(char.UserID)

		if err := p.SendDM(char.UserID, summary); err != nil {
			slog.Error("babysit: failed to send expiry summary DM", "user", char.UserID, "err", err)
		}
	}
}

// ── Summary Rendering ───────────────────────────────────────────────────────

func renderBabysitSummary(char *AdventureCharacter, logs []babysitLogEntry) string {
	totalGold, totalXP, itemsClaimed, rivalsRefused := babysitLogStats(logs)

	var sb strings.Builder
	sb.WriteString("🍼 **BABYSITTING SERVICE — END OF REPORT**\n\n")
	sb.WriteString(fmt.Sprintf("Duration: %d days\n", len(logs)))
	sb.WriteString(fmt.Sprintf("Tasks completed: %d\n", len(logs)))
	sb.WriteString(fmt.Sprintf("Skill focused: %s\n", titleCase(char.BabysitSkillFocus)))
	sb.WriteString(fmt.Sprintf("Gold earned from hauls: €%d\n", totalGold))
	sb.WriteString(fmt.Sprintf("XP gained: %d\n", totalXP))
	sb.WriteString(fmt.Sprintf("Items dropped: %d items. Claimed by the babysitter as per the terms.\n", itemsClaimed))

	if rivalsRefused > 0 {
		sb.WriteString(fmt.Sprintf("\nRival challenges: %d declined\n", rivalsRefused))
		// Pick a rival refusal flavor (generic — no specific rival name available)
		for _, log := range logs {
			if log.RivalRefused != "" {
				line := pickBabysitFlavor(babysitRivalRefusalLines)
				sb.WriteString(fmt.Sprintf("  %s\n", fmt.Sprintf(line, log.RivalRefused, log.LogDate)))
			}
		}
	}

	// Diaper line
	sb.WriteString("\n" + pickBabysitFlavor(babysitDiaperLines))

	// Closing
	sb.WriteString(fmt.Sprintf("\n\nYour adventurer is fed, rested, and slightly better at %s.", char.BabysitSkillFocus))

	return sb.String()
}

// ── Babysit Log CRUD ────────────────────────────────────────────────────────

type babysitLogEntry struct {
	ID            int64
	UserID        id.UserID
	LogDate       string
	Activity      string
	Outcome       string
	GoldEarned    int
	XPGained      int
	ItemsDropped  string
	RivalRefused  string
}

func logBabysitActivity(userID id.UserID, activity, outcome string, gold, xp int, items string) {
	d := db.Get()
	_, err := d.Exec(`INSERT INTO adventure_babysit_log (user_id, log_date, activity, outcome, gold_earned, xp_gained, items_dropped)
		VALUES (?, DATE('now'), ?, ?, ?, ?, ?)`,
		string(userID), activity, outcome, gold, xp, items)
	if err != nil {
		slog.Error("babysit: failed to log activity", "user", userID, "err", err)
	}
}

func logBabysitRivalRefusal(userID id.UserID, rivalName string) {
	d := db.Get()
	_, err := d.Exec(`INSERT INTO adventure_babysit_log (user_id, log_date, activity, outcome, rival_refused)
		VALUES (?, DATE('now'), 'rival_refused', 'declined', ?)`,
		string(userID), rivalName)
	if err != nil {
		slog.Error("babysit: failed to log rival refusal", "user", userID, "err", err)
	}
}

func loadBabysitLogs(userID id.UserID) ([]babysitLogEntry, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT id, user_id, log_date, activity, outcome, gold_earned, xp_gained,
		COALESCE(items_dropped,''), COALESCE(rival_refused,'')
		FROM adventure_babysit_log WHERE user_id = ? ORDER BY log_date`, string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []babysitLogEntry
	for rows.Next() {
		var l babysitLogEntry
		if err := rows.Scan(&l.ID, &l.UserID, &l.LogDate, &l.Activity, &l.Outcome,
			&l.GoldEarned, &l.XPGained, &l.ItemsDropped, &l.RivalRefused); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

func clearBabysitLogs(userID id.UserID) {
	d := db.Get()
	if _, err := d.Exec(`DELETE FROM adventure_babysit_log WHERE user_id = ?`, string(userID)); err != nil {
		slog.Error("babysit: failed to clear logs", "user", userID, "err", err)
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func babysitSkillLevel(char *AdventureCharacter, skill string) int {
	switch skill {
	case "mining":
		return char.MiningSkill
	case "fishing":
		return char.FishingSkill
	case "foraging":
		return char.ForagingSkill
	}
	return 0
}

func babysitLogStats(logs []babysitLogEntry) (totalGold, totalXP, itemsClaimed, rivalsRefused int) {
	for _, l := range logs {
		totalGold += l.GoldEarned
		totalXP += l.XPGained
		if l.ItemsDropped != "" {
			// Count comma-separated items
			itemsClaimed += len(strings.Split(l.ItemsDropped, ", "))
		}
		if l.RivalRefused != "" {
			rivalsRefused++
		}
	}
	return
}
