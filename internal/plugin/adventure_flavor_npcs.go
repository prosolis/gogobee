package plugin

// ── Misty — Opening Lines ──────────────────────────────────────────────────

var mistyOpenings = []string{
	"Excuse me. I'm sorry to bother you. I don't usually ask but -- do you have any gold to spare? Even a little would help.",
	"I hate to stop you. I can see you're busy. I just -- do you have any gold? It doesn't have to be much. Anything at all.",
	"Sorry. I'm so sorry. I know this is awkward. Do you have 100 gold you could spare? I wouldn't ask if I didn't need to.",
	"You look like a kind person. I hope you are. Do you have any gold? Just 100. I'll remember it.",
	"I don't usually do this. I want you to know that. Do you have 100 gold? I'll be out of your way in a moment either way.",
}

// ── Misty — Accept Lines ───────────────────────────────────────────────────

var mistyAcceptLines = []string{
	"Oh. Thank you. Really. You didn't have to do that and you did. That means more than you know. Good luck out there.",
	"Thank you so much. I mean it. You're a good person. I hope the arena treats you well today.",
	"You're very kind. I won't forget it. Thank you. Truly. Go on -- I've kept you long enough.",
	"Oh, thank you. Thank you. I hope something wonderful happens to you today. I really do.",
	"That's -- thank you. You didn't have to. I hope you know that mattered. Go show them what you've got.",
}

// ── Misty — Decline ────────────────────────────────────────────────────────

const mistyDeclineLine = "She nods once and walks away. She doesn't look back."

// ── Arina — Opening Lines ──────────────────────────────────────────────────

var arinaOpenings = []string{
	"You. Yes, you. I've been watching your arena performances and I've decided -- against my better judgment -- to take an interest. 5,000 gold. I'll make it worth your while. Don't embarrass me.",
	"I'll be brief because my time is valuable and yours demonstrably isn't. 5,000 gold. I have a sniper I'm not currently using. We could have an arrangement. Or not. It's entirely your loss.",
	"I've decided you have potential. It pains me to admit it. 5,000 gold and I'll have someone watch your fights. Professionally. Do try to look capable when you answer.",
	"You look like someone who could use help. Most people in your position do. 5,000 gold. I have resources. You have need. This is called an arrangement. Say yes and try not to gloat about it.",
	"Don't make that face. I'm offering you something. 5,000 gold and a week of professional support in the arena. The alternative is continuing as you have been, which I think we both find embarrassing.",
}

// ── Arina — Accept ─────────────────────────────────────────────────────────

const arinaAcceptLine = "Where's my thank you? Someone of my stature is accepting your dirty peasant money."

// ── Arina — Decline Lines ──────────────────────────────────────────────────

var arinaDeclineLines = []string{
	"Remarkable. You've somehow managed to be both broke and stupid. A rare combination. Enjoy your mediocrity.",
	"I see. You'd rather fail on your own terms. How quaint. How completely, utterly quaint.",
	"Fine. Go back to whatever it is you do. I'll find someone with half a brain and twice the ambition. Shouldn't be hard.",
	"I offered you a lifeline and you looked at it and said no. I want you to think about that. Later, when it's relevant. And it will be relevant.",
	"No. You said no. To me. I'll remember that. Not out of spite -- I'm above spite -- but because it's funny, and I collect funny things.",
	"You're turning down professional assistance because -- what exactly? Pride? You can't afford pride. You can barely afford that equipment.",
}

// ── Gourmet Food Pool (Misty buff — arena) ─────────────────────────────────

var mistyGourmetFoodLines = []string{
	"The crowd has thrown a Seared Foie Gras with Fig Reduction at you. You catch it without thinking. It is perfect. You eat it in the arena. {enemy} stops moving for a moment. {enemy} takes 5 damage.",
	"Someone in the upper tier has thrown a Wagyu Beef Tartare with Truffle Oil at you. You eat it immediately and without shame. {enemy} watches this happen. {enemy} takes 5 damage. {enemy} is reconsidering the fight.",
	"A Lobster Bisque in a warmed ceramic bowl lands in your hands. You drink it. The crowd roars. {enemy} takes 5 damage from witnessing this.",
	"The crowd has provided a Tasting Menu Amuse-Bouche. Three bites. All perfect. You finish it in four seconds. {enemy} takes 5 damage. {enemy} does not understand what is happening.",
	"Someone throws a hand-rolled Truffle Pasta at you. You eat it like a feral animal. It heals you. {enemy} takes 5 damage and briefly forgets what they were doing.",
	"A Deconstructed Beef Wellington lands nearby. You eat the components separately and in the wrong order. It is still extraordinary. {enemy} takes 5 damage.",
	"The crowd has thrown a Michelin-starred Tuna Tataki at you. You catch it, eat it in one motion, and keep moving. {enemy} takes 5 damage. {enemy} will think about this later.",
	"Someone in the crowd has thrown a perfectly tempered Chocolate Fondant with Salted Caramel at you mid-fight. You eat it immediately. {enemy} takes 5 damage from the indignity of the situation.",
	"A Burrata with Heirloom Tomatoes and Aged Balsamic lands at your feet. You eat it off the arena floor without hesitation. The crowd erupts. {enemy} takes 5 damage.",
	"The crowd throws a Saffron Risotto at you. It is warm. It is perfectly seasoned. You eat it with your hands. {enemy} takes 5 damage. {enemy} files this moment away somewhere dark.",
}

// ── Crowd Revenge Pool (Misty debuff — arena) ──────────────────────────────

var mistyCrowdRevengeLines = []string{
	"The crowd has remembered something. They're booing. Something has been thrown. It hits you. {damage} damage. The arena has a long memory.",
	"Someone in the crowd throws something that is definitely not food. {damage} damage. The booing intensifies.",
	"The arena crowd has opinions about you specifically today. Something lands. {damage} damage. You don't see where it came from. You don't need to.",
	"The crowd is restless. An object arrives from the upper tier. {damage} damage. The booing is organized. That's somehow worse.",
	"Something hits you from the stands. {damage} damage. The crowd is not finished. The crowd is never finished.",
}

// ── Sniper Log Lines (Arina buff — arena) ──────────────────────────────────

var arinaSniperLines = []string{
	"Something arrives from the upper tier. {enemy} doesn't finish their round.",
	"A bolt. One bolt. {enemy} is down. Nobody in the crowd reacts. They saw nothing.",
	"The shot comes from somewhere in the stands. {enemy} had a bad week. It ended here.",
	"From the upper tier: one shot. {enemy} goes down. The fight continues without them.",
	"The arena lights caught it briefly. Just briefly. {enemy} is down.",
}
