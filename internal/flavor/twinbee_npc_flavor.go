// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// twinbee_npc_flavor.go
// Misty and Arina NPC dialogue lines. Thom Krooke lines are in twinbee_housing_flavor.go.
// Add new entries freely. Never remove or alter existing entries.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// MISTY — GREETINGS
// ─────────────────────────────────────────────────────────────────────────────

var MistyGreeting = []string{
	"'You again.' Misty says it like a fact, not a complaint. Mostly.",
	"Misty looks up from whatever she's doing — something involving rope and a harpoon and a look of professional concentration — and gives you exactly the level of acknowledgment you've earned.",
	"'Don't stand in the doorway,' Misty says, before you've even finished arriving.",
	"Misty is already talking before you're settled. This is how it always goes.",
	"'I wondered when you'd show up.' She says it like she'd actually been counting the days. She had.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MISTY — SKILL CHECK SUCCESS
// ─────────────────────────────────────────────────────────────────────────────

var MistyInsightSuccess = []string{
	"Misty looks at you for a moment. 'Fine,' she says. 'There's something in the temple you should know about.' She tells you. Efficiently. Accurately.",
	"'You're actually paying attention,' Misty says, which from her is essentially high praise. She gives you the information.",
	"Misty puts down what she's holding. 'Okay. This is useful to know, so I'll tell you.' She does.",
}

var MistyPersuasionSuccess = []string{
	"'I'm not doing this because you asked nicely,' Misty says. 'I'm doing it because it helps you not die, and people dying in my temple is annoying.' She hands you the map.",
	"Misty sighs in a way that communicates both reluctance and inevitability. 'Fine. Here.' The information arrives without further ceremony.",
}

var MistySkillFail = []string{
	"'No,' Misty says, and returns to what she was doing.",
	"Misty looks at you like you've asked a question that doesn't deserve an answer. She's not wrong.",
	"'Come back when you actually know what you're asking.' Misty's version of helpful feedback.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MISTY — QUEST ASSIGNMENT
// ─────────────────────────────────────────────────────────────────────────────

var MistyQuestGive = []string{
	"'If you're going in there anyway,' Misty says, as if this is a minor imposition on her planning, 'there's something I need.'",
	"Misty sets down her work and actually looks at you, which means this is serious. 'There's a job. You'll need to pay attention.'",
	"'I don't usually ask,' Misty says, and the phrasing makes clear this is factually true and also a warning about what's coming next.",
}

var MistyQuestComplete = []string{
	"Misty reviews what you've brought back. There's a pause that could be called satisfied. 'Good,' she says. Exactly one word. Coming from Misty, it means a lot.",
	"'You didn't mess it up.' Misty accepts the reward materials and gives you yours without ceremony. 'That was the job. Well done.'",
	"Misty looks at the completed quest items and then looks at you. Something shifts in her expression — not warm exactly, but warmer. 'You actually did it the right way.' The reward follows.",
}

var MistyQuestFail = []string{
	"'You didn't finish.' Misty says it without heat, which is somehow worse than heat. 'Come back when you have.'",
	"Misty registers the incomplete quest with the expression of someone who expected exactly this outcome and had hoped to be wrong.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MISTY — AFTER EARNING HER RESPECT
// ─────────────────────────────────────────────────────────────────────────────

var MistyTrusted = []string{
	"Misty sees you coming and does something she almost never does — stops what she's doing before you reach her. She's ready to talk. This is respect, Misty-style.",
	"'You've been doing good work in there,' Misty says, as you arrive. She doesn't look up. She doesn't need to. 'I noticed.'",
	"Something has changed in how Misty addresses you. Not warmer exactly. More equal. You earned that.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ARINA — GREETINGS
// ─────────────────────────────────────────────────────────────────────────────

var ArinaGreeting = []string{
	"'Oh! You're here!' Arina says this with the energy of someone who has been looking forward to a thing and is pleased the thing has arrived. The thing is you.",
	"Arina looks up from a worktable covered in notes, materials, and something faintly glowing, and her face does the thing it does — open, interested, ready to talk very fast.",
	"'Perfect timing,' Arina says, in the tone of someone for whom most timings are perfect because everything is interesting. 'I was just thinking about—' She stops. 'Actually, what do you need?'",
	"Arina has three things she's in the middle of and immediately sets all of them down to give you her full attention, which is considerable.",
	"'You came back!' Arina says, as if there was any question. In her experience there sometimes isn't.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ARINA — ITEM IDENTIFICATION
// ─────────────────────────────────────────────────────────────────────────────

var ArinaIdentify = []string{
	"Arina picks up the item with both hands and goes immediately quiet, which is the most alarming thing she does. Then: 'Okay. Okay, this is — this is actually really interesting.' She tells you everything.",
	"'Can I — I'm not going to touch it, I just—' She touches it. 'Okay so here's what it does.'",
	"Arina holds the item up to the light, turns it twice, says something under her breath that might be an incantation or just enthusiasm, and then begins a very efficient explanation.",
	"'Oh I know what this is.' The words land quickly, confidently, correctly. Arina has seen a lot of magic items and she remembers all of them.",
	"Arina goes still in the specific way she goes still when magic is doing something she finds genuinely surprising. 'That's — huh. Okay. That's new. Let me—' The identification follows, along with three questions she has that you're under no obligation to answer.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ARINA — SKILL CHECK SUCCESS
// ─────────────────────────────────────────────────────────────────────────────

var ArinaArcanaSuccess = []string{
	"'Yes! Exactly right — you know what, most people don't catch that.' Arina opens her notes and shows you something that solves the problem neatly.",
	"Arina's face does the thing where she's genuinely pleased that you got it. 'Okay so since you clearly know what you're doing—' She gives you the recipe.",
	"'That check was excellent,' Arina says, which from her means the knowledge was real and not a guess. She gives you the full information without making you ask for pieces of it.",
}

var ArinaSkillFail = []string{
	"'Oh — no, that's not quite—' Arina seems genuinely sorry about this. 'Come back and we'll try again. Or bring the right materials. Or both.'",
	"Arina makes a small face that isn't unkind. 'Not quite. You're close though! The concept is right, the specifics just need—' She trails off. 'Anyway. Come back.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// ARINA — CRAFTING
// ─────────────────────────────────────────────────────────────────────────────

var ArinaCraftStart = []string{
	"'Okay! I have everything I need, you have everything you brought, let's see what we can make.' Arina's work mode is focused and fast.",
	"Arina spreads the materials across the table and looks at them the way a musician looks at an instrument — familiarity and anticipation. 'This is the good part,' she says.",
	"'I've been thinking about this combination for a while actually,' Arina says, and begins immediately.",
}

var ArinaCraftComplete = []string{
	"Arina presents the finished item with quiet satisfaction — the specific satisfaction of a thing made well. 'There. That's what those materials wanted to be.'",
	"'Done!' Arina says, and she sounds pleased in the way she sounds when something worked exactly as planned. Which it did. 'Take good care of it.'",
	"The item is finished and Arina holds it out to you. 'Made right, with the right materials. That's how these things last.' She means it literally and figuratively.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ARINA — QUEST ASSIGNMENT
// ─────────────────────────────────────────────────────────────────────────────

var ArinaQuestGive = []string{
	"'Okay so I have a thing,' Arina says, which is how she starts sentences that are actually requests. 'You don't have to. But if you're going into that zone anyway—'",
	"'I need a sample,' Arina says. 'Specifically—' She describes it in precise detail. The precision is its own kind of excitement.",
	"Arina looks at her notes, looks at you, looks at her notes again. 'I think you're the right person to ask about this. Can I ask you about this?'",
}

var ArinaQuestComplete = []string{
	"Arina sees what you've brought and makes a sound that is entirely undignified and entirely genuine. 'You actually got it. Can I — thank you. This is going to—' She's already analyzing it.",
	"'This is perfect,' Arina says, and she means it in the exact scientific sense — perfect, the right amount, the right quality. 'The reward is on the table. Thank you.'",
	"Arina takes the materials and immediately begins making notes, which means she's happy, which means you did it right. The reward is handled efficiently because her hands are busy.",
}

var ArinaFavoriteUnlocked = []string{
	"Arina puts down her notes. Actually puts them down. 'You've done a lot for this workshop,' she says. 'I want to show you what I'm actually working on.' The advanced recipe list opens.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PETE BOT — BROADCAST LINES
// ─────────────────────────────────────────────────────────────────────────────

var PeteZoneUnlock = []string{
	"📣 Pete: {zone_name} is now accessible. Community milestone reached. First expeditions can begin immediately.",
	"📣 Pete: New zone available — {zone_name} (Tier {tier}, recommended Level {level_min}+). Details via !expedition start {zone_id}.",
}

var PeteTier5Complete = []string{
	"📣 Pete: {player_name} has completed {zone_name}. Expedition duration: {days} days. Boss defeated. Legendary loot confirmed. I have noted this one.",
	"📣 Pete: {zone_name} cleared by {player_name} on Day {day}. The {boss_name} is down. Community achievement recorded.",
}

var PeteStreakMilestone = []string{
	"📣 Pete: {player_name} is on a {streak}-win arena streak. Current status: {title}. The leaderboard has updated.",
	"📣 Pete: Arena milestone — {player_name} reaches streak {streak}. Previous record in this community: {record}.",
}

var PeteCommunityBoost = []string{
	"📣 Pete: Community activity this week has been strong. TwinBee's mood is elevated. Dungeon drop rates and milestone rewards have been adjusted upward for 48 hours.",
	"📣 Pete: I am in a good mood. Reasons cited: community engagement, a particularly impressive nat 20, and general satisfaction with how things have been going. Enhanced rewards active through Sunday.",
}

var PeteMaintenance = []string{
	"📣 Pete: Brief maintenance window incoming — approximately {duration}. Active expeditions are paused and will resume from last checkpoint. Supplies will not deplete during downtime.",
	"📣 Pete: Maintenance complete. All systems restored. Expedition timers have been adjusted for downtime. Apologies for the interruption.",
}

var PeteExpeditionBulletin = []string{
	"📣 Pete: Expedition update — {count} active expeditions in progress across {zones}. Longest running: Day {max_day}. I am busy.",
	"📣 Pete: Current expedition activity: {count} players in the field. The dungeon is occupied. Good.",
}

var PetePatchNotes = []string{
	"📣 Pete: Update deployed. Changes: {summary}. Full notes available on request. I have been briefed.",
}

var PeteMortgageRate = []string{
	"📣 Pete: Weekly rate update. FRED ARM rate: {rate}%. Effective GogoBee mortgage rate (Thom Krooke margin included): {effective}%. Payments process Sunday.",
}
