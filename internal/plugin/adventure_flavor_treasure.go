package plugin

// ── TREASURE DISCOVERY ────────────────────────────────────────────────────────
// Fired when a rare treasure drops alongside normal loot resolution.
// The treasure DM arrives after the normal outcome DM.
// Tone scales by tier: Tier 1 is unhinged excitement for garbage.
// Tier 5 is one paragraph, terse, slightly wrong-feeling.
// {treasure_name}, {bonus_desc}, {location} are substituted at send time.

var TreasureDiscovery = map[int][]string{

	// ── TIER 1 ───────────────────────────────────────────────────────────────
	// These are bad items described as world-historical finds.
	// The bot has fully lost the plot. The item is a button or a bent coin.
	// The description insists otherwise with complete sincerity.
	1: {
		"WAIT.\n\n" +
			"WAIT. Something else was in there.\n\n" +
			"Among the rat droppings and damp and general misery of {location}, " +
			"you have found THE {treasure_name}.\n\n" +
			"Historians — future historians, the ones who will one day write " +
			"about THIS SPECIFIC MOMENT in a dungeon that smells like wet rat — " +
			"will call this a turning point. The turning point has happened. " +
			"You are the turning point.\n\n" +
			"BONUS: {bonus_desc}. The universe has acknowledged you. " +
			"The acknowledgement smells faintly of damp. That's fine. It's still acknowledgement.",

		"ADDITIONAL FINDING.\n\n" +
			"You almost missed it. You ALMOST MISSED IT. " +
			"In the corner, behind the loose stone, wrapped in what was once " +
			"probably cloth and is now mostly an aspiration: the {treasure_name}.\n\n" +
			"This is real. This is a real thing that is now in your possession. " +
			"You have {treasure_name} and nobody else does and the rats " +
			"were RIGHT THERE and didn't even know what they were sitting next to.\n\n" +
			"BONUS: {bonus_desc}.\n" +
			"You're welcome for finding it. You found it. It's found. It's yours.",

		"STOP EVERYTHING ELSE.\n\n" +
			"The loot is fine. The loot can wait. " +
			"There is something more important than the loot and it is the {treasure_name} " +
			"which you have just found in {location} and which is NOW YOURS.\n\n" +
			"Is it large? No. Is it impressive to look at? " +
			"That depends on what you're looking for, and what you're looking for is HISTORY, " +
			"and history is right here in your hand and it is the {treasure_name} " +
			"and it provides {bonus_desc} and that is not nothing.\n\n" +
			"That is SOMETHING. That is EXACTLY SOMETHING.",

		"A RARE TREASURE HAS BEEN FOUND.\n\n" +
			"Category: Incredible.\n" +
			"Location found: {location}, which will now be famous for this.\n" +
			"Item: {treasure_name}.\n" +
			"Bonus: {bonus_desc}.\n" +
			"Rarity: extremely rare, extremely exciting, extremely yours.\n\n" +
			"The rats didn't find it. Other adventurers didn't find it. " +
			"You found it. In a damp cellar. Wearing the Goddamn Offensive Helmet, " +
			"possibly, depending on your gear situation. " +
			"The helmet doesn't diminish this. Nothing diminishes this.",

		"Oh.\n\nOH.\n\n" +
			"Under the thing, behind the other thing, in the part of {location} " +
			"that smelled worst and seemed least likely to contain anything good:\n\n" +
			"The {treasure_name}.\n\n" +
			"You've read about items like this. Well — you haven't, specifically, " +
			"because items like this aren't in books, because people who find them " +
			"don't write books, they just carry the items and have better outcomes. " +
			"Now you're one of those people.\n\n" +
			"BONUS: {bonus_desc}. Go. Go tell someone. " +
			"Tell everyone. They won't understand. Tell them anyway.",

		"TREASURE ALERT. RARE TREASURE ALERT.\n\n" +
			"This is not a drill. The {treasure_name} has been found. " +
			"In {location}. By you. Today.\n\n" +
			"The {treasure_name} provides {bonus_desc}, which is a passive bonus " +
			"that will apply every single day that you hold it, " +
			"which should be every single day going forward, " +
			"because why would you ever not hold it, " +
			"because it is THE {treasure_name} and you FOUND IT.\n\n" +
			"The rats had no idea. They were RIGHT THERE.",

		"Hidden. It was hidden. Not dramatically hidden — " +
			"not behind a puzzle or a boss or a sealed door — " +
			"just quietly hidden in a way that means " +
			"most people walked past it without looking correctly.\n\n" +
			"You looked correctly. You found the {treasure_name}.\n\n" +
			"BONUS: {bonus_desc}. " +
			"The {location} has been keeping this from everyone. " +
			"It is no longer keeping it from you. " +
			"You have it. It's in your inventory right now. Go look.",

		"Congratulations are in order and they are being given now.\n\n" +
			"Congratulations.\n\n" +
			"You have found the {treasure_name} in {location} " +
			"and you have done so under conditions that were not ideal " +
			"(they are never ideal in {location}) " +
			"and the {treasure_name} provides {bonus_desc} " +
			"and this is one of the better things that has happened to you.\n\n" +
			"The bar for 'better things that have happened to you' includes some dark entries. " +
			"This one is good. Genuinely good. Take the win.",
	},

	// ── TIER 2 ───────────────────────────────────────────────────────────────
	// Still excited, but dialing back to merely very enthusiastic.
	// The item is clearly better than a button. The description knows it.
	2: {
		"Something else came out of {location} today.\n\n" +
			"Beyond the normal loot, beyond what was expected: the {treasure_name}.\n\n" +
			"This is a real find. Not a bent button or an ambiguous artifact — " +
			"a real, useful, genuinely interesting thing that provides {bonus_desc} " +
			"as long as you hold it, which you should do indefinitely.\n\n" +
			"The goblins had no idea what they were sitting on. " +
			"Classic goblins.",

		"The {treasure_name}. In {location}.\n\n" +
			"You found it past the goblins, past the traps, past the part of the dungeon " +
			"that most people turn around in because it gets uncomfortable. " +
			"You didn't turn around. The {treasure_name} was the reason not to turn around, " +
			"even though you didn't know that yet.\n\n" +
			"BONUS: {bonus_desc}. " +
			"Hold onto it. This is not a thing you found every day.",

		"A rare treasure. Actual rarity — not 'this seems unusual' rarity " +
			"but statistically unlikely, specifically here, specifically today, specifically you.\n\n" +
			"The {treasure_name} from {location}.\n" +
			"Bonus: {bonus_desc}.\n\n" +
			"The goblins were between you and this. You went through the goblins. " +
			"In retrospect the goblins were guarding this, badly, " +
			"without knowing it, which is the best kind of guarding " +
			"from your perspective.",

		"There it was. Sitting in a room in {location} that didn't announce itself, " +
			"didn't have dramatic lighting, didn't have a boss standing in front of it. " +
			"Just sitting there. The {treasure_name}.\n\n" +
			"You almost didn't check the room. You almost walked past it. " +
			"You didn't walk past it.\n\n" +
			"BONUS: {bonus_desc}. " +
			"The almost doesn't matter. You have it.",

		"Rare find from {location}: the {treasure_name}.\n\n" +
			"This doesn't happen every run. It doesn't happen most runs. " +
			"Today it happened, and you were the one it happened to, " +
			"and {bonus_desc} is now a permanent feature of your operation " +
			"as long as this stays in your inventory.\n\n" +
			"Keep it in your inventory.",

		"Beyond the loot, behind what looked like a dead end, " +
			"in the part of {location} where the map gets vague:\n\n" +
			"The {treasure_name}.\n\n" +
			"BONUS: {bonus_desc}.\n\n" +
			"Rare. Real. Yours. " +
			"The kobolds who were using that room as a shortcut " +
			"had been walking past it for years.",
	},

	// ── TIER 3 ───────────────────────────────────────────────────────────────
	// Matter-of-fact but notable. The item is genuinely interesting.
	// The description respects it without losing the voice.
	3: {
		"The {treasure_name}.\n\n" +
			"It was in {location}, past everything that tried to stop you getting to it. " +
			"The things that tried to stop you did not fully stop you. " +
			"The {treasure_name} was what they were protecting and now it's yours.\n\n" +
			"BONUS: {bonus_desc}. " +
			"This is a significant find. The crypt knew what it had.",

		"Past the Draugr, past the skeletons, past the ghost that let you through " +
			"for reasons it didn't share: the {treasure_name}.\n\n" +
			"It was waiting. Not dramatically — things don't wait dramatically in crypts, " +
			"they wait in the dark, quietly, correctly. " +
			"The {treasure_name} waited correctly and now it's yours.\n\n" +
			"BONUS: {bonus_desc}. Keep it.",

		"A rare treasure from {location}.\n\n" +
			"The {treasure_name}. {bonus_desc}.\n\n" +
			"At this tier, rare means genuinely rare — not 'unusual' rare, " +
			"not 'interesting' rare, but 'most people who come here don't come back with this' rare. " +
			"You came back with this. Note the difference.",

		"The {treasure_name} was in {location}.\n" +
			"You were in {location}.\n" +
			"These two facts are now connected in a way that benefits you.\n\n" +
			"BONUS: {bonus_desc}. " +
			"The benefit is ongoing. The {treasure_name} stays in your inventory " +
			"and the benefit continues. Don't lose it.",

		"Something was in the inner chamber of {location} " +
			"that the outer chambers were keeping people away from.\n\n" +
			"The {treasure_name}.\n\n" +
			"You got to the inner chamber. You have the thing the outer chambers were hiding. " +
			"BONUS: {bonus_desc}. " +
			"The crypt is less well-guarded now than when you arrived.",
	},

	// ── TIER 4 ───────────────────────────────────────────────────────────────
	// Understated. The item has gravity. The description doesn't oversell it
	// because it doesn't need to. One or two sentences of context.
	4: {
		"The {treasure_name}.\n\n" +
			"You found it in {location}. It came with you.\n" +
			"BONUS: {bonus_desc}.\n\n" +
			"At this tier, 'rare' is a significant understatement. " +
			"Hold onto it.",

		"Among everything that came out of {location} today, " +
			"the {treasure_name} is the thing that matters most.\n\n" +
			"BONUS: {bonus_desc}.\n\n" +
			"It's been in there for a long time. " +
			"It came out with you. That means something, probably.",

		"The {treasure_name} was in {location}.\n" +
			"It is now in your inventory.\n" +
			"BONUS: {bonus_desc}.\n\n" +
			"This is not the kind of item that turns up in Tier 1 dungeons. " +
			"This is not the kind of item that turns up often anywhere. " +
			"You are at Tier 4, doing Tier 4 things, and this is one of them.",

		"A Tier 4 rare find.\n\n" +
			"The {treasure_name} from {location}.\n" +
			"BONUS: {bonus_desc}.\n\n" +
			"The Troll Bridge Depths held onto this for a long time. " +
			"You are the reason it stopped holding on.",

		"The {treasure_name}.\n\n" +
			"It has history. It doesn't need to explain its history to you. " +
			"The bonus is {bonus_desc} and that's the relevant part right now.\n\n" +
			"The rest of the history is available to you if you look. " +
			"Most people who look wish they hadn't. " +
			"Most people look anyway.",
	},

	// ── TIER 5 ───────────────────────────────────────────────────────────────
	// Terse. One paragraph. Slightly wrong-feeling, like the item is aware.
	// No exclamation marks. No enthusiasm. Just weight.
	5: {
		"The {treasure_name}.\n\n" +
			"You found it in the Abyssal Maw. It found you in the Abyssal Maw. " +
			"The distinction matters less at depth.\n" +
			"BONUS: {bonus_desc}.\n\n" +
			"It came with you. Some things don't come with you. This one did. " +
			"Think about what that means.",

		"The {treasure_name} is in your inventory.\n\n" +
			"BONUS: {bonus_desc}.\n\n" +
			"This is a Tier 5 rare. The Abyssal Maw doesn't give these up. " +
			"The Abyssal Maw gave this one up. " +
			"That's a sentence worth sitting with.",

		"You have the {treasure_name}.\n\n" +
			"BONUS: {bonus_desc}.\n\n" +
			"It was in the Abyssal Maw. The things that were between you and it " +
			"are not between anything and anything anymore. " +
			"The Maw is noting this. So is the item.",

		"The {treasure_name}.\n\n" +
			"It's warm. It was warm when you found it, which it shouldn't be, " +
			"which is information you've decided not to investigate right now.\n" +
			"BONUS: {bonus_desc}.\n\n" +
			"Right now is for coming home. " +
			"The investigation can happen later.",

		"Rare. Tier 5 rare, which means: most people who try for this don't get it. " +
			"Some of them tried for a very long time.\n\n" +
			"The {treasure_name}. {bonus_desc}.\n\n" +
			"You have it. " +
			"Keep it somewhere it won't be lost. " +
			"The Abyssal Maw does not give second chances.",

		"The {treasure_name} is yours.\n\n" +
			"BONUS: {bonus_desc}.\n\n" +
			"The Abyssal Maw had it. You have it now. " +
			"The Maw is aware of the transfer. " +
			"The Maw is considering its position. " +
			"You should not be in the Maw when it finishes considering.",
	},
}

// Room announcement for Tier 5 treasure finds only.
// Posted to the game room, separate from the player's DM.
// Terse. One line. No fanfare. The rarity does the work.
var TreasureRoomAnnouncement = []string{
	"🔴 {name} found the {treasure_name} in {location}. It came with them.",
	"🔴 {name} recovered the {treasure_name} from {location}. The Maw noticed.",
	"🔴 {name} has the {treasure_name}. They found it in {location}. It let them take it.",
	"🔴 The {treasure_name} has left {location}. {name} has it now.",
	"🔴 {name} came out of {location} with the {treasure_name}. They came out.",
}

// Inventory cap — player found a 4th treasure and must discard one.
var TreasureInventoryCap = []string{
	"You found the {treasure_name}.\n\n" +
		"This is good news. The bad news is your pockets.\n\n" +
		"You are already carrying three treasures and your inventory " +
		"has filed a formal structural objection:\n\n" +
		"1. {treasure_1} — {bonus_1}\n" +
		"2. {treasure_2} — {bonus_2}\n" +
		"3. {treasure_3} — {bonus_3}\n\n" +
		"You must discard one to make room. Reply with 1, 2, or 3.\n" +
		"The discarded item leaves forever and will take it personally.\n\n" +
		"Or reply 'keep' to leave the {treasure_name} behind.\n" +
		"It will stay where you found it.\n" +
		"It will be there when you come back.\n" +
		"It will remember that you left it.",

	"The {treasure_name}. Here. Now. Yours, conditionally.\n\n" +
		"The condition is that you're already carrying three treasures " +
		"and the universe has a firm three-treasure policy " +
		"that it enforces through inventory mechanics.\n\n" +
		"Current treasures:\n" +
		"1. {treasure_1} — {bonus_1}\n" +
		"2. {treasure_2} — {bonus_2}\n" +
		"3. {treasure_3} — {bonus_3}\n\n" +
		"One of these goes. Reply 1, 2, or 3 to discard it.\n" +
		"Or reply 'keep' to leave the {treasure_name} in {location}.\n" +
		"The {treasure_name} will wait. It has nowhere else to be.",

	"Good news and a problem, arriving together as they usually do.\n\n" +
		"Good news: the {treasure_name}.\n" +
		"Problem: you have three treasures already and can only hold three.\n\n" +
		"1. {treasure_1} — {bonus_1}\n" +
		"2. {treasure_2} — {bonus_2}\n" +
		"3. {treasure_3} — {bonus_3}\n\n" +
		"Reply 1, 2, or 3 to discard one and take the {treasure_name}.\n" +
		"Reply 'keep' to leave the {treasure_name} where it is.\n\n" +
		"Either way: you've found it. " +
		"The finding counts whether you take it or not.\n" +
		"The bonus doesn't apply unless you take it.\n" +
		"Just noting that.",
}

// ── DAILY SUMMARY ONE-LINERS ──────────────────────────────────────────────────
// Short outcome summaries for the room-facing daily summary.
// These replace the full narrative for the summary view.
// Format: one punchy line, past tense, fits in a table row.
// Substitutions: {name}, {location}, {item}, {value}, {outcome}

var SummaryDungeonSuccess = []string{
	"Found {item} worth €{value} in {location}. Made it out.",
	"Cleared {location}. Retrieved {item}, €{value}. Home before dark.",
	"{location}: success. {item} at €{value}. No hospitalizations.",
	"Went into {location}. Came back with {item}. €{value}. Alive.",
	"Extracted {item} worth €{value} from {location}. Equipment is worse. Net positive.",
	"{item} from {location}. €{value}. The dungeon has been informed.",
	"Profitable run through {location}. {item}, €{value}, {xp} XP.",
	"{location} cleared, partially. {item} recovered. €{value} assessed.",
	"Found {item} in {location}. €{value}. Didn't die. A complete run.",
}

var SummaryDungeonExceptional = []string{
	"EXCEPTIONAL run in {location}. {item} at €{value}. Ask them about it.",
	"{location} boss dropped {item} worth €{value}. {name} is having a very good day.",
	"Inner chamber of {location} reached. {item}, €{value}. Historic.",
	"Critical run. {location}. {item} at €{value}. Screenshot this.",
	"{item} worth €{value} from the depths of {location}. The dungeon is embarrassed.",
}

var SummaryDungeonDeath = []string{
	"Died in {location}. Back in {hours}h. The {location} remains standing.",
	"{location}: death. Equipment damaged. Healthcare involved. {hours}h respawn.",
	"Did not survive {location}. The {location} did. {hours}h recovery.",
	"Killed in {location}. American healthcare has the rest. {hours}h.",
	"{location} wins this round. {name} recovers in {hours}h.",
	"Dead. {location}. {hours} hours. The equipment is worse.",
	"{name} lost the argument with {location}. Healthcare is mediating. {hours}h.",
}

var SummaryDungeonEmpty = []string{
	"Searched {location}. Found nothing. Came home.",
	"{location}: empty. Goblins/skeletons/trolls present. Treasure absent.",
	"Nothing in {location} today. The XP was also mostly absent.",
	"Three hours in {location}. Zero haul. One pot smashed out of principle.",
	"{location} gave up nothing. {name} gave up on {location} eventually.",
	"Cleared {location} of everything that could be killed. Nothing else was there.",
}

var SummaryMiningSuccess = []string{
	"Extracted {ore} worth €{value} from {location}. The mountain cooperated.",
	"{location}: {ore}, €{value}. One near-miss. Positive outcome.",
	"Mining run at {location}: {ore} recovered, €{value}, no cave-ins.",
	"Came back from {location} with {ore} and €{value} and a worse {tool}.",
	"{ore} from {location}. €{value}. The back is filing a complaint.",
	"You went mining. You tripped. You fell into the wall. You're out cold. But a rock broke free. You may have lost 8 hours. But hell, you gained {ore}. €{value}.",
}

var SummaryMiningDeath = []string{
	"Cave-in. {location}. Healthcare called. {hours}h.",
	"Died in {location}. Mining death. Rarer than dungeon death. Still death. {hours}h.",
	"{location} expressed structural concerns physically. {name}: {hours}h recovery.",
	"The {location} ceiling had a perspective. {name}: medical. {hours}h.",
	"Dead in {location}. The ore is still in there. {hours}h.",
}

var SummaryMiningEmpty = []string{
	"Mined {location} for hours. The mountain was empty. The back was not.",
	"{location}: nothing worth taking. Three hours of nothing worth taking.",
	"Empty vein in {location}. Copper-coloured rock, wall to wall.",
	"The {location} had no ore today. Just rock. Very thorough rock.",
}

var SummaryForagingSuccess = []string{
	"Foraged {item} from {location}. €{value}. No hornets.",
	"{location}: {item}, €{value}. A peaceful morning. Suspicious in retrospect.",
	"Returned from {location} with {item} worth €{value} and zero injuries. Good day.",
	"Clean haul from {location}. {item}, €{value}. The forest cooperated.",
	"{item} from {location}. €{value}. One bear made eye contact. Moved on.",
	"You went foraging. You foraged. Yep. Stuff found. Good job. What was it? Who cares? It's foraging. Does it really matter? Okay. Fine. It was {item}. €{value}.",
}

var SummaryForagingEmpty = []string{
	"Wandered through {location}. Found nothing. Not even hornets.",
	"{location}: searched every bush. Came home empty-handed.",
	"Nothing from {location}. The forest was uncooperative today.",
	"Went foraging in {location}. The forest kept its things.",
}

var SummaryForagingDeath = []string{
	"Died foraging. In {location}. Yes, foraging. {hours}h.",
	"{location} produced a fatal outcome via non-combat means. {hours}h.",
	"Dead in {location}. Bear/hornets/river/tree/mushroom involvement. {hours}h.",
	"Foraging death. {location}. The forest won. {hours}h.",
}

var SummaryForagingHornets = []string{
	"Hornets. {location}. No loot. Significant swelling.",
	"Found hornets in {location}. The hornets found back.",
	"{location}: hornets. No haul. Home eventually.",
	"Hornets in {location}. As always. No loot. Many regrets.",
}

var SummaryForagingBear = []string{
	"Bear encounter in {location}. Armor damaged. Loot lost. Home.",
	"{location}: bear. Equipment worse. Running occurred. Home.",
	"The {location} bear had opinions. Armor absorbed the first opinion.",
}

var SummaryForagingRiver = []string{
	"River crossing in {location}. Lost {item}. Boots ruined. Oregon Trail.",
	"The river kept {item}. {name} kept everything else, soaking wet.",
	"{location} river: took {item}, gave back nothing. Boots are planters now.",
}

var SummaryResting = []string{
	"Did not leave the house. Gained nothing. Avoided death. Low bar.",
	"Resting today. XP: zero. Loot: zero. Alive: yes.",
	"No action. Hovel. Wall. Nothing. Home.",
	"Sat this one out. The dungeons noticed. The dungeons don't care.",
	"Rest day. TwinBee noticed.",
}

var SummaryDead = []string{
	"Currently dead. Returns {time} UTC.",
	"In healthcare. {time} UTC release expected.",
	"Recovering. The insurance paperwork is ongoing. Back {time} UTC.",
	"Dead. Healthcare. {time}. The usual.",
	"Not available. Medical reasons. {time} UTC.",
}

// Standout lines — bottom of the daily summary, one per day.
// Picks the most notable outcome across all adventurers.
var SummaryStandoutGood = []string{
	"🏆 {name} found {item} worth €{value} in {location}. A good day by any measure.",
	"🏆 {name} had the best haul today: {item}, €{value}, {location}.",
	"🏆 Today's standout: {name}, {location}, {item} at €{value}. Well done.",
	"🏆 {name} cleared {location} and came back with €{value}. Leading today.",
	"🏆 Best outcome: {name}, €{value} from {location}. Not bad at all.",
}

var SummaryStandoutDeath = []string{
	"💀 {name} died in {location}. The {location} is getting too confident.",
	"💀 {name} lost to {location}. The rats/goblins/trolls send their regards.",
	"💀 Notable loss: {name} in {location}. Healthcare is familiar with the file.",
	"💀 {name} did not survive {location} today. A learning experience. Expensive.",
	"💀 {location} claimed {name}. {hours}h recovery. The dungeon has been noted.",
}

var SummaryStandoutTreasure = []string{
	"✨ {name} found the {treasure_name} in {location}. Rare. Real. Theirs.",
	"✨ Rare find: {name} recovered the {treasure_name} from {location}.",
	"✨ {name} found a rare treasure today. The {treasure_name}. In {location}.",
}

var SummaryStandoutHornets = []string{
	"🐝 {name} found hornets in {location}. Again.",
	"🐝 {name} and the hornets of {location} have met again. The hornets won again.",
	"🐝 Today's hornet report: {name}, {location}, outcome as expected.",
}

// TwinBee summary one-liners (used in the TwinBee section of daily summary)
var TwinBeeSummarySuccess = []string{
	"Visited {location}. Retrieved {loot} worth €{value}. Returned in excellent condition.",
	"{location}: success. {loot} recovered at €{value}. Standard operation.",
	"Cleared {location}. {loot}, €{value}. TwinBee has reported this as typical.",
	"{loot} worth €{value} from {location}. TwinBee is unsurprised.",
	"Professional operation in {location}. {loot}, €{value}. No further comment.",
}

var TwinBeeSummaryWithdrawal = []string{
	"Visited {location}. Tactical withdrawal executed. Haul: none. Assessment: ongoing.",
	"{location}: early departure. Strategic positioning. Returns tomorrow.",
	"Reconnaissance of {location} complete. Phase Two: tomorrow.",
	"Withdrew from {location} on schedule. The schedule has been updated.",
	"{location} assessed. TwinBee returns tomorrow. The {location} has been warned.",
}

var TwinBeeSummaryEmpty = []string{
	"Visited {location}. Nothing there today. TwinBee notes this for the record.",
	"{location}: empty. The dungeon has failed to provide. TwinBee has noted it.",
	"Nothing found in {location}. TwinBee is filing feedback with the dungeon.",
}
