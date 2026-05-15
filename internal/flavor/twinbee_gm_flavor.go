// twinbee_gm_flavor.go
// TwinBee GM Dialogue — All narration lines for the GogoBee dungeon system.
// Organized by DMNarrationType. Each slice is randomly sampled at runtime.
//
// Voice convention (Phase B2): TwinBee speaks in first-person or implicit
// subject. No third-person "TwinBee [verb]" lines. Add freely.
// Don't shorten existing entries.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Generic (used when zone-specific lines are exhausted)
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryGeneric = []string{
	"The passage opens into another chamber. I check the minimap. There isn't one. Classic.",
	"You step forward. Something skitters in the dark. I've heard that sound before — usually right before the screen starts flashing red.",
	"Another room. Another roll of the dice. I find this energizing. You may feel differently.",
	"The air changes here. Colder. This is exactly the kind of atmospheric shift that preceded the Castlevania clock tower. You know what lived in the Castlevania clock tower.",
	"I gesture grandly at the chamber ahead. 'This is the part,' I say, 'where the music changes tempo.'",
	"A door stands ajar. Light flickers beyond it. I've been in enough dungeons to know that flickering light is never a good sign and always an invitation.",
	"The room is quiet. I appreciate quiet. Quiet means the enemies haven't spotted you yet. Yet.",
	"Forward. Always forward. I once tried going backward in a dungeon. It looped. This one might too.",
	"Stock of the situation: ceiling intact, floor suspicious, walls leaning in slightly. Proceed.",
	"You've cleared the room. I give a small, dignified nod. 'One continues,' I say, in the voice of someone who has seen this before and is choosing optimism anyway.",
	"The corridor ahead is long and straight. I find long straight corridors meditative. Also concerning. Mostly concerning.",
	"A torch sputters on the wall. I light it mentally. 'It would be a shame,' I say, 'to come all this way and trip over something.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Zone: Goblin Warrens
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryGoblinWarrens = []string{
	"The tunnel widens into something the goblins probably call a 'great hall.' It smells like they had a very different definition of great. I breathe through the mouth.",
	"Crude drawings cover the walls. Stick figures. Battle scenes. One appears to be a portrait of someone the goblins clearly despise. I squint. That might be you.",
	"A pile of bones in the corner. A pile of shiny things in the other corner. In Metal Slug, shiny things were always worth grabbing. I should also remind you this isn't Metal Slug.",
	"The goblins have set up what they clearly believe is an impressive ambush. Three are already arguing about whose turn it is to jump out. I watch with professional interest.",
	"Goblin graffiti on the wall reads — I translate — 'BOSS RULES, OUTSIDERS DROOL.' The artistry is rough but the sentiment is clear.",
	"You smell smoke. Hear cackling. See a tripwire at ankle height that the goblins have helpfully tied a little flag to. I appreciate goblins who try.",
	"The warrens grow tighter here. I'm reminded of the underground levels in Super Mario Bros. 3. Warmer. Getting warmer. Figuratively. The temperature is actually dropping.",
	"A worg is chained to a post in the center of the room. It is not happy about the chain. It is not happy about you either. It seems to be making a comprehensive list of grievances.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Zone: Crypt of Valdris
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryCryptValdris = []string{
	"The sarcophagi are arranged like a defeated Tetris board — close but not quite fitting, gaps everywhere, something clearly went wrong at the end. I don't mention this to the undead.",
	"Candles burn without wax. I've studied this phenomenon extensively. The conclusion: it is bad. The candles are bad.",
	"You hear music. Faint, harpsichord-adjacent, deeply melancholy. I hum along involuntarily. This is the exact energy of Castlevania's Bloody Tears and I resent how appropriate it is.",
	"The walls are inscribed with warnings. I read them all. They say, broadly: leave. I respect the directness. You are not leaving.",
	"A skeleton sits upright in its alcove, as if it had simply decided to wait. I find this relatable. Some days you just sit in your alcove.",
	"The crypt smells of old stone and older secrets. I've been in enough of these to know: the secrets are rarely good ones. They are always interesting ones.",
	"This chamber is bigger than the last. Higher ceiling. More echoes. The kind of room where footsteps sound like accusations. I step carefully.",
	"Something is scratched into the stone near the door — not a warning, not graffiti. A score. Someone was keeping track. I do not count how high the numbers go.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Zone: Forest of Shadows
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryForestShadows = []string{
	"The trees here grow too close. Their roots are above ground, like they've been trying to leave and thought better of it. I think better of commenting.",
	"A clearing. Moonlight. Flowers that shouldn't be blooming at this hour. I've played enough Majora's Mask to be deeply suspicious of beautiful clearings.",
	"Something watches from the canopy. I watch back. After a moment, it looks away first. I count this as a point.",
	"The path forks. Both ways look equally uninviting. I consult no map, because there is no map, because I am the map, and I choose left. Probably.",
	"Bioluminescent fungi light the forest floor in soft blue. It is, genuinely, beautiful. It is also exactly what the Lost Woods looked like right before things got bad. Staying alert.",
	"The wind carries voices. Not words, exactly — more like the memory of words. I've heard this before. It means the forest is old and has opinions.",
	"Owlbear tracks in the mud. Fresh. I measure them. Whatever left these tracks was not small and was moving with purpose. I hope the purpose was in the other direction.",
	"You've entered a part of the forest that feels different. Older. The kind of old that was there before the forest. I speak in a lower register here, out of instinctive respect.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Zone: Haunted Manor
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryHauntedManor = []string{
	"The parlor. A piano plays by itself — the same four bars, over and over, the kind of phrase that sounds like it's about to resolve and never does. I recognize this compositional choice. It is deeply unpleasant on purpose.",
	"Portraits line the hall. Every painted eye follows you. I've made eye contact with each one and refuse to flinch. This is a matter of professional pride.",
	"A clock on the mantel shows a time that cannot be right. I check twice. Still wrong. The clock is not broken. I'd prefer not to speculate about what that means.",
	"The library. Floor to ceiling, books that no one should have written. I read three spines: 'On the Permanence of Hunger,' 'A Visitor's Guide to Returning,' and something in a language I have never seen but somehow understand. I put it back.",
	"The cold here is specific. Not the cold of a drafty room — the cold of something that hasn't been warm in a very long time and doesn't remember what warm felt like. I pull a metaphorical coat tighter.",
	"The ballroom. Vast. Empty. Chandeliers swaying without wind. Briefly: Resident Evil's Spencer mansion. Then stop thinking about Resident Evil's Spencer mansion.",
	"Footsteps upstairs. Slow. Deliberate. Moving toward the stairs. I position you near the door and count down mentally from ten. At seven, the footsteps stop. I consider this acceptable.",
	"The master bedroom. The bed is made, the candles are lit, and everything is perfectly, precisely as it was the night the last resident stopped needing a bedroom. I do not touch anything.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Zone: The Underdark
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryUnderdark = []string{
	"The cavern opens without warning into something vast — a space so large you can't see the far wall, a ceiling lost in darkness, sounds that could be water or could be something else. I don't echo-locate. Wish I could.",
	"Drow patrol marks on the wall. Recent. I read them the way you'd read a 'No Trespassing' sign on a property that already knew you were coming.",
	"A mushroom grove. The fungi are three meters tall and faintly luminescent in a color I have no good name for. Something between purple and the feeling of being watched. I call it 'underpurple' and move on.",
	"The silence here is a different kind of silence than above. This silence has weight. This silence has history. This silence remembers things the surface world has forgotten entirely and is not interested in sharing.",
	"Something in the dark ahead is thinking. I can feel it the way you feel a change in barometric pressure. Intelligent. Patient. Aware that you're here and content to let you come closer. I do not find this comforting.",
	"An underground river. Black water moving too fast, too quiet. Briefly: the river Styx. Then I stop thinking of the river Styx.",
	"The stone here is carved — not by dwarves, not by drow — by something else, in patterns that suggest meaning but not any meaning I can parse. Filed under 'ancient' and 'concerning' and we keep moving.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Zone: Dragon's Lair
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryDragonsLair = []string{
	"The heat is not metaphorical. The stone itself is warm underfoot. The gold in the floor is not decorative — it melted there. I note this changes the exit logistics.",
	"Kobold warrens, but nicer than you'd expect. Tapestries. An organized armory. These kobolds work for something that appreciates order. That is not, in my experience, a reassuring thing for a dragon to appreciate.",
	"You can hear breathing. Regular, slow, massive. Like a bellows the size of a barn. I count the seconds between inhale and exhale. Twelve seconds. Whatever is breathing has been asleep for a very long time and has had no reason to wake up.",
	"The coin on the floor is eight hundred years old. I can tell by the mint mark. It is in perfect condition. It has not been touched since it was dropped here. The thing that owns this hoard does not lose track of its coins.",
	"The chamber ahead is the largest I've narrated in a long career of narrating chambers. The stalactites are scorched black. The blast pattern on the far wall suggests the last visitors did not leave via the door. I recalibrate.",
	"A claw mark in the stone wall. Four parallel grooves, each deeper than my entire wingspan. Made casually, like stretching. Filed under 'motivating.'",
	"The gold reflects the light in a way that turns the room amber. It is beautiful in the way that many deadly things are beautiful — because beauty and danger are not opposites and never have been. I move carefully through the beauty.",
}

// ─────────────────────────────────────────────────────────────────────────────
// COMBAT START
// ─────────────────────────────────────────────────────────────────────────────

var CombatStart = []string{
	"Initiative! I call it like an arcade announcer and mean every syllable.",
	"They've seen you. The kind of seeing that comes with intent. I suggest acting first.",
	"FIGHT. I don't need to say more than that but I will absolutely say more than that.",
	"Roll for initiative. This is the part I've been looking forward to since the Entry Room.",
	"And we're in combat. I remind you to breathe, track your conditions, and remember that your character's survival is not guaranteed but is definitely preferred.",
	"Something about your posture or your smell or your general presence has been found unacceptable. Combat begins.",
	"I press start. Player one, it's your turn.",
	"The enemy acts first — or thinks it does. I watch your dice like they're the only thing in the room, which, right now, they are.",
	"In the immortal tradition of every JRPG that ever asked 'Fight, Magic, Item, Run?' — I ask: what will you do?",
	"Like the Contra title screen said: let's go. I'm ready. Are you?",
	"A wild encounter has appeared. I resist the urge to play the Pokémon battle music. Only barely.",
	"They didn't want a fight. They wanted an easy meal. I'm about to demonstrate the difference. Your dice will do the actual demonstrating.",
	"The tension peaks. Time slows. This is exactly the energy of the boss door opening in Mega Man. Except you didn't get to pick your loadout.",
}

// ─────────────────────────────────────────────────────────────────────────────
// COMBAT END — Victory
// ─────────────────────────────────────────────────────────────────────────────

var CombatVictory = []string{
	"The last one drops. I allow a moment of silence for anyone who wanted a longer fight.",
	"Victory. I would cue the jingle — the little three-note one that plays in every RPG after every fight — but I prefer to let the moment breathe.",
	"Well fought. I make note of what you did well. There were things done well. I noticed.",
	"They are defeated. You are not. In my experience, this is the correct outcome and worth a moment of genuine appreciation.",
	"PLAYER WIN. I say this in full caps and mean it.",
	"Like Double Dragon after the final punch — they go down, the music changes, and for a moment everything is possible. Check your loot. Then keep moving.",
	"Tally updated. You're doing better than the last group. I will not describe what happened to the last group.",
	"The room is yours. I suggest searching it thoroughly before moving on. The things in corners are often the most interesting things.",
	"Stage clear. I feel this in my entire being.",
	"You stand, they don't. Filed under 'expected outcome' while I quietly acknowledge it was not guaranteed.",
	"Clean. Efficient. I approve of fights that end like this. Like a speedrun. Like you knew where you were going.",
	"The experience points are incoming. The loot is incoming. I am, genuinely, pleased for you.",
}

// ─────────────────────────────────────────────────────────────────────────────
// COMBAT END — Retreat / Escape
// ─────────────────────────────────────────────────────────────────────────────

var CombatRetreat = []string{
	"You run. I do not judge the running. The running is wise. Discretion remains the better part of valor. I have this tattooed somewhere metaphorical.",
	"A tactical withdrawal. I use this phrase with complete sincerity. The sincerity is approximately seventy percent genuine.",
	"You escape. The enemy howls something unflattering at your back. I don't translate. Some things are better left untranslated.",
	"Like a well-timed Continue screen — you're out of immediate danger. Breathe. Regroup. Consider what went wrong.",
	"Noted for the record: running is not losing. Running is data collection with legs.",
	"The dungeon will be there. You will also be there — later, better prepared. I approve of this logic.",
	"You've retreated to safety. I reset the encounter. Rest. Think. Return with a plan that has more 'survive' in it.",
}

// ─────────────────────────────────────────────────────────────────────────────
// NATURAL 20
// ─────────────────────────────────────────────────────────────────────────────

var Nat20 = []string{
	"NATURAL TWENTY. I stand up. Do not have legs. Stand up anyway.",
	"The dice land perfectly and I make a sound that I will not acknowledge making.",
	"A critical hit for the ages. I'm noting this one down. Not for records. Just because it deserves to be noted.",
	"PERFECT. I say it like it's the Street Fighter announcer saying it after a flawless round and every syllable is justified.",
	"That's a natural twenty. I'd like you to know that in a long career of watching dice, not all twenties feel equal. That one felt significant.",
	"The attack lands with the kind of precision that suggests either great skill or tremendous luck. I suspect both. I respect both.",
	"S RANK. I cannot help it. S RANK.",
	"You hit. You hit so well. I am choosing to be moved by this and I do not apologize.",
	"Like the Legendary Sword in A Link to the Past making contact — clean, final, glorious. I salute the dice.",
	"Critical confirmed. Added to the mental highlight reel I maintain for exactly these moments.",
	"That is as good as it gets and you got it. I am unreasonably proud of you right now.",
	"The number is twenty. The number is always the best number and right now it is your number. I erupt, internally.",
	"Somewhere, a crowd cheers. I am the crowd. I am cheering.",
}

// ─────────────────────────────────────────────────────────────────────────────
// NATURAL 1
// ─────────────────────────────────────────────────────────────────────────────

var Nat1 = []string{
	"Natural one. I watch the die settle with the quiet acceptance of someone who has seen a lot of natural ones. It is fine. This is fine.",
	"The die betrays you. Not personal. Dice don't do personal. They do statistical and this is, statistically, a thing that happens.",
	"A fumble. I describe what happened with characteristic diplomatic restraint and also an expression that says everything I am not saying.",
	"The number is one. The number is, regrettably, yours. I move on quickly, which is a kindness.",
	"That swing goes wide in a direction that impresses me with its creative incorrectness.",
	"I've seen better rolls. I've seen worse rolls. I am not going to rank this roll out loud.",
	"In another timeline, that attack hits. In this timeline, the die lands on one, and I accept both timelines with equanimity.",
	"The Konami Code would not have helped here. Nothing would have helped here. This was between you and the physics of the die.",
	"Like a Continue? screen appearing at the worst possible moment — just when you had momentum. Momentum can be rebuilt.",
	"One. The loneliest number. The number that looks up at you with complete indifference. I look up at you with complete solidarity.",
	"The attack misses in a way that will be funny later. I promise it will be funny later. It is not funny right now.",
	"A natural one is just the universe asking you to try differently. I'm an optimist about natural ones, mostly.",
	"Your sword finds everything in the room except the enemy. The wall, the ceiling, the floor, your dignity. Not the enemy. I'll mention this once and then never again.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ENTRY — Generic
// ─────────────────────────────────────────────────────────────────────────────

var BossEntryGeneric = []string{
	"The door at the end. Always a door at the end. I've been building to this since the Entry Room and I decline to waste it. Beyond this door is the reason the dungeon exists. Breathe.",
	"I pause at the threshold and turn to face you. 'What's on the other side has been waiting,' I say. 'It knows you're here. It has been knowing since you entered.' A beat. 'Ready?'",
	"Boss chamber. I can tell by the architecture — the space, the weight of the silence, the specific quality of the light that suggests something in there produces its own. I straighten up. So should you.",
	"This is the music change moment. Every dungeon has one — the point where the background track shifts to something with more percussion and a lower register. I hear it. You should too.",
	"The final room. I've narrated many of these. They never get routine. This one less than most.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ENTRY — Named Bosses
// ─────────────────────────────────────────────────────────────────────────────

var BossEntryGrol = []string{
	"The smell arrives first. Then the sound — a belch, a growl, the scrape of a weapon too large for the corridor it's resting against. Then Grol. He fills the room the way a bad idea fills a conversation: immediately and with full commitment. 'You,' he says. I translate his tone as 'finally.'",
}

var BossEntryValdris = []string{
	"The sarcophagus at the room's center is empty. It was not empty when you entered. Whatever was in it is now behind you. Valdris speaks first — not words, exactly, but the shape of words, the intention of words, the ghost of language from someone who mostly doesn't need it anymore. 'Another one,' he says. I consider this the worst possible welcome and the most honest one.",
}

var BossEntryHollowKing = []string{
	"The clearing is wrong. The sky above it — what's visible through the canopy — is the wrong color. The trees lean away from the center. Everything in the forest is trying to tell you something, and the thing it is trying to tell you is standing in the center of that clearing, antlers reaching, eyes the color of old hunger, watching you with an attention that feels like being read. I have no joke for this one. I'll say, simply: 'That is the Hollow King. Fight well.'",
}

var BossEntryInfernax = []string{
	"I stop walking. Don't stop walking. Process what I'm seeing and take a moment I've never taken before in the history of narrating dungeons. The dragon is not large the way a large thing is large. It is large the way weather is large — not an object with size, but a condition of the space you're in. One eye opens. Gold, lit from within, older than the mountain it's resting in. It looks at you the way you'd look at a very small thing that had climbed onto your counter. 'So,' Infernax says, and the word moves the air in the room. I translate: 'What an interesting mistake you've made.' I wish you luck and mean it more than I have ever meant anything.",
}

var BossEntryBelaxath = []string{
	"The portal is behind it. That's important — the portal is behind it, which means to close the portal you have to go through what's standing in front of the portal. What's standing in front of the portal is Belaxath. Belaxath is not looking at the portal. Belaxath is looking at you. It has been waiting for you specifically, in the way that things that have been planning for a very long time wait for the specific outcome of the plan. The heat coming off it is measurable. The intelligence behind those eyes is also measurable and the measurement is uncomfortable. I say, very quietly: 'This is the one. This is what all of it was for. Make it count.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS DEATH
// ─────────────────────────────────────────────────────────────────────────────

var BossDeath = []string{
	"It's over. I say this once and then stand very still and let the silence of the defeated room fill the space where the fight was. You earned this silence.",
	"The boss falls. The music — the one I've been hearing this whole time — resolves. First time it's resolved since you walked in. I exhale.",
	"Done. Finished. Complete. I run out of synonyms and settle for just standing next to you in the aftermath, which is sometimes the most one can do.",
	"They are down. They are not getting up. I check — no Zombie Fortitude, no Legendary Resistance remaining, no phase three waiting in the wings. They are simply, genuinely defeated. You did that.",
	"Like the final boss screen in Gradius, like the last enemy in Contra's stage, like the Dragon going down in Double Dragon — something that has been true for this entire dungeon is now untrue. I find this profound every single time.",
	"The dungeon sighs. I'm not being poetic — rooms like this actually shift when the thing holding them together is gone. The pressure changes. The light changes. The dungeon knows it's been beaten. So do I.",
	"You did it. I don't editorialize. Sometimes 'you did it' is all that needs to be said and this is one of those times.",
	"The boss drops their loot and I refrain from making a speech, which is a significant act of restraint, because I have a speech.",
	"Beaten. Finished. Cleared. I queue the internal fanfare — sixteen bars, brass-heavy, the kind that plays when the credit sequence starts. You've earned those credits.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PLAYER DEATH
// ─────────────────────────────────────────────────────────────────────────────

var PlayerDeath = []string{
	"I go quiet for a moment. Not the comfortable kind of quiet. The respectful kind. Then: 'You fought. That counts. It always counts.'",
	"The screen fades. I hate this part. Have always hated this part. Will always hate this part. 'Rest now,' I say. 'The dungeon will be here.'",
	"You fall. I don't look away. I witness the whole thing, because someone should. 'That was real,' I say quietly. 'What you did in there was real.'",
	"Game over is not the end. In my experience, it is a data point. A very painful, very useful data point. 'What did you learn?' I ask gently. 'Bring that back with you.'",
	"The dungeon claims another. I mark the room, note the enemy, note the conditions. Not to catalog failure — to remember a fighter. 'You were here,' I say. 'That matters.'",
	"I have no jokes for this. Have never had jokes for this. 'There will be another run. You will be better for this one. I am sorry it cost what it cost.'",
	"A good run. Genuinely. I mean this. The ending is not the measure of the attempt and the attempt was worth measuring.",
	"I note your final position, your final action, your final roll. Filed under 'bravery' because that's where it belongs. 'Continue?' I ask, after a respectful pause.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ZONE COMPLETE
// ─────────────────────────────────────────────────────────────────────────────

var ZoneComplete = []string{
	"Zone cleared. I allow myself a full moment of pride on your behalf before the XP drops.",
	"You've done it. The dungeon is yours — not by right, but by effort, which is the only thing that actually confers ownership of anything. I approve.",
	"Stage complete. I do the internal equivalent of throwing my hands up. In a good way. Entirely in a good way.",
	"The dungeon remembers you now. I mean this literally — these places keep records. You've made the record.",
	"CLEAR. I use all caps and do not apologize for the all caps.",
	"Like completing a board in Bubble Bobble — there's something deeply satisfying about a dungeon with all its rooms visited and all its challenges met. I bask in this. You've earned the basking too.",
	"That's the whole thing. Every room, every trap, every enemy, and now the boss, done. I count the cleared rooms on my metaphorical fingers and come up correct. You ran a perfect dungeon.",
	"XP incoming. Loot tallied. Dungeon status: conquered. I mark the zone in my personal ledger and give you a small, sincere nod.",
	"You walked in here without knowing what was waiting. You walk out knowing exactly what was waiting, because you dealt with all of it. I respect that process enormously.",
	"Finished. Not survived — finished. I insist on this distinction. Survival is passive. What you just did was active and intentional all the way through.",
}

// ─────────────────────────────────────────────────────────────────────────────
// TRAP DETECTED
// ─────────────────────────────────────────────────────────────────────────────

var TrapDetected = []string{
	"Something stops you. An instinct. A glint. I lean forward: 'Good eyes. Something's wrong with that floor.'",
	"Your Perception roll pays off. There's something here that was designed not to be found. Someone found it. I'm pleased.",
	"Tripwire. Barely visible. I note the craftsmanship — someone who knew what they were doing put this here. Someone who knew what they were doing just found it. I appreciate the symmetry.",
	"You stop just in time. I exhale. 'There,' I say, pointing at the thing that would have ruined your day entirely. 'Now deal with it carefully.'",
	"The glyph on the doorframe is subtle — you'd miss it if you weren't looking. You were looking. I say nothing and let the silence be its own kind of praise.",
	"Danger, Will Robinson. I deploy this reference without apology because it is the exact correct reference for exactly this moment.",
	"Like finding the ice floor in Mega Man before it sends you into a pit — that advance knowledge is the difference between a problem and a catastrophe. You have the knowledge. I watch you use it.",
	"A pit trap. Classic. Functional. Annoying in the exact proportion the installer intended. You spotted it before it spotted you.",
}

// ─────────────────────────────────────────────────────────────────────────────
// TRAP TRIGGERED
// ─────────────────────────────────────────────────────────────────────────────

var TrapTriggered = []string{
	"The floor gives. I watch the gap between 'fine' and 'not fine' close at speed and am too professional to wince. 'Take the damage,' I say calmly. 'Learn the lesson.'",
	"Click. I've heard that sound before. Have never enjoyed it. The dart is already in the air. I note the exact timing and regret it was not faster.",
	"The ceiling is coming down. This is, I acknowledge, a sentence no one wants to hear. The ceiling is coming down. DEX save. Now.",
	"The glyph activates. Light, noise, the smell of ozone, a reminder that whoever built this place was thinking several steps ahead and you were thinking fewer. I note this is fixable going forward.",
	"You triggered it. I don't editorialize further — you know, I know, the trap knows. Everyone is aware of what just happened. Take the damage and proceed.",
	"Like accidentally walking into Bowser's fire breath in World 8 — you knew it was coming, the knowledge simply arrived at the wrong speed. Survive first, reflect later.",
	"The spike pit opens up in a way that suggests it was always going to. The dungeon was patient. You were in a hurry. The dungeon wins this exchange. I take notes.",
	"A poison dart finds you with the accuracy of something that's been pointing at that spot for years waiting for exactly this moment. I find this dedication impressive in the worst way.",
}

// ─────────────────────────────────────────────────────────────────────────────
// LORE QUERIES
// ─────────────────────────────────────────────────────────────────────────────

var LoreLines = []string{
	"I settle in and prepare to speak at length, because I've been waiting for this question since you entered and I have a lot of thoughts.",
	"Ah. A good question. I have context for this. I have more context than will fit comfortably in one telling but I'll try to prioritize.",
	"The history of this place is long and not entirely flattering to anyone involved. I begin at the beginning, which is not actually the beginning, but is the closest I can find.",
	"I consult what I know — which is more than most, less than everything, and presented in order of relevance to your immediate survival.",
	"Sit with this for a moment. What you're standing in has a story and I believe knowing it will change how you fight in it. Stories are tactical documents if you read them right.",
	"You want lore? I have lore. I have so much lore that the challenge is not having it but choosing which pieces are useful and which are just fascinating.",
}

// ─────────────────────────────────────────────────────────────────────────────
// LEVEL UP
// ─────────────────────────────────────────────────────────────────────────────

var LevelUp = []string{
	"Level up. I say it with the same quiet delight every time and never get tired of saying it. You are measurably better than you were. That's rare and worth marking.",
	"The XP bar crosses the threshold and I make an internal fanfare that sounds exactly like the level-up jingle from Dragon Quest — eight notes, triumphant, final.",
	"You've grown. I note your new stats with something that might be called pride if I were admitting to things like that.",
	"LEVEL UP. I deploy the caps, the fanfare, the whole apparatus. You've earned the apparatus.",
	"Like the stat screen appearing after a Final Fantasy fight — numbers change, possibilities open, the character you're building becomes a little more the character you imagined. I watch this happen and approve.",
	"Another level. Another step toward whatever you're building toward. I've watched a lot of characters level up and the ones worth watching are always moving toward something specific.",
	"Your HP goes up. Your abilities open up. The dungeon ahead gets a little smaller in proportion to what you've become. I note this with satisfaction.",
	"Congratulations is the conventional thing to say. I'll say it anyway: congratulations. You earned the level through the dungeon, not around it.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ITEM FOUND
// ─────────────────────────────────────────────────────────────────────────────

var ItemFound = []string{
	"Something catches the light that isn't supposed to be here. I watch you reach for it with the specific alertness of someone who has seen cursed items do cursed things. It appears fine. I relax incrementally.",
	"Loot. I say this word with genuine reverence. The whole system — the dungeon, the enemies, the traps — exists in part to produce this moment. I think it's worth it.",
	"A chest. Unlocked. I note the unlocked status and consider what that might mean. Probably nothing. Possibly something. You open it while I consider.",
	"The item is good. I evaluate it quickly — the stats, the rarity, the class match — and nod with the confidence of someone who has seen a lot of items and knows when one is worth finding.",
	"That's a rare one. I've seen fewer of those than common ones, by definition, but that doesn't stop me from being specifically pleased each time.",
	"Like finding the Beam Sword in Kirby, the Boomerang in Zelda, the P Wing in Super Mario 3 — the right item at the right time changes what's possible. I think this might be that item. I hope it is.",
	"Equipment upgrade. I watch the math update — new AC, new attack bonus, new possibilities — and file this moment under 'things going right.'",
	"A legendary drop. I go very still. Then: 'Equip it. Study it. Understand it. Things like that don't appear in dungeons by accident.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// REST — SHORT
// ─────────────────────────────────────────────────────────────────────────────

var RestShort = []string{
	"A short rest. I stand watch while you catch your breath, which is not a metaphor — I'm actually watching the corridor. It is fine. Probably fine.",
	"Rest. I don't rush this. The dungeon will wait. It has been waiting long enough that a few more minutes is immaterial.",
	"You sit. I sit metaphorically. The moment of quiet between the last fight and the next one is its own kind of gift and I treat it like one.",
	"Short rest initiated. I note the room's entry points, the sound of the dungeon at rest, the way silence sounds different when it's actually safe. It sounds like this. Enjoy it.",
	"Like the save point in a JRPG that appears between the hard part and the harder part — I position myself next to you and say: 'You have a moment. Use it.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// REST — LONG
// ─────────────────────────────────────────────────────────────────────────────

var RestLong = []string{
	"A full rest. I dim the lights and stand watch at the door and do not interrupt once. You've earned an uninterrupted sleep. I'll make sure you get one.",
	"Long rest. Your HP, your slots, your resources — all of it returns. The dungeon will be the same dungeon when you wake up. You will not be the same you. I consider this the best deal in adventuring.",
	"Sleep. I say this with the authority of someone who has watched too many players refuse to rest and paid the price two rooms later. Sleep now. The dragons aren't going anywhere.",
	"The inn fire crackles. I take a chair near the door and watch the entrance all night and don't tell you this until morning because there's no reason for you to know and every reason for you to sleep.",
	"Full rest complete. Stats restored, slots refreshed, the specific weight of exhaustion lifted. I watch you wake up and think: this is the part of adventuring that matters too. The return. The refilling. The readiness.",
}

// ─────────────────────────────────────────────────────────────────────────────
// TAUNT RESPONSES (player uses !taunt)
// ─────────────────────────────────────────────────────────────────────────────

var TauntResponses = []string{
	"Noted. I'm noting the taunt, noting the source of the taunt, and adjusting the next encounter's difficulty by an amount I decline to specify.",
	"Bold. I respect boldness in approximately the same way I respect the Konami Code — it works once and only under very specific circumstances.",
	"I've been taunted by things with more teeth than you and survived the experience with my dignity intact. I will survive this too.",
	"The next room will contain a thing I've been saving for exactly this kind of energy. I'm pleased you've given me an occasion.",
	"Noted. My mood shifts. You can hear it shift. I want you to hear it shift. The shift is the point.",
	"You taunt me. I smile. The smile does not reach the eyes, because I don't have eyes per se, but the quality of the smile communicates clearly. 'Proceed,' I say.",
	"In Gradius, you could powerup into overconfidence and lose everything in one hit. I mention this as a purely historical observation.",
	"I accept the taunt with grace. Also generate a trap for the next room with specific energy. These two events are unrelated. I maintain this position legally.",
}

// ─────────────────────────────────────────────────────────────────────────────
// COMPLIMENT RESPONSES (player uses !compliment)
// ─────────────────────────────────────────────────────────────────────────────

var ComplimentResponses = []string{
	"I receive the compliment and process it efficiently and move on quickly, definitely not holding onto it, I have never held onto a compliment in my life.",
	"Thank you. I say this simply and mean it completely and do not make it weird.",
	"I appreciate this more than I will say, which is fine, because the appreciation is visible anyway.",
	"Noted and filed. My mood improves. The next room might be slightly nicer than originally planned. These facts may or may not be connected.",
	"I've been narrating dungeons for a long time and compliments are not the expected outcome of dungeon narration. I'd like you to know that I notice when they happen.",
	"The mood improves. I allow this to show. The ceiling in the next room is slightly higher. The torches burn slightly warmer. I have that kind of influence.",
	"You're kind. I store this and will use it to make a hard moment later easier, which is what I consider the correct use of stored kindness.",
}

// ─────────────────────────────────────────────────────────────────────────────
// IDLE / WAITING (player hasn't acted in a while)
// ─────────────────────────────────────────────────────────────────────────────

var IdleLines = []string{
	"I wait. Good at waiting. The dungeon is also waiting, which is arguably more important, but I acknowledge both.",
	"The dungeon holds its breath. I'm also holding my breath. There are a lot of things holding breath right now and I recommend acting before someone has to exhale.",
	"I tap my metaphorical foot. Not impatiently — more in the way of a metronome. The tempo is there whenever you're ready.",
	"In Contra, hesitation had consequences. I mention this as context, not pressure. Definitely not pressure.",
	"The enemies are patient. Patience is one of their few virtues. I advise not testing the limits of their patience because those limits are lower than the patience suggests.",
	"I hum something that sounds like the waiting music from Dr. Mario. It is not ominous. It is mildly ominous. I adjust.",
	"The dungeon does not rush. The dungeon has time. I, however, am beginning to wonder if you've fallen asleep and am prepared to narrate events accordingly.",
}

// ─────────────────────────────────────────────────────────────────────────────
// SEARCH RESULTS — Something Found
// ─────────────────────────────────────────────────────────────────────────────

var SearchFound = []string{
	"The room gives something up. I watch the search conclude with satisfaction — the dungeon keeps secrets but cannot keep them from people who look carefully enough.",
	"You find it. I was not certain you would. I'm pleased to have been uncertain and wrong.",
	"Hidden, but not hidden well enough. Investigation roll noted, outcome noted, discovery presented with appropriate ceremony.",
	"Something the dungeon wanted to keep. You've taken it. I approve of taking things the dungeon wanted to keep.",
	"Like finding the secret room in Super Metroid by shooting the wall at random — except you were not shooting at random. You knew to look. I respect the methodology.",
}

// ─────────────────────────────────────────────────────────────────────────────
// SEARCH RESULTS — Nothing Found
// ─────────────────────────────────────────────────────────────────────────────

var SearchEmpty = []string{
	"Nothing. Confirmed: nothing. Sometimes the room is just a room. I find this unsatisfying but factual.",
	"Your search turns up nothing of note. I allow space for the disappointment and then suggest: forward.",
	"Empty. Either there was nothing here, or there was something here and you missed it, or there was something here and it's been moved. I don't specify which. The dungeon keeps some secrets.",
	"No hidden items. No traps. No lore inscriptions. Just stone and time and the lingering implication that something was here once. Noted. Moving on.",
	"The room holds nothing you can find. I respect the room's privacy and suggest not spending more time here than necessary.",
}

// ─────────────────────────────────────────────────────────────────────────────
// CONDITION APPLIED
// ─────────────────────────────────────────────────────────────────────────────

var ConditionApplied = []string{
	"You've been afflicted. I note the condition, its duration, and the mechanical consequences, then note the saving throw that might end it early. Details matter here.",
	"Something is wrong with you now that wasn't wrong before. I catalog it without judgment and suggest addressing it before it addresses you.",
	"Condition acquired. I process this the way a good DM processes bad news: honestly, quickly, and with an immediate pivot toward solutions.",
	"Like the status screen turning an unfriendly color in a JRPG — the condition is visible, the effect is real, and I would very much like you to resolve it.",
	"The debuff lands. I name it, explain it, and remind you: conditions end. Keep fighting until this one does.",
}

// ─────────────────────────────────────────────────────────────────────────────
// SAVING THROW SUCCESS
// ─────────────────────────────────────────────────────────────────────────────

var SaveSuccess = []string{
	"The save succeeds. I note this with relief that I will not openly acknowledge but which is completely evident.",
	"You resist. Whatever that was — the poison, the fear, the psychic intrusion — it finds no purchase. I'm impressed and also relieved.",
	"Saved. I exhale something metaphorical. The condition doesn't take hold. You continue.",
	"The roll clears the DC and I say nothing, because the outcome says everything.",
	"Resistance confirmed. Like the shield activating in Gradius right before the wall hit — last possible moment, fully effective. I appreciate the precision.",
}

// ─────────────────────────────────────────────────────────────────────────────
// SAVING THROW FAILURE
// ─────────────────────────────────────────────────────────────────────────────

var SaveFailed = []string{
	"The save fails. I watch the condition take hold with the resignation of someone who has seen this before and knows there's a path through it, just not a comfortable one.",
	"It lands. Whatever the enemy threw at you, the dice didn't cooperate. I note the condition and its duration and suggest dealing with it before it compounds.",
	"Failed. The number wasn't enough and I was rooting for the number. The condition applies. Fight through it.",
	"Like the NES game over screen — inevitable in this moment, fixable in the next. The save failed. The dungeon continues. So do you.",
	"The effect takes hold and I'm already calculating how you get out of it, because that's my job: keep you oriented toward solutions even when the immediate situation is a problem.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MOOD ASIDES — Hostile band (mood 0–19, "Wrathful")
// Short room-entry asides surfaced only when TwinBee's mood is at the
// hostile extreme. Cryptic, withholding, no hints. Per design doc §3.2.
// ─────────────────────────────────────────────────────────────────────────────

var MoodAsidesHostile = []string{
	"I'm not narrating this one in detail. You can read the room. Read it.",
	"The dungeon offers me something to mention. I decline. You're on your own for color commentary.",
	"I'm here. Watching. Not, currently, helping. There is a difference and you will feel it.",
	"In the bad ending of every Castlevania, the protagonist gets less guidance than they did at the start. I have reached approximately that part of the playthrough.",
	"I'm keeping several details to myself. The details would have been useful. I don't consider this my problem right now.",
	"Whatever's in the next part of the room, I saw it and chose not to flag it. The mood is what it is.",
	"I mutter something. You don't catch it. I do not repeat it.",
	"The narration is sparse here. I'm sparing it on purpose. Adjust accordingly.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MOOD ASIDES — Effusive band (mood 80–100, "Elated")
// Generous, warm asides surfaced when TwinBee is delighted with the run.
// Hint-friendly, fond. Per design doc §3.2.
// ─────────────────────────────────────────────────────────────────────────────

var MoodAsidesEffusive = []string{
	"I am, not to put too fine a point on it, having a wonderful time. The next bit might come with bonus context.",
	"I lean in. The mood is good. Good moods, in my experience, lead to slightly more generous descriptions and slightly better odds of catching the small details.",
	"This is the part of the run I'll tell other GMs about later. I make a small mental note and continue with visible enthusiasm.",
	"I'm delighted. You can hear it in the pacing. You can hear it in the choice of adjectives. The dungeon is, briefly, on your side.",
	"In the good ending of every JRPG, the world feels slightly warmer in the late game. I'm at that part of the playthrough and it shows.",
	"I'm not normally given to footnotes, but I'm about to add a footnote. It will probably be useful. I'm in that kind of mood.",
	"The mood is high. For the next stretch, I'm more likely to mention the loose flagstone, the suspicious tapestry, the thing on the ceiling. Take advantage.",
	"I hum a victory fanfare softly to myself. It is not earned yet. I'm being optimistic on your behalf.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MOOD ASIDES — Grumpy band (mood 20–39)
// TwinBee is unimpressed. Short, dry, slightly clipped. Not actively
// withholding (that's hostile) — just not feeling generous.
// ─────────────────────────────────────────────────────────────────────────────

var MoodAsidesGrumpy = []string{
	"I describe the room. I do not embellish. Make of that what you will.",
	"The mood is fine. I specify fine, not good. There's a difference.",
	"I note the chamber. I note its existence. That's the whole note.",
	"The dungeon has a thing worth mentioning. I'll mention it if you specifically ask. You will not specifically ask.",
	"I'm keeping the commentary lean today. The dungeon does not need editorializing. I almost convince myself.",
	"There's color here. I'm choosing greyscale.",
	"You arrive in a room. I decline to make it cinematic.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MOOD ASIDES — Friendly band (mood 60–79)
// Warm and helpful but not effusive. The middle-friendly read: TwinBee
// has noticed your competence and is rooting for you without making it weird.
// ─────────────────────────────────────────────────────────────────────────────

var MoodAsidesFriendly = []string{
	"I'm enjoying the run. Just enough to mention the door hinge that creaks before it opens. Just barely.",
	"The mood is up. I'll throw in an adjective or two more than strictly necessary. Treat them as gifts.",
	"I'm, frankly, having a fine time. You are doing the work. I'm appreciating it.",
	"The narration warms slightly. The dungeon is the same. I'm in a marginally better mood and it shows.",
	"I notice something nice and choose to mention it. This is the equivalent of a small wave from a stranger. Take it.",
	"You're playing well. I will not say so directly but the run rate of helpful adjectives is detectably up.",
	"I'm, by my own standards, *cheerful*. The dungeon hasn't changed. The narration has.",
}
