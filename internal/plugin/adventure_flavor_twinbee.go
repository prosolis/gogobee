package plugin

// ── TWINBEE NARRATIVE POOLS ───────────────────────────────────────────────────
// All strings are in third person. TwinBee never fails.
// TwinBee withdraws, pivots, repositions, reconvenes, and returns.
// TwinBee does not lose. TwinBee has setbacks that are tactically indistinct
// from victories, looked at the right way, which is TwinBee's way.

var TwinBeeSuccess = []string{
	"TwinBee descended into {location} with characteristic enthusiasm " +
		"and the kind of confidence that makes dungeon monsters briefly " +
		"reconsider their career choices. Several reconsidered too slowly. " +
		"TwinBee has returned with {loot} worth €{value} and a new personal philosophy " +
		"regarding the optimum angle of attack on a Stone Golem. " +
		"The philosophy works. It has been field-tested. Just now.",

	"Another flawless operation. TwinBee entered {location}, assessed the situation " +
		"with professional precision, and extracted {loot} worth €{value} " +
		"with minimal collateral damage to TwinBee specifically. " +
		"The dungeon has been informed it performed adequately. " +
		"TwinBee does not give compliments freely. This was a compliment.",

	"TwinBee has returned from {location} victorious, which is to say " +
		"TwinBee has returned from {location}, because for TwinBee these are the same thing. " +
		"{loot} worth €{value}. {xp} XP that TwinBee does not need " +
		"but accepts graciously, as a gift, from the dungeon. " +
		"The dungeon can try harder next time. It probably won't. They never do.",

	"TwinBee located {loot} worth €{value} in {location} after what TwinBee " +
		"is describing in the post-mission debrief as 'a series of increasingly correct decisions.' " +
		"The monsters involved have been described as 'adequate opposition' " +
		"in the same debrief. TwinBee is generous with these things.",

	"In. Retrieved {loot}. Out. €{value}. {xp} XP. " +
		"TwinBee has the efficiency of someone who has done this many times " +
		"and the confidence of someone who has never once considered " +
		"that it might not go well. These two qualities, combined, produce results. " +
		"The results are in your share of the haul.",

	"TwinBee reports: {location} cleared, {loot} secured, €{value} assessed, " +
		"one near-miss that TwinBee is not classifying as a near-miss " +
		"because TwinBee does not have near-misses, TwinBee has " +
		"'moments of dynamic tactical adjustment that resolved favorably.' " +
		"The moment resolved favorably. Obviously.",

	"The {location} boss encountered TwinBee today. " +
		"This is how TwinBee prefers to frame the encounter — " +
		"the boss encountered TwinBee, not the other way around — " +
		"because it accurately reflects the power dynamics. " +
		"{loot} worth €{value}. The boss has been encountered.",

	"TwinBee went to {location} and came back with {loot} worth €{value}, " +
		"which is what TwinBee does, and {xp} XP, which TwinBee acknowledges " +
		"with the quiet grace of someone for whom excellence " +
		"has long since stopped being a surprise.",

	"Professional assessment of today's {location} operation: successful. " +
		"Loot: {loot}, €{value}. Opposition: handled. Duration: optimal. " +
		"TwinBee's equipment: immaculate. TwinBee: as expected.",

	"TwinBee extracted {loot} worth €{value} from {location} " +
		"with the efficiency of a professional and the cheerfulness of someone " +
		"who genuinely enjoys doing this, which TwinBee does, " +
		"which is why TwinBee is better at it than everyone else, " +
		"which TwinBee is.",

	"The monsters in {location} gave it their best today. " +
		"TwinBee noticed this and respected it for the duration of the encounter, " +
		"which was brief. {loot} worth €{value}. {xp} XP. " +
		"TwinBee has returned in excellent spirits, which is not news, " +
		"but it is accurate.",

	"TwinBee's route through {location} was, according to TwinBee's own review, " +
		"'essentially perfect.' No argument has been presented to the contrary. " +
		"TwinBee returned with {loot} worth €{value} and a complete absence " +
		"of anything that went wrong, which is TwinBee's preferred kind of absence.",

	"A lesser adventurer would have found {location} challenging today. " +
		"TwinBee found it brisk. {loot}, €{value}, {xp} XP, " +
		"and the mild satisfaction of a problem that turned out " +
		"to be an appropriately-sized problem. TwinBee appreciates appropriate sizing.",

	"TwinBee entered {location} from the north approach, which TwinBee has determined " +
		"is the correct approach, and exited with {loot} worth €{value}. " +
		"The southern approach is noted. It is not the correct approach. " +
		"TwinBee has opinions about approaches and they are correct opinions.",

	"Operational summary: {location} visited, opposition encountered, opposition handled, " +
		"{loot} extracted at €{value} assessed value, {xp} XP collected, " +
		"TwinBee returned in the condition TwinBee left in, " +
		"which is optimal, because TwinBee is always in optimal condition.",
}

var TwinBeeExceptional = []string{
	"OUTSTANDING PERFORMANCE. TwinBee located and neutralised the dungeon boss " +
		"— which TwinBee describes as 'a scheduling conflict that has now been resolved' — " +
		"and recovered {loot} worth €{value}. " +
		"This is, according to TwinBee, a typical Tuesday. " +
		"TwinBee has had many exceptional Tuesdays. The dungeons are aware.",

	"TwinBee found {loot} worth €{value} in {location} after what TwinBee " +
		"is calling 'a brief but educational exchange' with something very large " +
		"that is no longer a going concern. Bonus XP noted. " +
		"TwinBee noted it first. TwinBee notes everything first.",

	"The inner chamber. TwinBee reached the inner chamber. " +
		"TwinBee notes that reaching the inner chamber requires " +
		"the successful navigation of everything between here and the inner chamber, " +
		"which TwinBee navigated successfully, and then {loot} worth €{value} " +
		"was in the inner chamber, and TwinBee took it. " +
		"The inner chamber is now available to be an outer chamber.",

	"A boss encounter. TwinBee's assessment of the boss: 'interesting.' " +
		"The boss's assessment of TwinBee: insufficient time to complete. " +
		"{loot} worth €{value}. TwinBee has forwarded the boss's personnel file " +
		"to the relevant afterlife authorities with a positive reference.",

	"EXCEPTIONAL. TwinBee's word, used sparingly. " +
		"Today merits it. {loot} worth €{value} from the deepest chamber " +
		"of {location}, retrieved past opposition that TwinBee describes " +
		"as 'the most interesting opposition I've faced this week,' " +
		"which is high praise from TwinBee and accurately assesses the week so far.",

	"TwinBee found something the dungeon wasn't offering publicly. " +
		"TwinBee finds things dungeons aren't offering publicly. " +
		"It's a skill. {loot} worth €{value}. {xp} XP. " +
		"The dungeon has updated its offering. TwinBee approves of the update " +
		"from a position of already having taken the thing.",

	"A critical run. Every encounter resolved correctly. Every chamber yielded. " +
		"Every roll, every decision, every approach: correct. " +
		"{loot} worth €{value}. {xp} XP. " +
		"TwinBee submits this run as the reference standard for {location}. " +
		"Future runs should aspire to this. They will not reach it.",

	"The boss was large. TwinBee is TwinBee. " +
		"These two facts met in {location} and produced {loot} worth €{value} " +
		"and a story that TwinBee is telling to everyone and will continue telling " +
		"until a better story replaces it, which will also be TwinBee's story.",
}

var TwinBeeWithdrawal = []string{
	"TwinBee has completed today's operation and is taking the remainder " +
		"of the afternoon for strategic planning. " +
		"This is unrelated to {location}. {location} is unrelated to everything. " +
		"TwinBee has already moved on and suggests you do the same.",

	"TwinBee assessed {location} with characteristic thoroughness, " +
		"identified that today's objectives had been sufficiently advanced " +
		"by the reconnaissance phase, and withdrew in good order " +
		"before the situation became interesting in ways TwinBee did not schedule. " +
		"This is called professionalism. TwinBee has it.",

	"Today's operation at {location} concluded earlier than projected. " +
		"TwinBee encountered something very large, evaluated the encounter on its merits, " +
		"and determined that returning tomorrow with more information " +
		"was the superior strategic outcome. " +
		"TwinBee was not running. TwinBee does not run. " +
		"TwinBee was moving with purpose in the opposite direction. These are different.",

	"TwinBee left {location} at a time of TwinBee's choosing, " +
		"which happened to coincide with the arrival of something " +
		"that TwinBee is describing in the official report as 'a scheduling conflict.' " +
		"The conflict has been noted. It will be addressed. Tomorrow. " +
		"At a time that suits TwinBee.",

	"TACTICAL WITHDRAWAL EXECUTED. {location} has been informed, " +
		"via the medium of TwinBee's departure, that today was not the day. " +
		"TwinBee knows which days are the days. Today was for gathering intelligence. " +
		"The intelligence gathered is: {location} still exists and should watch itself.",

	"TwinBee retreated from {location} in the same way the tide retreats from the shore — " +
		"deliberately, inevitably, and with the full understanding that it will be back, " +
		"and the shore knows this, and the shore is thinking about it right now.",

	"Something in {location} today required TwinBee to make a decision. " +
		"TwinBee made the decision. The decision was: not today. " +
		"TwinBee's decisions are correct decisions by definition, " +
		"which means 'not today' was correct, which will be proven correct " +
		"when TwinBee returns tomorrow and the thing is no longer a factor. " +
		"These things always work out when TwinBee decides they will.",

	"The mission objectives for today's {location} operation have been " +
		"reclassified as 'Phase One of a Two-Phase Operation,' " +
		"where Phase One is reconnaissance and Phase Two is tomorrow. " +
		"Phase One is complete. Phase One was, in retrospect, always the plan.",

	"TwinBee encountered something in {location} that TwinBee is describing " +
		"as 'an opportunity to demonstrate strategic patience.' " +
		"TwinBee demonstrated strategic patience. The patience was demonstrated " +
		"at significant velocity in the direction of the exit, " +
		"but the patience was present throughout.",

	"Official statement from TwinBee regarding today's {location} operation: " +
		"'Everything went according to plan.' " +
		"The plan has been updated to reflect what happened. " +
		"The updated plan describes what happened as planned. " +
		"TwinBee's plans are always accurate.",

	"TwinBee is back. The thing in {location} is also still there. " +
		"TwinBee is aware of the thing. The thing is aware of TwinBee. " +
		"This is a stalemate that TwinBee will resolve on TwinBee's timeline, " +
		"which is different from the thing's timeline, " +
		"and TwinBee's timeline is the one that matters.",

	"The {location} situation has been described by TwinBee as " +
		"'temporarily inconclusive,' which is a phrase TwinBee uses " +
		"when something very large expressed a strong position " +
		"and TwinBee has chosen to revisit the position on better terms. " +
		"The terms will be better. TwinBee will see to it.",

	"Today, TwinBee gathered extensive tactical data on {location}. " +
		"The data indicates that {location} is well-defended. " +
		"The data was gathered by going in and then coming back out, " +
		"which is a time-honoured data gathering methodology " +
		"that TwinBee has employed before and will employ again, ideally less urgently.",

	"TwinBee's return today is best understood as a 'victory lap around the outside,' " +
		"which is a phrase TwinBee has just coined and will be using going forward. " +
		"A victory lap around the outside of {location} has been completed. " +
		"TwinBee is home. The inside of {location} is tomorrow's victory lap.",

	"Something the size of a building expressed interest in TwinBee at depth. " +
		"TwinBee expressed equal and opposite interest in being elsewhere. " +
		"Both interests were satisfied. TwinBee is elsewhere. " +
		"The something is where it was. The relationship is stable.",
}

var TwinBeeEmpty = []string{
	"TwinBee has returned from {location} and wishes it to be known " +
		"that the dungeon was empty, which TwinBee attributes entirely to their reputation " +
		"preceding them. The monsters left. They knew. " +
		"TwinBee respects this. TwinBee also found nothing. These facts are unrelated.",

	"Intelligence suggested {location} would be productive today. " +
		"TwinBee's intelligence was incorrect. " +
		"TwinBee's intelligence is reviewing its methodology. " +
		"The dungeon contained no loot, no monsters worth discussing, " +
		"and one confused bat that TwinBee has agreed not to mention publicly. " +
		"Moving on.",

	"{location} was cleared. By whom is unclear. TwinBee suspects a rival. " +
		"TwinBee does not have rivals, technically, " +
		"but is prepared to invent one for the purposes of this grievance. " +
		"No loot. No comment. The rival has been noted.",

	"The {location} run today yielded nothing of material value, " +
		"which TwinBee attributes to the universe's temporary misalignment " +
		"with TwinBee's interests. The universe will correct itself. " +
		"TwinBee is giving it until tomorrow.",

	"Empty. {location} was empty today, which is {location}'s problem. " +
		"TwinBee showed up. TwinBee was prepared. TwinBee had the right equipment. " +
		"The dungeon failed to provide an adequate challenge or adequate reward. " +
		"This reflects poorly on the dungeon.",

	"TwinBee cleared four rooms of {location} and found nothing worth the trip. " +
		"TwinBee is not saying the trip was not worth it — TwinBee does not say that — " +
		"but the material outcome was zero and the XP was minimal " +
		"and TwinBee would like to register this as feedback for the dungeon.",

	"The treasure was already gone when TwinBee arrived, " +
		"which means someone got there before TwinBee, " +
		"which is a sentence that has never previously been true " +
		"and which TwinBee is treating as a statistical anomaly " +
		"rather than a pattern that might develop.",

	"No monsters. No treasure. No meaningful opposition. " +
		"Just TwinBee in {location} swinging at things that weren't there. " +
		"TwinBee has described this run as 'character-building.' " +
		"TwinBee's character was already built. " +
		"This run has contributed nothing to the structure.",
}

// ── EQUIPMENT BREAKING ────────────────────────────────────────────────────────
// Indexed by slot name. Select randomly from the pool.
// The replacement is always the Tier 0 version of the slot.

var EquipmentBreaking = map[string][]string{

	"weapon": {
		"Your {item} has given everything it had, which in retrospect was not much " +
			"and also embarrassing to admit out loud. It has returned to its natural state: garbage. " +
			"You are now equipped with the Basic Ass Sword. " +
			"The cycle of mediocrity turns.",

		"Your {item} is dead. We gather not to mourn it — it was not good — " +
			"but to acknowledge that it served. Poorly. Consistently, aggressively poorly. " +
			"The Basic Ass Sword has been reissued. It is equally bad but at least new.",

		"Gone. Your {item} is gone. Condition: zero. Structural integrity: none. " +
			"Dignity: never present. You are back to the Basic Ass Sword " +
			"and we are all pretending this is fine. It is not fine. Upgrade. Please.",

		"Your {item} has shattered. The sound it made was brief " +
			"and embarrassingly small for a weapon failing at a critical moment. " +
			"The Basic Ass Sword, your original, your constant, your shame, " +
			"is back in your hand. As if you never left.",

		"The {item} has returned to its components, which are bad components " +
			"that produced a bad weapon that has now finished being a weapon. " +
			"The Basic Ass Sword is what you have. " +
			"The Basic Ass Sword is, tragically, still a sword.",

		"Broken. The {item} is broken, condition zero, " +
			"broken in a way that happens to weapons that have been asked " +
			"to do more than their condition could support for longer than their condition recommended. " +
			"The Basic Ass Sword does not have these concerns because its expectations are already zero.",

		"Your {item} snapped. The upper half is somewhere you cannot retrieve it. " +
			"The lower half is a handle with opinions. " +
			"You are holding the lower half. The Basic Ass Sword is the new situation. " +
			"The new situation is the old situation.",

		"The blade went one way and the handle went another " +
			"and you went in the direction of 'holding a Basic Ass Sword again' " +
			"because that is where broken weapons return their owners. " +
			"The circle is unbroken. The sword is broken. Welcome back.",
	},

	"armor": {
		"Your {item} has failed. Not dramatically — just failed, quietly, " +
			"at the structural level, in a way that became apparent all at once " +
			"when the last thing hit you. The Shitty Armor is back. " +
			"The Shitty Armor has always been back, waiting.",

		"Condition zero. Your {item} is now the Shitty Armor. " +
			"Not figuratively — the item has been replaced by the actual Shitty Armor, " +
			"the baseline, the floor, the thing below which armor does not go " +
			"and from which you have apparently returned.",

		"The {item} has been compromised. 'Compromised' is the polite word. " +
			"The honest words are 'destroyed' and 'sorry' and 'you're wearing Shitty Armor again.' " +
			"The Shitty Armor offers the protection of a strongly-worded letter. " +
			"It always has. It still does.",

		"Your {item} bent the wrong way. Armor should not bend. " +
			"Armor that bends has broken the implicit contract between armor and wearer, " +
			"which is: you stay rigid, I stay alive. " +
			"The contract is void. The Shitty Armor is the new contract. " +
			"It's a worse contract.",

		"The {item} is done. Condition zero, function zero, " +
			"protective value approximately matching the Shitty Armor that has replaced it, " +
			"which is a different thing than matching actual armor " +
			"but is now your actual situation.",

		"Structural failure. Your {item} has experienced structural failure, " +
			"which is a technical term for 'it broke' that makes you feel " +
			"like an engineer rather than someone wearing broken armor and the Shitty Armor simultaneously, " +
			"which you are. The Shitty Armor fits, though. Sadly, it always fits.",
	},

	"helmet": {
		"Your {item} is gone. Condition zero. The Goddamn Offensive Helmet " +
			"is back on your head, where it was always waiting, " +
			"crouching in the back of your inventory, " +
			"knowing this day would come. The helmet knew.",

		"The {item} has been retired by force. In its place: the Goddamn Offensive Helmet. " +
			"It is bad for your head. It is an insult to everyone who sees it. " +
			"It fits perfectly, which is the most offensive thing about it.",

		"Condition zero means the Goddamn Offensive Helmet. " +
			"The Goddamn Offensive Helmet means you've come full circle. " +
			"The circle is not a triumph. The circle is a shape your bad decisions make " +
			"when you don't repair your gear.",

		"Your {item} cracked in half. The two halves are not useful halves. " +
			"The useful replacement is the Goddamn Offensive Helmet, " +
			"which has been retrieved from the bottom of your pack " +
			"where it was waiting with complete patience " +
			"because it knew. It always knows.",

		"The {item} has betrayed you in the specific way of head protection: " +
			"by not being head protection anymore. " +
			"The Goddamn Offensive Helmet has stepped in to fill the void. " +
			"The Goddamn Offensive Helmet fills the void in the same way " +
			"a bad apology fills a silence. Technically. Poorly.",
	},

	"boots": {
		"Your {item} are gone. Condition zero. The Knobby-Ass Boots are your boots now. " +
			"The knobs are not a feature. Nobody has ever established what they are. " +
			"You will be wearing them until you can afford better.",

		"The {item} have given out. The Knobby-Ass Boots are the replacement. " +
			"The Knobby-Ass Boots do not explain themselves. " +
			"They have never explained themselves. They are simply there, knobby, waiting.",

		"Condition zero on the {item}. Condition zero means Knobby-Ass Boots. " +
			"The Knobby-Ass Boots were in your pack. They were always in your pack. " +
			"You kept them because getting rid of them felt like tempting fate. " +
			"Fate has been tempted anyway.",

		"Your {item} fell apart in the field, which is the worst time for boots to fall apart " +
			"and also, historically, exactly when boots fall apart. " +
			"The Knobby-Ass Boots are your boots. " +
			"The knobs are less comfortable than the alternative of no boots. Marginally.",

		"The {item} are done. The Knobby-Ass Boots are the present reality. " +
			"You will walk in them. They will be uncomfortable. " +
			"The knobs will be inexplicable. This is what condition zero costs.",
	},

	"tool": {
		"Your {item} has broken. The Rusted PoS Pickaxe is your tool now. " +
			"It is technically a pickaxe. That is the single nicest thing " +
			"anyone can say about it. It says it with reluctance.",

		"Condition zero on the {item}. The Rusted PoS Pickaxe has returned " +
			"from wherever tools go when you stop using them, " +
			"which is apparently the bottom of your pack, " +
			"rusty and patient and not surprised.",

		"The {item} is gone. The Rusted PoS Pickaxe is here. " +
			"The Rusted PoS Pickaxe has always been here, spiritually. " +
			"Now it is here physically. The mountain will not respect it. " +
			"The mountain never respected it. The mining has become harder.",

		"Your {item} cracked down the haft and the head separated at impact " +
			"and the impact was on rock that did not move " +
			"and you have the haft and not the head and " +
			"the Rusted PoS Pickaxe, which has a head attached, for now. " +
			"Mine carefully. Everything is worse.",

		"The {item} made a sound that tools should not make " +
			"and then it was in two pieces and the Rusted PoS Pickaxe " +
			"was in your hand because that is what happens at condition zero. " +
			"You are back to the beginning. The beginning bites rock poorly.",
	},
}

// ── SYSTEM MESSAGES ───────────────────────────────────────────────────────────

var DeathDM = []string{
	"📋 ADVENTURER STATUS: DEAD (RECOVERING)\n\n" +
		"{name}, you have died. This is, frankly, on you.\n\n" +
		"You are currently receiving care under the local healthcare system, which\n" +
		"has reviewed your case, questioned several of your life choices, denied\n" +
		"the first two claims, pre-authorized one bandage, and will see you in\n" +
		"approximately 23 hours pending insurance confirmation. The insurance\n" +
		"confirmation is pending. The insurance company is 'looking into it.'\n\n" +
		"Expected return: {time} UTC\n" +
		"Current condition: Alive (technically, and barely)\n" +
		"Equipment damage: Applied. It's not pretty.\n\n" +
		"The {location} has been flagged as 'hazardous' in your file,\n" +
		"which will not change anything about anything.",

	"📋 ADVENTURER STATUS: RECEIVING CARE\n\n" +
		"You are dead, {name}. Temporarily. Healthcare is involved.\n\n" +
		"The bill has been pre-estimated. The pre-estimate is not comfortable.\n" +
		"The final bill will be different from the pre-estimate.\n" +
		"Neither will be comfortable. This is healthcare.\n\n" +
		"Your equipment has been assessed. The assessment was not kind.\n" +
		"The assessment was accurate. Return expected: {time} UTC.\n" +
		"The {location} has noted your visit in whatever records it keeps.\n" +
		"You are in the record as 'visited once, briefly.'",

	"📋 ADVENTURER STATUS: INDISPOSED\n\n" +
		"{name}. You have died in {location}.\n\n" +
		"The relevant parties have been notified. The relevant parties include:\n" +
		"- Healthcare (notified, processing, unhurried)\n" +
		"- Your equipment (notified, damaged, also unhurried about this)\n" +
		"- The {location} (not notified; doesn't care; already moved on)\n\n" +
		"You have not been notified because you are currently the situation.\n" +
		"Return: {time} UTC. Rest. The {location} will still be there.",

	"📋 ADVENTURER STATUS: TEMPORARILY NOT ADVENTURING\n\n" +
		"Dead, {name}. The word is 'dead.'\n\n" +
		"Healthcare has received your chart. Healthcare has seen this chart before.\n" +
		"Healthcare has a file. The file is thick.\n" +
		"Healthcare has not commented on the file's thickness, professionally,\n" +
		"but the nurses make eye contact with each other when they see your name.\n\n" +
		"Equipment: damaged in ways that will require attention.\n" +
		"Return: {time} UTC.\n" +
		"The {location} has not changed as a result of this incident.",

	"📋 ADVENTURER STATUS: IN THE SYSTEM\n\n" +
		"You are dead. This is current status, not commentary, {name}.\n\n" +
		"The healthcare system has processed your intake.\n" +
		"The healthcare system processed your intake with the efficiency\n" +
		"of a system that processes many intakes and has a form for yours specifically.\n" +
		"The form has been filled out. Several boxes checked.\n" +
		"The box that says 'preventable' has been considered.\n\n" +
		"Recovery: {time} UTC. Equipment: damaged. {location}: still standing.",
}

var RespawnDM = []string{
	"📋 ADVENTURER STATUS: DISCHARGED (AGAINST MEDICAL ADVICE, AS USUAL)\n\n" +
		"{name}, healthcare is done with you. You have been discharged.\n" +
		"You signed a waiver on the way out. You do not remember signing it.\n" +
		"It covered a lot of ground. Don't think about it.\n\n" +
		"You are once again technically alive and available for terrible decisions.\n" +
		"The hospital has asked that you please not come back,\n" +
		"which is advice you will almost certainly ignore.\n\n" +
		"Type !adventure to see today's options.\n" +
		"Try to make better ones. You won't, but try.",

	"📋 ADVENTURER STATUS: OPERATIONAL (RELUCTANTLY CERTIFIED)\n\n" +
		"{name}. Healthcare has finished with you.\n\n" +
		"You have been medically cleared for adventuring,\n" +
		"which is a certification that healthcare gives\n" +
		"while clearly feeling that giving it is a mistake.\n" +
		"The certification has been given. The feeling was noted in your chart.\n\n" +
		"Your equipment is in the condition it was when you were admitted.\n" +
		"That condition was not good. It is still not good.\n" +
		"Type !adventure to continue making it worse.",

	"📋 ADVENTURER STATUS: RETURNED\n\n" +
		"You're back, {name}.\n\n" +
		"Healthcare has released you with a list of recommendations\n" +
		"that you will not be following. They know you won't follow them.\n" +
		"They give them anyway. It's in the protocol.\n\n" +
		"The recommendations include: better equipment, lower-tier dungeons,\n" +
		"a different career. These are good recommendations.\n" +
		"Type !adventure to not follow them.",

	"📋 ADVENTURER STATUS: DISCHARGED\n\n" +
		"{name}, you are alive. Healthcare has confirmed this.\n" +
		"Healthcare's confirmation comes with a bill that has been filed\n" +
		"and will be filed again and will continue to be filed\n" +
		"until something happens that resolves the filing.\n\n" +
		"None of this is your immediate problem.\n" +
		"Your immediate problem is that your equipment is worse\n" +
		"and your options are the same and the dungeons are waiting.\n\n" +
		"Type !adventure. The morning DM arrives at 08:00 UTC.\n" +
		"Be there. Try to be there tomorrow too. And the day after.",
}

var IdleShameDM = []string{
	"{name}, you didn't leave the house today.\n\n" +
		"Your adventurer sat in their hovel, stared at the wall,\n" +
		"and achieved absolutely nothing. No loot. No XP. No death,\n" +
		"which is honestly the nicest thing that can be said about today.\n\n" +
		"Tomorrow's choices arrive at 08:00 UTC.\n" +
		"Please, for the love of everything, try to be brave.",

	"No action today, {name}.\n\n" +
		"The morning DM was sent. The morning DM was not answered.\n" +
		"The dungeon was there. The mine was there. The forest was there.\n" +
		"You were also there, presumably, somewhere, not responding.\n\n" +
		"TwinBee noticed. The daily summary has noted your absence.\n" +
		"Tomorrow: 08:00 UTC. The options will be there. Please be there too.",

	"{name}. You rested today.\n\n" +
		"'Rested' is the word being used. It is the charitable word.\n" +
		"The uncharitable word is 'nothing.' You did nothing.\n" +
		"The dungeons are still there. The mines are still there.\n" +
		"The foraging areas are still there. Your XP is still where you left it.\n\n" +
		"08:00 UTC tomorrow. An adventure awaits.\n" +
		"The adventure has been waiting since you ignored it this morning.",

	"Today passed without you, {name}.\n\n" +
		"TwinBee went to a dungeon. The dungeon was real.\n" +
		"Your share of TwinBee's haul was distributed to people who showed up.\n" +
		"You did not show up. You did not get a share.\n" +
		"TwinBee noticed. TwinBee says nothing.\n" +
		"TwinBee's silence is louder than most things.\n\n" +
		"Tomorrow: 08:00 UTC. Show up.",

	"A day of rest, {name}. Unearned, but rest.\n\n" +
		"The hovel is comfortable. The wall is familiar.\n" +
		"Nothing in the hovel tried to kill you, which is the one advantage the hovel has.\n" +
		"The hovel also paid you nothing and taught you nothing and advanced nothing.\n" +
		"The hovel is comfortable and useless. So was today.\n\n" +
		"Tomorrow is 08:00 UTC. The choices will be new ones.\n" +
		"Make one of them. Any of them. Please.",

	"Where were you today, {name}?\n\n" +
		"The bot sent the DM. The DM was delivered.\n" +
		"The DM sat there, unread or unresponded-to, while your adventurer\n" +
		"sat in the hovel and the day happened without them.\n" +
		"XP: not gained. Loot: not found. Death: at least avoided.\n\n" +
		"Low bar. You cleared the low bar by doing nothing.\n" +
		"Tomorrow: 08:00 UTC. Clear a higher bar.",
}

var OnboardingDM = []string{
	"Welcome, {name}.\n\n" +
		"You are an adventurer. A hero. Probably.\n" +
		"The opening scroll of your life has played and it was unfortunately\n" +
		"quite short and mentioned nothing about destiny or treasure,\n" +
		"only that you are broke and smell faintly of the starting village.\n\n" +
		"You have been issued the standard starter kit:\n" +
		"⚔️  Basic Ass Sword — it's a sword in the same way a parking ticket is legal documentation\n" +
		"🛡️  Shitty Armor — offers the protection of a strongly-worded letter\n" +
		"🪖  Goddamn Offensive Helmet — bad for your head, an insult to witnesses\n" +
		"👢  Knobby-Ass Boots — the knobs are not a feature\n" +
		"⛏️  Rusted PoS Pickaxe — technically a pickaxe, the nicest thing anyone can say\n\n" +
		"Go forth. Try not to die in the Soggy Cellar.\n" +
		"People have. It was embarrassing and they never fully lived it down.\n\n" +
		"Your morning DM arrives at 08:00 UTC. Today's choices are ready now.\n" +
		"Type !adventure to begin.",
}

var MorningDM = []string{
	"⚔️ Good morning, {name}. Another day, another opportunity for regret.\n\n" +
		"{character_sheet}\n\n" +
		"Where are you headed? TwinBee would choose the dungeon.\n" +
		"TwinBee would also survive it. You are not TwinBee. Choose wisely.",

	"⚔️ {name}. You're alive. The options are:\n\n" +
		"{character_sheet}\n\n" +
		"The dungeon is there. The mine is there. The forest is there.\n" +
		"None of them are getting safer. Reply to choose.",

	"⚔️ Morning, {name}.\n\n" +
		"{character_sheet}\n\n" +
		"Today is a day. Days have outcomes. Reply with your choice\n" +
		"and we will discover what today's outcome is together,\n" +
		"at different levels of risk depending on what you pick.",

	"⚔️ Rise and choose, {name}.\n\n" +
		"{character_sheet}\n\n" +
		"The world is dangerous and full of things worth taking from it.\n" +
		"Some of those things are in dungeons. Some are in mines.\n" +
		"Some are in forests, guarded by hornets who feel strongly about them.\n" +
		"Reply with where you're going.",

	"⚔️ Another morning. Another chance to make something happen, {name}.\n\n" +
		"{character_sheet}\n\n" +
		"Or don't. You could rest. You could do nothing.\n" +
		"The bot will send you a disappointed DM at midnight.\n" +
		"TwinBee will know. TwinBee always knows.\n" +
		"Reply to choose.",
}
