package plugin

import "math/rand/v2"

// ── Blacksmith Flavor Text Pools ────────────────────────────────────────────

var blacksmithGreetings = []string{
	"*wipes hands on apron and looks you over slowly* You've seen some action. I can tell. Come inside. Let me experience what you've been working with.",
	"*sets down hammer* Don't be shy. I've seen worse. Bring it here. I'll get it back to its full size in no time.",
	"I don't judge the condition things arrive in but I *always* know how to make them feel... oh so much better. Weapons and armor too.",
	"*gestures broadly at the forge* Everything in here gets my full and undivided attention. You have my word on that. Unless a ball gag is involved. \u2764\ufe0f",
	"You look like someone who's been putting their equipment through a lot. That's what I'm here for. Show me everything.",
}

var blacksmithInspection = []string{
	"Oh. Oh that's seen some use. *runs thumb along the edge* Don't worry. I know exactly what this needs.",
	"This one's been neglected. I can tell. People don't appreciate what they have until it's been beaten down until it can't take any more. Then it's my job to *slowly* build you... it... back up completely from bottom to the top.",
	"*whistles low* That's a lot of damage. I'm not complaining. I'm just saying I'm going to need some time with this one.",
	"Good bones. Good bones. A little beaten up but my skilled fingers have brought worse back from the brink to highs rarely seen nor experienced on Earth! ...Oh I can fix your item too.",
	"*holds it up to the light* I've had my hands on a lot of these. This one needs work. The good kind of work. *looks you deeply in the eyes* The only kind of work I know how to do.",
}

var blacksmithPayment = []string{
	"*pockets the gold without looking at it* Money is fine. The work is the reward.",
	"Generous. I'd have done it for less. Don't tell anyone.",
	"*nods slowly* Fair. Now let me get to it. I work better when I'm not being watched. Come back in a bit.",
	"You're paying for the expertise. Anyone can hit metal. Not everyone knows where to hit it. Or how to hit it to get it quivering after each strike until it's begging for more! *there's an awkward silence* Anyway. Time to get to work.",
	"The gold is appreciated. Now. Unless you have need for my services from the *ahem* hidden menu, come back later.",
}

var blacksmithCompletion = []string{
	"There. *slides it across the counter* Good as new. Better, actually. I know what I'm doing.",
	"*breathes heavily* Done. That one took something out of me. Worth it though. Always worth it.",
	"You feel that? That's what it's supposed to feel like. Full. Firm. Ready for anything at any time. Just like me.",
	"I worked it until it couldn't take any more. Then I worked it a little more. I'm more than happy to give you a demonstration sometime.",
	"*wipes forehead* The difficult ones are always the most satisfying. Take it. Come back when it needs attention again. And it will need attention again.",
	"It's hot right now. Give it a moment before you put it on. Trust me on that.",
	"*stares at the finished piece for a moment longer than necessary* Beautiful. Okay. Take it. Go.",
}

var blacksmithMasterwork = []string{
	"*pauses* This is Masterwork. *sets everything else down* This gets my complete focus. Nothing else exists right now.",
	"I don't rush Masterwork. I don't care how long it takes. Some things deserve to be done properly.",
	"*closes eyes briefly* I've been waiting for one of these to come through here. You have no idea how long I've been waiting.",
	"Extra charge for Masterwork. Not because it's harder. Because it deserves more of me. It demands more of me. And oh god it will get it, don't you worry at all.",
}

var blacksmithArena = []string{
	"*goes very still* Is that... Arena gear? *looks up at you* Where did you get this. Never mind. Don't tell me. Just leave it.",
	"I've heard about this. I've never had one on my table before. *voice drops* Close the door.",
	"The Arena gear gets the good tools. I don't use these for everything. Just for things that matter.",
}

var blacksmithFullCondition = []string{
	"*looks it over* There's nothing to do here. It doesn't need me. *sounds slightly disappointed* Come back when it does.",
	"Full condition. You've been taking care of it. Good. I appreciate that in a person.",
	"Nothing to fix. Come back when you've broken something. Or you need me to break you. Special offer. Just for you.",
}

var blacksmithBrokenCondition = []string{
	"*long pause* ...How. *another pause* You know what. It doesn't matter. I've seen things. Sit down.",
	"This is what happens when people don't come see me regularly. *not accusatory. somehow worse than accusatory.*",
	"I can fix this. It'll take longer. And cost more. And I'm going to need you to just... not talk while I work. Can you do that.",
	"*picks it up gingerly* It's not dead. It just needs someone who knows what they're doing. Lucky for you I always know what to do in *every* situation. \u2764\ufe0f",
	"*looks at the condition, looks at you, looks back at the condition* You know it costs more when you let it get like this. Of course you know. You just didn't care. That's fine. I care enough for both of us. It'll cost you.",
	"This could have been avoided with regular visits. *slides the cost estimate across the counter without breaking eye contact*",
}

func pickBlacksmithFlavor(pool []string) string {
	if len(pool) == 0 {
		return ""
	}
	return pool[rand.IntN(len(pool))]
}
