package plugin

// ── Robbie the Friendly Bandit — Flavor Text ─────────────────────────────────

var robbieOpenings = []string{
	"Ello there! Robbie here. I had a little look at your inventory and -- well. " +
		"I'm just going to say it. You've been holdin' onto some gear that's well below your station " +
		"and I can't in good conscience let that stand. So I've gone ahead and sorted it for ya. You're welcome.",

	"_tips hat_ Robbie, at your service. I noticed you had some items in there that were frankly " +
		"bringing down the average and I've taken the liberty of relieving you of them. " +
		"Left ya somethin' for the trouble. Fair's fair.",

	"Right so. I was havin' a look -- as I do -- and I couldn't help but notice the state of your inventory. " +
		"No judgment! Well. Some judgment. I've handled it. Check your gold.",

	"Hiya! It's Robbie. You had some things you didn't need. I took 'em. " +
		"I left ya €%d for the lot. I also donated everything to a good cause " +
		"so technically you're a philanthropist now. You're welcome for that too.",
}

var robbieClosings = []string{
	"Right then. I'll be off. Don't go hoarding gear again now -- I'll know. _taps nose_",

	"Pleasure doing business. As always, Robbie leaves things better than he found them. " +
		"Debatable, but that's his position and he's sticking to it.",

	"You're all sorted. Don't thank me. Actually -- do thank me. I worked hard today.",

	"_already gone by the time you read this_",
}

var robbieMasterworkGotCard = "Now THIS -- _holds it up_ -- this is a lovely piece. " +
	"I'm not going to pretend otherwise. I've left you something special for this one. " +
	"Consider it a bonus from Robbie. Don't spend it all at once. _winks and disappears_"

var robbieMasterworkAlreadyHas = "Oh. I see you already have one of these. Excellent. " +
	"I'll keep this baby for myself then -- in case one of yous wakes up again and gives me a wallopping. o_o"

var robbieAllShopGear = "Nothing fancy today but that's alright. Clean inventory is its own reward. " +
	"Well. The €%d is the reward. The clean inventory is a bonus. Cheerio!"

// ── Room Announcements ───────────────────────────────────────────────────────

var robbieRoomStandard = "🎩 Robbie paid %s a visit and collected %d item(s) from their inventory. " +
	"€%d left behind. Everything donated to the community pot. " +
	"Robbie considers this a net positive for all parties."

var robbieRoomMasterworkCard = "🎩 Robbie paid %s a visit and helped himself to a Masterwork piece. " +
	"He left a Get Out of Medical Debt Free card and €%d. Robbie's words: \"Fair's fair.\""

var robbieRoomMasterworkAlreadyHas = "🎩 Robbie paid %s a visit, found they already had a " +
	"Get Out of Medical Debt Free card, and pocketed the Masterwork's card for himself. " +
	"He still left €%d. Robbie is if nothing else consistent."
