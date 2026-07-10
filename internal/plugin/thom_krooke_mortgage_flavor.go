package plugin

// ── IMPORTANT: DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE ──
//
// This file contains hand-written flavor text. Every entry is the length it is
// on purpose. The voice, pacing, and specific wording are intentional.
// If an entry seems too long: it isn't. Leave it alone.
// If you are tempted to "clean up" or "condense" entries: don't.
// If your context window is too small to fit this file: that is your problem, not ours.
//
// ── THOM KROOKE — MORTGAGE RATE CHANGE NOTIFICATIONS ─────────────────────────
//
// Pulled weekly from FRED API (series: MORTGAGE5US).
// Rate change DM sent to player if rate differs from previous week.
// No change: no DM. Thom does not send neutral news.
//
// Pre-pet: terse, professional, zero warmth.
// Post-pet: still about the money, immediately about the pet.
// The pet is mentioned before you are. You are not mentioned.

// ── RATE INCREASE (NO PET) ────────────────────────────────────────────────────

var ThomRateIncrease = []string{
	"Rates adjusted. Your new weekly payment is €{amount}. Market conditions. You understand.",

	"Weekly update. Rates went up. Payment is now €{amount}. " +
		"This is not personal. It is also not not personal. Pay on time.",

	"The Fed moved. Your payment moved. €{amount} weekly going forward. " +
		"I don't make the rates. I just apply them promptly and without sympathy.",

	"Rate adjustment. €{amount} per week. " +
		"I'd say I'm sorry but the contract was quite clear about this possibility. " +
		"You signed it.",

	"Rates up. €{amount}. " +
		"If you'd like to discuss this I am available during business hours. " +
		"Business hours are whenever I feel like it.",
}

// ── RATE DECREASE (NO PET) ────────────────────────────────────────────────────

var ThomRateDecrease = []string{
	"Rates adjusted down. Your new weekly payment is €{amount}. Don't get used to it.",

	"Good news, technically. Rates dropped. €{amount} weekly going forward. " +
		"The market giveth. The market will taketh back. Enjoy it briefly.",

	"Rate adjustment in your favour this week. €{amount}. " +
		"I want to be clear that this has nothing to do with me. " +
		"The Fed did this. I merely informed you.",

	"Rates down. €{amount} per week. " +
		"You're welcome, even though I did nothing. Thom Krooke, Krooke Realty.",

	"Weekly update. Rates dropped. Your payment is €{amount}. " +
		"I've noted your relief. I've also noted that rates can go back up. " +
		"Good week.",
}

// ── RATE INCREASE (WITH PET) ──────────────────────────────────────────────────

var ThomRateIncreasePet = []string{
	"Rates went up. Weekly payment is now €{amount}. " +
		"You might need to skip a dinner or two and give yours to {pet_name} for a while. " +
		"Whether you eat or not is unimportant. Only {pet_name}'s stomach matters here.",

	"Rate adjustment. €{amount} per week. " +
		"I'd tighten the belt if I were you. " +
		"{pet_name} should not feel this. You should feel this. Adjust accordingly.",

	"Weekly update. Rates up. €{amount}. " +
		"I've been thinking about {pet_name}'s next armor upgrade. " +
		"You should also be thinking about that. After you figure out the payment. " +
		"Priority order is: {pet_name}, payment, everything else.",

	"Rates moved. Not in your favour. €{amount} weekly going forward. " +
		"{pet_name} doesn't need to know about this. Keep things normal for {pet_name}. " +
		"You'll figure out the rest.",

	"The Fed raised rates. Your payment is €{amount}. " +
		"Before you panic: {pet_name} is fine. {pet_name} will continue to be fine. " +
		"You may need to make some adjustments. {pet_name} will not.",

	"Rate increase. €{amount} per week. " +
		"I've seen people in your position make difficult choices. " +
		"Make sure none of those choices involve {pet_name}'s meals. " +
		"I'm watching.",
}

// ── RATE DECREASE (WITH PET) ──────────────────────────────────────────────────

var ThomRateDecreasePet = []string{
	"Rates went down. Weekly payment is now €{amount}. " +
		"You can buy better things for {pet_name} now. " +
		"I'd start with the shop. I may have restocked recently. For {pet_name}.",

	"Good news. Rates dropped. €{amount} going forward. " +
		"The difference between what you were paying and what you're paying now " +
		"should go directly toward {pet_name}. " +
		"I'm not telling you what to do. I'm telling you what to do.",

	"Rate adjustment in your favour. €{amount} weekly. " +
		"I thought of {pet_name} immediately when I saw the numbers. " +
		"You probably thought of yourself. " +
		"Think about what that says. Then go buy {pet_name} something.",

	"Rates down this week. Your payment is €{amount}. " +
		"I've already set aside some things in the shop that {pet_name} would benefit from. " +
		"The timing is not a coincidence. Come in when you're ready.",

	"Weekly update. Rates dropped. €{amount} per week going forward. " +
		"{pet_name} deserves something nice this week. " +
		"You've been holding back on the shop. I've noticed. " +
		"The rate drop is a sign. Take it.",

	"The Fed cut rates. Your payment is €{amount}. " +
		"I'm genuinely pleased about this, which is unusual for me. " +
		"{pet_name} is the reason I'm genuinely pleased about this. " +
		"You can draw your own conclusions.",
}

// ── FINAL PAYOFF — THE LAST HOUSE (D2 NPC ARC) ────────────────────────────────
//
// Sent once, when the Tier-4 "Established" mortgage hits zero — the last house
// Thom has to sell. There is no next tier, so this is the last time Thom has
// business with the player. A letter, and (if they keep a pet) a treat left in
// the bag. Substitute {pet} with strings.ReplaceAll at the call site.

var thomFinalLetterNoPet = "Paid off. All of it. There is no next tier -- you've bought the last house I had to sell you.\n\n" +
	"I've closed a great many mortgages. Thirty-one years, Krooke Realty. Most people I stop thinking about the hour the balance hits zero, and I have never once considered that a loss. I am writing to tell you that you appear to be an exception. I have not decided how I feel about that. I am telling you anyway, which should indicate something.\n\n" +
	"The door is yours, the roof is yours, the ground it stands on is yours. Nobody sends you a bill for it ever again. Walk through it whenever you like.\n\n" +
	"-- Thom Krooke, Krooke Realty"

var thomFinalLetterPet = "Paid off. All of it. There is no next tier -- you own it outright now, walls and roof and the ground it sits on.\n\n" +
	"I've enclosed something for {pet}. A treat. The good kind -- not the sort they sell by the sack, I had it sent from a place I trust. Give it to {pet} tonight and don't you dare ration it, and don't 'save it for a special occasion' either. Tonight IS the occasion. {pet} has spent this entire mortgage living in a house that wasn't finished being paid for, and now it is, and {pet} should know the difference even if {pet} can't be told in words.\n\n" +
	"You did a fine thing, getting {pet} a home that is truly yours. I won't say it a second time.\n\n" +
	"-- Thom Krooke, Krooke Realty"

// ── NO CHANGE (NEVER SENT) ────────────────────────────────────────────────────
// Thom does not send neutral news.
// If rate is unchanged week over week: no DM. No notification. Nothing.
