package plugin

import "math/rand/v2"

// ── Rival Trash Talk Pools ──────────────────────────────────────────────────

// rivalOpeningTaunts are delivered with the challenge DM (Round 1).
var rivalOpeningTaunts = []string{
	"Your strategy is filled with more holes than your boots. This will be like taking candy from a baby. Actually I shouldn't joke about that -- it looks like that's how you sustain yourself.",
	"I've seen better odds on a three-legged horse. The horse at least had a plan.",
	"I want you to know I considered not crossing the street. I crossed anyway. That tells you everything about how this is going to go.",
	"You look nervous. You should look nervous. That's the first sensible thing I've seen from you.",
	"I've done this before. You can tell by how little I'm sweating. Look at you. Look at me. See the difference?",
	"I'm not saying this will be easy. I'm saying it will be quick. There's a distinction.",
	"Take your time. Think it through. It won't help but I want you to feel like you had a fair shot.",
}

// rivalRoundWon are said by the rival after they win a round.
var rivalRoundWon = []string{
	"Looks like you won't be buying any tacos today. You don't look like you're used to eating real food anyway.",
	"One down. The math is unkind to you right now. As is most things, I suspect.",
	"Did that hurt? The losing, I mean. Or is this familiar enough to be comfortable?",
	"I knew what you were going to throw before you did. I know what you're going to throw next. I won't tell you.",
	"You should have thrown something else. You know that now. I knew it before you threw.",
	"_takes a moment to write something down_ Sorry. Just keeping notes. Continue.",
	"The expression on your face right now is doing a lot of work. Most of it sad.",
}

// rivalRoundLost are said by the rival after they lose a round.
var rivalRoundLost = []string{
	"Even a battered, dirty, _sniffs_ smelly, and broken beyond any hope of repair clock is right twice a day.",
	"Fine. You got one. Don't read into it. Actually -- you won't know what to read into. Never mind.",
	"I want you to enjoy this moment. Really sit in it. It may be all you have to take home today.",
	"I respect the throw. I don't respect the thrower. These are two separate things.",
	"Lucky. I've seen luck before and that's what that was. Make peace with it.",
	"_pauses_ Okay. _pauses again_ I wasn't expecting that. That changes nothing. Next round.",
	"You threw the right thing at the right time. The stopped clock principle applies. Ask someone to explain it to you later.",
}

// rivalTied are said by the rival after a tied round (before re-throw).
var rivalTied = []string{
	"Same throw. Neither of us embarrassed ourselves. One of us is about to rectify that.",
	"We're going again. That's fine. I have nowhere to be. Do you have somewhere to be? It doesn't matter.",
	"You matched me. I want to be clear that this is the ceiling of your performance and we both know it.",
	"_narrows eyes_ Interesting. Going again. Don't get comfortable.",
}

// rivalClosingWin are said by the rival when they win the match.
var rivalClosingWin = []string{
	"I told you. I always tell people. People don't listen. Then it's over and they know I was right. Every time.",
	"I hope the walk home gives you some time to think. Not about what you could have done differently. Just in general. Thinking is good for people.",
	"Take care of yourself out there. Eat something. You look like you haven't in a while. _folds hands_ Actually that's your business. Good day.",
	"The gold is mine. The record is updated. You may go.",
	"I've done my good deed. The world is slightly more balanced now. _nods slowly and walks away_",
	"You gave it everything. _looks you over_ Everything wasn't enough. But still. Points for showing up.",
}

// rivalClosingLoss are said by the rival when they lose the match.
var rivalClosingLoss = []string{
	"You look like you've won the damn lottery. The sheer excitement on your face from winning a handful of gold is genuinely depressing. _sighs_ I suppose I've done my good deed for today. _checks off an item on a list titled 'Feed a waif today'_",
	"Fine. Take it. You've clearly never had this much at one time and I'm not going to be the one to take that from you. Today.",
	"I'll be back. Enjoy the gold. Buy yourself something warm to eat. You look cold.",
	"_stares at you for a long moment_ No. _turns and walks away_",
	"You won. I lost. The sun is back out. These things are unrelated. _looks up at the sky anyway_",
	"I'm not angry. I'm not even surprised. I'm something else entirely and I don't have the energy to name it right now. Congratulations.",
	"Keep it. Consider it a loan. _has not indicated when or if repayment is expected_ You chuckle to yourself, convinced this was said in jest. You notice them scribbling a collection date.",
}

// rivalRoundOutcomeWin are the player-wins-round outcome lines.
var rivalRoundOutcomeWin = []string{
	"%s loses to %s. This is known.",
	"You read them perfectly. Or you guessed. Either way.",
	"%s takes it. Round to you.",
}

// rivalRoundOutcomeLoss are the player-loses-round outcome lines.
var rivalRoundOutcomeLoss = []string{
	"%s beats %s. The math is what it is.",
	"Wrong call. It happens.",
	"%s takes it. Round to them.",
}

// rivalUnlockDM is sent once when a player reaches Combat Level 5.
const rivalUnlockDM = `You have reached Combat Level 5.

This means something now. Not in the way you think. Somewhere out there, someone else hit level 5 too. They're going about their day. So are you. At some point, one of you will receive a DM.

That DM will not be friendly.

Good luck.`

// rivalForfeitLines are used when a challenge expires without response.
var rivalForfeitLines = []string{
	"You didn't respond. The gold has been collected. The rival walked away satisfied. You weren't even there to see it.",
	"Time's up. The rival waited. You didn't show. The gold is gone.",
	"The 24 hours have passed. The rival has claimed their winnings by default. They seemed disappointed. Not about the gold.",
}

// pickRivalFlavor returns a random entry from the given pool.
func pickRivalFlavor(pool []string) string {
	if len(pool) == 0 {
		return ""
	}
	return pool[rand.IntN(len(pool))]
}
