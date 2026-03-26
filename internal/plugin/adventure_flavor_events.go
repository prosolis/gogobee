package plugin

// ── RANDOM MID-DAY EVENTS ─────────────────────────────────────────────────────
//
// Trigger chance: 0.5% per player per day, checked at a random time
// between 10:00 and 16:00 UTC.
// Player has 2 hours to reply !adventure respond.
// No response: event expires silently, no penalty, no reward.
// Response: OutcomeDM sent, reward applied, one-liner posted to room.

// AdvRandomEvent defines a single mid-day event with trigger/outcome text and rewards.
type AdvRandomEvent struct {
	Key             string
	TriggerDM       string
	TriggerRoomLine string
	OutcomeDM       string
	OutcomeRoomLine string
	GoldMin         int64
	GoldMax         int64
	XP              int
	XPSkill         string // "combat", "mining", "foraging", or "" for activity-based
	Activity        string // "any" | "dungeon" | "mining" | "foraging"
}

var advRandomEvents = []AdvRandomEvent{

	// ── FOOD & BODILY CONSEQUENCES ────────────────────────────────────────────

	{
		Key: "roast_chicken_wall",
		TriggerDM: "While picking at a crumbling wall for no reason you can adequately explain, " +
			"you find a roast chicken.\n\n" +
			"It has been in there a while. How long is unclear. " +
			"The colour is wrong. The smell is wrong. " +
			"A part of your brain is screaming at you and another part is already eating it.\n\n" +
			"You eat it. All of it. You lick your fingers.\n\n" +
			"Twenty minutes later your stomach has filed a formal complaint " +
			"that is escalating rapidly through your entire body.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} found something in a wall and ate it. This is developing.",
		OutcomeDM: "Instead of holding it like an idiot — which is exactly what you would have done — " +
			"you drop trow and handle your situation in the nearest available ditch.\n\n" +
			"There is a person in the ditch.\n\n" +
			"The person immediately stands, climbing out of the ditch toward you, " +
			"covered in the consequences of your recent dietary decision. " +
			"You notice the name on their uniform: S. Kelly.\n\n" +
			"S. Kelly does not confront you. S. Kelly makes a sound. " +
			"An excited sound. Reaches into a pocket and throws a damp wad of bills at your feet.\n\n" +
			"You pick them up. You make eye contact with S. Kelly one final time. " +
			"You leave at pace.\n\n" +
			"Some questions are better left unasked. You have €{gold}.\n\n" +
			"S. Kelly watches you go.",
		OutcomeRoomLine: "✅ {name} handled the situation. S. Kelly was involved. Nobody is elaborating.",
		GoldMin:         35,
		GoldMax:         80,
		XP:              0,
		Activity:        "any",
	},

	{
		Key: "burning_house",
		TriggerDM: "There is a house on fire.\n\n" +
			"You run in.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has run into a burning building.",
		OutcomeDM: "There is nobody inside.\n\n" +
			"There was never anyone inside. You are an idiot who ran into a burning building " +
			"for no reason that holds up to scrutiny.\n\n" +
			"What IS inside is a truly remarkable amount of unsecured valuables " +
			"belonging to someone who left in a hurry. " +
			"You fill your pockets. A beam falls nearby. " +
			"You fill your pockets faster. Another beam falls. " +
			"You make a mental note about the beams and immediately forget it " +
			"because your pockets are full.\n\n" +
			"You emerge from the building on fire, briefly, " +
			"which a neighbour extinguishes with a bucket they were apparently already holding.\n\n" +
			"They applaud. They assume you saved someone. " +
			"You accept this.\n\n" +
			"€{gold}. You are on fire slightly less than before.",
		OutcomeRoomLine: "✅ {name} ran into a burning building. Nobody was inside. {name} was briefly on fire.",
		GoldMin:         80,
		GoldMax:         200,
		XP:              10,
		Activity:        "any",
	},

	{
		Key: "bandit_carriage",
		TriggerDM: "You come across a band of bandits robbing a carriage.\n\n" +
			"There are four of them. They are large. " +
			"The carriage owner is inside with the curtains drawn.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has spotted a bandit ambush and is deciding what kind of person they are.",
		OutcomeDM: "You dash over and immediately confront the bandits. " +
			"You point at them and proclaim that if they resist, " +
			"you will be forced to inflict great pain upon them.\n\n" +
			"They stomp the ever-loving shit out of you.\n\n" +
			"All four. In sequence and then simultaneously. " +
			"Your armor absorbs the first two hits and then stops absorbing. " +
			"You are making sounds you have not made since childhood.\n\n" +
			"The carriage door opens. The owner steps out with an auto-crossbow " +
			"and dispatches all four in approximately eight seconds.\n\n" +
			"The owner walks over to you on the ground and says, calmly, " +
			"that they could have stepped out considerably earlier. " +
			"They say the sight of you getting the stuffing knocked out of you " +
			"was honestly the most entertainment they'd had in weeks. " +
			"They press €{gold} into your hand and return to the carriage, " +
			"already mimicking the high-pitched yelps you made as each blow landed.\n\n" +
			"You walk away. The owner is still laughing. " +
			"You can hear it for quite some time.",
		OutcomeRoomLine: "✅ {name} confronted four bandits. The carriage owner let it play out for a while first.",
		GoldMin:         120,
		GoldMax:         250,
		XP:              25,
		Activity:        "any",
	},

	{
		Key: "mystery_stew",
		TriggerDM: "A stranger shoves a bowl of stew into your hands and walks away.\n\n" +
			"The stew is hot. The stranger is gone. " +
			"You are standing in the street holding someone else's dinner.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ A stranger gave {name} a bowl of stew and immediately left.",
		OutcomeDM: "You eat the stew.\n\n" +
			"It is the best thing you have ever eaten. Not 'best thing today.' Best thing. Ever. " +
			"You stand holding the empty bowl for a full minute just thinking about it.\n\n" +
			"Then you feel incredible. Not good. Incredible. " +
			"Like someone has gone into your settings and fixed something you didn't know was broken.\n\n" +
			"You turn the bowl over. €{gold} taped to the bottom.\n\n" +
			"You will think about this stew for the rest of your life. " +
			"You will never find the stranger. " +
			"This is fine. Some things are better as open wounds.",
		OutcomeRoomLine: "✅ {name} ate the stew. {name} will not be discussing the stew.",
		GoldMin:         20,
		GoldMax:         50,
		XP:              15,
		Activity:        "any",
	},

	// ── CRIME & MORAL FLEXIBILITY ─────────────────────────────────────────────

	{
		Key: "drunk_merchant",
		TriggerDM: "A merchant is asleep at their stall. Aggressively asleep. " +
			"The kind that follows a serious lunch.\n\n" +
			"Their coin purse is on the counter. Three feet away. Unattended.\n\n" +
			"The merchant snores.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has found an unattended coin purse. A moral crossroads.",
		OutcomeDM: "You take the coin purse.\n\n" +
			"You were never not going to take the coin purse.\n\n" +
			"Inside: €{gold}, a very small portrait of someone's dog, " +
			"and a note that says 'DO NOT LOSE THIS.' " +
			"You pocket the money. You leave the note on the counter as a courtesy. " +
			"You leave the portrait because you are not a monster.\n\n" +
			"The merchant snores.\n\n" +
			"You walk away. The merchant wakes up three hours later " +
			"with a note that says DO NOT LOSE THIS " +
			"and no memory of what it referred to.\n\n" +
			"This haunts them. Good.",
		OutcomeRoomLine: "✅ {name} made a financial decision at the market. The merchant will be confused later.",
		GoldMin:         40,
		GoldMax:         120,
		XP:              0,
		Activity:        "any",
	},

	{
		Key: "wrongful_arrest",
		TriggerDM: "A guard grabs you by the collar. " +
			"You match the description of someone who robbed a bakery this morning.\n\n" +
			"You did not rob a bakery this morning. " +
			"You were somewhere significantly worse, which is a different problem.\n\n" +
			"The guard is waiting.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ A guard has {name} by the collar. Bakery robbery. {name} has an alibi of sorts.",
		OutcomeDM: "You explain where you actually were this morning in considerable detail.\n\n" +
			"The guard listens with the expression of someone who cannot determine " +
			"whether a dungeon is a better alibi than a bakery robbery. " +
			"They're still deciding when a runner arrives: actual robber caught three streets over.\n\n" +
			"The guard releases you. You ask about compensation.\n\n" +
			"A long pause. The guard produces a voucher for a free loaf " +
			"from the robbed bakery, which they apparently issue to witnesses as goodwill. " +
			"You go to the bakery. You sell the loaf you bought before knowing about the voucher for €{gold}.\n\n" +
			"The justice system has, in its way, provided.",
		OutcomeRoomLine: "✅ {name} was detained and compensated with bread. The system works, loosely.",
		GoldMin:         25,
		GoldMax:         60,
		XP:              5,
		Activity:        "any",
	},

	{
		Key: "tax_refund",
		TriggerDM: "A tax collector approaches you with a ledger and an apologetic expression.\n\n" +
			"The city has been over-collecting from you for several years. " +
			"There is a refund.\n\n" +
			"You have never paid city taxes. Not once. Not ever.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ A tax collector has found {name} in a ledger. A refund is being discussed.",
		OutcomeDM: "You accept the refund.\n\n" +
			"You nod seriously while the amount is calculated. " +
			"You say 'that sounds about right' in the tone of someone " +
			"who has been thinking about this number for years. " +
			"You sign the form. The collector apologises on behalf of the city.\n\n" +
			"€{gold} is counted into your hand.\n\n" +
			"Somewhere in the city, someone who actually paid those taxes " +
			"is still waiting on their refund.\n\n" +
			"Not your problem.",
		OutcomeRoomLine: "✅ {name} accepted a refund for taxes they never paid. The city apologised.",
		GoldMin:         100,
		GoldMax:         250,
		XP:              0,
		Activity:        "any",
	},

	{
		Key: "pickpocket_reversed",
		TriggerDM: "Someone has just tried to pick your pocket.\n\n" +
			"You felt it. They're still standing right next to you, " +
			"hand inside your coat, making eye contact and smiling " +
			"with the frozen confidence of someone whose plan has not fully resolved.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ Someone is in {name}'s pocket. Both parties are aware of this.",
		OutcomeDM: "You look down at the hand. You look up at the person. " +
			"The person looks at you. A long moment.\n\n" +
			"You pick their pocket.\n\n" +
			"While they are processing what just happened, " +
			"while the hand is still technically inside your coat, " +
			"you reach into their jacket and remove their wallet " +
			"with the smooth efficiency of someone who has spent considerable time in dungeons " +
			"and has a working relationship with fast decisions.\n\n" +
			"The pickpocket looks at their own empty pocket. Looks at you. " +
			"Has the expression of someone revising a career.\n\n" +
			"You hand them back their own wallet, minus €{gold} for the inconvenience, " +
			"and walk away while they work through the sequence of events.\n\n" +
			"They do not follow you. They sit down on the nearest step " +
			"and think about their choices. Good.",
		OutcomeRoomLine: "✅ {name} was pickpocketed and immediately pickpocketed back. Net outcome: positive.",
		GoldMin:         30,
		GoldMax:         85,
		XP:              10,
		Activity:        "any",
	},

	// ── WILDLIFE ─────────────────────────────────────────────────────────────

	{
		Key: "runaway_pig",
		TriggerDM: "A pig is running directly at you at a speed pigs should not be capable of.\n\n" +
			"It is large. It has committed to its direction. You are in its direction.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} is in the path of a large fast pig.",
		OutcomeDM: "You sidestep the pig at the last possible moment.\n\n" +
			"The pig continues. You hear it round a corner. " +
			"Then shouting. Then nothing. You choose not to investigate.\n\n" +
			"A farmer arrives thirty seconds later, completely out of breath, " +
			"and asks which way. You point. The farmer presses coins into your hand " +
			"without breaking stride.\n\n" +
			"You stood in a road and moved slightly to the left. €{gold}.\n\n" +
			"The shouting was bad. " +
			"You do not ask what happened to the pig.",
		OutcomeRoomLine: "✅ {name} avoided the pig and was paid for it. The pig's status is unknown.",
		GoldMin:         15,
		GoldMax:         40,
		XP:              0,
		Activity:        "any",
	},

	{
		Key: "suspicious_crow",
		TriggerDM: "A crow has been following you for twenty minutes.\n\n" +
			"Not flying past. Following. Fence to fence, tree to tree, " +
			"making sustained eye contact every time you check.\n\n" +
			"You have checked six times.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ A crow is following {name}. Sustained eye contact has been established.",
		OutcomeDM: "You stop and face the crow.\n\n" +
			"It lands in front of you and drops a ring from its beak. " +
			"Silver. Engraved with initials you don't recognise. " +
			"Dropped with the deliberate energy of something being returned.\n\n" +
			"The crow watches you pick it up. Then it leaves. " +
			"Just flies away. Done. No further communication.\n\n" +
			"You stand in the road holding a ring a crow gave you.\n\n" +
			"A jeweller gives you €{gold} and asks no questions.\n\n" +
			"You do not ask questions either.\n\n" +
			"On the way home you see the crow drop a ring at someone else's feet.\n\n" +
			"The crow looks up at you. Brief eye contact.\n\n" +
			"The crow looks away. Business as usual.",
		OutcomeRoomLine: "✅ The crow gave {name} a ring and left. It gives everyone rings. Questions: none.",
		GoldMin:         50,
		GoldMax:         110,
		XP:              0,
		Activity:        "any",
	},

	{
		Key: "horse_standoff",
		TriggerDM: "A horse is standing in the middle of the road and will not move.\n\n" +
			"Not startled. Not lost. Just standing there with the settled energy " +
			"of something that has made a decision and is at peace with it. " +
			"A queue of people on both sides. Nobody doing anything.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ A horse is blocking the road. {name} is the nearest person with any credentials.",
		OutcomeDM: "You approach the horse.\n\n" +
			"The horse looks at you. You say something — more of a tone than words. " +
			"The horse considers this and then steps aside.\n\n" +
			"The crowd applauds. A merchant whose cargo has been stuck forty minutes " +
			"presses €{gold} into your hand.\n\n" +
			"You do not know what you said to the horse. " +
			"You walk away before anyone asks you to try.\n\n" +
			"Behind you, someone else approaches the horse with great confidence.\n\n" +
			"The horse bites them immediately.\n\n" +
			"You do not look back.",
		OutcomeRoomLine: "✅ {name} said something to a horse and it worked. The next person was bitten immediately. {name} cannot explain this.",
		GoldMin:         20,
		GoldMax:         55,
		XP:              0,
		Activity:        "any",
	},

	{
		Key: "cat_with_something",
		TriggerDM: "A cat has dropped something at your feet and is sitting back " +
			"looking at you with the expectant energy of a cat that has done its part " +
			"and is waiting for you to do yours.\n\n" +
			"The something is small and wrapped in cloth " +
			"and the cat is not going to explain itself.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ A cat has left something at {name}'s feet and is waiting.",
		OutcomeDM: "You pick up the cloth and unwrap it.\n\n" +
			"A key. Old. Specific. The kind that opens one thing " +
			"and that one thing is somewhere.\n\n" +
			"You spend an hour finding what the key opens. " +
			"The cat follows you the entire time, maintaining three feet of distance, " +
			"watching with total composure.\n\n" +
			"The key opens a small lockbox behind a loose brick " +
			"in a wall the cat led you to by sitting in front of it and waiting.\n\n" +
			"Inside: €{gold} and a note that says 'good cat.'\n\n" +
			"The cat has already left when you look up. " +
			"The note was for the cat. " +
			"The money was apparently for you. " +
			"You do not understand any part of this and you never will.",
		OutcomeRoomLine: "✅ A cat led {name} to a lockbox. The note inside was for the cat. The money was for {name}.",
		GoldMin:         45,
		GoldMax:         100,
		XP:              0,
		Activity:        "any",
	},

	// ── PEOPLE & SOCIAL DISASTERS ─────────────────────────────────────────────

	{
		Key: "funeral_wrong_procession",
		TriggerDM: "You have accidentally joined a funeral procession.\n\n" +
			"You don't know how. One moment you were walking, " +
			"the next you were part of a slow column of mourners " +
			"and everyone assumed you belonged " +
			"and you didn't correct anyone " +
			"and now the church is very close.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has accidentally joined a funeral. The church is very close.",
		OutcomeDM: "You attend the entire funeral.\n\n" +
			"An hour and forty minutes. Third pew. You accept a memorial card " +
			"for one Roderick Hannaway, beloved, " +
			"and fold it carefully into a pocket.\n\n" +
			"At the reception a woman assumes you were a business colleague " +
			"and spends twenty minutes telling you what a genuinely difficult man Roderick was. " +
			"She cries twice. You hand her a napkin both times " +
			"because the napkins are right there and it would be weird not to.\n\n" +
			"She presses €{gold} into your hand and says Roderick would have wanted you to have it.\n\n" +
			"Based on everything you've heard today, " +
			"Roderick would have wanted no such thing. " +
			"Roderick sounds like he was awful. " +
			"You have his money now. " +
			"Karma is imprecise but directionally sound.",
		OutcomeRoomLine: "✅ {name} attended Roderick Hannaway's funeral uninvited. Roderick was difficult. {name} has the money.",
		GoldMin:         60,
		GoldMax:         130,
		XP:              10,
		Activity:        "any",
	},

	{
		Key: "bar_fight_aftermath",
		TriggerDM: "The bar fight is over.\n\n" +
			"You weren't in it. You were at a corner table minding your business " +
			"when it erupted and now it's done and there are people on the floor " +
			"in various states of consciousness and the barkeep is looking at everyone " +
			"like they're all someone else's problem.\n\n" +
			"You look at the people on the floor.\n\n" +
			"You look at your medical kit.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ A bar fight has concluded near {name}. {name} has a medical kit and is looking at it.",
		OutcomeDM: "You open the medical kit and get to work.\n\n" +
			"You also get out your pricing chart.\n\n" +
			"The first one is a large man with a broken nose. " +
			"Standard rate. You patch him up, he pays, he leaves without making eye contact.\n\n" +
			"The second is a woman who is, frankly, very attractive, " +
			"which means she has had it easy her entire life " +
			"and can afford to have it slightly less easy right now. " +
			"You charge her triple. She pays it. You feel nothing about this.\n\n" +
			"The third is a child.\n\n" +
			"A sweet child. A small child with enormous eyes " +
			"who had absolutely no business being in a bar during a bar fight " +
			"and has a gash on their forehead that needs cleaning.\n\n" +
			"You look at the child with sympathetic eyes.\n\n" +
			"You charge the child the most of all.\n\n" +
			"'This,' you say, cleaning the wound with practiced efficiency, " +
			"'is what happens when you meddle in grown folks' business.'\n\n" +
			"The parent shows up and pays you while glaring at you the entire time. " +
			"In fact the whole room is staring daggers at you " +
			"but you don't mind at all.\n\n" +
			"You smile and wave back at them as you count your money " +
			"while continuing on your way.\n\n" +
			"Friends made today: -20.\n" +
			"Money made today: €{gold}. A whole lot.",
		OutcomeRoomLine: "✅ {name} treated the bar fight wounded on a sliding scale. The child paid the most. Friends made: -20.",
		GoldMin:         85,
		GoldMax:         190,
		XP:              10,
		Activity:        "any",
	},

	{
		Key: "duel_wrong_person",
		TriggerDM: "Someone has challenged you to a duel.\n\n" +
			"Wrong person. Wrong name. Wrong grievance. " +
			"The passion with which they're delivering the challenge " +
			"suggests this has been building for a long time " +
			"in someone else's direction.\n\n" +
			"There is a crowd. They are waiting for your response.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has been challenged to a duel. Wrong person. {name} hasn't clarified this.",
		OutcomeDM: "You accept.\n\n" +
			"Correcting the situation now feels impossible. " +
			"There is a crowd. A duelling ground. A ritual. " +
			"You go through all of it.\n\n" +
			"Your opponent is very good. Specifically trained, " +
			"for years probably, for a fight against a person who is not you.\n\n" +
			"Halfway through the duel they stop, squint at you, " +
			"and say slowly that you are not who they thought.\n\n" +
			"A long silence.\n\n" +
			"They sheathe their weapon. Press €{gold} into your hand " +
			"as 'compensation for the inconvenience' and leave at speed.\n\n" +
			"The crowd disperses. You stand on the duelling ground alone " +
			"wondering who they were actually looking for " +
			"and whether that person knows what's coming.\n\n" +
			"They don't know. They will find out. " +
			"That is not your problem.",
		OutcomeRoomLine: "✅ {name} accepted a duel meant for someone else. Error discovered mid-duel. Compensation paid.",
		GoldMin:         80,
		GoldMax:         170,
		XP:              30,
		Activity:        "any",
	},

	{
		Key: "wrong_house_dinner",
		TriggerDM: "You have walked into the wrong house.\n\n" +
			"The door was open. You thought it was somewhere else. " +
			"It is very much not somewhere else.\n\n" +
			"There is a family eating dinner. " +
			"They are looking at you. " +
			"You are looking at them.\n\n" +
			"Nobody has said anything yet.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} is standing in a stranger's home. Dinner is in progress. Nobody has spoken.",
		OutcomeDM: "You sit down.\n\n" +
			"There is an empty chair and your body uses it " +
			"before your brain can propose an alternative.\n\n" +
			"The family, after a moment, continues dinner.\n\n" +
			"They serve you a plate. The food is excellent. " +
			"Nobody explains the empty chair. You don't ask. " +
			"Conversation happens around you the way weather happens — " +
			"you're present without being the cause of it.\n\n" +
			"The eldest person at the table gives you €{gold} at the end " +
			"and says 'for the road' and shows you out.\n\n" +
			"On the way out you notice a painting on the wall.\n\n" +
			"It is a painting of you.\n\n" +
			"Not someone who looks like you. You.\n\n" +
			"The door closes before you can say anything.\n\n" +
			"You stand on the step for a moment.\n\n" +
			"You walk home.",
		OutcomeRoomLine: "✅ {name} sat at a stranger's dinner table. Was served. Was paid. The chair was waiting.",
		GoldMin:         30,
		GoldMax:         80,
		XP:              10,
		Activity:        "any",
	},

	{
		Key: "street_prophet",
		TriggerDM: "A street prophet has stopped their sermon to point at you.\n\n" +
			"The crowd is looking at you. The prophet is describing a prophecy " +
			"that is clearly about you — your description, your general situation, " +
			"several accurate details that are uncomfortably specific.\n\n" +
			"The prophet is waiting.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ A street prophet has identified {name} in a prophecy. The crowd is watching.",
		OutcomeDM: "You step forward.\n\n" +
			"The prophet delivers the rest of it directly to you, " +
			"in detail, for eleven minutes. A great task. A difficult road. " +
			"A door that 'will know you when you find it.'\n\n" +
			"You have no idea what door. You go through a lot of doors. " +
			"One of them is apparently significant " +
			"and you will not recognise the moment when it happens.\n\n" +
			"The crowd donates €{gold} to you afterward, " +
			"unprompted, as if this is simply what you do when someone is in a prophecy.\n\n" +
			"You walk away with money and a prophecy you don't understand " +
			"and the distinct feeling that something is now your problem " +
			"that wasn't your problem this morning.\n\n" +
			"Behind you the prophet points at someone else in the crowd " +
			"and begins describing a new prophecy with the same specific details. " +
			"Your description. Your equipment. Your general situation.\n\n" +
			"The prophet makes eye contact with you briefly.\n\n" +
			"The prophet looks away.",
		OutcomeRoomLine: "✅ {name} is in a prophecy. A door will know them. Nobody can say which door.",
		GoldMin:         50,
		GoldMax:         110,
		XP:              20,
		Activity:        "any",
	},

	// ── FOUND OBJECTS ─────────────────────────────────────────────────────────

	{
		Key: "mysterious_clock",
		TriggerDM: "There is a package on your doorstep.\n\n" +
			"Your name on it, spelled correctly. " +
			"No return address. " +
			"A small but definite amount of ticking.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has received an anonymous package. It is ticking.",
		OutcomeDM: "You open it.\n\n" +
			"A clock. Small, ornate, running accurately. " +
			"A note: 'Sorry about the clock. Keep it. — T'\n\n" +
			"You don't know anyone whose name starts with T.\n\n" +
			"You sell it to a jeweller immediately " +
			"because a clock that arrives with a preemptive apology " +
			"is not a clock you want in your home.\n\n" +
			"€{gold}. The jeweller asks no questions.\n\n" +
			"You are half a street away when the explosion happens.\n\n" +
			"You turn around.\n\n" +
			"The jeweller is standing in the smoking doorway, " +
			"entirely unharmed, holding something aloft, " +
			"screaming at the top of their lungs.\n\n" +
			"'I'm rich! I'm rich! " +
			"Fuck all of you dirty bitches because I'm stinking wealthy!'\n\n" +
			"Congratulations. " +
			"That explosion would have probably had your ears ringing for a bit. " +
			"Whew.",
		OutcomeRoomLine: "✅ {name} received a clock and an apology from T. Both have been processed.",
		GoldMin:         55,
		GoldMax:         120,
		XP:              0,
		Activity:        "any",
	},

	{
		Key: "well_box",
		TriggerDM: "There is something shining at the bottom of a well.\n\n" +
			"The bucket is right there.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} is looking at something shining at the bottom of a well.",
		OutcomeDM: "You go in.\n\n" +
			"Not in the bucket — the bucket wouldn't hold you — " +
			"but using the rope and the wall and a technique you invent on the way down " +
			"that works better than it has any right to.\n\n" +
			"At the bottom: an ornate box. Waterlogged. Locked. Heavy.\n\n" +
			"You get it in the bucket and climb back out, " +
			"which is harder than going down in the specific way " +
			"that makes you hate yourself for not planning this.\n\n" +
			"The box contains €{gold} in old currency and a letter.\n\n" +
			"You read the letter.\n\n" +
			"You read it again.\n\n" +
			"You drop it into a different well.\n\n" +
			"The money is yours. " +
			"The letter is the other well's problem. " +
			"You are not thinking about the letter.",
		OutcomeRoomLine: "✅ {name} went into a well. Came out. The letter has been re-disposed of in a different well.",
		GoldMin:         90,
		GoldMax:         210,
		XP:              10,
		Activity:        "any",
	},

	{
		Key: "treasure_map",
		TriggerDM: "Someone has pressed a piece of paper into your hand and walked away very fast.\n\n" +
			"It is old. There are markings. There is an X.\n\n" +
			"There is always an X.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has been given a piece of paper by someone who left immediately.",
		OutcomeDM: "You follow the map.\n\n" +
			"It leads, through three wrong turns and one moment of genuine doubt " +
			"about your entire direction in life, " +
			"to a loose flagstone in an unremarkable courtyard.\n\n" +
			"Under the flagstone: a tin. Inside the tin: €{gold} " +
			"and a note that says TELL NO ONE.\n\n" +
			"You replace the flagstone. You pocket the money.\n\n" +
			"You are telling everyone.",
		OutcomeRoomLine: "✅ {name} followed the X. It worked. {name} has been asked to tell no one.",
		GoldMin:         70,
		GoldMax:         160,
		XP:              5,
		Activity:        "any",
	},

	// ── ACTIVITY-SPECIFIC ─────────────────────────────────────────────────────

	{
		Key: "dungeon_staff_entrance",
		TriggerDM: "You've found a back entrance to a dungeon.\n\n" +
			"A side door. Half-hidden. " +
			"The kind that exists because someone who worked here " +
			"wanted a way in that the front-door things didn't know about.\n\n" +
			"It's open.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has found an unmarked open door in a dungeon.",
		OutcomeDM: "You go in.\n\n" +
			"Staff area. A schedule in a language you don't read. " +
			"A considerable amount of food the monsters keep back here. " +
			"Three goblins on break who look up at you.\n\n" +
			"Under the table: a strongbox the goblins were clearly not supposed to be near.\n\n" +
			"You take the strongbox. You take some of the food. " +
			"The goblins watch and say nothing, " +
			"possibly because you've caught them somewhere they're not supposed to be " +
			"and the situation is mutually awkward for everyone.\n\n" +
			"You make eye contact with each goblin in turn. " +
			"A tacit agreement is reached. " +
			"You leave. The goblins file a complaint with management.\n\n" +
			"Management investigates and finds nothing useful.\n\n" +
			"Management is one of the goblins.\n\n" +
			"The goblins know exactly what happened.\n\n" +
			"The lock has already been changed.\n\n" +
			"€{gold}. The food was decent.",
		OutcomeRoomLine: "✅ {name} found the dungeon staff area. The goblins were on break. An agreement was reached.",
		GoldMin:         80,
		GoldMax:         180,
		XP:              20,
		XPSkill:         "combat",
		Activity:        "dungeon",
	},

	{
		Key: "surface_ore",
		TriggerDM: "There is ore sitting on the ground.\n\n" +
			"Not in a mine. Not behind anything. " +
			"Just on the ground, in the open. " +
			"Several people have walked past it.\n\n" +
			"You don't know why nobody has taken it.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has found ore on the ground. Nobody has taken it. {name} is suspicious.",
		OutcomeDM: "You pick it up.\n\n" +
			"Nothing happens.\n\n" +
			"No one objects. No one appears. " +
			"The ground doesn't care. " +
			"The ore is just ore that was on the ground " +
			"and is now in your pocket.\n\n" +
			"You sell it for €{gold}.\n\n" +
			"The people who walked past it will think about this for the rest of the day. " +
			"Some of them will still be thinking about it next week.\n\n" +
			"You will not be thinking about it. " +
			"€{gold} has a clarifying effect on the mind.",
		OutcomeRoomLine: "✅ {name} picked up free ore off the ground. Nothing happened. €{gold}.",
		GoldMin:         20,
		GoldMax:         65,
		XP:              5,
		XPSkill:         "mining",
		Activity:        "mining",
	},

	{
		Key: "unusual_plant",
		TriggerDM: "You've spotted a plant you've never seen before.\n\n" +
			"It's doing something. Not moving — present in a way regular plants aren't. " +
			"It's not on any foraging chart you've seen.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has found an unidentified plant that is present in an unusual way.",
		OutcomeDM: "You take a cutting.\n\n" +
			"A small piece. Just enough. " +
			"The plant doesn't react. " +
			"You're not certain what a reaction would look like " +
			"and you don't push your luck.\n\n" +
			"A herbalist in town goes very still when you show it to them. " +
			"The kind of still that means they know exactly what it is " +
			"and are working very hard not to show you that they know.\n\n" +
			"They buy it for €{gold} with the focused calm " +
			"of someone containing a significant emotion.\n\n" +
			"You don't ask what it is. " +
			"Whatever you just sold, the foraging skill has decided it counts.",
		OutcomeRoomLine: "✅ {name} sold an unidentified plant to a herbalist who was very controlled about wanting it.",
		GoldMin:         60,
		GoldMax:         140,
		XP:              25,
		XPSkill:         "foraging",
		Activity:        "foraging",
	},

	{
		Key: "army_enlistment",
		TriggerDM: "A company is giving away free clothing.\n\n" +
			"You can see the table from here. Nice stuff too — " +
			"proper fabric, clean lines, nothing corroded or held together by optimism. " +
			"An actual upgrade over what you're currently wearing, " +
			"which the game has already described at length and does not need to describe again.\n\n" +
			"You get in line.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has spotted a free clothing giveaway and is getting in line.",
		OutcomeDM: "You wait your turn. You get to the front. " +
			"They hand you a very nice set of clothing that costs you absolutely nothing.\n\n" +
			"You are shuffled into a changing room.\n\n" +
			"You emerge from the changing room onto a battlefield.\n\n" +
			"This was not a clothing giveaway. " +
			"This was an army enlistment event. " +
			"Your reading ability, never your strongest attribute, " +
			"has failed you at a meaningful moment.\n\n" +
			"You are assigned to the Commanding General's meat shield group. " +
			"The arrangement is explained to you clearly: " +
			"stand in front of the General with your shield raised, " +
			"keep the General alive, receive 20,000 gold per day. " +
			"This is, mathematically, an excellent rate of pay.\n\n" +
			"You stand tall on the battlefield. Shield raised. General behind you. " +
			"The others in your group doing the same. " +
			"You are, for the first time in your life, part of something larger than yourself.\n\n" +
			"You notice a shiny button on the ground.\n\n" +
			"You bend over to pick it up.\n\n" +
			"The arrow that would have been stopped by your shield " +
			"strikes the Commanding General in the head, killing him instantly.\n\n" +
			"You are discharged from the army on the spot. " +
			"No gold. No commendation. No acknowledgement that you were " +
			"technically doing your job correctly right up until the button.\n\n" +
			"You do have a shiny button and €{gold} in back-pay for the twelve minutes you were technically enlisted.\n\n" +
			"Huzzah.",
		OutcomeRoomLine: "✅ {name} accidentally enlisted in an army and immediately got the General killed. Has a button and €{gold}.",
		GoldMin:         8,
		GoldMax:         22,
		XP:              5,
		Activity:        "any",
	},

	{
		Key: "tower_bird",
		TriggerDM: "There is a tall abandoned lookout tower nearby.\n\n" +
			"If you climbed it, you could survey the surrounding area. " +
			"Tactically useful. Good information. A sound decision.\n\n" +
			"It's time to do this like a professional.\n\n" +
			"Let's climb this ridiculously tall and abandoned lookout tower that is in extreme stages of disrepair. " +
			"The view from up there could be very enlightening!\n\n" +
			"The tower sways slightly in the wind from down here.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has decided to climb an abandoned tower that is visibly swaying.",
		OutcomeDM: "The tower sways violently back and forth as you climb. " +
			"Each gust is a conversation between the wind and the structural integrity " +
			"of something that has been abandoned for good reason.\n\n" +
			"After what seemed like ages — but thanks to the miracles of narrative " +
			"storytelling was just the next sentence for us — " +
			"you reach the top.\n\n" +
			"Before you can survey anything, something catches your eye. " +
			"Something sparkly. Inside a bird's nest.\n\n" +
			"A treasure, perhaps.\n\n" +
			"From the size of the nest, even a half-wit could tell " +
			"this is no bird to be trifled with. " +
			"Thankfully, you are not the average half-wit, " +
			"and you proceed to dig through the nest.\n\n" +
			"You find what you're seeking because it cuts you. " +
			"A worthless piece of broken glass. " +
			"You yank your hand back in pain, knocking something out of the nest " +
			"that makes a sound like an egg shattering.\n\n" +
			"It was an egg.\n\n" +
			"As luck would have it, mum is right about to arrive home.\n\n" +
			"A miracle occurs: you finally understand the situation you are in " +
			"and make haste down the ladder. " +
			"The bird lands on top of the tower. " +
			"For one brief moment you breathe a sigh of relief.\n\n" +
			"The bird looks down.\n\n" +
			"The bird begins descending.\n\n" +
			"You put up what few would consider an effective counterattack. " +
			"You are knocked off the ladder.\n\n" +
			"Down and down and down.\n\n" +
			"Splat.\n\n" +
			"You are not dead.\n\n" +
			"As luck would have it, you have landed on an enormous and moist pile of manure. " +
			"The bits on the very top are still warm. " +
			"You turn to see a farmer walking away with a shovel. " +
			"Must be fresh.\n\n" +
			"As you dig yourself out you notice something stuck in your hand. " +
			"A claw. A beautiful one, from that very large and very angry bird. " +
			"This ought to sell for a decent amount.\n\n" +
			"Mission accomplished.",
		OutcomeRoomLine: "✅ {name} climbed a tower, broke an egg, got knocked off the ladder, and landed in manure. Has a claw.",
		GoldMin:         45,
		GoldMax:         95,
		XP:              10,
		Activity:        "any",
	},

	{
		Key: "monkey_coin",
		TriggerDM: "You're walking home from the market.\n\n" +
			"You were so unsuccessful at haggling that you ended up " +
			"paying more than list price for your items, " +
			"which is an achievement in the wrong direction.\n\n" +
			"As you make your way home, you spot a monkey.\n\n" +
			"Ordinarily, most sensible folks would question what a monkey " +
			"is doing out here, whether it escaped from somewhere, " +
			"what the implications are.\n\n" +
			"You are fixated on what's in its hand.\n\n" +
			"A shiny gold coin.\n\n" +
			"Reply `!adventure respond` within 2 hours.",
		TriggerRoomLine: "⚡ {name} has spotted a monkey with a gold coin and is devising a plan.",
		OutcomeDM: "What's a monkey doing with a gold coin? " +
			"It doesn't need it. You'll use your superior wits to outsmart it " +
			"and claim that coin for yourself.\n\n" +
			"You devise an ingenious plan.\n\n" +
			"You'll toss an interesting-looking rock at it and it'll grab the rock instead, " +
			"leaving the gold coin behind. And why not? " +
			"This stupid monkey will only care that the rock is way bigger than the coin.\n\n" +
			"After a few moments you finally stop congratulating yourself " +
			"long enough to put the plan into action.\n\n" +
			"You gently toss the rock towards the monkey.\n\n" +
			"Eureka.\n\n" +
			"The monkey drops the coin exactly as planned and runs for the rock. " +
			"You hurry over and snatch up the coin. " +
			"You are about to perform your god awful victory dance " +
			"when something clocks you in the back of the head " +
			"causing you to drop your groceries.\n\n" +
			"Instead of turning around to confront your assailant " +
			"you focus on picking up the groceries, " +
			"as getting smacked in the head is a common occurrence for you " +
			"and the groceries are not.\n\n" +
			"You finally get yourself together and look up.\n\n" +
			"The monkey is holding the gold coin and your bananas.\n\n" +
			"It laughs.\n\n" +
			"It tosses you the coin out of pity and scurries away.\n\n" +
			"You did it.\n\n" +
			"Congratulations.",
		OutcomeRoomLine: "✅ {name} outsmarted a monkey. The monkey took the bananas and gave back the coin out of pity.",
		GoldMin:         1,
		GoldMax:         1,
		XP:              40,
		Activity:        "any",
	},
}

// ── FORMAT CONSTANTS ──────────────────────────────────────────────────────────

const advEventRoomTriggerWrapper = "{trigger_room_line}\n" +
	"They have 2 hours to respond with `!adventure respond`."

const advEventRoomOutcomeWrapper = "{outcome_room_line}\n" +
	"(+€{gold}{xp_suffix})"
