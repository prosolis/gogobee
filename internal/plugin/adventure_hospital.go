package plugin

import (
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

// ── Pending Interaction Types ──────────────────────────────────────────────

type advPendingHospitalPay struct {
	Cost       int64 // after-insurance amount (recomputed at confirm time for TOCTOU safety)
	Discounted bool  // pinned haggle outcome — when true, recomputed base is halved
	Waived     bool  // pinned nat-20 outcome — full waiver, no debit
}

// hospitalCostsForUser returns (beforeInsurance, afterInsurance) for the
// hospital revival bill. `Level × 5_000` after insurance, 5× that before.
// Floors at level 1.
func hospitalCostsForUser(char *AdventureCharacter) (int64, int64) {
	level := 1
	if dnd, err := LoadDnDCharacter(char.UserID); err == nil && dnd != nil && dnd.Level > 0 {
		level = dnd.Level
	}
	after := int64(level) * 5_000
	before := after * 5
	return before, after
}

// hospitalHaggleDC is the target for the CHA-based haggle check. Pass = 50% off.
const hospitalHaggleDC = 15

type hospitalHaggleOutcome int

const (
	haggleFumble hospitalHaggleOutcome = iota // nat 1 — auto-fail, full price
	haggleFail                                // d20+CHA < 15
	haggleWin                                 // d20+CHA ≥ 15
	haggleCrit                                // nat 20 — bill waived
)

// rollHospitalHaggle rolls d20 + CHA modifier vs DC 15. Returns the raw roll,
// the CHA modifier, and the outcome. Nat 1 is an auto-fumble (full price);
// nat 20 is an auto-crit (full waiver) regardless of modifier.
// Falls back to 0 mod if the DnD row can't be loaded.
func rollHospitalHaggle(userID id.UserID) (int, int, hospitalHaggleOutcome) {
	mod := 0
	if dnd, err := LoadDnDCharacter(userID); err == nil && dnd != nil {
		mod = abilityModifier(dnd.CHA)
	}
	r := rand.IntN(20) + 1
	switch {
	case r == 20:
		return r, mod, haggleCrit
	case r == 1:
		return r, mod, haggleFumble
	case r+mod >= hospitalHaggleDC:
		return r, mod, haggleWin
	default:
		return r, mod, haggleFail
	}
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

	// Compute costs (level × 5k after insurance, 5× before)
	beforeInsurance, baseAfterInsurance := hospitalCostsForUser(char)

	// Haggle: CHA save vs DC 15. Nat 20 waives the bill, nat 1 is auto-full,
	// otherwise pass = half off.
	haggleRoll, haggleMod, haggleOutcome := rollHospitalHaggle(ctx.Sender)
	afterInsurance := baseAfterInsurance
	switch haggleOutcome {
	case haggleCrit:
		afterInsurance = 0
	case haggleWin:
		afterInsurance /= 2
	}

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

	// Act 2 — The Bill (always shows the un-haggled after-insurance figure;
	// the haggle adjustment is announced separately below).
	sb.WriteString(generateItemizedBill(beforeInsurance, baseAfterInsurance, visits, isHol))
	sb.WriteString("\n\n")

	delivery, _ := advPickFlavor(nurseJoyBillDelivery, ctx.Sender, "hospital_delivery")
	sb.WriteString(delivery)
	sb.WriteString("\n\n───\n\n")

	// Act 3 — After Insurance (pre-haggle figure)
	afterText, _ := advPickFlavor(nurseJoyAfterInsurance, ctx.Sender, "hospital_after")
	// Some entries have two %d placeholders (she says the number twice)
	count := strings.Count(afterText, "%d")
	switch count {
	case 1:
		afterText = fmt.Sprintf(afterText, baseAfterInsurance)
	case 2:
		afterText = fmt.Sprintf(afterText, baseAfterInsurance, baseAfterInsurance)
	}
	sb.WriteString(afterText)
	sb.WriteString("\n\n───\n\n")

	// Act 4 — The Haggle. Nurse Joy lets you make a charisma save on your bill.
	total := haggleRoll + haggleMod
	flavor, _ := advPickFlavor(nurseJoyHaggleFlavor[haggleOutcome], ctx.Sender, fmt.Sprintf("hospital_haggle_%d", haggleOutcome))
	var verdict string
	switch haggleOutcome {
	case haggleCrit:
		verdict = "🎉 **NAT 20 — bill waived!**"
	case haggleWin:
		verdict = "✅ *Passed!* The bill is halved."
	case haggleFumble:
		verdict = "💀 **NAT 1 — auto-fail.** Full price stands."
	default:
		verdict = "❌ *Failed.* Full price stands."
	}
	sb.WriteString(fmt.Sprintf(
		"🎲 **Haggle Check (CHA DC %d)**\n"+
			"You attempt to charm Nurse Joy into a discount...\n"+
			"d20 = **%d**, CHA mod %+d → **total %d** vs DC %d\n"+
			"%s\n\n%s\n\n───\n\n",
		hospitalHaggleDC, haggleRoll, haggleMod, total, hospitalHaggleDC, verdict, flavor))

	// Payment prompt — show base → final adjustment when it changed.
	sb.WriteString("**St. Guildmore's Memorial Hospital**\n")
	switch haggleOutcome {
	case haggleCrit:
		sb.WriteString(fmt.Sprintf("Original: ~~%s~~ → **WAIVED**\n", fmtEuro(baseAfterInsurance)))
		sb.WriteString("Amount due: **€0**\n\n")
	case haggleWin:
		sb.WriteString(fmt.Sprintf("Original: ~~%s~~ → **%s** (haggled)\n", fmtEuro(baseAfterInsurance), fmtEuro(afterInsurance)))
		sb.WriteString(fmt.Sprintf("Amount due: **%s**\n\n", fmtEuro(afterInsurance)))
	default:
		sb.WriteString(fmt.Sprintf("Amount due: **%s**\n\n", fmtEuro(afterInsurance)))
	}
	sb.WriteString("Pay now? (yes / no)\n\n")
	sb.WriteString("*Note: Declining payment will result in discharge to the natural respawn queue. " +
		"You'll be back tomorrow. The chair in the waiting room is available in the meantime.*")

	// Store pending interaction
	p.pending.Store(string(ctx.Sender), &advPendingInteraction{
		Type:      "hospital_pay",
		Data: &advPendingHospitalPay{
			Cost:       afterInsurance,
			Discounted: haggleOutcome == haggleWin,
			Waived:     haggleOutcome == haggleCrit,
		},
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

		// Recompute cost from current DnD level (don't trust cached value).
		// The haggle outcome was pinned at prompt time — re-applying it here
		// preserves the discount even if level changed mid-flow, and prevents
		// any reroll exploit.
		_, cost := hospitalCostsForUser(char)
		waived := false
		if pay, ok := interaction.Data.(*advPendingHospitalPay); ok {
			switch {
			case pay.Waived:
				cost = 0
				waived = true
			case pay.Discounted:
				cost /= 2
			}
		}

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

		// Debit (skipped on a nat-20 waiver)
		if !waived {
			if !p.euro.Debit(ctx.Sender, float64(cost), "hospital_revival") {
				text, _ := advPickFlavor(nurseJoyCantAfford, ctx.Sender, "hospital_broke")
				return p.SendDM(ctx.Sender, text)
			}

			// Community health fund: 10% of revival cost.
			if potCut := int(math.Round(float64(cost) * 0.1)); potCut > 0 {
				communityPotAdd(potCut)
				trackTaxPaid(ctx.Sender, potCut)
			}
		}

		// Revive. Restore DnDCharacter HP to full (canonical post-L5 path);
		// AdvCharacter.Alive flip + HospitalVisits++ propagate through the
		// saveAdvCharacter fan-out into player_meta.
		char.Alive = true
		char.DeadUntil = nil
		char.HospitalVisits++
		if err := saveAdvCharacter(char); err != nil {
			slog.Error("hospital: failed to save character after revival", "user", char.UserID, "err", err)
			// Refund on save failure (no-op when nothing was debited)
			if !waived {
				p.euro.Credit(ctx.Sender, float64(cost), "hospital_revival_refund")
			}
			return p.SendDM(ctx.Sender, "Something went wrong during recovery. Your payment has been refunded.")
		}
		if dnd, err := LoadDnDCharacter(char.UserID); err == nil && dnd != nil {
			dnd.HPCurrent = dnd.HPMax
			if err := SaveDnDCharacter(dnd); err != nil {
				slog.Error("hospital: failed to restore DnDCharacter HP", "user", char.UserID, "err", err)
			}
		}

		// Discharge DM
		if waived {
			p.SendDM(ctx.Sender, "✅ **Discharged — you're alive!**\n\n"+
				"Nurse Joy waved the deductible entirely. _\"On the house, sweetie. "+
				"You earned it.\"_ Derek the billing orderly looks personally offended.\n\n"+
				"Go get 'em. Type `!adventure` when you're ready.")
		} else {
			p.SendDM(ctx.Sender, fmt.Sprintf(
				"✅ **Discharged — you're alive!**\n\n"+
					"%s deducted. Nurse Joy wishes you the best. "+
					"She means it. She always means it.\n\n"+
					"Go get 'em. Type `!adventure` when you're ready.",
				fmtEuro(cost)))
		}

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
