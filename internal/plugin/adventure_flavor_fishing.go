package plugin

// ── FISHING FLAVOR TEXT ───────────────────────────────────────────────────────
//
// Tier 1: Muddy Pond — garbage, sad fish, puns, Stardew energy
// Tier 2: Iron Creek — slightly less sad fish, mild danger
// Tier 3: Silver Lake — actually fishing now, occasional menace
// Tier 4: The Deep Current — serious water, serious things in it
// Tier 5: Abyssal Trench — Hemingway. Something old. Terse.
//
// Deaths: their own category. Drowning. Hooks. Sea monsters. Things that aren't fish.

var FishingDeath = map[int][]string{

	// ── TIER 1: MUDDY POND ────────────────────────────────────────────────────
	1: {
		"You fell in.\n\nThe Muddy Pond is four feet deep.\n\nYou drowned anyway.\n\n" +
			"The healthcare system has seen this before. They have a form for it. " +
			"The form says 'Muddy Pond' at the top and has checkboxes.",

		"Your stick with string caught on something. You pulled. It pulled back. " +
			"You went in face-first.\n\nThe something was a boot.\n\n" +
			"A boot pulled you into a pond.\n\nYou have 24 hours to think about this.",

		"You leaned over too far.\n\nThat's it.\n\nThat's the whole story.\n\n" +
			"Healthcare is not judging you. Healthcare is absolutely judging you.",

		"The frog startled you. One frog. Sitting on a lily pad, doing nothing, being a frog, " +
			"and you went backwards into the Muddy Pond like a plank falling over.\n\n" +
			"The frog watched you go.\n\nThe frog is still there.",

		"Your fishing line got tangled in your own boots somehow. " +
			"You investigated this while standing at the edge of the pond.\n\n" +
			"Investigating complete.",

		"You caught a fish. You were so surprised that you caught a fish that you forgot " +
			"where you were standing.\n\nThe fish got away.\n\nYou did not get away.",

		"A duck bit you. You recoiled. Pond.\n\nThe duck is fine.",
	},

	// ── TIER 2: IRON CREEK ───────────────────────────────────────────────────
	2: {
		"The current took you. Iron Creek has a current that the name does not adequately convey. " +
			"You found out what the name was trying to say. Healthcare found out where you were.",

		"Something bit your line, your rod, and then you, in that order, with increasing commitment. " +
			"It let go eventually. You were already in the water.",

		"You slipped on a wet rock. Every single wet rock in every fishing location has done this " +
			"to someone. The wet rock does not care. The wet rock has always been wet.",

		"A Cheep Cheep came from nowhere.\n\nYou know what a Cheep Cheep is. " +
			"Everyone knows what a Cheep Cheep is. The knowledge did not help you.\n\nIt never helps.",

		"The fishing line snapped back and took your footing with it. " +
			"Quick, efficient, no malice. Just physics and a creek that was never on your side.",
	},

	// ── TIER 3: SILVER LAKE ──────────────────────────────────────────────────
	3: {
		"Something large surfaced directly under your boat.\n\n" +
			"You don't have a boat.\n\nYou had a boat.",

		"The Silver Lake has a legend. A local thing — old fishermen, hushed tones, the usual. " +
			"You heard the legend and went anyway.\n\nThe legend is accurate.",

		"A Blooper. In a lake. You're not going to ask how. " +
			"You're going to spend 24 hours in healthcare not asking how.",

		"The thing you hooked was bigger than you. " +
			"This is not always a problem. This was a problem.",

		"You were doing fine until the water level started rising for no reason.\n\n" +
			"Water levels rising for no reason is always a reason.\n\n" +
			"You know this.\n\nYou stayed anyway.",
	},

	// ── TIER 4: THE DEEP CURRENT ─────────────────────────────────────────────
	4: {
		"Something came up from the Deep Current and looked at you.\n\n" +
			"Not at your bait.\n\nAt you.\n\n" +
			"You were in the water shortly after that.",

		"The thing on your line was not a fish. You knew it wasn't a fish by the second pull. " +
			"You kept pulling anyway.\n\nIt was not a fish.",

		"The Deep Current has a whirlpool. The whirlpool is not on any map. " +
			"The whirlpool introduced itself personally.",

		"A sea serpent. Not a large eel. A sea serpent, in a river, because the world doesn't " +
			"have rules anymore.\n\nHealthcare has a new checkbox now. Just for you.",

		"You went too deep. Wading, which you were explicitly not supposed to be doing, " +
			"and the current and the depth and the something that lives at that depth " +
			"all agreed on the same outcome.",
	},

	// ── TIER 5: ABYSSAL TRENCH ───────────────────────────────────────────────
	// Terse. Whatever is down there is old.
	5: {
		"It took the line, the rod, and most of the pier.\n\n" +
			"You were attached to the rod.\n\nYou are alive.\n\nIt let you go.",

		"You saw it before it saw you. This did not help.",

		"The Abyssal Trench is not a fishing location. The Abyssal Trench is a place where " +
			"things live that do not acknowledge the concept of fishing. You went anyway.\n\n" +
			"You're back.\n\nSomething down there made a decision about you.",

		"The thing on your line pulled once.\n\nOnce was enough.",

		"You heard it before you saw it. You saw it for approximately one second. " +
			"You were in the water for the rest of the encounter.\n\n" +
			"You are alive.\n\nThink about that.",
	},
}

var FishingEmpty = map[int][]string{

	1: {
		"Three hours at the Muddy Pond.\n\nNothing.\n\nNot even a boot.\n\n" +
			"Just you, a stick with string, and the specific silence of a pond " +
			"that has decided you're not worth the effort.",

		"You caught something. You reeled it in with great excitement. " +
			"It was pond water in the shape of disappointment.\n\nAlso a leaf.",

		"The fish saw you coming.\n\nAll of them.\n\nThey made a group decision.",

		"The Muddy Pond is four feet deep and approximately the size of a large puddle " +
			"and somehow contains nothing you can sell. You checked every inch. " +
			"The pond is empty. The pond has always been empty. The pond is laughing at you in pond.",

		"You sat there for four hours.\n\nA duck swam past.\n\n" +
			"The duck had somewhere to be.\n\nYou did not.",

		"Nothing bit. Not the bait, not the hook, not even the increasingly desperate " +
			"energy you were projecting at the water. The Muddy Pond was unmoved.",

		"Your stick with string snapped on the third cast. You fashioned a replacement " +
			"stick with string. The replacement performed identically to the original, " +
			"which is to say: nothing happened.",
	},

	2: {
		"Iron Creek fished clean today. Clean meaning: nothing.\n\n" +
			"Not the clean of a well-managed fishery.\n\n" +
			"The clean of a creek that has been thoroughly fished by someone faster than you.",

		"You had bites. Genuine bites — the line moved, something was interested, " +
			"the interest peaked and then vanished like it remembered who you were.",

		"The fish in Iron Creek are not stupid. You are fishing with a " +
			"Dull Copper Pickaxe energy-tier fishing rod in a creek that has been " +
			"here longer than you. They've seen this before.",
	},

	3: {
		"Silver Lake: famously full of fish. The fish were elsewhere today. " +
			"Where fish go when they're elsewhere is not known. They were not here.",

		"Four hours. The lake. A good rod. The right bait. Perfect conditions.\n\n" +
			"Absolutely nothing.\n\nSilver Lake shrugs in lake.",

		"You caught a reflection of yourself in the water.\n\n" +
			"You look tired.\n\nYou came home with nothing.",
	},

	4: {
		"The Deep Current gave you nothing today and the nothing felt intentional.\n\n" +
			"Something down there made a decision about your haul.\n\nThe decision was no.",

		"You were this close. Whatever 'this close' means at the Deep Current, " +
			"which is a river where things live that have never been catalogued. " +
			"You felt the line move. Then it stopped moving. Then nothing.",
	},

	5: {
		"The Abyssal Trench does not give up its catch easily.\n\n" +
			"Today it didn't give up anything.\n\n" +
			"You fished for hours in the dark above water that has no bottom anyone has measured.\n\n" +
			"You came home.\n\nThe Trench noticed.",

		"Nothing rose to meet you today.\n\n" +
			"This is either very good news or the other kind.\n\n" +
			"You are choosing to believe the former.",
	},
}

var FishingSuccess = map[int][]string{

	1: {
		"You caught a fish from the Muddy Pond.\n\nA small fish.\n\n" +
			"It looked at you with an expression that can only be described as resigned.\n\n" +
			"You felt bad.\n\nYou sold it anyway.\n\n{item} — €{value}.",

		"Something on the line. Reeled it in.\n\nA boot.\n\n" +
			"Inside the boot: €{value} and a note that says 'GOOD LUCK.'\n\n" +
			"Someone has been here before you.\n\nThe boot is yours now.",

		"A tin can. Inside the tin can: a smaller tin can. Inside the smaller tin can: " +
			"{item} worth €{value}.\n\nThe Muddy Pond has layers.\n\n" +
			"You did not expect the Muddy Pond to have layers.",

		"The fish you caught was technically too small to sell.\n\n" +
			"You sold it anyway.\n\nThe buyer didn't ask.\n\nYou didn't offer.\n\n" +
			"€{value}. Everyone moved on.",

		"You caught {item} worth €{value} and {xp} XP and the quiet dignity of a person " +
			"who caught something from the Muddy Pond and is choosing to feel good about it.\n\n" +
			"The dignity is thin.\n\nIt is still dignity.",

		"Three small fish, two of which got away, one of which didn't.\n\n" +
			"The one that didn't is worth €{value}.\n\nIt tried very hard.\n\n" +
			"You respect that.\n\nYou sold it.",

		"The pond gave up {item} today.\n\nNo drama. No fight. It just came up.\n\n" +
			"The Muddy Pond felt sorry for you.\n\n" +
			"You choose not to think about that.\n\n€{value}.",

		"You pulled up an old helmet from the pond floor.\n\n" +
			"Inside the helmet: a fish.\n\nThe fish had been living in the helmet.\n\n" +
			"You have displaced a fish from its home.\n\nYou sold both.\n\n€{value}.",

		"A rusted sword from the pond floor.\n\n" +
			"On it: an inscription you can't read.\n\n" +
			"In a better story this would be significant.\n\n" +
			"It sold for €{value}.\n\nThis is not a better story.",
	},

	2: {
		"Iron Creek delivered {item} today. Not enthusiastically — more like it was " +
			"clearing out storage. But delivered.\n\n€{value}. {xp} XP.",

		"The fish fought. Good fight actually. You won, which surprised the fish " +
			"and mildly surprised you.\n\n{item}, €{value}.",

		"You caught something in Iron Creek that you don't have a name for.\n\n" +
			"The fishmonger had a name for it.\n\nThe fishmonger paid €{value} without blinking.\n\n" +
			"You're choosing not to ask what it was.",

		"Two fish. Both annoyed about it. €{value} total.\n\n{xp} XP.\n\n" +
			"Iron Creek fished well today if you don't count the water level.",
	},

	3: {
		"Silver Lake, finally cooperating.\n\n{item} worth €{value}.\n\n" +
			"The lake didn't make it easy.\n\nThe lake never makes it easy.\n\nBut here you are.",

		"You caught {item} from Silver Lake and gained {xp} XP and the specific satisfaction " +
			"of a lake that tried to defeat you and failed today.\n\n" +
			"Just today.\n\nIt'll try again tomorrow.\n\n€{value}.",

		"The catch practically jumped into the boat.\n\nYou don't have a boat.\n\n" +
			"It jumped onto the bank.\n\nThis has never happened before and you are " +
			"not questioning it.\n\n{item}, €{value}.",
	},

	4: {
		"The Deep Current gave up {item} worth €{value} and only tried to kill you once, " +
			"which at this tier is practically a warm welcome.\n\n{xp} XP.",

		"You landed {item} from the Deep Current.\n\n" +
			"The fight took forty minutes.\n\nYour arms are not okay.\n\n" +
			"The fish is €{value}.\n\nYour arms will recover.",

		"Something worth €{value} from the depths.\n\n" +
			"You don't know exactly what it is.\n\nThe buyer knew exactly what it was.\n\n" +
			"The buyer's face when you walked in told you everything about " +
			"whether you charged enough.\n\n{xp} XP.",
	},

	5: {
		"The Abyssal Trench released {item}.\n\nNot gave. Released.\n\n" +
			"Like it had been holding it and made a decision.\n\n" +
			"€{value}. {xp} XP.\n\nYou came home.\n\nThat's the whole win.",

		"You fished the Abyssal Trench and came back with {item} worth €{value}.\n\n" +
			"Something down there let you.\n\nRemember that.",

		"An hour at the Trench.\n\n{item} worth €{value}.\n\n" +
			"The thing that lives down there watched you the entire time.\n\n" +
			"You could feel it watching.\n\nIt let you keep the fish.\n\n{xp} XP.",
	},
}

var FishingExceptional = map[int][]string{

	1: {
		"THE MUDDY POND HAS PRODUCED SOMETHING REMARKABLE.\n\n" +
			"Not a fish. Not a boot. Not a tin can inside a tin can.\n\n" +
			"{item} worth €{value}.\n\nIN THE MUDDY POND.\n\n" +
			"YOU FOUND THIS IN THE MUDDY POND.\n\n" +
			"The pond has been keeping secrets.\n\n" +
			"The pond has been keeping €{value} worth of secrets.",

		"Critical catch from the Muddy Pond.\n\nThe Muddy Pond.\n\n" +
			"Where the sad fish live.\n\nWhere boots go to die.\n\n" +
			"{item} — €{value} — {xp} XP.\n\nSomeone go check on the Muddy Pond.",

		"You caught the legendary fish of the Muddy Pond.\n\n" +
			"There is no legendary fish of the Muddy Pond.\n\nThere is now.\n\n" +
			"You caught it.\n\n{item}, €{value}, {xp} XP.\n\nThe frog witnessed this.",
	},

	2: {
		"Iron Creek: exceptional haul.\n\n{item} worth €{value}.\n\n" +
			"The creek is annoyed about this.\n\nYou can tell.\n\n" +
			"The creek is definitely annoyed.\n\n{xp} XP.",

		"The fish that got away last time came back.\n\nWith friends.\n\n" +
			"The friends were a mistake on the fish's part.\n\n{item}, €{value}.",
	},

	3: {
		"Silver Lake, fully cooperating, no asterisks.\n\n{item} worth €{value}.\n\n" +
			"{xp} XP.\n\nA genuinely good day at a lake that usually has something to say about it.\n\n" +
			"Today: nothing to say.\n\nJust the fish.",

		"You caught something Silver Lake clearly didn't mean to let go.\n\n" +
			"The water seemed surprised.\n\nWater doesn't get surprised.\n\n" +
			"Silver Lake did.\n\n{item}, €{value}, {xp} XP.",
	},

	4: {
		"The Deep Current tried everything.\n\nThe current. The cold. The things that live in it.\n\n" +
			"You caught {item} worth €{value} anyway.\n\n{xp} XP.\n\n" +
			"Something down there is going to be thinking about this for a while.",

		"An exceptional haul from the Deep Current and you're only slightly broken.\n\n" +
			"{item}, €{value}.\n\nThe fish put up a fight that will honestly be hard to " +
			"explain to people.\n\nYou're going to try anyway.",
	},

	5: {
		"The Abyssal Trench, against all precedent and possibly against its will, " +
			"gave up {item} worth €{value}.\n\n{xp} XP.\n\n" +
			"Something old and enormous watched you take it.\n\n" +
			"It didn't stop you.\n\nThat's the part to think about.",

		"You caught something from the deepest part of the Abyssal Trench that has no name " +
			"because the people who named things didn't come back from down there.\n\n" +
			"You came back.\n\n{item}. €{value}. {xp} XP.\n\n" +
			"The Trench is reconsidering its position on you.",

		"LEGENDARY CATCH from the Abyssal Trench.\n\n{item} worth €{value}.\n\n" +
			"The thing that lives down there surfaced briefly.\n\n" +
			"Looked at what you caught.\n\nSubmerged again.\n\n" +
			"No comment from the thing.\n\n{xp} XP.",
	},
}

// ── FISH PUNS ─────────────────────────────────────────────────────────────────
// Used as optional one-line appends to success outcomes.
// Rotate through pool, skip last 5 used per player.
// These should cause physical pain.

var FishPuns = []string{
	"You're on a reel roll today.",
	"Quite the net result.",
	"That's a catch worth carping about.",
	"You really flounder-ed your way into that one. Wait, no. The opposite.",
	"Looks like today was your lucky day. No trout about it.",
	"You're krilling it.",
	"That went swimmingly.",
	"Fin-tastic work.",
	"You really showed that fish who's bass.",
	"Cod you believe that?",
	"This is a moray than you expected.",
	"For eel real though.",
	"You've really got sole.",
	"Quite the dab hand at this.",
	"That's just plain e-fish-ent.",
	"You really reeled that one in. Because fishing. That's the pun.",
	"Water day.",
	"Scale of one to ten: excellent.",
	"You de-fin-itely earned that.",
	"Pike-fect execution.",
	"Something smells fishy. It's the fish. You caught fish.",
	"Another one bites the lure.",
	"Cheep cheep. Oh wait, wrong water enemy.",
	"You avoided every Cheep Cheep, every Blooper, and every inexplicable underwater obstacle " +
		"and came home with fish. Mario would be proud. Mario drowned in world 3-3.",
	"The Bloopers were not invited. They came anyway. They left.",
	"The water level did not rise today. This is exceptional and deserves acknowledgement.",
	"You navigated the underwater section without losing a single life. The underwater section " +
		"of what? Of life. This is the underwater section of your life.",
}

// ── LOCATION-SPECIFIC FLAVOR ──────────────────────────────────────────────────

// MuddyPondTrash — catches a trash item (not a fish)
var MuddyPondTrash = []string{
	"A boot. Just the one.",
	"An old helmet. A fish was living in it. The fish did not appreciate the eviction.",
	"A tin can containing a smaller tin can containing nothing.",
	"Someone's shield. Tier 0. Worse condition than yours.",
	"A fishing rod. Better than yours. Belonging to whoever is at the bottom of the Muddy Pond.",
	"A locked box. You cannot open it. The lock is interesting. The box is staying.",
	"Seventeen copper coins and a note that says 'DO NOT SPEND THESE.' You spend them.",
	"A boot. A different boot. Not a pair. Just another different single boot.",
	"A sword. Inscribed. The inscription says 'RETURN TO GERALD.' You don't know Gerald. " +
		"You are confident that the shop to which you sold his sword for €{value} will return it to him.",
	"A duck's nest. Occupied. The duck's position on this is immediate and physical.",
}

// MuddyPondSadFish — catches a sad fish
var MuddyPondSadFish = []string{
	"A fish. Small. It looked at you and somehow it managed to scoff at the sight of your visage. " +
		"You sold it immediately to salvage what little remains of your self-respect.",

	"The saddest fish you have ever seen.\n\nIt didn't fight.\n\nIt just came up.\n\n" +
		"When it sees you, its disposition dramatically improved as if somehow it felt immensely " +
		"better after witnessing someone with a life far worse than its own. " +
		"It is delighted to be sold for €{value} now.",

	"A fish that was clearly having a bad day before you showed up.\n\n" +
		"When it saw you, it stopped struggling entirely.\n\nNot out of defeat.\n\nOut of pity.\n\n" +
		"It sold for €{value}.",

	"The fish sighed when you pulled it out.\n\nNot from exhaustion.\n\n" +
		"Because it was ready to have its life ended by a superior lifeform, " +
		"but instead it was you that caught it.\n\nSold for €{value}.",

	"A very small fish.\n\nIt took one look at you and immediately stopped trying to get back " +
		"in the water.\n\nYou made the mistake of believing this was due to the fish being " +
		"frozen by your dominating aura.\n\nCloser inspection reveals the fish to be laughing " +
		"hysterically. So much so that it was unable to move.\n\n" +
		"It was still doing so when you sold it for €{value}.",

	"The fish turned to get a look at you and once it did, it immediately turned around " +
		"to poop in your general direction.\n\nThis wasn't a coincidence as it proceeded " +
		"to do so each time it saw you until you eventually sold it for €{value}.",
}

// AbyssalCatch — things that aren't fish
var AbyssalCatch = []string{
	"Not a fish. Something with too many eyes and a completely calm energy about being caught. " +
		"The fishmonger paid €{value}. You both looked at it, only the fishmonger's gaze never " +
		"left the creature.. with eyes filled with lust and biting their lip slightly. " +
		"You immediately felt uncomfortable and vacated the premises.",

	"Ancient. Whatever it is, it has been down there since before the Trench was called " +
		"the Trench. Worth €{value}. The buyer's hands shook.",

	"A fish, technically. In the same way the Trench is technically a body of water. " +
		"€{value}. {xp} XP. The thing looked at you on the way in like it recognized you.",

	"Something that makes a sound when it opens its mouth. Not thrashing. A sound. Deliberate. " +
		"You finally recognize it.. the sound it makes every time it looks at you and opens its " +
		"mouth.. it's a bleep censor. €{value}.",
}

// ── FISHING SKILL LEVEL UP MESSAGES ──────────────────────────────────────────

var FishingLevelUp = []string{
	"🎣 Fishing Lv.{n}. You are slightly better at sitting near water and waiting.",
	"🎣 Fishing Lv.{n}. The fish have gained some respect for you now. " +
		"I would tell you how much, if the amount were large enough to measure.",
	"🎣 Fishing Lv.{n}. You've graduated from 'stick with string' energy, technically.",
	"🎣 Fishing Lv.{n}. A milestone. Definitely not one to brag about but you will anyway. " +
		"I shall pray for your friends.",
	"🎣 Fishing Lv.{n}. You understand fish now in the way that a predator understands prey. " +
		"Which is to say: commercially.",
	"🎣 Fishing Lv.{n}. Something at the bottom of the Abyssal Trench stirred briefly. " +
		"You get the feeling that something was looking at you the same way you look at a " +
		"steak dinner.. rather the way you WOULD look at a steak dinner if you could ever afford one.",
	"🎣 Fishing Lv.{n}. Whatever lives at the bottom is planning to invite you to a feast.. " +
		"no wait, I read that wrong, nevermind.",
}

// ── SUMMARY ONE-LINERS ───────────────────────────────────────────────────────

var SummaryFishingSuccess = []string{
	"Fished {location}. Caught {item}. €{value}.",
	"{location} cooperated. {item}, €{value}. A good day on the water.",
	"Caught something in {location}. Sold for €{value}. The fish disagreed with this outcome.",
}

var SummaryFishingEmpty = []string{
	"Fished {location}. Nothing. The water won.",
	"{location}: hours of sitting. Zero fish. Many thoughts.",
	"Nothing from {location}. The fish made a collective decision.",
}

var SummaryFishingDeath = []string{
	"Drowned at {location}. The water was involved. {hours}h.",
	"Fishing death. {location}. Something in the water had opinions. {hours}h.",
	"Dead at {location}. Fishing-related. Healthcare has the form. {hours}h.",
}
