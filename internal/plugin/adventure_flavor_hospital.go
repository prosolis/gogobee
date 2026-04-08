package plugin

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// ── St. Guildmore's Memorial Hospital — Nurse Joy Flavor Text ──────────────

// nurseJoyFirstVisit is shown once on the player's first hospital visit.
var nurseJoyFirstVisit = "🏥 **Welcome to St. Guildmore's Memorial Hospital!**\n\n" +
	"We're the official healthcare provider for members of the Adventurer's Guild. " +
	"Even better, we've modeled our pricing after the United States of America, " +
	"which means we offer only the quickest and best care. _whispers_ ...for those who can afford it.\n\n" +
	"Your Guild insurance is on file. Nurse Joy will be right with you.\n\n"

var nurseJoyAdmission = []string{
	"_looks up from clipboard with a bright smile_ Hi! Name? Guild card? Oh you're covered under the Collective, that's wonderful! Let me get Nurse Joy to come take a look at you. _is Nurse Joy_",
	"Oh you're with the Guild? That's great, we love Guild members! Very comprehensive plan. Someone will be right with you! _gestures at chairs_ Can I get you anything while you wait? We have water. It's complimentary. The water is complimentary.",
	"_already typing, still cheerful_ Cause of death? Temporary obviously, just for the record! And your Guild card number? Perfect! We'll get you all sorted out. _sorted out does not mean quickly but she means it warmly_",
	"Welcome to St. Guildmore's! You're in such good hands here. _has said this ten thousand times and means it every single time_ Insurance on file? Wonderful. Take a number and we'll be right with you!",
	"_looks up, immediately sympathetic in a clinical way_ Oh you don't look great! That's okay, that's what we're here for. Guild member? Amazing. Nurse Joy will be right out. _is Nurse Joy. will be right out._",
}

var nurseJoyBillDelivery = []string{
	"_slides it across with a smile_ So that's the total before insurance! Your plan is really good though -- the Collective is wonderful. We're going to call them right now. _already reaching for the phone, still smiling_",
	"Here's the full rate! _sets it down cheerfully_ Don't worry about that number, insurance is going to take care of so much of it. So much. Give us just a sec! _hums while dialing_",
	"That's what we call the pre-insurance figure! It's a big number but honestly that's just how we price things here. The Collective will bring it way down. Way down. _wanders off still visibly pleased with how this is going_",
	"_hands it over like she's giving you a gift_ Your Guild plan is really comprehensive -- one of the better ones we see honestly. Let me just get them on the line! _does not register your expression at all_",
}

// nurseJoyAfterInsurance — format with afterInsurance cost.
// The last entry has two %d placeholders (intentional — she says the number twice).
var nurseJoyAfterInsurance = []string{
	"_practically skips back in_ Great news! The Collective was amazing -- they covered so much of it. Your portion is only **€%d**! _says \"only\" like she's reporting a minor inconvenience_",
	"_beaming_ Okay so the insurance came through and honestly? Really good result today. You're looking at **€%d** out of pocket! That's after everything they covered. _genuinely proud of this outcome_",
	"_sits across from you, clasps hands_ I have such good news. The Collective took care of the bulk of it. Your share is just **€%d**! I know it sounds like a lot but you should see what it was before. _you have seen what it was before_ Actually you did see it. But still -- this is so much better!",
	"_slides over a new, smaller, still alarming piece of paper_ So they covered everything above your threshold! You just owe **€%d**. _taps it encouragingly_ And then you're all set! Good as new! _has never once considered that **€%d** might be a problem_",
}

var nurseJoyCantAfford = []string{
	"Oh! It looks like your balance isn't quite there today! _signals to the orderlies_ That's okay! We'll get you back to where we found you and you can rest up overnight! _waves as the stretcher is wheeled back out_",
	"Hmm! So the balance isn't quite covering it! No worries at all -- _already nodding to someone behind you_ -- the natural recovery process works just as well! We'll get you back to your ditch! Have a great night!",
	"Oh that's a little short! That's okay! _cheerfully_ We're going to pop you back where you were and you'll be right as rain tomorrow morning! _you are deposited back in the ditch_",
}

var nurseJoyNudge = []string{
	"Hi! This is Nurse Joy at St. Guildmore's Memorial! We noticed you haven't come in yet.\n\n" +
		"We have a bed ready and your insurance is all on file! We even have Derek standing by but that's only relevant if there's a billing issue and I'm sure there won't be!\n\n" +
		"Come in whenever you're ready! We're open! _we are always open_\n\n" +
		"Type `!hospital` to check in.",
}

var nurseJoyAlive = []string{
	"_looks up from clipboard_ You don't appear to be dead! That's great news! We love when people aren't dead. Come back if that changes! _cheerfully_",
	"_tilts head_ Our records show you're... alive? That's wonderful! We don't actually treat alive people here. It's a whole different department. _waves vaguely_",
}

var nurseJoyAlreadyRevived = "You seem to have recovered on your own! " +
	"That's the body's natural healing process at work. Beautiful thing. " +
	"No charge! _beams_"

// ── Room Announcements ───────────────────────────────────────────────────��─

var hospitalDischargeAnnounce = "🏥 %s has been discharged from St. Guildmore's Memorial Hospital. " +
	"Recovered. Back in action. The bill has been described as \"a lot.\""

var hospitalDitchAnnounce = "🏥 %s was brought into St. Guildmore's on a stretcher. " +
	"They have been returned to the ditch. They'll be fine tomorrow."

// ── Itemized Bill Generation ───────────────────────────────────────────────

type hospitalBillItem struct {
	Name string
	Note string  // optional italicized sub-line
	Min  float64 // min fraction of total
	Max  float64 // max fraction of total
}

// hospitalBillPool is the pool of possible line items. Each bill picks 8-12.
var hospitalBillPool = []hospitalBillItem{
	{"Resuscitation services (standard)", "", 0.04, 0.10},
	{"Resuscitation services (premium)", "(performed simultaneously with standard, non-optional)", 0.03, 0.08},
	{"Facility fee", "", 0.02, 0.05},
	{"Facility fee (after hours)", "(you died at 2pm. this is contested. we will not contest it.)", 0.02, 0.04},
	{"Physician consultation", "", 0.03, 0.06},
	{"Physician consultation (second opinion)", "(same physician. different hat.)", 0.03, 0.06},
	{"Gauze", "", 0.01, 0.02},
	{"Gauze (application fee)", "", 0.01, 0.02},
	{"Gauze (removal, future, pre-billed)", "", 0.01, 0.03},
	{"Convenience fee", "(thank you for allowing us to exist)", 0.02, 0.05},
	{"Inconvenience fee", "(deterrent to prevent emergency room abuse or usage in general)", 0.02, 0.05},
	{"Guild membership verification fee", "(your guild card was checked. this takes resources.)", 0.01, 0.03},
	{"Administrative processing", "", 0.01, 0.03},
	{"Spiritual realignment surcharge", "(your soul was briefly elsewhere. retrieval is billable.)", 0.02, 0.04},
	{"Bed occupancy charge", "(per minute, retroactive to estimated time of death)", 0.02, 0.05},
	{"Oxygen (ambient)", "(you breathed hospital air. the air is not free.)", 0.01, 0.03},
	{"Emotional support services", "(Nurse Joy smiled at you. that's a service.)", 0.01, 0.02},
	{"Post-mortem orientation fee", "(the pamphlet you did not read.)", 0.01, 0.02},
	{"Equipment sterilization", "(your gear was near our equipment. proximity counts.)", 0.01, 0.03},
	{"Discharge planning", "(we are already planning your discharge. this costs money.)", 0.01, 0.03},
}

// hospitalHolidaySurcharge is included only on holidays.
var hospitalHolidaySurcharge = hospitalBillItem{
	"Holiday surcharge", "(applied if death occurs on or adjacent to a recognized holiday)", 0.02, 0.02,
}

// hospitalFrequentCustomer is included when hospitalVisits > 0.
// It's a "discount" that is actually a positive charge. Because healthcare.
var hospitalFrequentCustomer = hospitalBillItem{
	"Frequent customer discount", "(thank you for your continued patronage.)", 0.01, 0.02,
}

// generateItemizedBill builds a procedural hospital bill that sums to the given total.
func generateItemizedBill(total int64, afterInsurance int64, hospitalVisits int, isHoliday bool) string {
	// Shuffle and pick 8-12 items from the pool
	pool := make([]hospitalBillItem, len(hospitalBillPool))
	copy(pool, hospitalBillPool)
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })

	count := 8 + rand.IntN(5) // 8-12
	if count > len(pool) {
		count = len(pool)
	}
	items := pool[:count]

	// Add conditional items
	if isHoliday {
		items = append(items, hospitalHolidaySurcharge)
	}
	if hospitalVisits > 0 {
		items = append(items, hospitalFrequentCustomer)
	}

	// Assign amounts
	type billLine struct {
		Name   string
		Note   string
		Amount int64
	}

	var lines []billLine
	var assigned int64

	for _, item := range items {
		frac := item.Min + rand.Float64()*(item.Max-item.Min)
		amount := int64(float64(total) * frac)
		if amount < 1 {
			amount = 1
		}
		lines = append(lines, billLine{Name: item.Name, Note: item.Note, Amount: amount})
		assigned += amount
	}

	// Miscellaneous absorbs the remainder
	misc := total - assigned
	if misc < 0 {
		// Over-assigned — scale everything down proportionally
		scale := float64(total) / float64(assigned)
		assigned = 0
		for i := range lines {
			lines[i].Amount = int64(float64(lines[i].Amount) * scale)
			if lines[i].Amount < 1 {
				lines[i].Amount = 1
			}
			assigned += lines[i].Amount
		}
		misc = total - assigned
	}
	if misc > 0 {
		lines = append(lines, billLine{
			Name:   "Miscellaneous",
			Note:   "",
			Amount: misc,
		})
	}

	// Find max name width for alignment
	maxName := 0
	for _, l := range lines {
		if len(l.Name) > maxName {
			maxName = len(l.Name)
		}
	}
	if maxName < 35 {
		maxName = 35
	}

	// Format
	var sb strings.Builder
	sb.WriteString("```\n")
	sb.WriteString("ST. GUILDMORE'S MEMORIAL HOSPITAL\n")
	sb.WriteString("─────────────────────────────────\n")

	for _, l := range lines {
		padding := maxName - len(l.Name) + 2
		if padding < 2 {
			padding = 2
		}
		sb.WriteString(l.Name)
		sb.WriteString(strings.Repeat(" ", padding))
		sb.WriteString(formatBillAmount(l.Amount))
		sb.WriteByte('\n')
		if l.Note != "" {
			sb.WriteString("    " + l.Note + "\n")
		}
	}

	sb.WriteString("─────────────────────────────────\n")
	totalLabel := "TOTAL (before insurance)"
	padding := maxName - len(totalLabel) + 2
	if padding < 2 {
		padding = 2
	}
	sb.WriteString(totalLabel)
	sb.WriteString(strings.Repeat(" ", padding))
	sb.WriteString(formatBillAmount(total))
	sb.WriteByte('\n')

	insLabel := "Guild Adventurer's Insurance"
	insPadding := maxName - len(insLabel) + 2
	if insPadding < 2 {
		insPadding = 2
	}
	sb.WriteString(insLabel)
	sb.WriteString(strings.Repeat(" ", insPadding))
	sb.WriteString("-" + formatBillAmount(total-afterInsurance))
	sb.WriteByte('\n')

	sb.WriteString("─────────────────────────────────\n")
	oweLabel := "AMOUNT DUE"
	owePadding := maxName - len(oweLabel) + 2
	if owePadding < 2 {
		owePadding = 2
	}
	sb.WriteString(oweLabel)
	sb.WriteString(strings.Repeat(" ", owePadding))
	sb.WriteString(formatBillAmount(afterInsurance))
	sb.WriteByte('\n')
	sb.WriteString("```")

	return sb.String()
}

// formatBillAmount formats an int64 as €X,XXX with comma separators.
func formatBillAmount(n int64) string {
	if n == 0 {
		return "€0"
	}
	neg := n < 0
	if neg {
		n = -n
	}

	// Format with commas
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		if neg {
			return "-€" + s
		}
		return "€" + s
	}

	// Insert commas from right
	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if result.Len() > 0 {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}

	if neg {
		return "-€" + result.String()
	}
	return "€" + result.String()
}
