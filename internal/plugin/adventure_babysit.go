package plugin

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// ── Pricing ─────────────────────────────────────────────────────────────────

func babysitDailyCost(combatLevel int) int {
	return 100 + (combatLevel * 20)
}

// ── Pet-care daily trickle ─────────────────────────────────────────────────

// petXPPerBabysitDay is the daily pet XP awarded while a babysit subscription
// is active. Picked to be a meaningful but not overwhelming push toward L10:
// roughly equivalent to a couple of player-driven actions per day.
const petXPPerBabysitDay = 3

// runBabysitDailyTrickle grants daily pet XP while a babysit subscription is
// active and logs an entry for the end-of-service summary. Caller is
// responsible for saving the character afterwards.
func (p *AdventurePlugin) runBabysitDailyTrickle(char *AdventureCharacter) {
	if !char.BabysitActive {
		return
	}
	leveled := false
	if char.HasPet() && char.PetLevel < 10 {
		// Bypass petGrantXP's per-action constant — we want a flat trickle.
		char.PetXP += petXPPerBabysitDay * 100
		for char.PetLevel < 10 {
			needed := petXPToNextLevel(char.PetLevel) * 100
			if char.PetXP < needed {
				break
			}
			char.PetXP -= needed
			char.PetLevel++
			leveled = true
		}
		if char.PetLevel >= 10 && char.PetLevel10Date == "" {
			char.PetLevel10Date = time.Now().UTC().Format("2006-01-02")
		}
	}
	outcome := "pet_care"
	if leveled {
		outcome = "pet_care_levelup"
	}
	logBabysitActivity(char.UserID, "pet_care", outcome, 0, petXPPerBabysitDay, "")
}

// BabysitSafeRest reports whether the user has an active babysit subscription
// that should let standard camps qualify for fortified-tier rest perks.
// Returns false on any error (treat as no babysit). Safe to call from tests
// where the global DB has not been initialized — the panic is swallowed.
func BabysitSafeRest(userID id.UserID) (active bool) {
	defer func() {
		if r := recover(); r != nil {
			active = false
		}
	}()
	char, err := loadAdvCharacter(userID)
	if err != nil || char == nil || !char.BabysitActive {
		return false
	}
	if char.BabysitExpiresAt != nil && time.Now().UTC().After(*char.BabysitExpiresAt) {
		return false
	}
	return true
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
			"Hire a babysitter to look after your camp and tend the pet while you sleep:\n"+
			"  • Daily pet XP trickle (your pet still grows while you focus elsewhere)\n"+
			"  • Standard camps act like fortified ones — rest deeply, no need for boss-cleared rooms\n"+
			"  • Rival duels declined on your behalf\n\n"+
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
		return p.SendDM(ctx.Sender, fmt.Sprintf("🍼 The babysitting service costs %s for %d days. You have %s. The service has standards. Not many, but some.", fmtEuro(totalCost), days, fmtEuro(balance)))
	}

	if !p.euro.Debit(char.UserID, float64(totalCost), "babysit_purchase") {
		return p.SendDM(ctx.Sender, "Payment failed. The babysitter looked at your wallet and walked away.")
	}

	clearBabysitLogs(char.UserID)

	expires := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
	char.BabysitActive = true
	char.BabysitExpiresAt = &expires
	char.BabysitSkillFocus = "" // legacy field; no longer used

	if err := saveAdvCharacter(char); err != nil {
		slog.Error("babysit: failed to save character", "user", char.UserID, "err", err)
		p.euro.Credit(char.UserID, float64(totalCost), "babysit_refund")
		return p.SendDM(ctx.Sender, "Something went wrong activating the service. Your gold has been refunded.")
	}
	if err := upsertPlayerMetaBabysitState(char.UserID, babysitStateFromAdvChar(char)); err != nil {
		slog.Error("player_meta: babysit start dual-write failed", "user", char.UserID, "err", err)
	}

	confirm := pickBabysitFlavor(babysitConfirmLines)
	durLabel := "1 week"
	if days == 30 {
		durLabel = "1 month"
	}

	petLine := "No pet to tend yet — the babysitter will keep that in mind."
	if char.HasPet() {
		petLine = fmt.Sprintf("Pet: %s (L%d) — daily care included", char.PetName, char.PetLevel)
	}

	text := fmt.Sprintf("🍼 **Adventurer Babysitting Service — Activated**\n\n"+
		"Duration: %s (%d days)\n"+
		"Cost: €%d\n"+
		"%s\n"+
		"Camp safety: standard camps now rest like fortified ones\n"+
		"Rival duels: declined on your behalf\n\n"+
		"_%s_", durLabel, days, totalCost, petLine, confirm)

	return p.SendDM(ctx.Sender, text)
}

func (p *AdventurePlugin) handleBabysitStatus(ctx MessageContext) error {
	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "No adventurer found.")
	}

	if !char.BabysitActive {
		return p.SendDM(ctx.Sender, "🍼 No active babysitting service.\n\nUse `!adventure babysit week` or `!adventure babysit month` to start. The babysitter tends your pet daily and lets you rest deeply at standard camps.")
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

	logs, err := loadBabysitLogs(char.UserID)
	if err != nil {
		slog.Error("babysit: failed to load logs", "user", char.UserID, "err", err)
	}
	totalXP, petDays, rivalsRefused := babysitLogStats(logs)

	petLine := "No pet to tend"
	if char.HasPet() {
		petLine = fmt.Sprintf("%s (L%d)", char.PetName, char.PetLevel)
	}

	text := fmt.Sprintf("🍼 **Babysitting Service — Status**\n\n"+
		"Time remaining: %s\n"+
		"Pet under care: %s\n"+
		"Days of pet care given: %d\n"+
		"Pet XP trickled: %d\n"+
		"Rivals declined: %d",
		remaining, petLine, petDays, totalXP, rivalsRefused)

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

	logs, err := loadBabysitLogs(char.UserID)
	if err != nil {
		slog.Error("babysit: failed to load logs", "user", char.UserID, "err", err)
	}
	summary := renderBabysitSummary(char, logs)

	char.BabysitActive = false
	char.BabysitExpiresAt = nil
	char.BabysitSkillFocus = ""
	if err := saveAdvCharacter(char); err != nil {
		slog.Error("babysit: failed to save character on cancel", "user", char.UserID, "err", err)
	}
	if err := upsertPlayerMetaBabysitState(char.UserID, babysitStateFromAdvChar(char)); err != nil {
		slog.Error("player_meta: babysit cancel dual-write failed", "user", char.UserID, "err", err)
	}

	return p.SendDM(ctx.Sender, "🍼 Service cancelled. No refund. The babysitter was already there.\n\n"+summary)
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
		if err := upsertPlayerMetaBabysitState(char.UserID, babysitStateFromAdvChar(&char)); err != nil {
			slog.Error("player_meta: babysit expiry dual-write failed", "user", char.UserID, "err", err)
		}

		if err := p.SendDM(char.UserID, summary); err != nil {
			slog.Error("babysit: failed to send expiry summary DM", "user", char.UserID, "err", err)
		}
	}
}

// ── Summary Rendering ───────────────────────────────────────────────────────

func renderBabysitSummary(char *AdventureCharacter, logs []babysitLogEntry) string {
	totalXP, petDays, rivalsRefused := babysitLogStats(logs)

	var sb strings.Builder
	sb.WriteString("🍼 **BABYSITTING SERVICE — END OF REPORT**\n\n")
	sb.WriteString(fmt.Sprintf("Days of service: %d\n", petDays))
	if char.HasPet() {
		sb.WriteString(fmt.Sprintf("Pet looked after: %s (L%d)\n", char.PetName, char.PetLevel))
	} else {
		sb.WriteString("Pet looked after: none — the babysitter played solitaire.\n")
	}
	sb.WriteString(fmt.Sprintf("Pet XP trickled: %d\n", totalXP))

	if rivalsRefused > 0 {
		sb.WriteString(fmt.Sprintf("\nRival challenges: %d declined\n", rivalsRefused))
		for _, log := range logs {
			if log.RivalRefused != "" {
				line := pickBabysitFlavor(babysitRivalRefusalLines)
				sb.WriteString(fmt.Sprintf("  %s\n", fmt.Sprintf(line, log.RivalRefused, log.LogDate)))
			}
		}
	}

	sb.WriteString("\n" + pickBabysitFlavor(babysitDiaperLines))
	sb.WriteString("\n\nYour adventurer is fed and rested. The pet is suspiciously well-trained.")

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

// ── Stats helper ────────────────────────────────────────────────────────────

func babysitLogStats(logs []babysitLogEntry) (totalXP, petDays, rivalsRefused int) {
	for _, l := range logs {
		totalXP += l.XPGained
		if l.Activity == "pet_care" {
			petDays++
		}
		if l.RivalRefused != "" {
			rivalsRefused++
		}
	}
	return
}
