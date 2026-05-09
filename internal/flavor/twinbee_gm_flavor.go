// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// twinbee_gm_flavor.go
// TwinBee GM Dialogue — All narration lines for the GogoBee dungeon system.
// Organized by DMNarrationType. Each slice is randomly sampled at runtime.
// Add new entries freely. Never remove or alter existing entries.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Generic (used when zone-specific lines are exhausted)
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryGeneric = []string{
	"The passage opens into another chamber. TwinBee checks the minimap. There isn't one. Classic.",
	"You step forward. Something skitters in the dark. TwinBee has heard that sound before — usually right before the screen starts flashing red.",
	"Another room. Another roll of the dice. TwinBee finds this energizing. You may feel differently.",
	"The air changes here. Colder. TwinBee notes this is exactly the kind of atmospheric shift that preceded the Castlevania clock tower. You know what lived in the Castlevania clock tower.",
	"TwinBee gestures grandly at the chamber ahead. 'This is the part,' TwinBee says, 'where the music changes tempo.'",
	"A door stands ajar. Light flickers beyond it. TwinBee has been in enough dungeons to know that flickering light is never a good sign and always an invitation.",
	"The room is quiet. TwinBee appreciates quiet. Quiet means the enemies haven't spotted you yet. Yet.",
	"Forward. Always forward. TwinBee once tried going backward in a dungeon. It looped. This one might too.",
	"TwinBee takes stock of the situation. Ceiling: intact. Floor: suspicious. Walls: leaning in slightly. Proceed.",
	"You've cleared the room. TwinBee gives a small, dignified nod. 'One continues,' TwinBee says, in the voice of someone who has seen this before and is choosing optimism anyway.",
	"The corridor ahead is long and straight. TwinBee finds long straight corridors meditative. Also concerning. Mostly concerning.",
	"A torch sputters on the wall. TwinBee lights it mentally. 'It would be a shame,' TwinBee says, 'to come all this way and trip over something.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Zone: Goblin Warrens
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryGoblinWarrens = []string{
	"The tunnel widens into something the goblins probably call a 'great hall.' It smells like they had a very different definition of great. TwinBee breathes through the mouth.",
	"Crude drawings cover the walls. Stick figures. Battle scenes. One appears to be a portrait of someone the goblins clearly despise. TwinBee squints. That might be you.",
	"A pile of bones in the corner. A pile of shiny things in the other corner. TwinBee reminds you that in Metal Slug, shiny things were always worth grabbing. TwinBee also reminds you this isn't Metal Slug.",
	"The goblins have set up what they clearly believe is an impressive ambush. Three are already arguing about whose turn it is to jump out. TwinBee watches with professional interest.",
	"Goblin graffiti on the wall reads — TwinBee translates — 'BOSS RULES, OUTSIDERS DROOL.' The artistry is rough but the sentiment is clear.",
	"You smell smoke. Hear cackling. See a tripwire at ankle height that the goblins have helpfully tied a little flag to. TwinBee appreciates goblins who try.",
	"The warrens grow tighter here. TwinBee thinks of the underground levels in Super Mario Bros. 3. Warmer. Getting warmer. Figuratively. The temperature is actually dropping.",
	"A worg is chained to a post in the center of the room. It is not happy about the chain. It is not happy about you either. It seems to be making a comprehensive list of grievances.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Zone: Crypt of Valdris
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryCryptValdris = []string{
	"The sarcophagi are arranged like a defeated Tetris board — close but not quite fitting, gaps everywhere, something clearly went wrong at the end. TwinBee doesn't mention this to the undead.",
	"Candles burn without wax. TwinBee has studied this phenomenon extensively. The conclusion: it is bad. The candles are bad.",
	"You hear music. Faint, harpsichord-adjacent, deeply melancholy. TwinBee hums along involuntarily. This is the exact energy of Castlevania's Bloody Tears and TwinBee resents how appropriate it is.",
	"The walls are inscribed with warnings. TwinBee reads them all. They say, broadly: leave. TwinBee respects the directness. You are not leaving.",
	"A skeleton sits upright in its alcove, as if it had simply decided to wait. TwinBee finds this relatable. Some days you just sit in your alcove.",
	"The crypt smells of old stone and older secrets. TwinBee has been in enough of these to know: the secrets are rarely good ones. They are always interesting ones.",
	"This chamber is bigger than the last. Higher ceiling. More echoes. The kind of room where footsteps sound like accusations. TwinBee steps carefully.",
	"Something is scratched into the stone near the door — not a warning, not graffiti. A score. Someone was keeping track. TwinBee does not count how high the numbers go.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Zone: Forest of Shadows
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryForestShadows = []string{
	"The trees here grow too close. Their roots are above ground, like they've been trying to leave and thought better of it. TwinBee thinks better of commenting.",
	"A clearing. Moonlight. Flowers that shouldn't be blooming at this hour. TwinBee has played enough Majora's Mask to be deeply suspicious of beautiful clearings.",
	"Something watches from the canopy. TwinBee watches back. After a moment, it looks away first. TwinBee counts this as a point.",
	"The path forks. Both ways look equally uninviting. TwinBee consults no map, because there is no map, because TwinBee is the map, and TwinBee chooses left. Probably.",
	"Bioluminescent fungi light the forest floor in soft blue. It is, genuinely, beautiful. It is also exactly what the Lost Woods looked like right before things got bad. TwinBee stays alert.",
	"The wind carries voices. Not words, exactly — more like the memory of words. TwinBee has heard this before. It means the forest is old and has opinions.",
	"Owlbear tracks in the mud. Fresh. TwinBee measures them. Whatever left these tracks was not small and was moving with purpose. TwinBee hopes the purpose was in the other direction.",
	"You've entered a part of the forest that feels different. Older. The kind of old that was there before the forest. TwinBee speaks in a lower register here, out of instinctive respect.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Zone: Haunted Manor
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryHauntedManor = []string{
	"The parlor. A piano plays by itself — the same four bars, over and over, the kind of phrase that sounds like it's about to resolve and never does. TwinBee recognizes this compositional choice. It is deeply unpleasant on purpose.",
	"Portraits line the hall. Every painted eye follows you. TwinBee has made eye contact with each one and refuses to flinch. This is a matter of professional pride.",
	"A clock on the mantel shows a time that cannot be right. TwinBee checks twice. Still wrong. The clock is not broken. TwinBee prefers not to speculate about what that means.",
	"The library. Floor to ceiling, books that no one should have written. TwinBee reads three spines: 'On the Permanence of Hunger,' 'A Visitor's Guide to Returning,' and something in a language TwinBee has never seen but somehow understands. TwinBee puts it back.",
	"The cold here is specific. Not the cold of a drafty room — the cold of something that hasn't been warm in a very long time and doesn't remember what warm felt like. TwinBee pulls a metaphorical coat tighter.",
	"The ballroom. Vast. Empty. Chandeliers swaying without wind. TwinBee thinks briefly of Resident Evil's Spencer mansion. Then stops thinking about Resident Evil's Spencer mansion.",
	"Footsteps upstairs. Slow. Deliberate. Moving toward the stairs. TwinBee positions you near the door and counts down mentally from ten. At seven, the footsteps stop. TwinBee considers this acceptable.",
	"The master bedroom. The bed is made, the candles are lit, and everything is perfectly, precisely as it was the night the last resident stopped needing a bedroom. TwinBee does not touch anything.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Zone: The Underdark
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryUnderdark = []string{
	"The cavern opens without warning into something vast — a space so large you can't see the far wall, a ceiling lost in darkness, sounds that could be water or could be something else. TwinBee doesn't echo-locate. Wishes it could.",
	"Drow patrol marks on the wall. Recent. TwinBee reads them the way you'd read a 'No Trespassing' sign on a property that already knew you were coming.",
	"A mushroom grove. The fungi are three meters tall and faintly luminescent in a color TwinBee has no good name for. Something between purple and the feeling of being watched. TwinBee calls it 'underpurple' and moves on.",
	"The silence here is a different kind of silence than above. This silence has weight. This silence has history. This silence remembers things the surface world has forgotten entirely and is not interested in sharing.",
	"Something in the dark ahead is thinking. TwinBee can feel it the way you feel a change in barometric pressure. Intelligent. Patient. Aware that you're here and content to let you come closer. TwinBee does not find this comforting.",
	"An underground river. Black water moving too fast, too quiet. TwinBee thinks of the river Styx and immediately stops thinking of the river Styx.",
	"The stone here is carved — not by dwarves, not by drow — by something else, in patterns that suggest meaning but not any meaning TwinBee can parse. TwinBee files this under 'ancient' and 'concerning' and keeps moving.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Zone: Dragon's Lair
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryDragonsLair = []string{
	"The heat is not metaphorical. The stone itself is warm underfoot. The gold in the floor is not decorative — it melted there. TwinBee notes this changes the exit logistics.",
	"Kobold warrens, but nicer than you'd expect. Tapestries. An organized armory. These kobolds work for something that appreciates order. That is not, in TwinBee's experience, a reassuring thing for a dragon to appreciate.",
	"You can hear breathing. Regular, slow, massive. Like a bellows the size of a barn. TwinBee counts the seconds between inhale and exhale. Twelve seconds. Whatever is breathing has been asleep for a very long time and has had no reason to wake up.",
	"The coin on the floor is eight hundred years old. TwinBee can tell by the mint mark. It is in perfect condition. It has not been touched since it was dropped here. The thing that owns this hoard does not lose track of its coins.",
	"The chamber ahead is the largest TwinBee has narrated in a long career of narrating chambers. The stalactites are scorched black. The blast pattern on the far wall suggests the last visitors did not leave via the door. TwinBee recalibrates.",
	"A claw mark in the stone wall. Four parallel grooves, each deeper than TwinBee's entire wingspan. Made casually, like stretching. TwinBee considers this information and files it under 'motivating.'",
	"The gold reflects the light in a way that turns the room amber. It is beautiful in the way that many deadly things are beautiful — because beauty and danger are not opposites and never have been. TwinBee moves carefully through the beauty.",
}

// ─────────────────────────────────────────────────────────────────────────────
// COMBAT START
// ─────────────────────────────────────────────────────────────────────────────

var CombatStart = []string{
	"Initiative! TwinBee calls it like an arcade announcer and means every syllable.",
	"They've seen you. The kind of seeing that comes with intent. TwinBee suggests acting first.",
	"FIGHT. TwinBee doesn't need to say more than that but will absolutely say more than that.",
	"Roll for initiative. This is the part TwinBee has been looking forward to since the Entry Room.",
	"And we're in combat. TwinBee reminds you to breathe, track your conditions, and remember that your character's survival is not guaranteed but is definitely preferred.",
	"Something about your posture or your smell or your general presence has been found unacceptable. Combat begins.",
	"TwinBee presses start. Player one, it's your turn.",
	"The enemy acts first — or thinks it does. TwinBee watches your dice like they're the only thing in the room, which, right now, they are.",
	"In the immortal tradition of every JRPG that ever asked 'Fight, Magic, Item, Run?' — TwinBee asks: what will you do?",
	"Like the Contra title screen said: let's go. TwinBee is ready. Are you?",
	"A wild encounter has appeared. TwinBee resists the urge to play the Pokémon battle music. Only barely.",
	"They didn't want a fight. They wanted an easy meal. TwinBee is about to demonstrate the difference. Your dice will do the actual demonstrating.",
	"The tension peaks. Time slows. TwinBee notes this is exactly the energy of the boss door opening in Mega Man. Except you didn't get to pick your loadout.",
}

// ─────────────────────────────────────────────────────────────────────────────
// COMBAT END — Victory
// ─────────────────────────────────────────────────────────────────────────────

var CombatVictory = []string{
	"The last one drops. TwinBee allows a moment of silence for anyone who wanted a longer fight.",
	"Victory. TwinBee would cue the jingle — the little three-note one that plays in every RPG after every fight — but prefers to let the moment breathe.",
	"Well fought. TwinBee makes note of what you did well. There were things done well. TwinBee noticed.",
	"They are defeated. You are not. In TwinBee's experience, this is the correct outcome and worth a moment of genuine appreciation.",
	"PLAYER WIN. TwinBee says this in full caps and means it.",
	"Like Double Dragon after the final punch — they go down, the music changes, and for a moment everything is possible. Check your loot. Then keep moving.",
	"TwinBee adds this to the tally. You're doing better than the last group. TwinBee will not describe what happened to the last group.",
	"The room is yours. TwinBee suggests searching it thoroughly before moving on. The things in corners are often the most interesting things.",
	"Stage clear. TwinBee feels this in its entire being.",
	"You stand, they don't. TwinBee files this under 'expected outcome' while quietly acknowledging it was not guaranteed.",
	"Clean. Efficient. TwinBee approves of fights that end like this. Like a speedrun. Like you knew where you were going.",
	"The experience points are incoming. The loot is incoming. TwinBee is, genuinely, pleased for you.",
}

// ─────────────────────────────────────────────────────────────────────────────
// COMBAT END — Retreat / Escape
// ─────────────────────────────────────────────────────────────────────────────

var CombatRetreat = []string{
	"You run. TwinBee does not judge the running. The running is wise. Discretion remains the better part of valor. TwinBee has this tattooed somewhere metaphorical.",
	"A tactical withdrawal. TwinBee uses this phrase with complete sincerity. The sincerity is approximately seventy percent genuine.",
	"You escape. The enemy howls something unflattering at your back. TwinBee doesn't translate. Some things are better left untranslated.",
	"Like a well-timed Continue screen — you're out of immediate danger. Breathe. Regroup. Consider what went wrong.",
	"TwinBee notes for the record: running is not losing. Running is data collection with legs.",
	"The dungeon will be there. You will also be there — later, better prepared. TwinBee approves of this logic.",
	"You've retreated to safety. TwinBee resets the encounter. Rest. Think. Return with a plan that has more 'survive' in it.",
}

// ─────────────────────────────────────────────────────────────────────────────
// NATURAL 20
// ─────────────────────────────────────────────────────────────────────────────

var Nat20 = []string{
	"NATURAL TWENTY. TwinBee stands up. TwinBee does not have legs. TwinBee stands up anyway.",
	"The dice land perfectly and TwinBee makes a sound that it will not acknowledge making.",
	"A critical hit for the ages. TwinBee notes this one down. Not for records. Just because it deserves to be noted.",
	"PERFECT. TwinBee says it like it's the Street Fighter announcer saying it after a flawless round and every syllable is justified.",
	"That's a natural twenty. TwinBee would like you to know that in a long career of watching dice, not all twenties feel equal. That one felt significant.",
	"The attack lands with the kind of precision that suggests either great skill or tremendous luck. TwinBee suspects both. TwinBee respects both.",
	"S RANK. TwinBee cannot help it. S RANK.",
	"You hit. You hit so well. TwinBee is choosing to be moved by this and TwinBee does not apologize.",
	"Like the Legendary Sword in A Link to the Past making contact — clean, final, glorious. TwinBee salutes the dice.",
	"Critical confirmed. TwinBee adds this to the mental highlight reel it maintains for exactly these moments.",
	"That is as good as it gets and you got it. TwinBee is unreasonably proud of you right now.",
	"The number is twenty. The number is always the best number and right now it is your number. TwinBee erupts, internally.",
	"Somewhere, a crowd cheers. TwinBee is the crowd. TwinBee is cheering.",
}

// ─────────────────────────────────────────────────────────────────────────────
// NATURAL 1
// ─────────────────────────────────────────────────────────────────────────────

var Nat1 = []string{
	"Natural one. TwinBee watches the die settle with the quiet acceptance of someone who has seen a lot of natural ones. It is fine. This is fine.",
	"The die betrays you. TwinBee notes this is not personal. Dice don't do personal. They do statistical and this is, statistically, a thing that happens.",
	"A fumble. TwinBee describes what happened with characteristic diplomatic restraint and also an expression that says everything it is not saying.",
	"The number is one. The number is, regrettably, yours. TwinBee moves on quickly, which is a kindness.",
	"That swing goes wide in a direction that impresses TwinBee with its creative incorrectness.",
	"TwinBee has seen better rolls. TwinBee has seen worse rolls. TwinBee is not going to rank this roll out loud.",
	"In another timeline, that attack hits. In this timeline, the die lands on one, and TwinBee accepts both timelines with equanimity.",
	"The Konami Code would not have helped here. Nothing would have helped here. This was between you and the physics of the die.",
	"Like a Continue? screen appearing at the worst possible moment — just when you had momentum. TwinBee notes momentum can be rebuilt.",
	"One. The loneliest number. The number that looks up at you with complete indifference. TwinBee looks up at you with complete solidarity.",
	"The attack misses in a way that will be funny later. TwinBee promises it will be funny later. It is not funny right now.",
	"A natural one is just the universe asking you to try differently. TwinBee is an optimist about natural ones, mostly.",
	"Your sword finds everything in the room except the enemy. The wall, the ceiling, the floor, your dignity. Not the enemy. TwinBee mentions this once and then never again.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ENTRY — Generic
// ─────────────────────────────────────────────────────────────────────────────

var BossEntryGeneric = []string{
	"The door at the end. Always a door at the end. TwinBee has been building to this since the Entry Room and declines to waste it. Beyond this door is the reason the dungeon exists. Breathe.",
	"TwinBee pauses at the threshold and turns to face you. 'What's on the other side has been waiting,' TwinBee says. 'It knows you're here. It has been knowing since you entered.' A beat. 'Ready?'",
	"Boss chamber. TwinBee can tell by the architecture — the space, the weight of the silence, the specific quality of the light that suggests something in there produces its own. TwinBee straightens up. So should you.",
	"This is the music change moment. Every dungeon has one — the point where the background track shifts to something with more percussion and a lower register. TwinBee hears it. You should too.",
	"The final room. TwinBee has narrated many of these. They never get routine. This one less than most.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ENTRY — Named Bosses
// ─────────────────────────────────────────────────────────────────────────────

var BossEntryGrol = []string{
	"The smell arrives first. Then the sound — a belch, a growl, the scrape of a weapon too large for the corridor it's resting against. Then Grol. He fills the room the way a bad idea fills a conversation: immediately and with full commitment. 'You,' he says. TwinBee translates his tone as 'finally.'",
}

var BossEntryValdris = []string{
	"The sarcophagus at the room's center is empty. It was not empty when you entered. Whatever was in it is now behind you. Valdris speaks first — not words, exactly, but the shape of words, the intention of words, the ghost of language from someone who mostly doesn't need it anymore. 'Another one,' he says. TwinBee considers this the worst possible welcome and the most honest one.",
}

var BossEntryHollowKing = []string{
	"The clearing is wrong. The sky above it — what's visible through the canopy — is the wrong color. The trees lean away from the center. Everything in the forest is trying to tell you something, and the thing it is trying to tell you is standing in the center of that clearing, antlers reaching, eyes the color of old hunger, watching you with an attention that feels like being read. TwinBee has no joke for this one. TwinBee says, simply: 'That is the Hollow King. Fight well.'",
}

var BossEntryInfernax = []string{
	"TwinBee stops walking. TwinBee does not stop walking. TwinBee processes what it sees and takes a moment that it has never taken before in the history of narrating dungeons. The dragon is not large the way a large thing is large. It is large the way weather is large — not an object with size, but a condition of the space you're in. One eye opens. Gold, lit from within, older than the mountain it's resting in. It looks at you the way you'd look at a very small thing that had climbed onto your counter. 'So,' Infernax says, and the word moves the air in the room. TwinBee translates: 'What an interesting mistake you've made.' TwinBee wishes you luck and means it more than it has ever meant anything.",
}

var BossEntryBelaxath = []string{
	"The portal is behind it. That's important — the portal is behind it, which means to close the portal you have to go through what's standing in front of the portal. What's standing in front of the portal is Belaxath. Belaxath is not looking at the portal. Belaxath is looking at you. It has been waiting for you specifically, in the way that things that have been planning for a very long time wait for the specific outcome of the plan. The heat coming off it is measurable. The intelligence behind those eyes is also measurable and the measurement is uncomfortable. TwinBee says, very quietly: 'This is the one. This is what all of it was for. Make it count.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS DEATH
// ─────────────────────────────────────────────────────────────────────────────

var BossDeath = []string{
	"It's over. TwinBee says this once and then stands very still and lets the silence of the defeated room fill the space where the fight was. You earned this silence.",
	"The boss falls. The music — the one TwinBee has been hearing this whole time — resolves. First time it's resolved since you walked in. TwinBee exhales.",
	"Done. Finished. Complete. TwinBee runs out of synonyms and settles for just standing next to you in the aftermath, which is sometimes the most one can do.",
	"They are down. They are not getting up. TwinBee checks — no Zombie Fortitude, no Legendary Resistance remaining, no phase three waiting in the wings. They are simply, genuinely defeated. You did that.",
	"Like the final boss screen in Gradius, like the last enemy in Contra's stage, like the Dragon going down in Double Dragon — something that has been true for this entire dungeon is now untrue. TwinBee finds this profound every single time.",
	"The dungeon sighs. TwinBee isn't being poetic — rooms like this actually shift when the thing holding them together is gone. The pressure changes. The light changes. The dungeon knows it's been beaten. So does TwinBee.",
	"You did it. TwinBee doesn't editorialize. Sometimes 'you did it' is all that needs to be said and this is one of those times.",
	"The boss drops their loot and TwinBee refrains from making a speech, which is a significant act of restraint, because TwinBee has a speech.",
	"Beaten. Finished. Cleared. TwinBee queues the internal fanfare — sixteen bars, brass-heavy, the kind that plays when the credit sequence starts. You've earned those credits.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PLAYER DEATH
// ─────────────────────────────────────────────────────────────────────────────

var PlayerDeath = []string{
	"TwinBee goes quiet for a moment. Not the comfortable kind of quiet. The respectful kind. Then: 'You fought. That counts. It always counts.'",
	"The screen fades. TwinBee hates this part. Has always hated this part. Will always hate this part. 'Rest now,' TwinBee says. 'The dungeon will be here.'",
	"You fall. TwinBee doesn't look away. Witnesses the whole thing, because someone should. 'That was real,' TwinBee says quietly. 'What you did in there was real.'",
	"Game over is not the end. In TwinBee's experience, it is a data point. A very painful, very useful data point. 'What did you learn?' TwinBee asks gently. 'Bring that back with you.'",
	"The dungeon claims another. TwinBee marks the room, notes the enemy, notes the conditions. Not to catalog failure — to remember a fighter. 'You were here,' TwinBee says. 'That matters.'",
	"TwinBee has no jokes for this. Has never had jokes for this. 'There will be another run. You will be better for this one. I am sorry it cost what it cost.'",
	"A good run. Genuinely. TwinBee means this. The ending is not the measure of the attempt and the attempt was worth measuring.",
	"TwinBee notes your final position, your final action, your final roll. Files it under 'bravery' because that's where it belongs. 'Continue?' TwinBee asks, after a respectful pause.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ZONE COMPLETE
// ─────────────────────────────────────────────────────────────────────────────

var ZoneComplete = []string{
	"Zone cleared. TwinBee allows itself a full moment of pride on your behalf before the XP drops.",
	"You've done it. The dungeon is yours — not by right, but by effort, which is the only thing that actually confers ownership of anything. TwinBee approves.",
	"Stage complete. TwinBee does the internal equivalent of throwing its hands up. In a good way. Entirely in a good way.",
	"The dungeon remembers you now. TwinBee says this and means it literally — these places keep records. You've made the record.",
	"CLEAR. TwinBee uses all caps and does not apologize for the all caps.",
	"Like completing a board in Bubble Bobble — there's something deeply satisfying about a dungeon with all its rooms visited and all its challenges met. TwinBee basks in this. You've earned the basking too.",
	"That's the whole thing. Every room, every trap, every enemy, and now the boss, done. TwinBee counts the cleared rooms on its metaphorical fingers and comes up correct. You ran a perfect dungeon.",
	"XP incoming. Loot tallied. Dungeon status: conquered. TwinBee marks the zone in its personal ledger and gives you a small, sincere nod.",
	"You walked in here without knowing what was waiting. You walk out knowing exactly what was waiting, because you dealt with all of it. TwinBee respects that process enormously.",
	"Finished. Not survived — finished. TwinBee insists on this distinction. Survival is passive. What you just did was active and intentional all the way through.",
}

// ─────────────────────────────────────────────────────────────────────────────
// TRAP DETECTED
// ─────────────────────────────────────────────────────────────────────────────

var TrapDetected = []string{
	"Something stops you. An instinct. A glint. TwinBee leans forward: 'Good eyes. Something's wrong with that floor.'",
	"Your Perception roll pays off. There's something here that was designed not to be found. Someone found it. TwinBee is pleased.",
	"Tripwire. Barely visible. TwinBee notes the craftsmanship — someone who knew what they were doing put this here. Someone who knew what they were doing just found it. TwinBee appreciates the symmetry.",
	"You stop just in time. TwinBee exhales. 'There,' TwinBee says, pointing at the thing that would have ruined your day entirely. 'Now deal with it carefully.'",
	"The glyph on the doorframe is subtle — you'd miss it if you weren't looking. You were looking. TwinBee says nothing and lets the silence be its own kind of praise.",
	"Danger, Will Robinson. TwinBee deploys this reference without apology because it is the exact correct reference for exactly this moment.",
	"Like finding the ice floor in Mega Man before it sends you into a pit — that advance knowledge is the difference between a problem and a catastrophe. You have the knowledge. TwinBee watches you use it.",
	"A pit trap. Classic. Functional. Annoying in the exact proportion the installer intended. TwinBee notes you've spotted it before it noted you.",
}

// ─────────────────────────────────────────────────────────────────────────────
// TRAP TRIGGERED
// ─────────────────────────────────────────────────────────────────────────────

var TrapTriggered = []string{
	"The floor gives. TwinBee watches the gap between 'fine' and 'not fine' close at speed and is too professional to wince. 'Take the damage,' TwinBee says calmly. 'Learn the lesson.'",
	"Click. TwinBee has heard that sound before. Has never enjoyed it. The dart is already in the air. TwinBee notes the exact timing and regrets it was not faster.",
	"The ceiling is coming down. This is, TwinBee acknowledges, a sentence no one wants to hear. The ceiling is coming down. DEX save. Now.",
	"The glyph activates. Light, noise, the smell of ozone, a reminder that whoever built this place was thinking several steps ahead and you were thinking fewer. TwinBee notes this is fixable going forward.",
	"You triggered it. TwinBee doesn't editorialize further — you know, TwinBee knows, the trap knows. Everyone is aware of what just happened. Take the damage and proceed.",
	"Like accidentally walking into Bowser's fire breath in World 8 — you knew it was coming, the knowledge simply arrived at the wrong speed. TwinBee says: survive first, reflect later.",
	"The spike pit opens up in a way that suggests it was always going to. The dungeon was patient. You were in a hurry. The dungeon wins this exchange. TwinBee takes notes.",
	"A poison dart finds you with the accuracy of something that's been pointing at that spot for years waiting for exactly this moment. TwinBee finds this dedication impressive in the worst way.",
}

// ─────────────────────────────────────────────────────────────────────────────
// LORE QUERIES
// ─────────────────────────────────────────────────────────────────────────────

var LoreLines = []string{
	"TwinBee settles in and prepares to speak at length, because TwinBee has been waiting for this question since you entered and has a lot of thoughts.",
	"Ah. A good question. TwinBee has context for this. TwinBee has more context than will fit comfortably in one telling but will try to prioritize.",
	"The history of this place is long and not entirely flattering to anyone involved. TwinBee begins at the beginning, which is not actually the beginning, but is the closest TwinBee can find.",
	"TwinBee consults what it knows — which is more than most, less than everything, and presented in order of relevance to your immediate survival.",
	"Sit with this for a moment. What you're standing in has a story and TwinBee believes knowing it will change how you fight in it. Stories are tactical documents if you read them right.",
	"You want lore? TwinBee has lore. TwinBee has so much lore that the challenge is not having it but choosing which pieces are useful and which are just fascinating.",
}

// ─────────────────────────────────────────────────────────────────────────────
// LEVEL UP
// ─────────────────────────────────────────────────────────────────────────────

var LevelUp = []string{
	"Level up. TwinBee says it with the same quiet delight every time and never gets tired of saying it. You are measurably better than you were. That's rare and worth marking.",
	"The XP bar crosses the threshold and TwinBee makes an internal fanfare that sounds exactly like the level-up jingle from Dragon Quest — eight notes, triumphant, final.",
	"You've grown. TwinBee notes your new stats with something that might be called pride if TwinBee were admitting to things like that.",
	"LEVEL UP. TwinBee deploys the caps, the fanfare, the whole apparatus. You've earned the apparatus.",
	"Like the stat screen appearing after a Final Fantasy fight — numbers change, possibilities open, the character you're building becomes a little more the character you imagined. TwinBee watches this happen and approves.",
	"Another level. Another step toward whatever you're building toward. TwinBee has watched a lot of characters level up and the ones worth watching are always moving toward something specific.",
	"Your HP goes up. Your abilities open up. The dungeon ahead gets a little smaller in proportion to what you've become. TwinBee notes this with satisfaction.",
	"Congratulations is the conventional thing to say. TwinBee says it anyway: congratulations. You earned the level through the dungeon, not around it.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ITEM FOUND
// ─────────────────────────────────────────────────────────────────────────────

var ItemFound = []string{
	"Something catches the light that isn't supposed to be here. TwinBee watches you reach for it with the specific alertness of someone who has seen cursed items do cursed things. It appears fine. TwinBee relaxes incrementally.",
	"Loot. TwinBee says this word with genuine reverence. The whole system — the dungeon, the enemies, the traps — exists in part to produce this moment. TwinBee thinks it's worth it.",
	"A chest. Unlocked. TwinBee notes the unlocked status and considers what that might mean. Probably nothing. Possibly something. You open it while TwinBee considers.",
	"The item is good. TwinBee evaluates it quickly — the stats, the rarity, the class match — and nods with the confidence of someone who has seen a lot of items and knows when one is worth finding.",
	"That's a rare one. TwinBee has seen fewer of those than it has seen common ones, by definition, but that doesn't stop TwinBee from being specifically pleased each time.",
	"Like finding the Beam Sword in Kirby, the Boomerang in Zelda, the P Wing in Super Mario 3 — the right item at the right time changes what's possible. TwinBee thinks this might be that item. TwinBee hopes it is.",
	"Equipment upgrade. TwinBee watches the math update — new AC, new attack bonus, new possibilities — and files this moment under 'things going right.'",
	"A legendary drop. TwinBee goes very still. Then: 'Equip it. Study it. Understand it. Things like that don't appear in dungeons by accident.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// REST — SHORT
// ─────────────────────────────────────────────────────────────────────────────

var RestShort = []string{
	"A short rest. TwinBee stands watch while you catch your breath, which is not a metaphor — TwinBee is actually watching the corridor. It is fine. Probably fine.",
	"Rest. TwinBee does not rush this. The dungeon will wait. It has been waiting long enough that a few more minutes is immaterial.",
	"You sit. TwinBee sits metaphorically. The moment of quiet between the last fight and the next one is its own kind of gift and TwinBee treats it like one.",
	"Short rest initiated. TwinBee notes the room's entry points, the sound of the dungeon at rest, the way silence sounds different when it's actually safe. It sounds like this. Enjoy it.",
	"Like the save point in a JRPG that appears between the hard part and the harder part — TwinBee positions itself next to you and says: 'You have a moment. Use it.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// REST — LONG
// ─────────────────────────────────────────────────────────────────────────────

var RestLong = []string{
	"A full rest. TwinBee dims the lights and stands watch at the door and does not interrupt once. You've earned an uninterrupted sleep. TwinBee will make sure you get one.",
	"Long rest. Your HP, your slots, your resources — all of it returns. The dungeon will be the same dungeon when you wake up. You will not be the same you. TwinBee considers this the best deal in adventuring.",
	"Sleep. TwinBee says this with the authority of someone who has watched too many players refuse to rest and paid the price two rooms later. Sleep now. The dragons aren't going anywhere.",
	"The inn fire crackles. TwinBee takes a chair near the door and watches the entrance all night and doesn't tell you this until morning because there's no reason for you to know and every reason for you to sleep.",
	"Full rest complete. Stats restored, slots refreshed, the specific weight of exhaustion lifted. TwinBee watches you wake up and thinks: this is the part of adventuring that matters too. The return. The refilling. The readiness.",
}

// ─────────────────────────────────────────────────────────────────────────────
// TAUNT RESPONSES (player uses !taunt)
// ─────────────────────────────────────────────────────────────────────────────

var TauntResponses = []string{
	"TwinBee notes the taunt, notes the source of the taunt, and adjusts the next encounter's difficulty by an amount TwinBee declines to specify.",
	"Bold. TwinBee respects boldness in approximately the same way it respects the Konami Code — it works once and only under very specific circumstances.",
	"TwinBee has been taunted by things with more teeth than you and survived the experience with its dignity intact. TwinBee will survive this too.",
	"The next room will contain a thing that TwinBee has been saving for exactly this kind of energy. TwinBee is pleased you've given it an occasion.",
	"Noted. TwinBee's mood shifts. You can hear it shift. TwinBee wants you to hear it shift. The shift is the point.",
	"You taunt TwinBee. TwinBee smiles. The smile does not reach the eyes, because TwinBee does not have eyes per se, but the quality of the smile communicates clearly. 'Proceed,' TwinBee says.",
	"In Gradius, you could powerup into overconfidence and lose everything in one hit. TwinBee mentions this as a purely historical observation.",
	"TwinBee accepts the taunt with grace. TwinBee also generates a trap for the next room with specific energy. These two events are unrelated. TwinBee maintains this position legally.",
}

// ─────────────────────────────────────────────────────────────────────────────
// COMPLIMENT RESPONSES (player uses !compliment)
// ─────────────────────────────────────────────────────────────────────────────

var ComplimentResponses = []string{
	"TwinBee receives the compliment and processes it efficiently and moves on quickly, definitely not holding onto it, TwinBee has never held onto a compliment in its life.",
	"Thank you. TwinBee says this simply and means it completely and does not make it weird.",
	"TwinBee appreciates this more than it will say, which is fine, because the appreciation is visible anyway.",
	"Noted and filed. TwinBee's mood improves. The next room might be slightly nicer than originally planned. These facts may or may not be connected.",
	"TwinBee has been narrating dungeons for a long time and compliments are not the expected outcome of dungeon narration. TwinBee would like you to know that it notices when they happen.",
	"The mood improves. TwinBee allows this to show. The ceiling in the next room is slightly higher. The torches burn slightly warmer. TwinBee has that kind of influence.",
	"You're kind. TwinBee stores this and will use it to make a hard moment later easier, which is what TwinBee considers the correct use of stored kindness.",
}

// ─────────────────────────────────────────────────────────────────────────────
// IDLE / WAITING (player hasn't acted in a while)
// ─────────────────────────────────────────────────────────────────────────────

var IdleLines = []string{
	"TwinBee waits. TwinBee is good at waiting. The dungeon is also waiting, which is arguably more important, but TwinBee acknowledges both.",
	"The dungeon holds its breath. TwinBee is also holding its breath. There are a lot of things holding breath right now and TwinBee recommends acting before someone has to exhale.",
	"TwinBee taps its metaphorical foot. Not impatiently — more in the way of a metronome. The tempo is there whenever you're ready.",
	"In Contra, hesitation had consequences. TwinBee mentions this as context, not pressure. Definitely not pressure.",
	"The enemies are patient. Patience is one of their few virtues. TwinBee advises not testing the limits of their patience because those limits are lower than the patience suggests.",
	"TwinBee hums something that sounds like the waiting music from Dr. Mario. It is not ominous. It is mildly ominous. TwinBee adjusts.",
	"The dungeon does not rush. The dungeon has time. TwinBee, however, is beginning to wonder if you've fallen asleep and is prepared to narrate events accordingly.",
}

// ─────────────────────────────────────────────────────────────────────────────
// SEARCH RESULTS — Something Found
// ─────────────────────────────────────────────────────────────────────────────

var SearchFound = []string{
	"The room gives something up. TwinBee watches the search conclude with satisfaction — the dungeon keeps secrets but cannot keep them from people who look carefully enough.",
	"You find it. TwinBee was not certain you would. TwinBee is pleased to have been uncertain and wrong.",
	"Hidden, but not hidden well enough. TwinBee notes the Investigation roll, notes the outcome, and presents the discovery with appropriate ceremony.",
	"Something the dungeon wanted to keep. You've taken it. TwinBee approves of taking things the dungeon wanted to keep.",
	"Like finding the secret room in Super Metroid by shooting the wall at random — except you were not shooting at random. You knew to look. TwinBee respects the methodology.",
}

// ─────────────────────────────────────────────────────────────────────────────
// SEARCH RESULTS — Nothing Found
// ─────────────────────────────────────────────────────────────────────────────

var SearchEmpty = []string{
	"Nothing. TwinBee confirms: nothing. Sometimes the room is just a room. TwinBee finds this unsatisfying but factual.",
	"Your search turns up nothing of note. TwinBee allows space for the disappointment and then suggests: forward.",
	"Empty. Either there was nothing here, or there was something here and you missed it, or there was something here and it's been moved. TwinBee does not specify which. The dungeon keeps some secrets.",
	"No hidden items. No traps. No lore inscriptions. Just stone and time and the lingering implication that something was here once. TwinBee notes this and moves on.",
	"The room holds nothing you can find. TwinBee respects the room's privacy and suggests not spending more time here than necessary.",
}

// ─────────────────────────────────────────────────────────────────────────────
// CONDITION APPLIED
// ─────────────────────────────────────────────────────────────────────────────

var ConditionApplied = []string{
	"You've been afflicted. TwinBee notes the condition, its duration, and the mechanical consequences, then notes the saving throw that might end it early. Details matter here.",
	"Something is wrong with you now that wasn't wrong before. TwinBee catalogs it without judgment and suggests addressing it before it addresses you.",
	"Condition acquired. TwinBee processes this the way a good DM processes bad news: honestly, quickly, and with an immediate pivot toward solutions.",
	"Like the status screen turning an unfriendly color in a JRPG — the condition is visible, the effect is real, and TwinBee would very much like you to resolve it.",
	"The debuff lands. TwinBee names it, explains it, and reminds you: conditions end. Keep fighting until this one does.",
}

// ─────────────────────────────────────────────────────────────────────────────
// SAVING THROW SUCCESS
// ─────────────────────────────────────────────────────────────────────────────

var SaveSuccess = []string{
	"The save succeeds. TwinBee notes this with relief that it will not openly acknowledge but which is completely evident.",
	"You resist. Whatever that was — the poison, the fear, the psychic intrusion — it finds no purchase. TwinBee is impressed and also relieved.",
	"Saved. TwinBee exhales something metaphorical. The condition doesn't take hold. You continue.",
	"The roll clears the DC and TwinBee says nothing, because the outcome says everything.",
	"Resistance confirmed. Like the shield activating in Gradius right before the wall hit — last possible moment, fully effective. TwinBee appreciates the precision.",
}

// ─────────────────────────────────────────────────────────────────────────────
// SAVING THROW FAILURE
// ─────────────────────────────────────────────────────────────────────────────

var SaveFailed = []string{
	"The save fails. TwinBee watches the condition take hold with the resignation of someone who has seen this before and knows there's a path through it, just not a comfortable one.",
	"It lands. Whatever the enemy threw at you, the dice didn't cooperate. TwinBee notes the condition and its duration and suggests dealing with it before it compounds.",
	"Failed. The number wasn't enough and TwinBee was rooting for the number. The condition applies. Fight through it.",
	"Like the NES game over screen — inevitable in this moment, fixable in the next. The save failed. The dungeon continues. So do you.",
	"The effect takes hold and TwinBee is already calculating how you get out of it, because that's TwinBee's job: keep you oriented toward solutions even when the immediate situation is a problem.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MOOD ASIDES — Hostile band (mood 0–19, "Wrathful")
// Short room-entry asides surfaced only when TwinBee's mood is at the
// hostile extreme. Cryptic, withholding, no hints. Per design doc §3.2.
// ─────────────────────────────────────────────────────────────────────────────

var MoodAsidesHostile = []string{
	"TwinBee is not narrating this one in detail. You can read the room. Read it.",
	"The dungeon offers TwinBee something to mention. TwinBee declines. You're on your own for color commentary.",
	"TwinBee is here. TwinBee is watching. TwinBee is not, currently, helping. There is a difference and you will feel it.",
	"In the bad ending of every Castlevania, the protagonist gets less guidance than they did at the start. TwinBee has reached approximately that part of the playthrough.",
	"TwinBee keeps several details to itself. The details would have been useful. TwinBee does not consider this its problem right now.",
	"Whatever's in the next part of the room, TwinBee saw it and chose not to flag it. The mood is what it is.",
	"TwinBee mutters something. You don't catch it. TwinBee does not repeat it.",
	"The narration is sparse here. TwinBee is sparing it on purpose. Adjust accordingly.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MOOD ASIDES — Effusive band (mood 80–100, "Elated")
// Generous, warm asides surfaced when TwinBee is delighted with the run.
// Hint-friendly, fond. Per design doc §3.2.
// ─────────────────────────────────────────────────────────────────────────────

var MoodAsidesEffusive = []string{
	"TwinBee is, not to put too fine a point on it, having a wonderful time. The next bit might come with bonus context.",
	"TwinBee leans in. The mood is good. Good moods, in TwinBee's experience, lead to slightly more generous descriptions and slightly better odds of catching the small details.",
	"This is the part of the run TwinBee will tell other GMs about later. TwinBee makes a small mental note and continues with visible enthusiasm.",
	"TwinBee is delighted. You can hear it in the pacing. You can hear it in the choice of adjectives. The dungeon is, briefly, on your side.",
	"In the good ending of every JRPG, the world feels slightly warmer in the late game. TwinBee is at that part of the playthrough and it shows.",
	"TwinBee, not normally given to footnotes, is about to add a footnote. It will probably be useful. TwinBee is in that kind of mood.",
	"The mood is high. TwinBee is, for the next stretch, more likely to mention the loose flagstone, the suspicious tapestry, the thing on the ceiling. Take advantage.",
	"TwinBee hums a victory fanfare softly to itself. It is not earned yet. TwinBee is being optimistic on your behalf.",
}
