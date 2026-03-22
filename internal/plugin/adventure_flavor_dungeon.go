package plugin

// DungeonFlavor holds all pre-written narrative strings for dungeon outcomes.
// Indexed by tier (1–5) and outcome type.
// Selection: pick randomly, skip last 5 used per player per category.

var DungeonDeath = map[int][]string{

	// ── TIER 1: THE SOGGY CELLAR ─────────────────────────────────────────────
	// Deaths here should be embarrassing above all else. You died to rats.
	// You died to slimes. The dungeon is four feet underground and damp.
	// History will not record this kindly.
	1: {
		"You entered the Soggy Cellar with a sword and left without consciousness. " +
			"The rats were not impressed. They had, if you're being honest, a point. " +
			"You have been retrieved from the floor by a passing merchant who charged you " +
			"for the inconvenience and did not make eye contact.",

		"A single rat. One rat. You were killed by one rat in a cellar that smells " +
			"of old vegetables and broken dreams. The rat has returned to its business. " +
			"You have been taken to the medical facility, which is a cot and a bill " +
			"and a nurse who has seen this before and has thoughts but keeps them to herself.",

		"YOU DIED. In large, honest letters, if this were a more transparent world. " +
			"It is not, so instead you simply died, quietly, in a damp place, " +
			"surrounded by rats who did not particularly want to kill you but " +
			"found themselves in the position regardless. Here we are.",

		"GAME OVER. INSERT COIN TO CONTINUE. You do not have a coin. You have " +
			"a copay and a 24-hour wait and a healthcare system that is processing " +
			"your claim and will get back to you. The arcade never prepared you for this. " +
			"Nothing prepared you for this. You went into a cellar.",

		"The Wet Slime got you. Not a fire slime. Not a poison slime. A wet slime. " +
			"An ambient, damp, fundamentally inoffensive slime that nonetheless " +
			"managed to be fatal to you specifically. The slime has resumed being wet. " +
			"You have resumed being a problem for the healthcare system.",

		"You slipped. That's it. You slipped on something unidentified on the " +
			"Soggy Cellar floor — the name should have been a warning, it was a warning, " +
			"you did not take the warning — hit your head, and that was that. " +
			"The Angry Badger nearby watched this happen and felt nothing.",

		"The Angry Badger was, it turns out, very angry. You were not prepared " +
			"for the depth of the anger. You are now in medical care reflecting " +
			"on the gap between 'Angry Badger' as a concept and 'Angry Badger' " +
			"as an experience. The gap is significant. The badger is still angry.",

		"You fought bravely. History will record this as 'bravely.' The rats " +
			"will record it as 'lunch,' which is a matter of perspective, " +
			"and the rats have the stronger perspective here because they won. " +
			"You are expected to make a full recovery. Expected is doing a lot of work " +
			"in that sentence.",

		"Your Basic Ass Sword broke on the first swing. Not bent. Not chipped. " +
			"Broke. In half. On a rat. You had a moment to process this fully " +
			"before losing consciousness, which is more processing time than " +
			"most people get in this situation. You used it to feel regret.",

		"Press F. Just press F. The Soggy Cellar has won this round, which is " +
			"a sentence that should embarrass everyone involved. Respawn in 24 hours. " +
			"Use the time to consider whether 'adventurer' was the right career path " +
			"or whether it is simply the path you are on.",

		"The ceiling dripped on you. You looked up. The rat hit you while you " +
			"were looking up. This sequence of events is, in its own way, " +
			"a complete story about who you are as an adventurer. " +
			"The healthcare system has received your chart. They are not surprised.",

		"You died to a Wet Slime and a Giant Rat working in what can only be " +
			"described as accidental coordination. They were not coordinating. " +
			"They just both happened to be there. This is worse, somehow. " +
			"A coordinated attack you could respect. This was just Tuesday for them.",

		"The dungeon is four feet underground. It is damp. It contains rats " +
			"and slimes and one badger with emotional problems. You did not survive it. " +
			"This information is being processed by multiple parties including " +
			"yourself, your healthcare provider, and the badger, who feels nothing.",

		"You have died in the Soggy Cellar for what the records indicate is " +
			"not the first time, which the records find notable. The records " +
			"have been updated. The update reads: 'again.' The healthcare system " +
			"is beginning to recognise your face. This is not the achievement it sounds like.",

		"Something bit you. You're not entirely sure what. The darkness was " +
			"involved, and the dampness, and a sound that in retrospect was " +
			"a warning and at the time seemed like ambience. You are now ambience " +
			"for the medical recovery ward. Try to be quieter than the Soggy Cellar.",

		"The Wet Slime engulfed your boots. The Knobby-Ass Boots, to be specific, " +
			"which offered less traction than anticipated on a wet floor, " +
			"which is to say none. You went down fast. The slime was apologetic " +
			"about it in the way that slimes are, which is not at all.",

		"You had a plan. The plan was good. The plan did not account for there " +
			"being two rats instead of one, which in retrospect is the kind of " +
			"assumption that ends careers. Your career has not ended. It has paused, " +
			"for 24 hours, in a medical facility, reconsidering its assumptions.",

		"An Angry Badger and a Wet Slime and a Giant Rat walked into a cellar. " +
			"You were already in the cellar. This is not a joke. There is no punchline. " +
			"You are the punchline. You are currently in medical care being the punchline.",
	},

	// ── TIER 2: THE GOBLIN WARRENS ───────────────────────────────────────────
	// Dying here is more understandable but no less embarrassing. Goblins are
	// real monsters. You still lost to them. They're goblins.
	2: {
		"The goblin did not look threatening. This was a trap. The goblin was, " +
			"in fact, threatening, and had been for some time, and had been waiting " +
			"for someone to underestimate him specifically. You were that someone. " +
			"He has returned to his warrens. You have been sent to medical care.",

		"Three goblins. You handled two. The third one was waiting. " +
			"Goblins, it turns out, have a concept of patience that you " +
			"failed to account for. The patient goblin is now slightly richer. " +
			"You are now slightly more medicated. The exchange rate favours the goblin.",

		"The Kobold was not listed as a threat. The Kobold disagreed with this " +
			"assessment at considerable length and velocity. Your Shitty Armor " +
			"absorbed the first disagreement and then stopped absorbing. " +
			"The Kobold made its point. You have conceded the point. Medically.",

		"A Trap Spider. In a warren. Obviously there was a Trap Spider. " +
			"The name of the dungeon is the Goblin Warrens and goblins are " +
			"famous for their traps and you walked into a web the size of a doorway " +
			"because you were looking at the goblins. The spider was not the goblins.",

		"You cleared the first room. You cleared the second room. The third room " +
			"cleared you, which is a thing that happens and a sentence that " +
			"will be on your medical chart indefinitely. Room three: dangerous. " +
			"This information is available to you now, retroactively, useless.",

		"The Goblin Warchief was not supposed to be there. That's the official " +
			"position. The unofficial position is that the Goblin Warchief " +
			"is absolutely supposed to be there, has always been there, " +
			"and you should have checked. You did not check. The Warchief checked for you.",

		"Your sword arm got tired. This is an embarrassing thing to admit " +
			"but it is the honest account of what happened in the Goblin Warrens " +
			"between the second and third encounter. Arm tired. Goblin fast. " +
			"Healthcare called. The sequence is complete.",

		"The goblins coordinated. Actually coordinated — flanked you, " +
			"drew your attention left while the attack came right, " +
			"used the terrain. You did not know goblins did that. " +
			"You know now. The knowledge cost 24 hours and some dignity.",

		"A Kobold with a crossbow. In the dark. Across a room you couldn't " +
			"see clearly. This is not a fair fight. Nothing about the Goblin Warrens " +
			"is fair. That's the entire deal. You knew the deal. " +
			"The bolt knew the deal better than you did.",

		"You slipped on something goblin-related. You do not want to know " +
			"what it was. The floor of the Goblin Warrens is not clean. " +
			"The fall was not dignified. The subsequent encounter with " +
			"the subsequent goblin was not survivable. Here we are.",

		"The warrens go deeper than the map suggested. You went deeper than " +
			"your level suggested. These two facts met in a corridor and " +
			"produced a medical emergency that the healthcare system " +
			"is processing with the enthusiasm of an organisation that " +
			"has processed this before.",

		"Trap. You found the trap. Specifically, you found it with your foot, " +
			"and then your face, and then your general body in a way that " +
			"involved the floor. The goblins who set the trap have noted " +
			"its success in whatever the goblin equivalent of a ledger is. " +
			"You are a successful data point for their trap program.",

		"The goblin was smaller than you. The goblin was faster than you. " +
			"The goblin had friends and you had a Slightly Less Shit Iron Sword " +
			"and misplaced confidence. The misplaced confidence did not survive " +
			"the encounter. Neither did you, technically, for 24 hours.",

		"You got lost. In the warrens. Which are, by definition, a warren — " +
			"a maze of tunnels built by creatures who live there and know " +
			"every turn. You got lost and then you got found, by goblins, " +
			"which is the worst way to be found in this context.",

		"Three arrows. You saw the first one coming. The second one you heard. " +
			"The third one you did not experience as a separate event " +
			"from the second one, which tells you everything about the timing. " +
			"The kobold archers have been congratulated by their colleagues.",

		"The Trap Spider's web was across a doorway you had already walked " +
			"through once, going in. It wasn't there going in. The spider " +
			"built it while you were dealing with the goblins. " +
			"The spider was waiting. Spiders are good at waiting. " +
			"You were not good at checking doorways twice.",

		"You ran out of dungeon before you ran out of goblins, " +
			"which sounds like it should be good news but meant " +
			"you were cornered in the last room with more goblins " +
			"than exits. The math did not work in your favour. " +
			"The math rarely does in the Goblin Warrens.",

		"The Goblin Warchief had a horn. You learned about the horn " +
			"thirty seconds before you learned about the twelve goblins " +
			"the horn summons. The horn was loud. The goblins were fast. " +
			"The medical transport was eventually located. A difficult day.",
	},

	// ── TIER 3: THE CURSED CRYPT ─────────────────────────────────────────────
	// Dying here is understandable. These are proper monsters. The shame
	// is less about competence and more about hubris.
	3: {
		"The Draugr was asleep when you entered. You were quiet. Not quiet enough. " +
			"The Draugr's definition of 'quiet' is stricter than yours " +
			"and its response to noise is well-documented and physical. " +
			"You are in medical care. The Draugr is asleep again.",

		"A ghost. You cannot hit a ghost. You tried to hit the ghost. " +
			"The ghost watched you try to hit it with what it correctly " +
			"identified as a sword, and then it did what ghosts do, " +
			"which is not care about swords and continue being a ghost.",

		"The Cursed Crypt is cursed. This is in the name. The curse " +
			"manifested as a compulsion to walk into the room " +
			"with the most skeletons and swing at the first one. " +
			"You did this. The curse was efficient. The skeletons were numerous.",

		"Three skeletons. Fine. Five skeletons. Manageable. " +
			"The eighth skeleton that came through the door you " +
			"thought was an exit was neither fine nor manageable. " +
			"The eighth skeleton was the end of the encounter from your perspective.",

		"The Draugr hit you once. Once was sufficient. " +
			"Your armor held the first blow and then had a brief existential crisis " +
			"about its structural integrity and resolved that crisis " +
			"by failing. The Draugr needed only the one.",

		"The ghost phased through your sword, through your armor, " +
			"through your carefully maintained sense of tactical awareness, " +
			"and through the part of your nervous system responsible " +
			"for staying upright. Ghosts do not fight fair. " +
			"They are not required to. They are ghosts.",

		"You found the crypt boss room. This was not the plan. " +
			"The plan was to find treasure and leave before finding " +
			"the crypt boss room. The crypt boss room found you instead, " +
			"which is a subtle but important distinction " +
			"that is not available to you right now.",

		"The curse got you on the way out. Not in — you navigated in fine. " +
			"The curse was waiting on the way out, which is deeply unfair " +
			"and also completely consistent with being called a curse. " +
			"You are in medical care. The crypt is still cursed.",

		"A Draugr, two skeletons, and a ghost in a room that was supposed " +
			"to be empty according to your extremely unreliable " +
			"pre-run intelligence. The intelligence was wrong. " +
			"The room was not empty. The distinction matters to your healthcare provider.",

		"The skeleton archers were a surprise. The crypt looked like " +
			"a melee crypt — close quarters, low ceilings, short sight lines. " +
			"The skeleton archers had compensated for this " +
			"by being very accurate at short range. Live and learn. " +
			"In 24 hours. When you are alive again.",

		"Something in the Cursed Crypt cursed you specifically, " +
			"which is both an achievement and a disaster. " +
			"You were cursed enough to be notable. The curse assessed you, " +
			"selected you, and applied itself with intention. " +
			"You are special. The specialness requires medical attention.",

		"The Draugr spoke. You did not know they spoke. Nobody warned you. " +
			"You stopped, briefly, because something in you wanted to hear " +
			"what a Draugr had to say. The Draugr used this pause efficiently. " +
			"You will not stop next time. There will be a next time in 24 hours.",

		"Dark room. Something in it. You had a torch. The torch went out. " +
			"The something in the room did not require light to function. " +
			"You required light to function. The something was aware of this asymmetry " +
			"and applied it professionally.",

		"You got cocky. There is no other explanation. You were doing well — " +
			"three rooms cleared, good haul developing, exit in sight — " +
			"and you got cocky and opened a door that had a very clear " +
			"'do not open this door' energy and opened it anyway. " +
			"The door was correct about itself.",

		"The skeleton fell apart when you hit it. You felt good about this. " +
			"The other eleven skeletons in the room did not fall apart. " +
			"You had a moment of feeling good followed by a longer moment " +
			"of not feeling anything, which is the medical situation.",
	},

	// ── TIER 4: TROLL BRIDGE DEPTHS ─────────────────────────────────────────
	// Dying here is a serious matter. These are high-level monsters.
	// The tone shifts from embarrassment to something closer to respect
	// for the thing that killed you.
	4: {
		"The Troll did not notice you at first. When it noticed you, " +
			"you were briefly grateful for the time you'd had when it hadn't. " +
			"Then it acted on the noticing. You are in medical care. " +
			"The Troll has returned to its business, which does not involve you.",

		"A Stone Giant. You knew there were Stone Giants in the Troll Bridge Depths. " +
			"Knowing and experiencing are different scales of the same information " +
			"and the experiential scale is considerably larger than you anticipated. " +
			"The Stone Giant was the appropriate size for a Stone Giant. " +
			"You were not the appropriate size for this encounter.",

		"The Cursed Knight had been down there a long time. " +
			"You could tell because of the armor — old, wrong era, " +
			"still functional in ways that defy the passage of centuries. " +
			"The Knight fought like someone who had been practicing " +
			"for exactly this encounter, in this corridor, for decades. " +
			"They had been. You had not.",

		"Two Trolls. You planned for one. The plan was good for one. " +
			"The second Troll arrived from a tunnel you had assessed " +
			"as non-threatening, which is the last assessment you made " +
			"before the assessment became irrelevant.",

		"The Stone Giant threw a rock. From across the chamber. " +
			"At you specifically, with what appeared to be accurate intent. " +
			"You did not know Stone Giants had that kind of range or precision. " +
			"You are in possession of this knowledge now. " +
			"The knowledge is available to you upon recovery.",

		"The Cursed Knight's sword went through your Enhanced Plate " +
			"in a way that Enchanted Plate is not supposed to permit. " +
			"The Knight is old enough to have fought enchanted armor before. " +
			"The Knight had notes. The Knight applied the notes.",

		"The depths went deeper. You followed them. " +
			"The things that live in the deepest parts of the Troll Bridge Depths " +
			"do not come up, generally, because the things above them are also " +
			"dangerous and the arrangement suits everyone. " +
			"You disrupted the arrangement. The arrangement corrected itself.",

		"A Troll, a Stone Giant, and a Cursed Knight in the same corridor " +
			"at the same time is not a coincidence. It is a configuration " +
			"that the Troll Bridge Depths produces when someone has been " +
			"pushing their luck across multiple rooms and the dungeon " +
			"has decided to consolidate the response.",

		"The bridge itself was the trap. You were on the bridge. " +
			"The Troll was under the bridge, which is where Trolls are, " +
			"which is information that was available to you before you " +
			"stepped onto the bridge, and which you had filed " +
			"under 'probably not relevant right now.'",

		"Your equipment held. Your stamina didn't. " +
			"The Troll Bridge Depths is a long dungeon and the things inside it " +
			"are not tiring in the way you are tiring and the gap " +
			"between your stamina curve and the dungeon's stamina curve " +
			"met in the fourth room and the dungeon's curve continued upward.",

		"The Cursed Knight spoke your name. Your full name. " +
			"You have never told anyone in this dungeon your full name. " +
			"You had a moment to think about this before the encounter concluded. " +
			"The moment was brief. The thought is available for review upon recovery.",

		"It was the small things. Not the Troll. Not the Stone Giant. " +
			"The small cursed things that live in the walls and margins " +
			"of the Troll Bridge Depths, the ones nobody lists in the dungeon " +
			"briefing because they're individually minor and collectively " +
			"the reason you are in medical care.",
	},

	// ── TIER 5: THE ABYSSAL MAW ──────────────────────────────────────────────
	// Dying here is not embarrassing. It is almost an achievement.
	// The tone is terse, respectful, slightly ominous.
	// The thing that killed you deserves one sentence.
	5: {
		"The demon didn't fight you. It waited until you were tired " +
			"and then it stopped waiting. There's a lesson in that. " +
			"The lesson is available in 24 hours.",

		"An Elder Drake. You know what that means. " +
			"You knew before you went in. You went in anyway. " +
			"This is either courage or a poor risk assessment " +
			"and the Abyssal Maw does not distinguish between the two.",

		"The Unnamed was there. You found it or it found you — " +
			"the distinction collapses at a certain depth. " +
			"You are alive. That's more than it usually leaves.",

		"Something in the Abyssal Maw does not have a name " +
			"because the people who named things didn't come back. " +
			"You came back. Consider this the win it technically is.",

		"The Elder Drake breathed once. Your Dragonscale armor " +
			"was, in its way, a message to the Drake about its predecessor. " +
			"The Drake received the message and responded at length.",

		"The demon was old. Older than the dungeon. Older than the concept " +
			"of dungeons. It has been down there since before people " +
			"started going into deep places looking for treasure. " +
			"It is still there. You are here. That's the update.",

		"YOU DIED in the Abyssal Maw. In large, honest letters. " +
			"The large, honest letters are impressed, which is " +
			"more than the large honest letters usually feel. " +
			"Recovery in 24 hours. The Maw will be there.",

		"The floor in the deepest chamber of the Abyssal Maw is not a floor. " +
			"You know this now. What it is remains unclear " +
			"because you did not have sufficient time to form a complete theory " +
			"before the theory became irrelevant.",

		"Three demons, one Elder Drake, and something that doesn't have " +
			"a category yet. You lasted longer than most. " +
			"The dungeon log reflects this. The dungeon log is not gentle " +
			"but it is accurate.",

		"The Abyssal Maw goes deeper than the maps indicate. " +
			"You found the level below the last mapped level. " +
			"Nobody has mapped it because nobody came back to do the mapping. " +
			"You are back. The map remains incomplete. " +
			"This is a conversation for after recovery.",

		"The Unnamed said something as you lost consciousness. " +
			"You don't remember what. You remember the shape of it. " +
			"This is probably fine. Probably.",

		"An Elder Drake, cornered, is different from an Elder Drake with room. " +
			"You cornered one. This seemed advantageous at the time. " +
			"The Drake had a different read on the geometry.",
	},
}

var DungeonEmpty = map[int][]string{

	1: {
		"You explored the entire Soggy Cellar and found: one dead rat (not your kill), " +
			"a suspicious smell, and the growing suspicion that adventure is complete bullshit. " +
			"You return home with nothing but the experience. " +
			"The experience was bad and you would like a refund.",

		"Three hours. Three hours in there. The rats were apparently " +
			"on some kind of rodent holiday. You found one button behind a loose stone. " +
			"It was a button, not a coin. An ordinary button. " +
			"You are home now. The button is in your pocket. You don't know why.",

		"Nothing. The Soggy Cellar had nothing in it today. " +
			"Not even a pot. There were pots, but smashing them yielded nothing, " +
			"which has never once been true in any dungeon worth the name. " +
			"You smashed all the pots anyway. Out of principle. Out of grief.",

		"A chest. There was an actual chest. You opened it. " +
			"It played a little tune — a proper, sincere, full-length discovery fanfare " +
			"for a chest that contained absolutely nothing. " +
			"You stood there and listened to the whole tune. " +
			"You don't know why. Respect for the process, maybe.",

		"The monsters had already been cleared. By whom is unclear. " +
			"Someone faster, better equipped, more prepared, and apparently " +
			"more punctual than you. They left footprints. " +
			"The footprints were mocking you. You felt it.",

		"The Soggy Cellar was, today, simply a cellar. Soggy, yes. " +
			"Damp, absolutely. Full of rats and opportunity? No. " +
			"Just a cellar underground with nothing in it " +
			"and you in it looking for things that weren't there.",

		"You searched for two hours and found: some damp, a rat hole " +
			"that went nowhere useful, a piece of wood that might have been " +
			"a weapon at some point in a different life, and the strong conviction " +
			"that someone else already had a very good day in here recently.",

		"The rats ran from you today. This is new. You don't know what it means. " +
			"It means there's nothing left to protect, as it turns out. " +
			"The rats had already eaten everything interesting. " +
			"You found the aftermath. The aftermath paid nothing.",

		"Empty. Completely, aggressively, personally empty. " +
			"The Soggy Cellar had one job today and declined it. " +
			"You have returned home with XP that barely qualifies as XP " +
			"and a new, deeper understanding of disappointment as a lifestyle.",

		"You found a locked door. You found a way to open the locked door. " +
			"Behind the locked door was another locked door. " +
			"Behind the second locked door was a small room with nothing in it " +
			"except the specific silence of a room that used to have something in it " +
			"and doesn't anymore.",

		"The dungeon was empty in the way that says someone was here recently. " +
			"Fresh tracks. Still-warm torch brackets. One rat looking nervous " +
			"in a way that suggests it witnessed something. " +
			"You got there second. Being second in dungeons is being last.",

		"Every door opened. Every chest empty. Every crevice checked. " +
			"Every pot smashed, every loose stone lifted, every shadow assessed. " +
			"Nothing. The Soggy Cellar has been professionally cleaned " +
			"by someone who was not you and did not leave a finder's fee.",
	},

	2: {
		"The Goblin Warrens were staffed today but not stocked. " +
			"Goblins everywhere, treasure nowhere. You fought your way " +
			"through three rooms of nothing valuable and came home " +
			"with XP and a strong sense of having been personally targeted " +
			"by the random number generator.",

		"The goblins had nothing on them. You checked. Thoroughly. " +
			"Goblins are supposed to hoard things — it's the whole deal with goblins. " +
			"These goblins had apparently subscribed to a minimalist philosophy " +
			"and gotten rid of everything. You got rid of the goblins. " +
			"The net result was nothing.",

		"A chest. A locked chest. A locked chest in a trapped room " +
			"that took you twenty minutes to navigate safely. " +
			"The chest contained a goblin IOU for 'eight copper, payable eventually.' " +
			"The goblin is not available to honour the IOU.",

		"The warren went deeper than the map suggested. " +
			"You followed it to the bottom, fighting the whole way. " +
			"The bottom contained a goblin who looked as surprised to see you " +
			"as you were to find nothing else there. " +
			"You both agreed, silently, that this wasn't how today was supposed to go.",

		"Room after room after room of goblins and kobolds " +
			"and trap spiders and none of them carrying anything worth the walk. " +
			"The Goblin Warrens had a sale recently. You were not informed. " +
			"Everything has already been liquidated. You are the liquidator of nothing.",

		"You found the goblin treasury. You know this because it said " +
			"'TREASURY' above the door in goblin script, which you cannot read, " +
			"but which a helpful kobold graffitied the translation of nearby. " +
			"The treasury was empty. Someone got here first. " +
			"The translation is the only treasure you're taking home.",
	},

	3: {
		"The Cursed Crypt was cursed today toward emptiness. " +
			"You fought the undead — all the undead, it felt like, " +
			"every skeleton in the crypt standing up to discuss your presence — " +
			"and none of them had anything worth taking. " +
			"Skeletons, it turns out, travel light.",

		"Three rooms cleared, zero treasure found. The fourth room had a sarcophagus. " +
			"You opened the sarcophagus. The sarcophagus was empty. " +
			"In retrospect the Draugr had already left for the day. " +
			"You opened an empty box dramatically and then went home.",

		"The ghosts had nothing. You cannot take things from ghosts regardless, " +
			"but usually there are things nearby that they're haunting " +
			"that you can take. Today the ghosts were haunting a bare room " +
			"with stone walls and the specific sadness of things that " +
			"have been haunting nothing for a very long time.",

		"The crypt was picked clean. Ancient clean — not recently cleaned, " +
			"but cleaned at some point in history by someone thorough " +
			"who left nothing and sealed it and the Draugr and skeletons " +
			"and ghosts have been guarding an empty crypt ever since " +
			"out of what can only be described as institutional inertia.",
	},

	4: {
		"The Troll Bridge Depths ran empty today in the way that deep things " +
			"sometimes do — everything present, nothing accessible. " +
			"You fought your way through Trolls and Cursed Knights " +
			"and Stone Giants and came out the other side with bruises " +
			"and XP and exactly nothing in your pockets.",

		"A treasury room. Well-defended. Three Trolls, two Cursed Knights, " +
			"a Stone Giant as a bonus. You cleared it. " +
			"The treasury had been emptied before you arrived. " +
			"Possibly before any of the monsters arrived. " +
			"They were guarding an empty room for reasons lost to history.",

		"The dungeon had a good day before you arrived. " +
			"The dungeon has bad days too, but today was not the dungeon's bad day. " +
			"Today was your bad day. You were in the dungeon on the dungeon's good day " +
			"and everything was taken and everything that tried to take you succeeded " +
			"and you are home now with the participation award, which is XP.",
	},

	5: {
		"The Abyssal Maw gave you nothing today except survival, " +
			"which at this tier is actually something. " +
			"You went in. You came out. The space between those two facts " +
			"contains several encounters you will be processing for some time. " +
			"Nothing of material value. Everything of experiential value. " +
			"The distinction matters less than it used to.",

		"The demons were between you and everything worth taking. " +
			"You dealt with the demons. Behind them: more demons. " +
			"Behind them: the Elder Drake. Behind the Elder Drake: nothing. " +
			"The Abyssal Maw is well-defended for a room that turns out " +
			"to have had nothing in it today.",

		"The Unnamed was in the way of the treasure room. " +
			"You did not attempt to move the Unnamed. " +
			"This was the correct decision. You went home with XP " +
			"and the knowledge that some doors are better left as doors.",
	},
}

var DungeonSuccess = map[int][]string{

	1: {
		"Success! Relative to your recent attempts, which mostly involved " +
			"screaming and losing consciousness. You found {item} worth €{value}. " +
			"The rats respected you marginally more on the way out. " +
			"You chose not to investigate why. Some respects are best left unexplored.",

		"You cleared the Soggy Cellar of one rat and found {item} worth €{value}. " +
			"The other rats were watching from the shadows and agreed not to intervene, " +
			"apparently out of pity. Pity! You have been pitied by rats. " +
			"And yet you have {item}. The rats have a hole. You're ahead.",

		"Not bad. Not good either, let's be honest, but not bad. " +
			"You found {item} worth €{value}, took a hit that'll bruise nicely by morning, " +
			"and made it out without dying. By your current standards, " +
			"this is a triumph. Sad, but measurably true.",

		"You killed a thing. The thing had {item} on it. " +
			"You have €{value} more than you started with and {xp} XP " +
			"and a new familiarity with the smell of the Soggy Cellar " +
			"that no amount of recovery time will fully address. " +
			"This is the whole job. This is what adventure is.",

		"Level up potential! Well — XP potential. " +
			"You found {item} worth €{value} and gained {xp} XP " +
			"and somewhere a small chime played that only you could hear. " +
			"Progress. Measurable, modest, hard-won progress in a damp cellar.",

		"In and out. {item} worth €{value}. One rat with an attitude problem " +
			"that has since been resolved. Your Basic Ass Sword is no worse " +
			"than when you started, which is its version of a good day. " +
			"Everything survived. You are counting this as a win.",

		"The Angry Badger had {item}. You weren't expecting that. " +
			"You weren't expecting the badger at all, honestly, " +
			"but the badger had {item} and now you have {item} " +
			"and the badger has consequences. €{value}. {xp} XP. " +
			"An unexpected Tuesday.",

		"Two rats, one Wet Slime, {item} worth €{value}, and you walked out " +
			"under your own power. The Shitty Armor is slightly worse. " +
			"Everything else is the same or better. " +
			"You have chosen to describe this as 'clean execution' " +
			"and nobody in the Soggy Cellar is alive to contradict you.",

		"A wild {item} appeared. You used ACQUIRE. It was effective. " +
			"€{value} secured, {xp} XP noted, zero hospitalizations. " +
			"A complete run. By Soggy Cellar standards, legendary.",

		"You found {item} behind a loose stone that three rats were " +
			"apparently using as a savings account. The rats objected. " +
			"Their objections were noted and overruled. €{value}. " +
			"This is the economy working as intended.",

		"The cellar was manageable today. That's the whole report. " +
			"Manageable. {item}, €{value}, {xp} XP, no deaths, " +
			"one close call that was closer than you're admitting " +
			"but resulted in nothing clinical. Manageable.",

		"You fought your way to the back of the Soggy Cellar " +
			"and found {item} worth €{value} in a corner that smelled like " +
			"rats had been storing things there for years. " +
			"They had. You have the things now. The rats are reconsidering their storage strategy.",

		"Solid. Not exciting, not legendary, just solid. " +
			"{item} worth €{value}. {xp} XP. All equipment functional. " +
			"You have returned home upright and slightly less broke than you left. " +
			"This is the dream. This specifically, unimpressively, is the dream.",
	},

	2: {
		"The Goblin Warrens cleared two rooms and {item} worth €{value} " +
			"and you are not dead, which this dungeon does not give away freely. " +
			"The goblins are impressed, in the way that goblins are impressed, " +
			"which is the same way they are everything: angrily.",

		"You took {item} from the goblins. The goblins had {item}. " +
			"They should not have had it — it's yours now — " +
			"but the how of them having it is a story " +
			"that ends with someone else having a worse day than yours. " +
			"€{value}. You're the winner today.",

		"The Goblin Warchief dropped {item} worth €{value} and you took it " +
			"before anyone else could form an opinion about that. " +
			"{xp} XP. The Warchief's ring is still on your thumb. " +
			"The other goblins are making decisions about you. " +
			"The decisions are not positive but they are respectful.",

		"Trap avoided. Kobold handled. Two goblins negotiated with " +
			"(negotiations were physical). {item} acquired, €{value} secured, " +
			"exit located and used. This is a textbook run " +
			"by the standards of a textbook nobody has written yet " +
			"because everyone who tried died in the Goblin Warrens.",

		"Three rooms. {item} in room two. €{value} total. " +
			"The Trap Spider was in room three and you went to room three " +
			"anyway because you had already committed emotionally " +
			"and the spider was manageable. You were right. " +
			"Being right in the Goblin Warrens is not guaranteed.",

		"You collected {item} worth €{value} from the body of a goblin " +
			"who was, frankly, carrying more than expected. " +
			"This is true of most goblins. They are sentimental creatures " +
			"who attach value to things that aren't valuable " +
			"and occasionally to things that are. Today: the latter.",

		"The warrens gave ground today. Not easily — {xp} XP worth of not easily — " +
			"but it gave ground and you took {item} and €{value} " +
			"and made it out with your equipment in the same condition it started. " +
			"The goblins are annoyed. The goblins are always annoyed. " +
			"Today they are additionally annoyed.",
	},

	3: {
		"The Cursed Crypt had {item} worth €{value} and you have it now. " +
			"You fought a Draugr and two skeletons and a ghost that " +
			"you mostly avoided by walking quickly through rooms " +
			"and pretending you couldn't hear it. " +
			"Sound tactical approach. {xp} XP. No deaths.",

		"Three skeletons, one Draugr, {item} worth €{value}. " +
			"The curse was present but mild today — present in the way " +
			"that all the torches went out simultaneously and something breathed " +
			"in your ear when nothing was near you, but not present in the way " +
			"that kills you. A good day by crypt standards.",

		"The ghost let you pass. You don't know why. " +
			"Ghosts make decisions on criteria that are not available to the living " +
			"and today's criteria allowed you through with {item} worth €{value}. " +
			"You did not ask questions. You took the {item} and left. " +
			"Correct response.",

		"You found {item} in the sarcophagus. The Draugr who was supposed to be " +
			"in the sarcophagus was in the next room, which gave you time to " +
			"take {item} before the situation became complicated. " +
			"€{value}. {xp} XP. Timing is everything in the Cursed Crypt.",

		"Four rooms. The fourth room had {item} worth €{value} " +
			"and the kind of ominous atmosphere that suggests something " +
			"terrible used to happen in it but currently doesn't. " +
			"Currently was enough. You took the {item} and left " +
			"before the 'currently' expired.",
	},

	4: {
		"The Troll went down. The Stone Giant went down. {item} worth €{value} " +
			"retrieved from behind them both. {xp} XP. " +
			"Your equipment is worse than it was — there are marks on it " +
			"that will require explanation — but you are home and solvent " +
			"and that is the transaction the Troll Bridge Depths offers.",

		"The Cursed Knight's sword went for your throat. " +
			"Your Guardian's Helm got in the way. The Knight adjusted. " +
			"You adjusted faster. {item} worth €{value}. {xp} XP. " +
			"The Helm has a new dent. You have new money. Fair exchange.",

		"{item} worth €{value} from the fourth level of the Troll Bridge Depths, " +
			"retrieved past a Troll, two Stone Giants, and the specific kind of " +
			"architectural malice that suggests the dungeon was designed by someone " +
			"who didn't want anyone to get to the fourth level. " +
			"You got to the fourth level. Here is what was there.",

		"You bribed the Troll. Not with money — Trolls don't want money — " +
			"but with the specific kind of tactical retreat that says " +
			"'I know where I am and I know what you are and I am choosing " +
			"this particular angle of withdrawal.' The Troll respected it. " +
			"{item} was in the room behind the Troll. €{value}. {xp} XP.",
	},

	5: {
		"The Abyssal Maw gave up {item} worth €{value} today, " +
			"which cost more to retrieve than the number suggests " +
			"and was worth every measure of that cost. " +
			"{xp} XP. You are home. The Maw is still there. " +
			"Both of these facts are remarkable.",

		"{item} worth €{value} from the deepest accessible chamber. " +
			"The Elder Drake was between you and it. " +
			"The Elder Drake is no longer between anything and anything. " +
			"You have the {item}. This is what success looks like at Tier 5. " +
			"Remember it. Not many people get to.",

		"You went into the Abyssal Maw and came back with {item} worth €{value} " +
			"and {xp} XP and a story that you will tell carefully, " +
			"to people you trust, in private, " +
			"because the full version contains several things " +
			"that are better experienced than described.",

		"The demon had {item}. The demon no longer has {item}. " +
			"€{value}. {xp} XP. The exchange was not peaceful. " +
			"The exchange was not brief. The exchange was ultimately yours " +
			"and that is what the ledger records.",
	},
}

var DungeonExceptional = map[int][]string{

	1: {
		"STOP EVERYTHING. You found {item} worth €{value} in the SOGGY CELLAR. " +
			"The SOGGY CELLAR. Where the rats live. Underground. In the damp. " +
			"You found something worth €{value} in there and gained {xp} XP " +
			"and came home upright. This does not happen. It has happened. " +
			"The rats are as surprised as you are.",

		"Against all probability and the general expectations of everyone " +
			"who has been watching your adventuring career — " +
			"which has not been impressive — you found {item} worth €{value} " +
			"in the Soggy Cellar. {xp} XP. The dungeon has filed a formal complaint. " +
			"The complaint will not be actioned. You have the {item}.",

		"A boss. The Soggy Cellar had a boss today. Not a rat. Not an angry badger. " +
			"A boss. You fought the boss. You won. You found {item} worth €{value}. " +
			"{xp} XP. The rats have appointed a new boss. " +
			"The new boss is watching you leave and reconsidering things.",

		"What the hell. You got a critical hit. In the Soggy Cellar. " +
			"The numbers came up. The RNG loved you. {item} worth €{value}. " +
			"{xp} XP. Screenshot this feeling. Frame it. " +
			"It is unlikely to repeat at this location.",

		"NEW PERSONAL RECORD for the Soggy Cellar. " +
			"{item} worth €{value}. {xp} XP. The rats acknowledge your authority " +
			"over this specific damp underground space. " +
			"This is the most authority you have had over anything. " +
			"It smells like rats. It is yours.",

		"The Soggy Cellar had a secret room. You found the secret room. " +
			"The secret room had {item} worth €{value} in it and {xp} bonus XP " +
			"and the specific satisfaction of a secret found, which is " +
			"the satisfaction you became an adventurer for. " +
			"This is what it's supposed to feel like. Remember this.",

		"You found {item} worth €{value} and gained {xp} XP and the Angry Badger " +
			"that attacked you on the way out was actually carrying " +
			"a second smaller item that you also took. " +
			"Two items from one run. The Soggy Cellar is not going to live this down.",
	},

	2: {
		"UNPRECEDENTED. The Goblin Warchief was in residence today " +
			"and you fought the Goblin Warchief and you won. " +
			"You have {item} worth €{value} and {xp} XP and the signet ring " +
			"of a Goblin Warchief on your thumb. " +
			"The goblins are in a governance crisis. This is your fault. Good.",

		"The Goblin Warrens had a treasury room today. " +
			"You found the treasury room. The treasury room had {item} worth €{value} " +
			"and several other things of lesser value that sum to a significant " +
			"improvement in your financial situation. {xp} XP. " +
			"The goblins are devastated. You are not.",

		"Critical run. Everything went right. Every roll landed. " +
			"Every trap avoided, every goblin dropped, " +
			"every room yielded something. {item} worth €{value} total. " +
			"{xp} XP. The Goblin Warrens is not supposed to work like this. " +
			"It worked like this today. For you. Today.",

		"The Kobold Cartographer had a map. You have the map now. " +
			"The map led to {item} worth €{value} in a room that isn't on " +
			"any other map because the Kobold was the only one who knew about it. " +
			"The Kobold no longer knows anything. {xp} XP. " +
			"The map is yours. The room was yours today.",
	},

	3: {
		"The Cursed Crypt's inner chamber. Nobody is supposed to reach " +
			"the inner chamber in one run. You reached the inner chamber. " +
			"{item} worth €{value}. {xp} XP. The Draugr in the inner chamber " +
			"had been waiting for centuries for someone to reach it. " +
			"They did not expect to lose. Neither did you, honestly.",

		"You broke the curse. Not permanently — curses don't break permanently — " +
			"but for long enough that the inner rooms opened and you took " +
			"{item} worth €{value} before anything could stop you. " +
			"{xp} XP. The crypt is cursed again now. You are home and rich. " +
			"The timing was everything.",

		"A boss encounter in the Cursed Crypt. The boss had been there " +
			"since before the crypt was a crypt. You fought the boss. " +
			"You won, which is a sentence the boss did not anticipate " +
			"being the ending of. {item} worth €{value}. {xp} XP. " +
			"The crypt is quieter now.",
	},

	4: {
		"The deepest chamber of the Troll Bridge Depths yielded {item} worth €{value}. " +
			"Three Trolls, two Stone Giants, and the Cursed Knight of the Third Bridge " +
			"disagreed with this outcome. They were overruled. {xp} XP. " +
			"Your equipment has a story to tell. " +
			"The story involves those three Trolls and is not suitable for all audiences.",

		"The Stone Giant had {item}. The Stone Giant was the size of a room. " +
			"The Stone Giant is now the size of a room that is lying down. " +
			"€{value}. {xp} XP. You found a way. " +
			"The way was neither clean nor dignified but it was a way " +
			"and it worked and here you are.",

		"EXCEPTIONAL OUTCOME in the Troll Bridge Depths, which is a sentence " +
			"that has been said approximately four times in recorded history. " +
			"You are one of the four times. {item} worth €{value}. " +
			"{xp} XP. The dungeon will remember this.",
	},

	5: {
		"Against all probability, several laws of narrative structure, " +
			"and the general understanding that the Abyssal Maw does not " +
			"give things up easily, you have returned with {item} worth €{value} " +
			"and {xp} XP and a story that begins 'so there were three demons' " +
			"and gets significantly more complicated from there.",

		"The Elder Drake is dead. You killed an Elder Drake. " +
			"Its hoard contained {item} worth €{value}. " +
			"This is now a fact about you that is true. " +
			"The Abyssal Maw has one fewer Elder Drake. " +
			"The dungeon log is noting this with what can only be described " +
			"as reluctant respect.",

		"The Unnamed retreated. You don't know why. " +
			"The Unnamed does not retreat. It retreated today, from you, " +
			"and left {item} worth €{value} in the chamber it was occupying. " +
			"{xp} XP. There will be a next time. " +
			"You will think about what it means that it retreated " +
			"and you will not reach a comfortable conclusion.",

		"LEGENDARY RUN. The Abyssal Maw at full depth, full clear, " +
			"{item} worth €{value}, {xp} XP. " +
			"Three demons, an Elder Drake, and something that doesn't have a name. " +
			"You beat all of them. You are home. " +
			"This is the best day anyone in this community has had " +
			"in a dungeon. Write it down. " +
			"The dungeon will tell this story differently.",
	},
}