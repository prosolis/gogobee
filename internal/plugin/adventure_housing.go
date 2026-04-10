package plugin

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"
)

// ── Housing Tier Definitions ───────────────────────────────────────────────

type HouseTierDef struct {
	Tier        int
	Name        string
	Description string
	BasePrice   int
}

// HouseTier 0 = no house. Purchasable tiers are 1-4.
var houseTiers = []HouseTierDef{
	{Tier: 1, Name: "Base House", Description: "Four walls and a roof. 'Livable' is a strong word.", BasePrice: 75000},
	{Tier: 2, Name: "Livable", Description: "A place you can sit down in without questioning your life choices.", BasePrice: 150000},
	{Tier: 3, Name: "Comfortable", Description: "Walls that don't leak. A door that locks. Luxuries of the modern age.", BasePrice: 300000},
	{Tier: 4, Name: "Established", Description: "This is a house that says things about you. Good things. Mostly.", BasePrice: 600000},
}

// houseTierByTier returns the tier definition for a given tier number, or nil.
func houseTierByTier(tier int) *HouseTierDef {
	for i := range houseTiers {
		if houseTiers[i].Tier == tier {
			return &houseTiers[i]
		}
	}
	return nil
}

// houseNextTier returns the next purchasable tier definition, or nil if maxed.
func houseNextTier(currentTier int) *HouseTierDef {
	for i := range houseTiers {
		if houseTiers[i].Tier == currentTier+1 {
			return &houseTiers[i]
		}
	}
	return nil
}

// houseClosingCosts returns 5% closing + 6% realtor fees = 11% of base price.
func houseClosingCosts(basePrice int) int {
	return int(float64(basePrice) * 0.11)
}

// houseTotalCost returns base price + closing costs + realtor fees.
func houseTotalCost(basePrice int) int {
	return basePrice + houseClosingCosts(basePrice)
}

// houseWeeklyPayment calculates weekly payment based on balance and rate.
// Uses a simple amortization: interest + fixed principal portion.
// Rate is annual percentage (e.g., 6.5 means 6.5%).
func houseWeeklyPayment(balance int, annualRate float64) int {
	if balance <= 0 || annualRate <= 0 {
		return 0
	}
	weeklyRate := annualRate / 100.0 / 52.0
	interest := float64(balance) * weeklyRate
	// Principal portion: 0.5% of balance per week (~26% of balance per year)
	// Ensures loans pay off in roughly 2-4 years of active play.
	principal := float64(balance) * 0.005
	payment := int(interest + principal)
	if payment < 1 {
		payment = 1
	}
	return payment
}

// houseMissedPenalty returns the penalty for a missed payment (5% of weekly payment).
func houseMissedPenalty(weeklyPayment int) int {
	penalty := int(float64(weeklyPayment) * 0.05)
	if penalty < 1 {
		penalty = 1
	}
	return penalty
}

// ── Pending Interaction Types ──────────────────────────────────────────────

type advPendingHouseConfirm struct {
	Tier        int
	TotalCost   int
	DownPayment int
}

type advPendingHouseDownPayment struct {
	Tier      int
	TotalCost int
}

type advPendingHouseAutopay struct{}

type advPendingPetArrival struct{}

type advPendingPetType struct{}

type advPendingPetName struct {
	PetType string
}

// ── Thom Krooke Greeting ───────────────────────────────────────────────────

var thomGreetingsPreAdoption = []string{
	"Welcome in! Great timing. I've got something perfect for you. I say that to everyone and I mean it every time.",
	"You don't look like someone ready to make a smart investment. I can work with that. Sit down. Don't touch anything.",
	"Thom Krooke, Krooke Realty. No relation to that other guy. Mostly.",
}

var thomGreetingsPostAdoption = []string{
	"You're back. Good. I've got things.",
	"Oh. You. Sure. What do you need.",
	"Come in. Wipe your feet. How's the animal.",
}

var thomGreetingsPostLevel10 = []string{
	"Hello, {pet_name}'s caretaker. The usual?",
	"Ah. {pet_name}'s caretaker. I've been expecting you. I have things {pet_name} needs.",
	"Come in, {pet_name} and the caretaker! Wipe your feet! How are things? *You open your mouth to answer but quickly notice Thom is speaking to your pet.*",
}

var thomHouseSellingLines = []string{
	"Charming property. Full of character. 'Character' means different things to different people and that's what makes real estate exciting.",
	"Minor structural considerations that a positive attitude will absolutely address. Price reflects the opportunity.",
	"Previous owner left in a hurry. Didn't say why. The chalk outlines in the living room are almost certainly decorative. Kids do that now apparently.",
}

const thomAnimalLine = "I heard an animal got in your house. Luckily for you, it's a sweet one that will give you the world in exchange for a tiny bit of kindness. ...that's what I heard anyway."

var thomMissedPaymentNoPet = "Missed payment. Penalty applied. Balance is now €%d. Sort it out."
var thomMissedPaymentPet = "Missed payment. Penalty applied. Balance is now €%d. %s doesn't need to know about this. Keep it together."
var thomFreezeNoPet = "Ten missed payments. I've stopped trying for now. The house is yours. The debt is yours. Come see me when you're ready to talk about it."
var thomFreezePet = "Ten missed payments. I've stopped trying for now. The house is yours. The debt is yours. Come see me when you're ready to talk about it. %s is fine. You should work on being fine."
var thomPaidOffNoPet = "Paid off. Noted. The next tier is available when you're ready. I'll be here."
var thomPaidOffPet = "Paid off. Good. Come in when you're ready for the next tier. I've been thinking about what %s would need in a better space. I have thoughts."

// thomGreeting returns an appropriate greeting based on pet state.
func thomGreeting(char *AdventureCharacter) string {
	if char.HasPet() && char.PetLevel >= 10 {
		line := thomGreetingsPostLevel10[rand.IntN(len(thomGreetingsPostLevel10))]
		return strings.ReplaceAll(line, "{pet_name}", char.PetName)
	}
	if char.HasPet() {
		return thomGreetingsPostAdoption[rand.IntN(len(thomGreetingsPostAdoption))]
	}
	return thomGreetingsPreAdoption[rand.IntN(len(thomGreetingsPreAdoption))]
}

// ── Thom Shop Display ──────────────────────────────────────────────────────

func thomShopView(char *AdventureCharacter, balance float64) string {
	var sb strings.Builder

	sb.WriteString("🏠 **Krooke Realty**\n")
	sb.WriteString(fmt.Sprintf("💰 Balance: €%.0f\n\n", balance))
	sb.WriteString(fmt.Sprintf("*%s*\n\n", thomGreeting(char)))

	// Check for animal line (fires once after pet appears)
	if char.HasPet() && !char.ThomAnimalLineFired {
		sb.WriteString(fmt.Sprintf("*%s*\n\n", thomAnimalLine))
	}

	// Current housing status
	if !char.HasHouse() {
		sb.WriteString("You don't own a house yet.\n\n")
	} else {
		tierDef := houseTierByTier(char.HouseTier)
		tierName := "Unknown"
		if tierDef != nil {
			tierName = tierDef.Name
		}
		sb.WriteString(fmt.Sprintf("🏡 Your house: **%s** (Tier %d)\n", tierName, char.HouseTier))
		if char.HouseLoanBalance > 0 {
			weekly := houseWeeklyPayment(char.HouseLoanBalance, char.HouseCurrentRate)
			if char.HouseAutopay {
				weekly = int(float64(weekly) * 0.98) // 2% autopay discount
			}
			sb.WriteString(fmt.Sprintf("💳 Loan balance: €%d | Weekly: €%d", char.HouseLoanBalance, weekly))
			if char.HouseAutopay {
				sb.WriteString(" (autopay)")
			}
			if char.HouseLoanFrozen {
				sb.WriteString(" ❄️ FROZEN")
			}
			sb.WriteString(fmt.Sprintf(" | Rate: %.2f%%\n", char.HouseCurrentRate))
		}
		sb.WriteString("\n")
	}

	// Available upgrades
	if char.HouseLoanBalance > 0 {
		sb.WriteString("Pay off your current loan before upgrading.\n")
		sb.WriteString(fmt.Sprintf("\nTo pay off early: `!thom payoff`\n"))
		if !char.HouseAutopay {
			sb.WriteString("To enable autopay (2% discount): `!thom autopay`\n")
		}
	} else {
		// Can purchase next tier
		nextDef := houseNextTier(char.HouseTier)
		if nextDef == nil {
			sb.WriteString("✨ Max tier reached. There is nothing left to sell you. This has never happened before.\n")
		} else {
			def := *nextDef
			total := houseTotalCost(def.BasePrice)
			selling := thomHouseSellingLines[rand.IntN(len(thomHouseSellingLines))]
			sb.WriteString(fmt.Sprintf("*%s*\n\n", selling))
			sb.WriteString(fmt.Sprintf("📋 **%s** (Tier %d)\n", def.Name, def.Tier))
			sb.WriteString(fmt.Sprintf("   Base price: €%d\n", def.BasePrice))
			sb.WriteString(fmt.Sprintf("   Closing costs (5%%): €%d\n", int(float64(def.BasePrice)*0.05)))
			sb.WriteString(fmt.Sprintf("   Realtor fees (6%%): €%d\n", int(float64(def.BasePrice)*0.06)))
			sb.WriteString(fmt.Sprintf("   **Total: €%d**\n\n", total))
			sb.WriteString(fmt.Sprintf("Reply `buy` to purchase, or `buy <amount>` for a down payment.\n"))
		}
	}

	// Pet supply shop (unlocks 1 week after pet level 10)
	if char.PetSupplyShopUnlocked && char.HasPet() {
		sb.WriteString("\n---\n")
		sb.WriteString(fmt.Sprintf("🐾 **Pet Supplies** — for %s\n\n", char.PetName))
		sb.WriteString(petArmorShopView(char))
	}

	return sb.String()
}

// ── Thom Shop Command Handler ──────────────────────────────────────────────

func (p *AdventurePlugin) handleThomCmd(ctx MessageContext) error {
	args := strings.TrimSpace(p.GetArgs(ctx.Body, "thom"))

	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}
	balance := p.euro.GetBalance(ctx.Sender)

	lower := strings.ToLower(args)

	switch {
	case args == "" || lower == "shop":
		text := thomShopView(char, balance)
		// Mark animal line as fired if applicable
		if char.HasPet() && !char.ThomAnimalLineFired {
			userMu := p.advUserLock(ctx.Sender)
			userMu.Lock()
			char.ThomAnimalLineFired = true
			_ = saveAdvCharacter(char)
			userMu.Unlock()
		}
		return p.SendDM(ctx.Sender, text)

	case lower == "payoff":
		return p.handleThomPayoff(ctx, char)

	case strings.HasPrefix(lower, "pay "):
		return p.handleThomPay(ctx, char, strings.TrimSpace(lower[4:]))

	case lower == "autopay":
		return p.handleThomAutopay(ctx, char)

	case lower == "buy" || strings.HasPrefix(lower, "buy "):
		return p.handleThomBuy(ctx, char, balance, strings.TrimSpace(strings.TrimPrefix(lower, "buy")))

	case strings.HasPrefix(lower, "petbuy "):
		return p.handlePetArmorBuy(ctx, char, strings.TrimSpace(args[7:]))

	default:
		return p.SendDM(ctx.Sender, "Unknown command. Type `!thom` to visit Krooke Realty.")
	}
}

// ── Buy House ──────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleThomBuy(ctx MessageContext, char *AdventureCharacter, balance float64, downPaymentStr string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	// Reload fresh
	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}
	balance = p.euro.GetBalance(ctx.Sender)

	if char.HouseLoanBalance > 0 {
		return p.SendDM(ctx.Sender, "You still have an outstanding loan. Pay it off first with `!thom payoff`.")
	}

	nextDef := houseNextTier(char.HouseTier)
	if nextDef == nil {
		return p.SendDM(ctx.Sender, "You've reached max tier. There is nothing left to sell you.")
	}

	def := *nextDef
	totalCost := houseTotalCost(def.BasePrice)

	// Parse optional down payment
	downPayment := 0
	if downPaymentStr != "" {
		dp := 0
		for _, c := range downPaymentStr {
			if c < '0' || c > '9' {
				return p.SendDM(ctx.Sender, "Down payment must be a number. Example: `!thom buy 10000`")
			}
			dp = dp*10 + int(c-'0')
		}
		downPayment = dp
	}

	if downPayment > 0 && float64(downPayment) > balance {
		return p.SendDM(ctx.Sender, fmt.Sprintf("You can't afford a €%d down payment. Balance: €%.0f.", downPayment, balance))
	}

	if downPayment >= totalCost {
		// Full purchase, no loan
		if !p.euro.Debit(ctx.Sender, float64(totalCost), "house_purchase_full") {
			return p.SendDM(ctx.Sender, "Transaction failed.")
		}
		char.HouseTier = def.Tier
		if err := saveAdvCharacter(char); err != nil {
			p.euro.Credit(ctx.Sender, float64(totalCost), "house_purchase_refund")
			return p.SendDM(ctx.Sender, "Failed to save. You've been refunded.")
		}
		return p.SendDM(ctx.Sender, fmt.Sprintf("🏠 **%s** purchased outright for €%d.\n\nNo loan. No weekly payments. Thom is visibly disappointed.", def.Name, totalCost))
	}

	// Loan purchase
	loanAmount := totalCost - downPayment

	// Get current mortgage rate
	rate := getCurrentMortgageRate()
	if rate <= 0 {
		rate = 6.5 // fallback default
	}

	// Debit down payment if any
	if downPayment > 0 {
		if !p.euro.Debit(ctx.Sender, float64(downPayment), "house_down_payment") {
			return p.SendDM(ctx.Sender, "Transaction failed.")
		}
	}

	char.HouseTier = def.Tier
	char.HouseLoanBalance = loanAmount
	char.HouseCurrentRate = rate
	char.HouseMissedPayments = 0
	char.HouseLoanFrozen = false

	if err := saveAdvCharacter(char); err != nil {
		if downPayment > 0 {
			p.euro.Credit(ctx.Sender, float64(downPayment), "house_dp_refund")
		}
		return p.SendDM(ctx.Sender, "Failed to save. You've been refunded.")
	}

	weekly := houseWeeklyPayment(loanAmount, rate)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🏠 **%s** purchased!\n\n", def.Name))
	if downPayment > 0 {
		sb.WriteString(fmt.Sprintf("Down payment: €%d\n", downPayment))
	}
	sb.WriteString(fmt.Sprintf("Loan balance: €%d\n", loanAmount))
	sb.WriteString(fmt.Sprintf("Weekly payment: ~€%d (%.2f%% ARM rate)\n", weekly, rate))
	sb.WriteString("\nPayments are collected weekly. Enable autopay for a 2% discount: `!thom autopay`")

	return p.SendDM(ctx.Sender, sb.String())
}

// ── Payoff ─────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleThomPayoff(ctx MessageContext, char *AdventureCharacter) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	// Reload fresh
	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}

	if char.HouseLoanBalance <= 0 {
		return p.SendDM(ctx.Sender, "You don't have an outstanding loan.")
	}

	balance := p.euro.GetBalance(ctx.Sender)
	if balance < float64(char.HouseLoanBalance) {
		return p.SendDM(ctx.Sender, fmt.Sprintf("You need €%d to pay off the loan. Balance: €%.0f.", char.HouseLoanBalance, balance))
	}

	payoffAmount := float64(char.HouseLoanBalance)
	if !p.euro.Debit(ctx.Sender, payoffAmount, "house_payoff") {
		return p.SendDM(ctx.Sender, "Transaction failed.")
	}

	char.HouseLoanBalance = 0
	char.HouseLoanFrozen = false
	char.HouseMissedPayments = 0
	if err := saveAdvCharacter(char); err != nil {
		p.euro.Credit(ctx.Sender, payoffAmount, "house_payoff_refund")
		return p.SendDM(ctx.Sender, "Failed to save. Your money has been refunded.")
	}

	if char.HasPet() {
		return p.SendDM(ctx.Sender, fmt.Sprintf("🏠 %s", fmt.Sprintf(thomPaidOffPet, char.PetName)))
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("🏠 %s", thomPaidOffNoPet))
}

// ── Extra Payment ─────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleThomPay(ctx MessageContext, char *AdventureCharacter, amountStr string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}

	if char.HouseLoanBalance <= 0 {
		return p.SendDM(ctx.Sender, "You don't have an outstanding loan.")
	}

	amount, err := strconv.Atoi(amountStr)
	if err != nil || amount <= 0 {
		return p.SendDM(ctx.Sender, "Usage: `!thom pay <amount>` — e.g. `!thom pay 5000`")
	}

	// Cap at remaining balance
	if amount > char.HouseLoanBalance {
		amount = char.HouseLoanBalance
	}

	balance := p.euro.GetBalance(ctx.Sender)
	if balance < float64(amount) {
		return p.SendDM(ctx.Sender, fmt.Sprintf("You need €%d but only have €%.0f.", amount, balance))
	}

	if !p.euro.Debit(ctx.Sender, float64(amount), "house_extra_payment") {
		return p.SendDM(ctx.Sender, "Transaction failed.")
	}

	char.HouseLoanBalance -= amount
	if char.HouseLoanBalance <= 0 {
		char.HouseLoanBalance = 0
		char.HouseLoanFrozen = false
		char.HouseMissedPayments = 0
	}

	if err := saveAdvCharacter(char); err != nil {
		p.euro.Credit(ctx.Sender, float64(amount), "house_extra_payment_refund")
		return p.SendDM(ctx.Sender, "Failed to save. Your money has been refunded.")
	}

	if char.HouseLoanBalance == 0 {
		if char.HasPet() {
			return p.SendDM(ctx.Sender, fmt.Sprintf("🏠 Payment of €%d received. %s", amount, fmt.Sprintf(thomPaidOffPet, char.PetName)))
		}
		return p.SendDM(ctx.Sender, fmt.Sprintf("🏠 Payment of €%d received. %s", amount, thomPaidOffNoPet))
	}

	return p.SendDM(ctx.Sender, fmt.Sprintf("🏠 Payment of €%d received. Remaining balance: €%d.", amount, char.HouseLoanBalance))
}

// ── Autopay Toggle ─────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleThomAutopay(ctx MessageContext, char *AdventureCharacter) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}

	if char.HouseLoanBalance <= 0 {
		return p.SendDM(ctx.Sender, "You don't have an outstanding loan.")
	}

	char.HouseAutopay = !char.HouseAutopay
	if err := saveAdvCharacter(char); err != nil {
		return p.SendDM(ctx.Sender, "Failed to save.")
	}

	if char.HouseAutopay {
		return p.SendDM(ctx.Sender, "✅ Autopay enabled. 2% discount on weekly payments. Thom calls this a favour. It is not.")
	}
	return p.SendDM(ctx.Sender, "Autopay disabled.")
}

// ── Weekly Mortgage Payment Processing ─────────────────────────────────────

func (p *AdventurePlugin) processMortgagePayments() {
	chars, err := loadAllAdvCharacters()
	if err != nil {
		slog.Error("mortgage: failed to load characters", "err", err)
		return
	}

	for _, char := range chars {
		if char.HouseLoanBalance <= 0 {
			continue
		}
		if char.HouseLoanFrozen {
			continue
		}

		userMu := p.advUserLock(char.UserID)
		userMu.Lock()

		// Reload fresh inside lock
		freshChar, err := loadAdvCharacter(char.UserID)
		if err != nil || freshChar.HouseLoanBalance <= 0 || freshChar.HouseLoanFrozen {
			userMu.Unlock()
			continue
		}

		weekly := houseWeeklyPayment(freshChar.HouseLoanBalance, freshChar.HouseCurrentRate)
		if freshChar.HouseAutopay {
			weekly = int(float64(weekly) * 0.98)
		}

		if p.euro.Debit(freshChar.UserID, float64(weekly), "mortgage_weekly") {
			freshChar.HouseLoanBalance -= weekly
			if freshChar.HouseLoanBalance <= 0 {
				freshChar.HouseLoanBalance = 0
				freshChar.HouseMissedPayments = 0
				_ = saveAdvCharacter(freshChar)
				userMu.Unlock()
				// Paid off!
				if freshChar.HasPet() {
					_ = p.SendDM(freshChar.UserID, fmt.Sprintf("🏠 %s", fmt.Sprintf(thomPaidOffPet, freshChar.PetName)))
				} else {
					_ = p.SendDM(freshChar.UserID, fmt.Sprintf("🏠 %s", thomPaidOffNoPet))
				}
				continue
			}
			freshChar.HouseMissedPayments = 0
			_ = saveAdvCharacter(freshChar)
			userMu.Unlock()
		} else {
			// Missed payment
			penalty := houseMissedPenalty(weekly)
			freshChar.HouseLoanBalance += penalty
			freshChar.HouseMissedPayments++

			if freshChar.HouseMissedPayments >= 10 {
				freshChar.HouseLoanFrozen = true
				_ = saveAdvCharacter(freshChar)
				userMu.Unlock()
				if freshChar.HasPet() {
					_ = p.SendDM(freshChar.UserID, fmt.Sprintf("🏠 %s", fmt.Sprintf(thomFreezePet, freshChar.PetName)))
				} else {
					_ = p.SendDM(freshChar.UserID, fmt.Sprintf("🏠 %s", thomFreezeNoPet))
				}
			} else {
				_ = saveAdvCharacter(freshChar)
				userMu.Unlock()
				if freshChar.HasPet() {
					_ = p.SendDM(freshChar.UserID, fmt.Sprintf("🏠 %s", fmt.Sprintf(thomMissedPaymentPet, freshChar.HouseLoanBalance, freshChar.PetName)))
				} else {
					_ = p.SendDM(freshChar.UserID, fmt.Sprintf("🏠 %s", fmt.Sprintf(thomMissedPaymentNoPet, freshChar.HouseLoanBalance)))
				}
			}
		}
	}
}

// ── Mortgage Rate Change DMs ───────────────────────────────────────────────

func (p *AdventurePlugin) sendMortgageRateChangeDMs(oldRate, newRate float64) {
	if oldRate == newRate {
		return
	}

	chars, err := loadAllAdvCharacters()
	if err != nil {
		slog.Error("mortgage: failed to load characters for rate DMs", "err", err)
		return
	}

	increased := newRate > oldRate

	for _, char := range chars {
		if char.HouseLoanBalance <= 0 {
			continue
		}

		// Update rate
		userMu := p.advUserLock(char.UserID)
		userMu.Lock()
		freshChar, err := loadAdvCharacter(char.UserID)
		if err != nil || freshChar.HouseLoanBalance <= 0 {
			userMu.Unlock()
			continue
		}
		freshChar.HouseCurrentRate = newRate
		_ = saveAdvCharacter(freshChar)
		userMu.Unlock()

		newWeekly := houseWeeklyPayment(freshChar.HouseLoanBalance, newRate)
		amount := fmt.Sprintf("%d", newWeekly)

		var pool []string
		if freshChar.HasPet() {
			if increased {
				pool = ThomRateIncreasePet
			} else {
				pool = ThomRateDecreasePet
			}
		} else {
			if increased {
				pool = ThomRateIncrease
			} else {
				pool = ThomRateDecrease
			}
		}

		line := pool[rand.IntN(len(pool))]
		line = strings.ReplaceAll(line, "{amount}", amount)
		if freshChar.HasPet() {
			line = strings.ReplaceAll(line, "{pet_name}", freshChar.PetName)
		}

		_ = p.SendDM(char.UserID, fmt.Sprintf("🏠 %s", line))

		// Jitter
		time.Sleep(time.Duration(500+rand.IntN(1500)) * time.Millisecond)
	}
}

// ── Pet Armor Shop ─────────────────────────────────────────────────────────

type PetArmorDef struct {
	Tier         int
	Name         string
	DeflectBonus float64
	Price        int
}

var petDogArmor = []PetArmorDef{
	{Tier: 1, Name: "Leather Dog Barding", DeflectBonus: 0.015, Price: 225},
	{Tier: 2, Name: "Chain Dog Barding", DeflectBonus: 0.03, Price: 675},
	{Tier: 3, Name: "Plate Dog Barding", DeflectBonus: 0.05, Price: 2250},
	{Tier: 4, Name: "Enchanted Dog Barding", DeflectBonus: 0.075, Price: 11250},
	{Tier: 5, Name: "Dragonscale Dog Barding", DeflectBonus: 0.105, Price: 45000},
}

var petCatArmor = []PetArmorDef{
	{Tier: 1, Name: "Leather Cat Armor", DeflectBonus: 0.015, Price: 225},
	{Tier: 2, Name: "Chain Cat Armor", DeflectBonus: 0.03, Price: 675},
	{Tier: 3, Name: "Plate Cat Armor", DeflectBonus: 0.05, Price: 2250},
	{Tier: 4, Name: "Enchanted Cat Armor", DeflectBonus: 0.075, Price: 11250},
	{Tier: 5, Name: "Dragonscale Cat Armor", DeflectBonus: 0.105, Price: 45000},
}

func petArmorDefs(petType string) []PetArmorDef {
	if petType == "dog" {
		return petDogArmor
	}
	return petCatArmor
}

func petArmorShopView(char *AdventureCharacter) string {
	var sb strings.Builder
	defs := petArmorDefs(char.PetType)

	if char.PetArmorTier >= 5 {
		sb.WriteString("✨ Max pet armor tier reached. There is nothing left to buy.\n")
		return sb.String()
	}

	for _, def := range defs {
		if def.Tier <= char.PetArmorTier {
			if def.Tier == char.PetArmorTier {
				sb.WriteString(fmt.Sprintf("🟢 %s (T%d) — Currently equipped\n", def.Name, def.Tier))
			}
			continue
		}
		indicator := "⬆️"
		sb.WriteString(fmt.Sprintf("%s %s (T%d) — €%d — +%.1f%% deflect\n", indicator, def.Name, def.Tier, def.Price, def.DeflectBonus*100))
	}
	sb.WriteString(fmt.Sprintf("\nTo buy: `!thom petbuy <tier>` (e.g., `!thom petbuy %d`)", char.PetArmorTier+1))
	return sb.String()
}

func (p *AdventurePlugin) handlePetArmorBuy(ctx MessageContext, char *AdventureCharacter, tierStr string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	char, _, err := p.ensureCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Failed to load your character.")
	}

	if !char.HasPet() {
		return p.SendDM(ctx.Sender, "You don't have a pet.")
	}
	if !char.PetSupplyShopUnlocked {
		return p.SendDM(ctx.Sender, "Pet supplies aren't available yet.")
	}

	tier := 0
	for _, c := range tierStr {
		if c < '0' || c > '9' {
			return p.SendDM(ctx.Sender, "Usage: `!thom petbuy <tier>`")
		}
		tier = tier*10 + int(c-'0')
	}

	if tier != char.PetArmorTier+1 {
		if tier <= char.PetArmorTier {
			return p.SendDM(ctx.Sender, "That's not an upgrade.")
		}
		return p.SendDM(ctx.Sender, fmt.Sprintf("You need to buy tier %d first.", char.PetArmorTier+1))
	}

	defs := petArmorDefs(char.PetType)
	var def *PetArmorDef
	for i := range defs {
		if defs[i].Tier == tier {
			def = &defs[i]
			break
		}
	}
	if def == nil {
		return p.SendDM(ctx.Sender, "Invalid tier.")
	}

	balance := p.euro.GetBalance(ctx.Sender)
	if balance < float64(def.Price) {
		return p.SendDM(ctx.Sender, fmt.Sprintf("%s deserves better, but you can't afford €%d. Balance: €%.0f.", char.PetName, def.Price, balance))
	}

	if !p.euro.Debit(ctx.Sender, float64(def.Price), "pet_armor_"+tierStr) {
		return p.SendDM(ctx.Sender, "Transaction failed.")
	}

	char.PetArmorTier = tier
	if err := saveAdvCharacter(char); err != nil {
		p.euro.Credit(ctx.Sender, float64(def.Price), "pet_armor_refund")
		return p.SendDM(ctx.Sender, "Failed to save. Refunded.")
	}

	return p.SendDM(ctx.Sender, fmt.Sprintf("🐾 **%s** equipped on %s.\n\n+%.1f%% deflect bonus. %s looks... objectively better than you.", def.Name, char.PetName, def.DeflectBonus*100, char.PetName))
}
