package plugin

import (
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

// ── Pending Interaction Types ──────────────────────────────────────────────

type advPendingHospitalPay struct {
	Cost int64 // after-insurance amount (recomputed at confirm time for TOCTOU safety)
}

// hospitalCostsForUser returns (beforeInsurance, afterInsurance) for the
// hospital revival bill. Phase L4a switches the formula off CombatLevel
// onto DnDCharacter.Level: `Level × 50_000` after insurance, 5× that
// before. Post-L5g every legacy player has a DnDCharacter row, so the
// CombatLevel-derived fallback has been retired. Floors at level 1.
func hospitalCostsForUser(char *AdventureCharacter) (int64, int64) {
	level := 1
	if dnd, err := LoadDnDCharacter(char.UserID); err == nil && dnd != nil && dnd.Level > 0 {
		level = dnd.Level
	}
	after := int64(level) * 50_000
	before := after * 5
	return before, after
}

// ── Hospital Command ───────────────────────────────────────────────────────

func (p *AdventurePlugin) handleHospitalCmd(ctx MessageContext) error {
	char, err := loadAdvCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "You're not registered for Adventure yet. Type `!adventure` to begin.")
	}

	// If alive, reject
	if char.Alive {
		text, _ := advPickFlavor(nurseJoyAlive, ctx.Sender, "hospital_alive")
		return p.SendDM(ctx.Sender, text)
	}

	// On-demand revive if death timer already expired
	if char.DeadUntil != nil && time.Now().UTC().After(*char.DeadUntil) {
		char.Alive = true
		char.DeadUntil = nil
		if err := saveAdvCharacter(char); err != nil {
			slog.Error("hospital: on-demand revive failed", "user", char.UserID, "err", err)
			return p.SendDM(ctx.Sender, "Something went wrong at the hospital. Try again in a moment.")
		}
		if dnd, err := LoadDnDCharacter(char.UserID); err == nil && dnd != nil {
			dnd.HPCurrent = dnd.HPMax
			if err := SaveDnDCharacter(dnd); err != nil {
				slog.Error("hospital: failed to restore DnDCharacter HP on natural revive", "user", char.UserID, "err", err)
			}
		}
		return p.SendDM(ctx.Sender, nurseJoyAlreadyRevived)
	}

	// Compute costs (Phase L4a: DnD level × 50k after insurance, 5× before)
	beforeInsurance, afterInsurance := hospitalCostsForUser(char)

	// Check balance
	balance := p.euro.GetBalance(ctx.Sender)
	if balance < float64(afterInsurance) {
		// Can't afford — back to the ditch
		text, _ := advPickFlavor(nurseJoyCantAfford, ctx.Sender, "hospital_broke")
		p.SendDM(ctx.Sender, text)

		// Room announcement
		gr := gamesRoom()
		if gr != "" {
			name, _ := loadDisplayName(char.UserID)
			p.SendMessage(gr, fmt.Sprintf(hospitalDitchAnnounce, name))
		}
		return nil
	}

	// Check if holiday for bill surcharge
	isHol, _ := isHolidayToday()

	// Build the full multi-act DM
	var sb strings.Builder

	// Phase L4a: HospitalVisits sourced from player_meta (with AdvCharacter
	// fallback during soak).
	visits, _ := loadHospitalVisits(char.UserID)

	// Act 1 — Admission
	if visits == 0 {
		sb.WriteString(nurseJoyFirstVisit)
	}

	admission, _ := advPickFlavor(nurseJoyAdmission, ctx.Sender, "hospital_admit")
	sb.WriteString(admission)
	sb.WriteString("\n\n")

	// Act 2 — The Bill
	sb.WriteString(generateItemizedBill(beforeInsurance, afterInsurance, visits, isHol))
	sb.WriteString("\n\n")

	delivery, _ := advPickFlavor(nurseJoyBillDelivery, ctx.Sender, "hospital_delivery")
	sb.WriteString(delivery)
	sb.WriteString("\n\n───\n\n")

	// Act 3 — After Insurance
	afterText, _ := advPickFlavor(nurseJoyAfterInsurance, ctx.Sender, "hospital_after")
	// Some entries have two %d placeholders (she says the number twice)
	count := strings.Count(afterText, "%d")
	switch count {
	case 1:
		afterText = fmt.Sprintf(afterText, afterInsurance)
	case 2:
		afterText = fmt.Sprintf(afterText, afterInsurance, afterInsurance)
	}
	sb.WriteString(afterText)
	sb.WriteString("\n\n")

	// Payment prompt
	sb.WriteString(fmt.Sprintf("**St. Guildmore's Memorial Hospital**\nAmount due: **%s**\n\n", fmtEuro(afterInsurance)))
	sb.WriteString("Pay now? (yes / no)\n\n")
	sb.WriteString("*Note: Declining payment will result in discharge to the natural respawn queue. " +
		"You'll be back tomorrow. The chair in the waiting room is available in the meantime.*")

	// Store pending interaction
	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "hospital_pay",
		Data:      &advPendingHospitalPay{Cost: afterInsurance},
		ExpiresAt: time.Now().Add(advDMResponseWindow),
	})
	p.advMarkMenuSent(ctx.Sender)

	return p.SendDM(ctx.Sender, sb.String())
}

// ── Payment Resolution ─────────────────────────────────────────────────────

func (p *AdventurePlugin) resolveHospitalPay(ctx MessageContext, interaction *advPendingInteraction) error {
	reply := strings.TrimSpace(strings.ToLower(ctx.Body))

	if reply == "yes" || reply == "y" {
		// Acquire lock
		mu := p.advUserLock(ctx.Sender)
		mu.Lock()
		defer mu.Unlock()

		// Reload fresh (TOCTOU protection)
		char, err := loadAdvCharacter(ctx.Sender)
		if err != nil {
			return p.SendDM(ctx.Sender, "Something went wrong loading your character.")
		}

		// Already alive (or timer expired while they were deciding)?
		if char.Alive {
			return p.SendDM(ctx.Sender, nurseJoyAlreadyRevived)
		}
		if char.DeadUntil != nil && time.Now().UTC().After(*char.DeadUntil) {
			char.Alive = true
			char.DeadUntil = nil
			_ = saveAdvCharacter(char)
			if dnd, err := LoadDnDCharacter(char.UserID); err == nil && dnd != nil {
				dnd.HPCurrent = dnd.HPMax
				_ = SaveDnDCharacter(dnd)
			}
			return p.SendDM(ctx.Sender, nurseJoyAlreadyRevived)
		}

		// Recompute cost from current DnD level (don't trust cached value)
		_, cost := hospitalCostsForUser(char)

		// Check balance
		balance := p.euro.GetBalance(ctx.Sender)
		if balance < float64(cost) {
			text, _ := advPickFlavor(nurseJoyCantAfford, ctx.Sender, "hospital_broke")
			p.SendDM(ctx.Sender, text)

			gr := gamesRoom()
			if gr != "" {
				name, _ := loadDisplayName(char.UserID)
			p.SendMessage(gr, fmt.Sprintf(hospitalDitchAnnounce, name))
			}
			return nil
		}

		// Debit
		if !p.euro.Debit(ctx.Sender, float64(cost), "hospital_revival") {
			text, _ := advPickFlavor(nurseJoyCantAfford, ctx.Sender, "hospital_broke")
			return p.SendDM(ctx.Sender, text)
		}

		// Community health fund: 10% of revival cost.
		if potCut := int(math.Round(float64(cost) * 0.1)); potCut > 0 {
			communityPotAdd(potCut)
			trackTaxPaid(ctx.Sender, potCut)
		}

		// Revive. Restore DnDCharacter HP to full (canonical post-L5 path);
		// AdvCharacter.Alive flip + HospitalVisits++ propagate through the
		// saveAdvCharacter fan-out into player_meta.
		char.Alive = true
		char.DeadUntil = nil
		char.HospitalVisits++
		if err := saveAdvCharacter(char); err != nil {
			slog.Error("hospital: failed to save character after revival", "user", char.UserID, "err", err)
			// Refund on save failure
			p.euro.Credit(ctx.Sender, float64(cost), "hospital_revival_refund")
			return p.SendDM(ctx.Sender, "Something went wrong during recovery. Your payment has been refunded.")
		}
		if dnd, err := LoadDnDCharacter(char.UserID); err == nil && dnd != nil {
			dnd.HPCurrent = dnd.HPMax
			if err := SaveDnDCharacter(dnd); err != nil {
				slog.Error("hospital: failed to restore DnDCharacter HP", "user", char.UserID, "err", err)
			}
		}

		// Discharge DM
		p.SendDM(ctx.Sender, fmt.Sprintf(
			"✅ **Discharged — you're alive!**\n\n"+
				"%s deducted. Nurse Joy wishes you the best. "+
				"She means it. She always means it.\n\n"+
				"Go get 'em. Type `!adventure` when you're ready.",
			fmtEuro(cost)))

		// Room announcement
		gr := gamesRoom()
		if gr != "" {
			name, _ := loadDisplayName(char.UserID)
			descriptor, _ := advPickFlavor(hospitalDischargeBillDescriptors, ctx.Sender, "hospital_discharge_bill")
			p.SendMessage(gr, fmt.Sprintf(hospitalDischargeAnnouncePrefix, name, descriptor))
		}

		return nil
	}

	// Declined or anything else — back to the ditch
	p.SendDM(ctx.Sender, "Understood. Nurse Joy nods cheerfully and signals the orderlies. "+
		"You're wheeled back to where they found you.\n\n"+
		"Natural respawn will occur in due time. Rest up.")

	char, err := loadAdvCharacter(ctx.Sender)
	if err == nil {
		gr := gamesRoom()
		if gr != "" {
			name, _ := loadDisplayName(char.UserID)
			p.SendMessage(gr, fmt.Sprintf(hospitalDitchAnnounce, name))
		}
	}

	return nil
}

// ── Hospital Ad (sent after death) ─────────────────────────────────────────

func (p *AdventurePlugin) sendHospitalAd(userID id.UserID, char *AdventureCharacter) {
	beforeInsurance, afterInsurance := hospitalCostsForUser(char)
	respawnTime := "unknown"
	if char.DeadUntil != nil {
		respawnTime = char.DeadUntil.Format("15:04")
	}

	text := fmt.Sprintf(
		"🏥 **St. Guildmore's Memorial Hospital**\n\n"+
			"Nurse Joy has been notified. A bed is being prepared.\n\n"+
			"Type `!hospital` to check in for same-day revival.\n"+
			"Estimated bill: %s (Guild insurance covers %s → you pay %s)\n\n"+
			"Or rest up — natural respawn at %s UTC.",
		fmtEuro(beforeInsurance), fmtEuro(beforeInsurance-afterInsurance), fmtEuro(afterInsurance), respawnTime)

	time.AfterFunc(10*time.Second, func() {
		if err := p.SendDM(userID, text); err != nil {
			slog.Error("hospital: failed to send hospital ad", "user", userID, "err", err)
			return
		}
		// Schedule nudge for 2 hours from now — checked by hospitalNudgeTicker
		p.hospitalNudges.Store(string(userID), time.Now().UTC().Add(2*time.Hour))
	})
}

// ── Hospital Nudge Ticker ──────────────────────────────────────────────────
// Runs every minute, checks for nudges that are due. No long-lived goroutines.

func (p *AdventurePlugin) hospitalNudgeTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			now := time.Now().UTC()
			p.hospitalNudges.Range(func(key, val any) bool {
				uid := key.(string)
				nudgeAt := val.(time.Time)
				if now.Before(nudgeAt) {
					return true
				}

				// Due — remove regardless of outcome
				p.hospitalNudges.Delete(uid)

				userID := id.UserID(uid)
				char, err := loadAdvCharacter(userID)
				if err != nil || char.Alive {
					return true
				}

				// Don't nudge if already in hospital flow
				if v, ok := p.pending.Load(uid); ok {
					if pi, ok := v.(*advPendingInteraction); ok && pi.Type == "hospital_pay" {
						return true
					}
				}

				text, _ := advPickFlavor(nurseJoyNudge, userID, "hospital_nudge")
				if err := p.SendDM(userID, text); err != nil {
					slog.Error("hospital: failed to send nudge", "user", userID, "err", err)
				}
				return true
			})
		}
	}
}
