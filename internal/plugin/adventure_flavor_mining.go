package plugin

var MiningDeath = map[int][]string{

	// ── TIER 1: SURFACE PITS ─────────────────────────────────────────────────
	1: {
		"The cave-in was technically not your fault. The ceiling disagrees. " +
			"You have been extracted from the rubble by a miner coming off shift " +
			"who charged you for the inconvenience and suggested, not unkindly, " +
			"that you consider a different career. You are considering it. " +
			"You are considering it from a medical facility.",

		"You hit what you thought was a copper vein. " +
			"It was a load-bearing wall. " +
			"The distinction mattered significantly more than anticipated. " +
			"You are now convalescing, which is a fancy word for lying very still " +
			"and questioning every decision that led to this exact moment.",

		"The Surface Pits are called Surface Pits because they are near the surface. " +
			"Near the surface means the ceiling is basically ground. " +
			"Ground is heavy. You know this now. " +
			"You knew it before, technically, but now you know it differently. " +
			"Medically.",

		"A bat. A single bat panicked and flew into your face " +
			"while you were mid-swing and you hit the wrong thing with the pickaxe " +
			"and the wrong thing was structural and the structure had opinions. " +
			"The bat is fine. The structure expressed its opinions. " +
			"You are in medical care.",

		"The ore was right there. Right there. One more swing and you had it. " +
			"The one more swing destabilised something adjacent to the ore " +
			"that was doing more structural work than it looked like it was doing. " +
			"You did not get the ore. The ore got you. From above. " +
			"Healthcare is processing this with familiar efficiency.",

		"You were humming while you worked. Miners hum. It's a thing miners do. " +
			"The vibration of the humming, combined with the vibration of the pickaxe, " +
			"combined with the fact that the Surface Pits are 'surface' in the sense " +
			"of 'barely underground at all' produced an outcome that " +
			"the structural engineer who assessed the pits afterward called " +
			"'statistically likely.' You were the statistic.",

		"The Rusted PoS Pickaxe broke on impact. The head flew backward. " +
			"This is not how pickaxes are supposed to work. " +
			"The resulting sequence of events was neither dignified nor brief " +
			"and ended with you in a medical facility and the pickaxe head " +
			"somewhere in the rubble that used to be the mine entrance. " +
			"Upgrade your tools. Please.",

		"Died in the Surface Pits. The shallowest mines available. " +
			"The mines where the copper is basically lying on the ground. " +
			"You died there. The copper is still lying on the ground. " +
			"You are in a different location. The copper is not.",

		"Something was already in the mine when you got there. " +
			"You didn't check before going in. " +
			"Miners check before going in. " +
			"You are a miner in the same way you are a lot of things: technically and poorly. " +
			"The something made its presence known. Healthcare has made its bill known.",

		"The water came in faster than the structural integrity left. " +
			"This is a sentence about priorities and physics and the Surface Pits' " +
			"proximity to a drainage ditch that you were not informed about. " +
			"You are now informed about it. From a medical facility.",
	},

	// ── TIER 2: IRON RIDGE ───────────────────────────────────────────────────
	2: {
		"A cave troll. In an iron mine. Nobody mentioned the cave troll. " +
			"The listing for Iron Ridge said: iron, lead, saltpetre. " +
			"The listing did not say cave troll. The cave troll did not consult the listing. " +
			"Your Chipped Iron Pickaxe bounced off it. Your armor absorbed one hit. " +
			"The troll has your hat now. Healthcare has your chart.",

		"The Iron Ridge cave-in took out the main tunnel. " +
			"Not a small cave-in — a real one, a committed one, " +
			"a cave-in that had prepared for this moment. " +
			"You were in the main tunnel. You are in medical care. " +
			"The relationship between these facts is direct.",

		"You found iron. Significant iron. More iron than your inventory could hold. " +
			"You were so focused on the iron that you did not notice " +
			"the floor of the main seam was a different colour than the floor " +
			"of the entrance tunnel, which is typically how you notice " +
			"that the floor is a different substance, which in this case was briefly nothing.",

		"Saltpetre is flammable. You know this. Everyone knows this. " +
			"The question is whether you knew it in the specific moment " +
			"when your torch got too close to the saltpetre deposit. " +
			"The answer is: you knew it right after. That's a different kind of knowing. " +
			"Healthcare is dealing with the result.",

		"The Iron Ridge cave troll had been there since before the mine. " +
			"The mine was built around it. The miners worked around it. " +
			"You did not get the briefing about the cave troll " +
			"because the briefing is for miners and you are not a miner, " +
			"you are an adventurer with a pickaxe, which is a critical distinction " +
			"the troll also made.",

		"Lead poisoning is not immediate. You were down there long enough " +
			"for it to start being immediate. The medical facility has noted this. " +
			"The medical facility has seen this before. " +
			"The medical facility has a specific intake form for it. " +
			"You have filled out the intake form.",
	},

	3: {
		"Silver Seam, Tier 3, and you found the silver. You actually found it. " +
			"And then the thing that was guarding the silver " +
			"found you, which is how things work at Tier 3, " +
			"and the thing was not a bat and not a cave troll " +
			"but something older and more specifically opposed to " +
			"people taking silver from this particular seam. " +
			"Healthcare has the details.",

		"The quartz formation was load-bearing in ways that are " +
			"not immediately obvious from looking at quartz formations. " +
			"You looked at the quartz formation. You did not correctly " +
			"identify its structural role. You swung the pickaxe anyway. " +
			"The quartz formation expressed its structural role clearly.",

		"Three levels down in the Silver Seam and something in the third level " +
			"decided you'd gone far enough. The decision was made forcefully " +
			"and immediately and without consultation. You are in medical care. " +
			"The silver is in the third level. The something is also in the third level. " +
			"These facts are related.",
	},

	4: {
		"The Deeprock is deep. This is noted in the name and in every " +
			"description of the Deeprock that has ever been written " +
			"by anyone who came back from it. Deep means pressure. " +
			"Pressure means the walls behave differently. " +
			"You found this out in a way that required medical intervention.",

		"Something in the Deeprock does not appear on any mining chart " +
			"because the people who compile mining charts don't go to the Deeprock " +
			"specifically because of the something. " +
			"You went to the Deeprock. You found the something. " +
			"The something found you first, actually. Healthcare is filing the paperwork.",

		"Titanium ore does not yield easily. You were swinging hard. " +
			"The hard swing destabilised a gold deposit adjacent to the titanium " +
			"that was doing significant structural work in a part of the mine " +
			"that turns out to have significant structural requirements. " +
			"You did not get the titanium or the gold. You got the ceiling.",
	},

	5: {
		"The Mythril Caverns at full depth. The ore was there. " +
			"The Voidstone was between you and the ore. " +
			"Voidstone is called Voidstone for a reason. " +
			"The reason is relevant to your current medical situation.",

		"Mythril does not want to be mined. This is not metaphorical. " +
			"Mythril ore in the Mythril Caverns has a documented defensive response " +
			"to mining attempts that involves the surrounding rock and significant velocity. " +
			"You attempted to mine it. The response was documented again. " +
			"You are the new documentation.",

		"The Dragon Crystal was there. Your Diamond Pickaxe was there. " +
			"You were there. The thing that the Dragon Crystal was protecting " +
			"was also there, which is the fact that changes the outcome. " +
			"Dragon Crystals don't form around nothing. " +
			"The nothing turned out to be something. Healthcare is involved.",
	},
}

var MiningCaveIn = []string{
	"A minor cave-in. Congratulations on the 'minor.' " +
		"You survived, which the mine did not expect and the mine's structural " +
		"integrity does not entirely endorse. You emerged with {ore} " +
		"and the distinct feeling that the mountain is personally annoyed with you. " +
		"Your {tool} is worse. Your back is worse. The ore is worth €{value}. " +
		"Two out of three is acceptable.",

	"Rocks fell. Most of them missed, which is more luck than you deserve. " +
		"Your {tool} took the worst of it and is now in a condition that can " +
		"generously be described as 'structurally questioning its own existence.' " +
		"You found {ore} before the ceiling made its feelings known. Worth it. Barely.",

	"The ceiling gave you a warning shot. One rock. Right next to your head. " +
		"You took the hint and got out with {ore} worth €{value} and a newfound " +
		"respect for geology that you will maintain for approximately three days " +
		"before forgetting it again. Your {tool} absorbed some of the exit.",

	"The mine expressed structural concerns via the medium of falling rock. " +
		"You heard the concerns. You addressed them by leaving, taking {ore} with you. " +
		"€{value}. Your {tool} has been reviewed by the falling rock and rated poorly. " +
		"The falling rock's review stands. Consider upgrades.",

	"A section of the ceiling reconsidered its commitment to being a ceiling. " +
		"This philosophical crisis occurred directly above you, as they do. " +
		"You got out with {ore} worth €{value}. Your armor is worse. " +
		"Your {tool} is worse. You are better for having survived it, " +
		"which is a way of looking at the situation that the mountain does not share.",

	"Something went wrong in a mine. Specifically: the ceiling went wrong, " +
		"in the direction of your head, at a speed that required running. " +
		"You ran. You kept {ore}. The ore is worth €{value}. " +
		"The {tool} was sacrificed to the structural gods and they accepted it. " +
		"You are home. The mine has made its point.",

	"Cave-in, partial, survived. The official report reads exactly that " +
		"because the official report is required to be accurate " +
		"and 'survived a cave-in and came home with {ore} worth €{value} " +
		"while your {tool} disintegrated and your armor took significant damage' " +
		"doesn't fit in the field provided. But that's what happened.",

	"The rock didn't fall so much as it slid, which is different in the way " +
		"that gives you time to move but not enough time to take everything. " +
		"You took {ore}. You left your dignity. €{value}. The {tool} is on the border " +
		"of being a different category of object now. The border is 'scrap.'",

	"Tectonic opinion expressed. You received the opinion. " +
		"You incorporated the opinion into your exit strategy, " +
		"taking {ore} worth €{value} as you incorporated. " +
		"Your {tool} participated in absorbing the opinion " +
		"and is available for review in its current condition, which is poor. " +
		"But you're here. The ore is here. The ceiling is also here, " +
		"lower than before.",
}

var MiningEmpty = map[int][]string{

	1: {
		"You mined for three hours. The mountain was empty. " +
			"Not metaphorically — there was literally nothing worth a damn in there. " +
			"You brought home some dust, a bad back, and a grudge against copper specifically.",

		"The promising vein of copper turned out to be copper-coloured rock. " +
			"All of it. The whole vein. Copper-coloured, worthless, " +
			"personally offensive rock. You have experienced this before. " +
			"It has not gotten easier or more interesting.",

		"Nothing. Sweet, simple nothing. You swung your {tool} at that mountain face " +
			"for the better part of a morning and it gave you absolutely nothing back " +
			"except sore arms and XP that barely qualifies as XP. " +
			"The mountain doesn't care about you. That's the lesson. Free of charge.",

		"The vein ran out three swings in. Three swings. You prepared for hours, " +
			"navigated to the seam, positioned yourself correctly, " +
			"executed the first three swings, and the vein " +
			"had the audacity to simply stop existing. The mountain has no shame.",

		"Someone else was here recently. You can tell by the shape of the worked stone — " +
			"someone competent, with better tools, who took everything and left " +
			"the bare walls behind as a message. The message is: " +
			"you came second. In mining, coming second is coming last.",

		"Coal. You found coal. An entire seam of coal " +
			"in a copper mine, because the universe is specific in its disappointments. " +
			"The coal is not worth your time. You mined it anyway, briefly, " +
			"out of frustration, and then stopped and stood there " +
			"in a coal seam feeling things.",

		"The Surface Pits were empty in the way that suggests they were always empty " +
			"and the optimism that put 'copper, tin, coal' on the listing " +
			"was the optimism of someone who hadn't personally checked. " +
			"You have personally checked. The listing was optimistic.",

		"You found tin. One piece of tin. A single, solitary, " +
			"inadequate piece of tin ore in three hours of mining. " +
			"The tin is worth €{value}. €{value} is a number. " +
			"The number does not justify the morning. Nothing justifies the morning.",

		"The bat colony was in the main seam. The entire main seam. " +
			"You could not mine around the bats. The bats were the seam, effectively. " +
			"You came home with nothing and the distinct sense " +
			"that the bats were watching you leave, satisfied.",

		"An entire morning of swinging at rock and the rock gave back: nothing. " +
			"Not even a piece of coal as a participation award. " +
			"The mountain took your time and your arm strength " +
			"and your {tool}'s remaining condition and gave you experience points. " +
			"Experience points that barely covered the walk.",
	},

	2: {
		"Iron Ridge has iron in it. Usually. Today it had rock. " +
			"Iron-coloured rock, lead-coloured rock, rock-coloured rock. " +
			"Rock and more rock and then, at the back of the main seam, " +
			"rock with an attitude. Nothing worth a euro. " +
			"You have learned more about rock than you wanted to know.",

		"The cave troll was in the way of the main vein. " +
			"You went around the cave troll. The around-the-cave-troll route " +
			"led to a worked-out section. Someone had already mined this section. " +
			"Someone had also apparently come to an arrangement with the cave troll, " +
			"which explains the troll's territorial behavior and your empty inventory.",

		"Lead. A full load of lead ore and no way to carry it all. " +
			"You took what you could. On the way out you dropped it. " +
			"You went back for it. It was gone. " +
			"You don't know what happened to the lead. " +
			"The mine knows. The mine is not telling you.",

		"The iron seam was there. The iron seam was right there. " +
			"The iron seam was also, it turns out, directly below a water table " +
			"that expressed itself the moment you broke through. " +
			"You ran. The water has the iron now. The water doesn't need the iron. " +
			"You needed the iron. The arrangement is unfair and non-negotiable.",
	},

	3: {
		"Silver Seam, third tier, and the silver had run thin. " +
			"Not empty — you could see the silver in the rock, faint veins " +
			"of it running through everything — but too embedded to extract " +
			"with your current tools, which are good tools that are not the right tools. " +
			"You mined nothing. You saw silver all day. " +
			"The specific cruelty of this is noted.",

		"Six hours in the Silver Seam. Six hours of promising rock " +
			"and misleading mineral formations and one moment of genuine excitement " +
			"that turned out to be quartz, which is beautiful and worthless. " +
			"You have quartz now. Quartz is pretty. " +
			"Quartz is worth the walk the same way a nice view is worth the walk: " +
			"it is, technically, but not when you were hoping for silver.",
	},

	4: {
		"The Deeprock does not give up Deeprock gold easily. " +
			"It did not give it up today at all. " +
			"You went to the fourth tier of underground, fought your way to the seam, " +
			"swung at the right rock for two hours, " +
			"and came home with the XP of having tried and the ore of having not succeeded.",

		"The sapphire deposit was right there and then it wasn't. " +
			"Not mined out — physically relocated by something that lives in the Deeprock " +
			"and has opinions about the sapphire that outrank yours. " +
			"You did not find what relocated it. You found its wake.",
	},

	5: {
		"The Mythril Caverns had mythril today. You know this because you could see it. " +
			"Voidstone adjacent. Dragon Crystal formation blocking the approach. " +
			"The mythril was there and inaccessible and beautiful " +
			"and you spent four hours confirming this and came home with nothing. " +
			"The mythril is still there. It will be there tomorrow. " +
			"Whether you can access it tomorrow is tomorrow's problem.",

		"The caverns were full today. Full of things that were not for you today. " +
			"The Voidstone pulsed when you came near the good seams. " +
			"The Dragon Crystals were warm in ways they shouldn't be. " +
			"The mythril was there. The space between you and the mythril " +
			"was too full of intent for you to close. Empty hands. " +
			"Full understanding of why. That's Tier 5.",
	},
}

var MiningSuccess = map[int][]string{

	1: {
		"Solid day's work. You extracted {ore} worth €{value} from {location} " +
			"with minimal personal injury, which around here counts as raging success. " +
			"Your {tool} held up, which is more than can be said for your back.",

		"The seam held. {ore} in respectable quantity, €{value} worth, " +
			"and only one near-miss with a very irate bat who had opinions " +
			"about your presence and then reconsidered. " +
			"You are calling this a win. You are not wrong.",

		"In and out. {ore}, €{value}, zero cave-ins, one bat that thought about it " +
			"and decided not to. Your {tool} is slightly worse. Everything else is better. " +
			"By Surface Pits standards, a professional operation.",

		"The copper vein ran further than the map suggested. Further and richer. " +
			"You took {ore} worth €{value} and left more than you could carry. " +
			"This is the correct response to a rich vein. Come back tomorrow. " +
			"Bring a bigger bag. Bring a better pickaxe.",

		"Clean extraction. {ore} from the main seam, €{value} assessed, " +
			"no structural events, one moment where the bat looked at you " +
			"and you looked at the bat and you both made sensible decisions. " +
			"{xp} XP. The mountain respects you today. Slightly.",

		"Level up potential accumulating. {ore}, €{value}, {xp} XP. " +
			"You swung the pickaxe correctly the correct number of times " +
			"and the rock rewarded you for it. This is the entire transaction. " +
			"It is, when it works, genuinely satisfying.",

		"The tin seam yielded more than expected and the coal beneath it " +
			"turned out to have a copper pocket behind it " +
			"that wasn't on any map because nobody had gone that far before. " +
			"You went that far. {ore} total, €{value}. " +
			"The {tool} is worse. The discovery is better.",
	},

	2: {
		"Iron Ridge delivered today. {ore} extracted, €{value} assessed, " +
			"cave troll acknowledged and navigated around with something approaching grace. " +
			"The {tool} performed admirably. Your back has filed a formal complaint. " +
			"The back's complaint is noted and will not change the outcome.",

		"A good seam. A real seam — proper iron, " +
			"the kind where the rock splits cleanly and the ore sits in it " +
			"like it's been waiting. {ore} worth €{value}. {xp} XP. " +
			"This is what mining is supposed to feel like. " +
			"Remember it for the days it doesn't.",

		"The lead pocket was deeper than the iron, which meant going deeper " +
			"than planned, which meant more exposure to everything Iron Ridge contains. " +
			"You made it. {ore} worth €{value}. The depth was worth it. " +
			"The {tool} has opinions about the depth. The opinions are structural.",
	},

	3: {
		"Silver. Real silver, properly embedded, not quartz, not illusion, " +
			"not iron-coloured rock lying about its nature. Silver. " +
			"{ore} worth €{value}. {xp} XP. Your {tool} is a real tool " +
			"and this was a real seam and you extracted the silver. " +
			"That's the whole job. Today the whole job got done.",

		"The Silver Seam at depth, past the quartz formations, " +
			"past the section that looks better than it is, " +
			"down to where the silver sits in the rock like an argument for effort. " +
			"{ore}, €{value}, {xp} XP, home by sunset. " +
			"A good day. In a mine. Those exist.",
	},

	4: {
		"Deeprock gold. Actual gold, not pyrite, not gold-coloured copper, " +
			"not optimism in mineral form. Gold. {ore} worth €{value}. " +
			"The Deeprock tried to discourage you with atmosphere and pressure. " +
			"You brought a Mithril Pickaxe and ignored the atmosphere.",

		"You went to the fourth level of the Deeprock and you came back " +
			"with {ore} worth €{value} and {xp} XP and a story " +
			"that mostly involves how far down the fourth level is. " +
			"It is very far down. The ore was worth being very far down. Today.",
	},

	5: {
		"Mythril. You found mythril and extracted mythril " +
			"and brought mythril home and mythril is worth €{value} per unit. " +
			"The {ore} you're carrying is worth €{value} total. " +
			"{xp} XP. The Diamond Pickaxe is a Diamond Pickaxe for a reason. " +
			"Today you proved the reason.",

		"The Voidstone was present but passive today. " +
			"The Dragon Crystals were warm but not hostile. " +
			"The mythril seam was open and you took {ore} worth €{value} from it. " +
			"{xp} XP. The Mythril Caverns had a good day at the same time you did. " +
			"This does not happen often. Don't examine it. Take the ore home.",
	},
}

// ── FORAGING ──────────────────────────────────────────────────────────────────

var ForagingDeath = []string{
	"Look — nobody expects to die foraging. That's the whole thing about foraging. " +
		"It lowers your guard. You were crossing the river when the current decided " +
		"your story ended here. It did not end here, but the margin was narrow. " +
		"The river kept your left boot and is not returning it.",

	"You disturbed what appeared to be an ordinary beehive. " +
		"It was not ordinary. The bees had a position on disturbance " +
		"and they held it with every individual sting they possessed. " +
		"You have been stung approximately four hundred times. " +
		"The doctor says you will probably be fine. " +
		"Probably is doing considerable work in that sentence.",

	"You were eaten by a bear. Partially. The bear reconsidered mid-process " +
		"and left. You are alive in the way that really makes you question " +
		"what 'alive' means as a category. Healthcare is sorting the paperwork. " +
		"The bear is eating something else now. You hope.",

	"A wild {item} appeared and you bent to pick it up " +
		"and what you bent toward was a hornet's nest you couldn't see " +
		"from standing height and the hornets had been waiting for exactly this. " +
		"The hornets had been waiting for a very long time. " +
		"You gave them what they'd been waiting for.",

	"The tree was dead. Dead trees fall. " +
		"You were at the base of the dead tree when it decided to fall, " +
		"which the tree did without significant warning, " +
		"which is the thing about dead trees that makes them different from live ones. " +
		"Live trees creak. Dead trees have already decided.",

	"The river crossing was knee-deep. The current was described to you as mild. " +
		"Both of these facts were technically accurate in the dry season, " +
		"and it is not the dry season, and nobody told you this, " +
		"and the current had opinions about your presence that it expressed at length.",

	"You found a mushroom. A beautiful mushroom, red-capped, white-spotted, " +
		"the kind that appears in illustrations and decorative contexts. " +
		"You ate some of it to check if it was edible. It was not edible. " +
		"Healthcare has your chart and a different entry form for this specific situation.",

	"Something in {location} that is not on any foraging chart " +
		"because the people who compile foraging charts do not go to the part of {location} " +
		"where the something lives. You went there. The something was there. " +
		"You are in medical care. The something is still there.",

	"You climbed the tree for the fruit. The branch held. " +
		"The other branch, the one you were holding onto, did not hold. " +
		"The fall was not long but the landing was conclusive " +
		"in the way that requires professional medical attention.",

	"The bear was not aggressive. The bear was eating berries. " +
		"The berries you were there for. The bear's position on sharing berries " +
		"with a person who walked into their berry clearing was immediate and physical. " +
		"Your armor is worse. The bear has the berries.",
}

var ForagingHornets = []string{
	"You found a beautiful old oak. It had hornets. So many goddamn hornets. " +
		"You ran. The hornets followed. You ran further. The hornets were committed " +
		"to the bit in a way you were completely unprepared for. " +
		"You returned home with no loot, significant swelling, and a new personal " +
		"relationship with the concept of consequences.",

	"The nest looked small. It was not small. It was the visible surface " +
		"of something vast and very angry. You escaped with your life " +
		"and approximately twelve percent of your dignity. " +
		"Your armor has forty-seven new dents. They are all from hornets. " +
		"Hornets did that. To armor.",

	"Hornets. Again. At some point this stops being bad luck and starts " +
		"being a you problem. You lost an hour running, gained nothing at all, " +
		"and look like you lost a fight with a pincushion. Because you did.",

	"You have contracted STATUS EFFECT: STUNG (×52). " +
		"There is no remedy in your inventory. There is no item for this. " +
		"You must wait for it to stop and think very carefully " +
		"about your relationship with trees that have a lot of buzzing near them.",

	"The hornets were not in the nest when you found the nest. " +
		"This seemed lucky. The hornets were getting back from wherever hornets go " +
		"and arrived home to find you standing next to their home " +
		"with {item} in your hand and nowhere to run that you hadn't already calculated. " +
		"The calculation was wrong.",

	"You heard the buzzing. You made the decision to continue anyway. " +
		"This is the moment, when you replay it later — and you will replay it — " +
		"where everything could have been different. You continued anyway. " +
		"The hornets respected the audacity. The hornets did not let it go unpunished.",

	"A forager's first lesson: if it hums, it has already made a decision about you. " +
		"You are learning the first lesson. The first lesson has forty-three individual " +
		"components today and they are all in your neck and arms.",

	"The tree looked ordinary. The tree was ordinary. " +
		"The hornets inside the tree were not ordinary — they were a specific kind " +
		"of hornet with a specific kind of patience and a very practiced response " +
		"to the sound of a woodcutting tool approaching their home. " +
		"They practiced it on you.",

	"You killed one hornet. You killed it immediately and with precision " +
		"because you are a practical person and it was there. " +
		"The hornet's colleagues noted the killing. " +
		"The hornet's colleagues had a meeting about it. " +
		"The meeting concluded quickly and unanimously and you were there for the vote.",

	"Pro tip, available retroactively: hornets near the water's edge " +
		"are more aggressive than woodland hornets because they've already " +
		"had a bad day from the moisture. You found water's-edge hornets. " +
		"They had had a bad day. They shared it.",

	"The entire foraging trip went fine until the tree. " +
		"Everything before the tree was excellent — haul building, " +
		"weather cooperative, no bears, no river crossings. " +
		"Then the tree. Then the hornets in the tree. " +
		"Then nothing. The haul is gone. The morning is gone. " +
		"The tree is still there. The hornets are very much still there.",
}

var ForagingBear = []string{
	"A bear. There was a goddamn bear. You and the bear had a brief negotiation " +
		"whose terms were entirely set by the bear. Your armor absorbed " +
		"the opening statement and is now in a condition the bear " +
		"would call 'fair exchange.' You ran. The bear watched you go " +
		"with what you chose to interpret as respect. It was not respect.",

	"The bear was eating berries. You were also there for berries. " +
		"The bear felt this was a resource conflict requiring immediate physical resolution. " +
		"You resolved it by leaving extremely fast. Your {armor} is worse. " +
		"Your pride is worse. The bear has all the berries.",

	"You saw the bear before it saw you. This did not help. " +
		"You had a head start. This also did not help. " +
		"Bears are fast. They are very fast. " +
		"Your armor took {damage} condition damage and you took the rest personally.",

	"The bear made eye contact. You made eye contact back. " +
		"In retrospect this was the wrong call — " +
		"bears interpret sustained eye contact as a challenge, " +
		"which is information you could have used thirty seconds ago. " +
		"Your {armor} absorbed the challenge response.",

	"Mother bear. Cubs nearby, which you noticed after the encounter " +
		"rather than before it, which is the incorrect order in which to notice cubs. " +
		"Mother bear's response to your presence near the cubs was immediate, " +
		"comprehensive, and educational. {armor} condition: reduced. " +
		"Bear education: complete.",

	"You threw {item} at the bear. The item did not deter the bear. " +
		"The bear was briefly curious about the item and then returned its attention to you. " +
		"You lost {item} and your armor took damage and you ran home " +
		"without the item and without anything else you'd collected. " +
		"The bear has your {item} now. It doesn't know what to do with it.",

	"The bear followed you for twenty minutes. Twenty full minutes. " +
		"Not charging — just walking, at bear pace, in your direction, " +
		"through everything you went through, with complete commitment. " +
		"You dropped your haul to run faster. The bear stopped then. " +
		"The bear wanted you to drop the haul. This is the worst kind of smart.",

	"The bear sniffed you before it attacked, which is a scientific note " +
		"that does not help you medically but which you have retained " +
		"because it seemed important in the moment. " +
		"The bear then attacked. Your {armor} is worse. You are home.",

	"A bear, unannounced, from the undergrowth. " +
		"The undergrowth was thick enough that you had no warning, " +
		"which is relevant information about {location}'s foraging conditions " +
		"that was not included in the briefing and absolutely should have been.",

	"The bear was smaller than bears are supposed to be. " +
		"This made you underestimate it. Smaller bears are faster. " +
		"This is not information that helped you in sequence — " +
		"it's information that arrived after the sequence concluded, " +
		"while you were sitting at home with {damage} less armor condition " +
		"and no loot and a new policy on small bears.",
}

var ForagingRiver = []string{
	"You needed to cross the river. The river had opinions. Strong ones. " +
		"You lost {item} to the current, your boots are ruined, " +
		"and you have gained a new, lasting, personal respect for bridges. " +
		"Oregon Trail sends its regards. It always does.",

	"A simple ford. Knee-deep at most, you thought. " +
		"The river was not at most. The river was considerably more than at most " +
		"and had been since the rains last week which you did not know about " +
		"because you did not check. You made it across. {item} did not. " +
		"Your boots are now decorative. Your socks are a closed topic.",

	"The current took {item} right out of your hands. Gone. " +
		"Just gone. Immediately gone in the way of things taken by fast water, " +
		"which is completely gone with no warning and no recovery. " +
		"Then it took your footing. Then your remaining dignity. " +
		"You sat on the far bank for a while and poured water out of your boots.",

	"You have lost {item} and {item_2} to the river today. " +
		"The river has gained {item} and {item_2}. " +
		"The river does not need these things. The river doesn't need anything. " +
		"The river is a river. You needed those things. " +
		"The exchange is the worst kind of one-sided.",

	"Stepping stones. You were using the stepping stones. " +
		"The stepping stones were moss-covered in a way " +
		"that was visible from where you were standing " +
		"and that you assessed as 'manageable.' " +
		"The moss had a different assessment. You met the river. " +
		"The river kept {item}.",

	"The crossing was going well until step seven. " +
		"Steps one through six: fine. Step seven: a rock that moved. " +
		"Not a stepping stone. A rock that happened to be there. " +
		"It moved. You moved differently. The {item} in your right hand " +
		"went into the river. The {item} in your left hand stayed. " +
		"Two items entered the crossing. One item came out.",

	"You crossed the river and then the river crossed you " +
		"on the way back, heavier with {item}, moving slower, " +
		"caring more about not dropping things and therefore " +
		"paying less attention to where your feet were. " +
		"The feet found out where they were. The river was where they were.",

	"The pack was too heavy for the crossing depth. " +
		"You knew this at step three. You continued anyway, " +
		"running the numbers on whether you could make it " +
		"while simultaneously making it, which split your attention " +
		"in a way the current noticed and exploited.",

	"You've crossed this river before. The river was lower before. " +
		"This river and your memory of this river are different rivers " +
		"at different water levels and your memory does not have {item} " +
		"at the bottom of it. The real river does now.",
}

var ForagingGoodHaul = map[int][]string{

	1: {
		"A peaceful morning in {location}. Birds. Dappled light. {item} in quantity. " +
			"€{value} worth of completely non-threatening nature doing exactly " +
			"what nature is supposed to do. You are suspicious. " +
			"Nothing bad happened. Yet. Check tomorrow.",

		"You returned from {location} with {item} worth €{value}, zero injuries, " +
			"one mildly alarming encounter with a mushroom that turned out to be fine, " +
			"and the quiet satisfaction of a morning where the worst thing " +
			"that happened was briefly being in the same meadow as a grumpy deer.",

		"In and out. Clean. Profitable. Nobody died, nothing stung you, " +
			"no bears had opinions about your presence. " +
			"{item} worth €{value} and you're home before lunch. " +
			"This is the life. Savour it. It won't last.",

		"You have collected {item}. Your inventory has increased. " +
			"There is a fanfare in your heart — brief, sincere, the kind that plays " +
			"when you pick up something good in a world that is mostly trying to kill you. " +
			"€{value}. {xp} XP. The fanfare was earned.",

		"A wild {item} appeared. You used FORAGE. It was super effective. " +
			"€{value} worth of nature obtained without significant trauma. " +
			"Your Foraging Skill is considering leveling up. It's thinking about it. " +
			"It's not ready yet but it's thinking.",

		"The meadow was generous today. {item} in the quantity that makes " +
			"you feel like the meadow wanted you to have it, " +
			"which it did not, meadows don't have intentions, " +
			"but the feeling was there and €{value} is €{value}.",

		"Good haul. Better than expected. {item} worth €{value} from {location} " +
			"with {xp} XP and nothing bad attached to it. " +
			"Just: went out, found things, came home. " +
			"Simple. Good. Uncomplicatedly good. Write this date down.",
	},

	2: {
		"The Old Forest delivered today. {item} in the kind of quantity " +
			"that makes you feel briefly competent as a forager, " +
			"which is a feeling worth pursuing because it doesn't happen constantly. " +
			"€{value}. {xp} XP. The forest was cooperative. You were prepared. " +
			"These two things aligned.",

		"Hardwood and wild fruit and one significant mushroom cluster " +
			"that you correctly identified as edible, which is the forager's " +
			"primary professional requirement and one you have now met. " +
			"{item} worth €{value}. No hornets. A successful day in the Old Forest.",

		"The Old Forest is old in ways that mean it has more things in it " +
			"than newer forests, which is either comforting or alarming depending on " +
			"what the things are. Today the things were {item} worth €{value}. " +
			"A good day for the old-things-in-old-forests category.",
	},

	3: {
		"Ancient timber. Real ancient timber — the kind where the grain is so tight " +
			"you can count centuries in it. You found it, took what you could carry, " +
			"left the rest undisturbed because the Ancient Grove has ways of noticing " +
			"when you take too much. {item} worth €{value}. {xp} XP. " +
			"The grove noticed you. The grove allowed it. Don't push it.",

		"The rare herb was where the foraging charts said it would be, " +
			"which happens less often than you'd hope given that the charts exist. " +
			"{item} worth €{value}. The Ancient Grove was quiet today — " +
			"present, watchful in the way old places are watchful, " +
			"but not hostile. You took the herbs and left quickly anyway. Good instinct.",
	},

	4: {
		"The Deep Jungle doesn't give up exotic wood easily. " +
			"It gave it up today, which means you either got lucky or the jungle " +
			"made a decision about you. You prefer not to examine which. " +
			"{item} worth €{value}. {xp} XP. Home before something changed its mind.",

		"Tropical fruits and something that might be spores " +
			"that you have chosen to identify as 'probably fine' " +
			"based on the colour and smell and a general optimism about your choices. " +
			"{item} worth €{value}. The jungle was loud today but not at you. " +
			"You'll take it.",
	},

	5: {
		"The Primal Wilds gave you Starfruit. You know what Starfruit means " +
			"in the Primal Wilds — it means the place you're standing " +
			"is one of the few places in the world where Starfruit grows " +
			"and you were there on a day it was accessible. " +
			"{item} worth €{value}. {xp} XP. " +
			"This will not happen on the same terms again.",

		"Spirit Herbs. Primordial Bark. {item} worth €{value} " +
			"from the furthest reaches of the Primal Wilds on a day " +
			"when the Wilds were, against all precedent, cooperative. " +
			"{xp} XP. You are home. The Wilds are still primal. " +
			"Today they were primal in your favour.",
	},
}
