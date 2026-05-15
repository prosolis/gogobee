// twinbee_housing_flavor.go
// Housing system narration and Pastel babysitter notes.
// Includes Thom Krooke mortgage/rent announcements, property events,
// and Pastel's daily notes to the player across all level tiers.
//
// Voice conventions (Phase B2):
//   - TwinBee narration: first-person / implicit subject ONLY. No
//     third-person "TwinBee [verb]" references.
//   - Thom Krooke speaks in third person about himself ("Thom Krooke
//     thanks you"). That's his established voice; leave it.
//   - Pastel speaks in first person. Leave it.
// Add new entries freely. Don't shorten existing entries.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// THOM KROOKE — PROPERTY ACQUISITION
// ─────────────────────────────────────────────────────────────────────────────

var ThomKrookeRentConfirm = []string{
	"Welcome, welcome! Your room is ready and the key is under the mat — well, there isn't a mat, but you understand the spirit of the thing. Rent processes weekly. Thom Krooke thanks you for choosing to stay!",
	"Excellent! A rented room is a wonderful first step. Modest, yes, but full of potential — like all beginnings. Your payment schedule is attached. Thom Krooke looks forward to a long and pleasant arrangement!",
	"The apartment is yours for the week! Everything is in order. The previous tenant left a small plant. Thom Krooke has chosen not to elaborate on the previous tenant. Enjoy the plant!",
}

var ThomKrookeBuyConfirm = []string{
	"Congratulations! The property is yours! Fully, completely, no asterisks — well, the mortgage paperwork has some asterisks, but they are the friendly kind. Thom Krooke is so very pleased for you!",
	"The deed is signed! A wonderful day. A property of one's own is a foundation, a root, a place to come back to. Thom Krooke finds this very moving. The first payment processes Sunday!",
	"Welcome to ownership! Thom Krooke has handled many transactions but never tires of this moment — the moment when someone says yes to something permanent. Congratulations. Truly.",
}

var ThomKrookeMortgageRate = []string{
	"Good morning, friends! The ARM rate this week is {rate}% — and with Thom Krooke's modest service margin, your mortgage rate sits at {effective}%. All payments process Sunday. Thank you for your continued trust!",
	"Weekly rate update! FRED reports {rate}% this week, so your effective rate with Thom Krooke is {effective}%. Nothing to worry about — Thom Krooke monitors these things so you don't have to. Mostly.",
	"Rate check! The market says {rate}%, Thom Krooke adds a small, reasonable {margin}%, and your total comes to {effective}%. Thom Krooke appreciates your understanding of the margin. It keeps the lights on. Literally!",
}

var ThomKrookeMortgageRateUp = []string{
	"A small update, friends — the ARM rate has moved to {rate}% this week, bringing your effective rate to {effective}%. Thom Krooke understands this is not ideal news. Thom Krooke is here if you'd like to discuss refinancing options. Thom Krooke is always here.",
	"The rate has adjusted upward — {rate}% from FRED, {effective}% total. Thom Krooke wants to assure you this is a market condition and not personal. Your payment adjusts next Sunday. Thom Krooke has full confidence in you.",
}

var ThomKrookeMortgageRateDown = []string{
	"Wonderful news! The ARM rate has come down to {rate}%, meaning your effective rate is now {effective}%. Your payment adjusts favorably on Sunday. Thom Krooke passes along the good news and takes no credit for the market. Only a little credit.",
	"The rate dropped this week — {rate}%, so {effective}% for you. Thom Krooke loves weeks like this. Everyone wins. Well — Thom Krooke wins slightly less, but Thom Krooke finds generosity its own reward.",
}

var ThomKrookeMissedPayment1 = []string{
	"Hello! A small notice — this week's payment didn't come through. Thom Krooke assumes it's an oversight. These things happen! A 10% penalty has been added to the balance. Please settle when you can. Thom Krooke is not worried. Thom Krooke is a little worried.",
	"Just a gentle reminder — Sunday's payment was missed. Thom Krooke has noted it and added a small fee. No urgency! Well. Some urgency. Thom Krooke would appreciate hearing from you.",
}

var ThomKrookeMissedPayment2 = []string{
	"Thom Krooke is visiting. Not in an alarming way — in a neighborly way. The second missed payment has been noted and the penalty has compounded. Thom Krooke would like to discuss options. Thom Krooke has brought a small pastry. The pastry is not a bribe. It is hospitality. There is a difference.",
	"Two payments outstanding now. Thom Krooke appears at your door with the expression of someone who is being very patient and would like credit for being very patient. 'Let's talk,' Thom says. The pastry is genuinely good.",
}

var ThomKrookeDefault = []string{
	"Thom Krooke is very sorry. Three payments missed is, unfortunately, the threshold — the property reverts to Thom Krooke's management, and your equity is returned at the agreed 50% rate. Thom Krooke takes no pleasure in this. Thom Krooke has placed your possessions in storage. They will be there for seven days. Thom Krooke wishes you well and means it sincerely and hopes you'll come back when you're ready.",
}

var ThomKrookeEarlyPayoff = []string{
	"Paid in full! Thom Krooke notes this with genuine delight — early payoff is a rare and admirable thing. The property is yours, free and clear. No more Sundays. No more rates. Just a home. Thom Krooke is proud of you. Don't tell the other clients.",
	"The balance is zero. Thom Krooke checks the ledger twice — once for accuracy and once for the pleasure of seeing it. Congratulations. The deed is fully transferred. Come by sometime. Not for business. Just to visit.",
}

var ThomKrookePassiveIncome = []string{
	"Your weekly property summary: {income} coins generated this week. Minus your mortgage payment of {payment} coins — net gain of {net}. Thom Krooke thinks that's rather nice, don't you?",
	"Income report! {income} coins from your property this week. After mortgage: {net} coins net. Thom Krooke notes the number has been growing as your upgrades compound. The investment is working. Thom Krooke approves of investments that work.",
	"Weekly summary from Thom Krooke: {income} coins passive income, {payment} coins mortgage. {net} coins to the good. Not bad for a week of not being home. Your property works while you don't. Thom Krooke finds this philosophically satisfying.",
}

var ThomKrookeEviction = []string{
	"Thom Krooke has to say something that Thom Krooke doesn't enjoy saying. The rent hasn't come through two weeks running and the room needs to be made available. Your things are in storage — seven days, no charge. Thom Krooke hopes you find your footing. The room will be here when you're ready to try again.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PROPERTY UPGRADES
// ─────────────────────────────────────────────────────────────────────────────

var UpgradeWorkshop = []string{
	"The workshop is installed. It smells like fresh sawdust and good intentions. Thom Krooke had the craftspeople use the good wood.",
	"Workbench, tool rack, and a small window that catches the morning light well. The workshop is ready. What gets made in it is up to you.",
}

var UpgradeHerbGarden = []string{
	"The herb garden is planted. Give it a few days to settle in. The soil is good — Thom Krooke insisted on the good soil, the kind that actually wants things to grow.",
	"Rows of small plants, most of them green, a few of them uncertain. The herb garden is in. Pastel has already noted which ones need more shade.",
}

var UpgradeVault = []string{
	"The vault door is heavier than it looks. The locksmith said it would be. What goes in there stays in there — even on the worst days. Especially on the worst days.",
	"The vault is installed. Thom Krooke double-checked the lock personally and would not share the combination until you were present. This is a trust. Thom Krooke treats it like one.",
}

var UpgradeTrophyRoom = []string{
	"Empty shelves and good lighting. The trophy room is ready for whatever you bring back. I have opinions about display arrangement. I'll share them if asked. I'll share them if not asked. I'll share them at three in the morning if the lighting catches a trophy just right.",
	"The plaques are engraved, the mounts are installed, and the lighting makes everything look slightly more legendary than it already is. The trophy room is yours.",
}

var UpgradeExpeditionOutpost = []string{
	"The outpost is stocked and linked. Signal fires, supply hooks, a map table with your last Base Camp location already marked. Whoever built this knew what they were doing. Thom Krooke selected the contractor personally.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PASTEL — HIRING
// ─────────────────────────────────────────────────────────────────────────────

var PastelHireConfirm = []string{
	"Hi! I'm Pastel. I know the place, I know where things go, and I'll make sure everything is looked after while you're out. Leave me a list if you want — or don't, I'll figure it out.",
	"Pastel here. I've done this before. I'll take good care of everything. You don't need to worry about home while you're in the dungeon. That's sort of the whole point of me.",
	"I'll be honest, I was hoping you'd call. The herb garden looked like it needed attention and the pets had that look they get. Everything will be fine. Go do your expedition. I've got it.",
}

var PastelFireConfirm = []string{
	"Of course. I'll wrap things up and leave the notes on the table — what I did, what still needs doing, current supply status. It's been good. Your home is in good shape.",
	"Understood. Everything is in order. The storage is labeled, the pets are fed, the passive income queue is current. Good luck out there.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PASTEL — DAILY NOTES (Level 1)
// Enthusiastic, slightly scattered, well-meaning.
// ─────────────────────────────────────────────────────────────────────────────

var PastelNoteLevel1 = []string{
	"Fed the pets! All of them. I think. The small one hid behind the storage chest for a while and I'm not completely sure it came out to eat but I left food where it could reach it. The garden is watered. I meant to check the passive income queue but got a bit turned around with the storage labels. Will do that first thing tomorrow.",
	"Good day! The herb garden got some attention, the pets were walked (or equivalent — the fish were observed), and I collected the income. I accidentally shelved three items in the wrong slots but found them eventually. Everything is where it should be. Mostly. The weapons rack might be slightly reorganized.",
	"Note from Pastel: pets fed, garden tended, income collected. I made one small mistake with the supply manifest — added a column that didn't need to be there — but the numbers are right, the column is just extra. Please ignore the extra column.",
	"All tasks completed! Well — most tasks. The greenhouse watering got a little delayed because I was making sure the workshop tools were hung correctly and then it was later than I thought. The plants look fine. Probably fine. I'll check again in the morning.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PASTEL — DAILY NOTES (Level 2)
// Finding her rhythm. Notes are more organized. Occasional slip.
// ─────────────────────────────────────────────────────────────────────────────

var PastelNoteLevel2 = []string{
	"Morning note from Pastel. Pets fed and happy — the small one came out on its own today, which I'm taking as a good sign. Garden watered, income collected (14 coins, recorded in the ledger I started keeping). Workshop is tidy. All good.",
	"Everything done, everything noted. The herb garden is coming in nicely — I moved one of the pots to the south window and it seems happier there. Let me know if you'd prefer I leave things where they are. I have opinions about light.",
	"Pastel here. Smooth day — fed, watered, collected, organized. I found a supply cache you'd left in the back of storage that wasn't logged. I've logged it now. You had more materials than you thought.",
	"Note: the passive income queue had a small delay in processing, maybe 40 minutes. Nothing lost — just late. I've noted the timing and will keep an eye on it. Also the pets had a disagreement about something and I mediated. Everyone is fine.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PASTEL — DAILY NOTES (Level 3)
// Reliable, efficient, minimal fuss.
// ─────────────────────────────────────────────────────────────────────────────

var PastelNoteLevel3 = []string{
	"All tasks complete. Pets fed, garden tended, income logged. Your storage is organized by zone and rarity now — took me an afternoon last week but I think you'll find it easier. Let me know if the system doesn't work for you.",
	"Good day here. The herb garden yielded a little extra — I've bagged the surplus and left it on the workshop table. The vault contents are accounted for and untouched. Quiet day otherwise.",
	"Pastel. Everything is in order. I handled a small issue with the supply staging — one of the SU bags had a slow leak, so I replaced it from the reserve and logged the loss. You're at full capacity. No interruption to the expedition.",
	"Note: Thom Krooke stopped by. Not about the mortgage — just checking in, he said. I offered tea. He had opinions about the trophy room arrangement that I've passed along at the end of this note, unedited, so you can decide what you want to do with them. Everything else: fine.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PASTEL — DAILY NOTES (Level 4)
// Anticipatory. Occasionally handles things before they become problems.
// ─────────────────────────────────────────────────────────────────────────────

var PastelNoteLevel4 = []string{
	"Everything in order. I noticed the mortgage payment date falls during a stretch where your passive income might be lower than usual — I've set aside a buffer in a separate ledger line so the Sunday draw won't cause an issue. Just in case. You can move it back if you don't need it.",
	"Pets, garden, income — all done. I also restocked the supply staging from the herb garden surplus, which means you're slightly above your starting SU load for the next expedition. I had a feeling you'd need it.",
	"Note: the greenhouse plants in the east corner were getting too much direct light. I moved them. Yield should improve next week. Also found a crafting recipe in the library you hadn't catalogued — left it on the workshop table with a note about which materials you'd need. You have most of them.",
	"Quiet today. The pets settled early, the garden is doing well, the income processed on time. I checked the vault — everything accounted for. I re-read the expedition outpost supply logs and noticed a small gap in the inventory count; I've corrected it and added a note about where the discrepancy probably started. Everything is accurate now.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PASTEL — DAILY NOTES (Level 5)
// Perfect. Zero miss chance. Sometimes leaves a gift. The right gift.
// ─────────────────────────────────────────────────────────────────────────────

var PastelNoteLevel5 = []string{
	"Everything is handled. It always is. Left something on the kitchen table — found it at the market and thought of you. No reason. Just seemed right.",
	"All done. The kind of day where nothing went wrong and I want you to know that's not an accident — it takes work for nothing to go wrong, and I put in the work. The pets are happy. The garden is happy. The vault has something new in it that I think you'll be pleased about.",
	"I noticed you've been in the same zone for several days now. I made sure the expedition outpost supply link is topped up and the signal fires are ready for when you come back. There's a warm meal that will be ready in about four hours — I timed it based on your usual return window. If you're late, it keeps.",
	"Note from Pastel: everything is perfect, all systems running exactly as they should, the pets are thriving, the garden is producing double what it was last month, and I've reorganized the trophy room so the legendary items catch the evening light better. I also fixed the thing with the storage manifest that's been slightly off for three weeks. You hadn't mentioned it but I could tell it was there. You're welcome.",
	"Home is ready for you. It's always ready for you. That's the job. That's what I do. See you when you get back.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PASTEL — MISSED TASK NOTES (Level 1–2 only)
// When miss_chance triggers — honest, not defensive.
// ─────────────────────────────────────────────────────────────────────────────

var PastelMissedTask = []string{
	"I have to be honest with you — I forgot to water the herb garden today. I remembered at midnight and went back and did it then but it had been a while. The plants look okay. I'm sorry. I'll set a better reminder.",
	"The passive income didn't get collected today. I got turned around with the storage reorganization and by the time I got to it the queue had backed up. It'll process double tomorrow. Nothing is lost. I'm a bit embarrassed about this one.",
	"The pets got fed late today. Not dangerously late — late in the way that meant they gave me a look, specifically the kind of look that knows you will mention it to your owner. I mentioned it first. Fed now. All fine.",
	"I missed the supply staging check today. Something came up with the herb garden (a good something — unexpected yield, which I've logged) and the morning got away from me. The staging is fine, just unchecked. I'll do it first thing tomorrow and every day after that.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PASTEL — SPECIAL EVENTS
// ─────────────────────────────────────────────────────────────────────────────

var PastelDeliveryArrived = []string{
	"Your supply order arrived from Thom Krooke. I've staged it in the expedition outpost and logged everything. The invoice is on the table if you want to check it against the order.",
	"Delivery came while you were out — signed for it, stowed it, logged it. Everything matched the manifest. Thom Krooke's packaging has gotten better, I'll give him that.",
}

var PastelPetEvent = []string{
	"The pets had an interesting day. I won't go into detail but by the end of it everyone was friends again and the storage chest is fine, structurally. A small note on the scratched corner.",
	"Your pet found something in the back of the storage room that I couldn't identify. I've put it on the workshop table. It doesn't seem dangerous. It is definitely something.",
	"One of the pets has been sitting by the expedition outpost since this morning. I think it knows you've been out a long time. Everything is fine. I just thought you'd want to know.",
	"The pets were restless today — I think they can tell you've been in a Tier 4 zone because they get like this around Day 10. Fed them an extra portion. They settled. They'll be glad to see you.",
}

var PastelLevelUpNote = []string{
	"I've been doing this for a while now and I think I've found my footing. Just wanted to say that. Back to work.",
	"Level up, I think they call it. Thom Krooke came by and seemed pleased. I'm not sure why Thom Krooke monitors this but he does. The work is the same either way.",
	"I know this job better now than I did when I started. I can feel the difference. The notes are shorter because less needs explaining. That's probably a good sign.",
}

var PastelGift = []string{
	"Left something on the table. Found it at the market — one of those rare materials you use for crafting, the kind that's hard to come by outside of high-tier zones. Seemed useful. No occasion.",
	"There's a potion of superior healing on the kitchen table. The kind Thom Krooke doesn't stock. Don't ask where I found it. Use it when you need it.",
	"I made something while you were gone. It's in the workshop — used the materials from the garden surplus and a recipe I found in the library. It should help with the next expedition. Consider it a gift for being a good employer.",
	"There's a warm meal on the table and a note under it. The note says: good luck out there. Come back in one piece. The meal is from scratch. I had time.",
}

var PastelPlayerReturn = []string{
	"You're back. Good. I'll give you the full rundown but first — sit down, eat something, the pets have been waiting. The report can wait five minutes. You look like you've been in a dungeon.",
	"Welcome home. Everything is in order. I've written up the full summary — it's on the table, organized by day. Take your time with it. Nothing is on fire.",
	"Back already? The expedition ran short. That's fine — I was prepared for longer. The full caretaking log is on the table. The pets are very happy to see you, which I'll note I find completely understandable.",
	"You made it. I had the outpost signal fire ready from Day 10 onward, just in case. Everything here is exactly as it should be. I'll let you settle in. The summary is on the table when you want it.",
}

var PastelPlayerDeathReturn = []string{
	"You're home. I heard from Thom Krooke — he manages things in an emergency, and I suppose this counted. Everything here is exactly where it was. Your things are safe. The pets have been fed twice today because I thought you might need to see that when you got back. Rest.",
	"I'm glad you're back. I won't ask about the hospital — Thom Krooke told me what he needed to tell me and I kept the home ready. It's still ready. Take whatever time you need.",
}

// ─────────────────────────────────────────────────────────────────────────────
// HOUSING — ARRIVAL / FIRST REST
// ─────────────────────────────────────────────────────────────────────────────

var HomeFirstArrival = []string{
	"This is yours now. A place with a door that closes, a floor that doesn't move, and a ceiling that belongs to you. After everything the dungeon does to make those things uncertain, a home is meaningful in a way that doesn't need a saving throw.",
	"Home. The word lands differently when there's an actual place attached to it. I watch you step inside and say nothing, which is the most eloquent thing I know how to do and which I am, frankly, very proud of pulling off.",
	"You have a home. I've narrated dungeons and bosses and legendary drops and this — this quiet moment of a door closing on your own place — is one of the better things I've watched happen. I'm not going to write a fanfare for it. The silence is the fanfare.",
}

var HomeLongRest = []string{
	"A long rest at home. I go quiet in a different way than I do in dungeons — not alert-quiet, not cautious-quiet. Just the good kind. The kind where the most threatening sound is the kettle.",
	"Your own bed. Your own walls. The full rest resolves everything the dungeon costs. Home is the best mechanic in the game and I'm telling you so, once, and then I'm letting you sleep.",
	"Home rest. HP restored, slots restored, conditions cleared. I don't narrate this one past the bare facts — some things don't need dramatic description. Coming home is one of them.",
}
