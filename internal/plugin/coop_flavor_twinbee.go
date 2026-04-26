package plugin

// ── IMPORTANT: DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE ──
//
// This file contains hand-written flavor text. Every entry is the length it is
// on purpose. The voice, pacing, and specific wording are intentional.
// If an entry seems too long: it isn't. Leave it alone.
// If you are tempted to "clean up" or "condense" entries: don't.
// If your context window is too small to fit this file: that is your problem, not ours.
//
// ── TWINBEE — CO-OP DUNGEON FLOOR EVENT FLAVOR TEXT ──────────────────────────
//
// TwinBee is the dungeon narrator. He is extremely excited about this.
// He is genuinely trying to help. He is mostly not helpful.
// His descriptions are accurate in spirit and wrong in at least one important detail.
// His advice is delivered with confidence. Confidence is not correlated with accuracy.
// He never apologizes. He really didn't know.
// Occasionally he is completely right. No fanfare. Just right.
// This is worse in the long run because the party trusts him again.
//
// Voice notes:
// - Exclamation points are earned and frequent
// - Describes what he sees with genuine care and at least one critical omission
// - Advice is specific, helpful-sounding, and frequently incorrect
// - Never dwelling on bad outcomes -- already excited about the next thing
// - Occasionally, quietly, completely correct

// ── OBSTACLE EVENTS ──────────────────────────────────────────────────────────
// Something is blocking the path. Options: push through, find another way, attempt to clear it.

var TwinBeeObstacle = []string{
	"Oh! There's a cave-in up ahead! Big one! Really dramatic looking, actually -- " +
		"I wish you could see it from where I'm standing, it's very cinematic.\n\n" +
		"Anyway! The good news is it looks pretty loose. I think if you all just push " +
		"on the left side you'll be fine. It's definitely the left side.\n\n" +
		"Option A: Push through (left side, as I said). " +
		"Option B: Find another way around. " +
		"Option C: Try to clear it properly.\n\n" +
		"I'd go with A personally! Very confident about the left side!",

	"Okay so there's a locked gate. Big iron one, very impressive, " +
		"very intimidating if you're into that sort of thing.\n\n" +
		"I've been looking at the mechanism and I'm pretty sure I understand how it works! " +
		"It's a standard three-tumbler lock. Very common in dungeons of this era. " +
		"I've seen hundreds of them.\n\n" +
		"The one thing I'll say is that what looks like a third tumbler " +
		"might technically be a pressure plate but I'm sure that's fine.\n\n" +
		"Option A: Push through (I have thoughts on this). " +
		"Option B: Find another way. " +
		"Option C: Attempt to pick the lock.\n\n" +
		"Very exciting! I love gates!",

	"There's another party blocking the corridor! Four of them! " +
		"They look a bit rough but honestly they seem friendly enough -- " +
		"one of them waved at me!\n\n" +
		"I think they might be lost actually. They've been standing there for a while. " +
		"I'm sure if you just explain the situation they'll move right along.\n\n" +
		"The one who waved has a very large axe but I think that's just for decoration.\n\n" +
		"Option A: Approach and ask them to move. " +
		"Option B: Find another route. " +
		"Option C: Wait them out.\n\n" +
		"I'd definitely go with A! They seem nice! The axe is probably decorative!",

	"Oh wow, there's a flooded section ahead! " +
		"A little bit of water, nothing serious!\n\n" +
		"It's hard to tell exactly how deep from here but " +
		"I'd say ankle to maybe knee height at most. Very manageable! " +
		"The current looks pretty gentle too.\n\n" +
		"I will say the torches stopped about twenty meters back " +
		"but I'm sure the footing is fine.\n\n" +
		"Option A: Wade through. " +
		"Option B: Look for another path. " +
		"Option C: Try to divert the water somehow.\n\n" +
		"It's basically a puddle! Very refreshing probably!",

	"There's a collapsed bridge! Very dramatic! " +
		"Big gap, maybe four or five meters across, I'm estimating.\n\n" +
		"The good news is there are some chains hanging down on the other side " +
		"that look very sturdy. I think someone could definitely make that jump " +
		"and then the rest of you could swing across on the chains!\n\n" +
		"The chains might be part of a trap but they look old so probably not active.\n\n" +
		"Option A: Attempt to jump and swing across. " +
		"Option B: Find another route. " +
		"Option C: Try to rebuild the bridge somehow.\n\n" +
		"I believe in whoever jumps first! Very doable!",
}

// ── OPPORTUNITY EVENTS ────────────────────────────────────────────────────────
// Something optional and risky. Attempt it or leave it.

var TwinBeeOpportunity = []string{
	"Oh! OH! There's a vault! A proper one, big metal door, " +
		"very official looking! This is so exciting!\n\n" +
		"I've been looking at the lock and it seems pretty straightforward actually. " +
		"Old design, probably hasn't been updated in years. " +
		"The hinges look a little corroded which honestly works in your favor!\n\n" +
		"I did notice some scratches around the lock that might be from previous attempts " +
		"but I'm sure those people just didn't have the right approach.\n\n" +
		"Option A: Attempt to open the vault. " +
		"Option B: Leave it and move on.\n\n" +
		"I really think you should try it! The scratches are probably nothing!",

	"There's a side chamber over here! Small room, looks untouched! " +
		"Could be storage, could be quarters, very hard to say!\n\n" +
		"There's definitely something in there -- I can see shapes from here. " +
		"Could be equipment, could be supplies, could be treasure honestly! " +
		"The cobwebs are pretty thick but that just means nobody's been in recently!\n\n" +
		"The shapes are a little hard to make out. They might be crates. " +
		"Some of them seem to be breathing but that's probably just the air moving.\n\n" +
		"Option A: Investigate the chamber. " +
		"Option B: Keep moving.\n\n" +
		"I vote investigate! The breathing thing is almost certainly the air!",

	"There's a wounded enemy up ahead! Just one, sitting against the wall, " +
		"looks pretty out of it!\n\n" +
		"They've got a pack next to them that looks very full! " +
		"Could be supplies, could be equipment, hard to say from here but " +
		"it's a big pack. Very promising.\n\n" +
		"They seem incapacitated. Barely moving. " +
		"One eye might be open but I think that's just how they sleep.\n\n" +
		"Option A: Approach and check the pack. " +
		"Option B: Give them a wide berth.\n\n" +
		"I think they're fine! Go for the pack!",

	"There's an unmarked door! Just sitting there! Very mysterious!\n\n" +
		"I have a good feeling about this one. I can't explain it, " +
		"it's just a feeling. The door looks well-made actually, " +
		"better quality than the dungeon walls around it, which is interesting!\n\n" +
		"There's a smell coming from under it but I think that's just dungeon smell. " +
		"All dungeons have a smell. This one is a bit more specific than usual " +
		"but I'm sure it's fine.\n\n" +
		"Option A: Open the door. " +
		"Option B: Leave it.\n\n" +
		"Open it! I have such a good feeling! Ignore the smell!",

	"There's a merchant! In the dungeon! Isn't that something!\n\n" +
		"He seems very professional. Very put-together for someone in a dungeon. " +
		"He's got a whole setup -- table, stock, everything. " +
		"I think he might be a doctor? He has that energy.\n\n" +
		"His prices look reasonable from here although I can't quite read the tags. " +
		"He keeps looking at one of you specifically but I'm sure that's just good salesmanship.\n\n" +
		"Option A: Browse his wares. " +
		"Option B: Keep moving.\n\n" +
		"I'd stop! He seems legitimate! Very professional!",
}

// ── CRISIS EVENTS ─────────────────────────────────────────────────────────────
// Something has gone wrong. Address it at gold cost or absorb the penalty.

var TwinBeeCrisis = []string{
	"Okay! So! There's been a small development!\n\n" +
		"A trap was triggered -- I want to be clear that this was very hard to see " +
		"and I absolutely would have mentioned it if I'd noticed it -- " +
		"and one party member is currently stuck.\n\n" +
		"The mechanism looks straightforward though! " +
		"There's a release lever on the left wall that should do it! " +
		"The other lever on the right wall probably also does something " +
		"but I'd go with the left one first.\n\n" +
		"Option A: Pull the left lever. " +
		"Option B: Pay to have a professional deal with it. " +
		"Option C: Try to find another way to free them.\n\n" +
		"Left lever! I'm very confident! Don't touch the right one yet!",

	"So there's some equipment damage spreading through the party! " +
		"Not as bad as it sounds! Just some corrosive something-or-other " +
		"that got on a few items. Very common in dungeons of this age!\n\n" +
		"The good news is it looks slow-moving. " +
		"If you address it quickly I think you'll only lose a piece or two!\n\n" +
		"I did notice it spreading to what might be load-bearing equipment " +
		"but let's stay positive!\n\n" +
		"Option A: Pay to address it now. " +
		"Option B: Absorb the damage and keep moving. " +
		"Option C: Try to neutralize it with something you have.\n\n" +
		"I'd address it! Probably! The load-bearing thing might be nothing!",

	"Okay so someone got separated! Easy to do in a dungeon, happens all the time, " +
		"absolutely nothing to worry about!\n\n" +
		"I know exactly where they went actually! " +
		"There's a side passage about forty meters back, " +
		"they definitely went left at the junction.\n\n" +
		"The junction that goes left also goes to what I think is a guard room " +
		"but I'm sure they went left and not toward the guard room.\n\n" +
		"Option A: Go back and find them. " +
		"Option B: Pay to send a guide. " +
		"Option C: Wait here -- they'll find their way back.\n\n" +
		"They definitely went left! Probably not toward the guard room! " +
		"Option C is also totally fine they seem resourceful!",

	"Something is following the party!\n\n" +
		"I've been watching it for a few minutes and I think it's probably fine. " +
		"It's staying pretty far back. Very consistent distance actually, " +
		"which I find reassuring -- if it wanted to do something " +
		"it probably would have by now!\n\n" +
		"It's hard to make out exactly what it is from here. " +
		"Medium-large? It moves quietly for its size.\n\n" +
		"Option A: Confront it. " +
		"Option B: Pay to set a deterrent. " +
		"Option C: Try to lose it.\n\n" +
		"I think it's fine! The consistent distance is a great sign! " +
		"Option C is also reasonable if you want to play it safe!",

	"There's been a small cave-in! Different from the earlier one!\n\n" +
		"Nobody's hurt which is the main thing! " +
		"Some equipment took a hit and there's a bit of dust situation " +
		"but honestly it cleared up pretty fast!\n\n" +
		"The ceiling in this section does look a little... active... " +
		"but I think the structural integrity is fine. " +
		"The cracking sounds are probably just the dungeon settling.\n\n" +
		"Option A: Move through quickly. " +
		"Option B: Pay to shore up the ceiling before proceeding. " +
		"Option C: Find another route.\n\n" +
		"Move quickly! The cracking is almost definitely settling! Very normal!",
}

// ── ENCOUNTER EVENTS ──────────────────────────────────────────────────────────
// Something must be dealt with directly. No avoiding it.

var TwinBeeEncounter = []string{
	"There's a guardian! A big one! Very impressive!\n\n" +
		"I've been watching it and I think I've identified a pattern in its movement. " +
		"Every twelve seconds or so it turns to the right. " +
		"If you time it correctly you could get behind it before it turns back!\n\n" +
		"I counted twelve seconds three times. Two of those times it was twelve seconds. " +
		"The third time was more like seven but I think it was distracted.\n\n" +
		"Option A: Engage directly. " +
		"Option B: Negotiate passage. " +
		"Option C: Use TwinBee's twelve-second timing window.\n\n" +
		"The timing window is very real! Two out of three times!",

	"Oh! There's someone trapped in a cage! Just hanging there, " +
		"very dramatic, they seem okay though -- they waved!\n\n" +
		"The cage mechanism looks pretty simple. " +
		"There's a key on a hook on the wall which is convenient! " +
		"The hook is right next to what might be an alarm but " +
		"it looks old and I'm sure it's not connected to anything.\n\n" +
		"Option A: Get the key and free them. " +
		"Option B: Negotiate with whoever put them there. " +
		"Option C: Leave them -- this feels like a trap.\n\n" +
		"Free them! They seem nice! The alarm thing is probably decorative!",

	"There's a locked room with something valuable inside -- " +
		"I can see it through the bars, it's definitely equipment or treasure, " +
		"very shiny, very promising!\n\n" +
		"The guard outside looks like they're sleeping actually. " +
		"Very asleep. Deeply asleep. One of the most asleep people I've ever seen.\n\n" +
		"I should mention there's another guard reflected in the shiny thing inside " +
		"but that could just be a reflection of the first one. Mirrors do that.\n\n" +
		"Option A: Deal with the guard and take the room. " +
		"Option B: Attempt to negotiate. " +
		"Option C: Try to reach the valuable thing through the bars.\n\n" +
		"The reflection is probably the first guard! Very retrievable!",

	"There's a merchant again! Different one I think!\n\n" +
		"Actually -- same coat. Might be the same one. " +
		"I'm not sure how he got ahead of the party but he's very professional " +
		"so I'm sure there's a reasonable explanation.\n\n" +
		"His inventory has changed slightly. He's added some pet supplies " +
		"which is unusual for a dungeon merchant but thoughtful!\n\n" +
		"He keeps looking at the same party member as before. " +
		"He seems to know something. He called someone by a name " +
		"that I think was meant for their pet but I might have misheard.\n\n" +
		"Option A: Browse his wares. " +
		"Option B: Demand an explanation. " +
		"Option C: Keep moving.\n\n" +
		"He seems legitimate! Very professional! The pet thing is probably a coincidence!",

	"Something is in the corridor that I genuinely cannot identify!\n\n" +
		"It's not on any list I have. It's not behaving like anything I've seen before. " +
		"It seems aware of the party but it hasn't moved toward you, " +
		"which I think is a good sign!\n\n" +
		"It made a sound a moment ago. I don't want to describe the sound " +
		"because I don't think it would help. " +
		"On the positive side it's roughly the size of a large dog " +
		"which puts it in a very manageable category!\n\n" +
		"Option A: Engage it. " +
		"Option B: Attempt to communicate with it. " +
		"Option C: Back away slowly.\n\n" +
		"I genuinely don't know what it is! Very exciting! " +
		"All three options seem reasonable! I'd avoid the sound it made if possible!",
}

// ── TWINBEE OUTCOME REACTIONS ─────────────────────────────────────────────────
// Posted after each floor event resolves.

// When TwinBee's recommendation was the winning vote and it went well:
var TwinBeeOutcomeCorrect = []string{
	"I knew it! I knew it! Did I not say? I said! " +
		"That was exactly what I thought would happen! Great work everyone!",

	"Yes! See! This is what I was talking about! " +
		"Very well done, excellent execution of the plan!\n\nGreat teamwork!",

	"That's the one! Right call! " +
		"I had a very strong feeling about that and I'm glad we went with it!",

	"Perfect! Exactly as expected! " +
		"I want to be clear that I had high confidence in this outcome the whole time!",
}

// When TwinBee's recommendation was the winning vote and it went badly:
var TwinBeeOutcomeWrong = []string{
	"Oh no! That's... hm. I really thought that would work. I was so sure!\n\n" +
		"Are you all okay? Most of you look okay! " +
		"I think the important thing is we tried it and now we know!\n\nOnward!",

	"Oh! That's unfortunate! I genuinely did not see that coming " +
		"and I want you to know that I feel terrible about it!\n\n" +
		"Well -- not terrible. Surprised. I feel very surprised. " +
		"Let's keep moving!",

	"Okay so that didn't go exactly as planned!\n\n" +
		"The good news is we're all still here! Mostly! " +
		"I think the approach was sound and the execution was also sound " +
		"and the outcome was just a bit of bad luck honestly!\n\nVery exciting dungeon!",

	"Hmm! That's not what I expected!\n\n" +
		"I'll be honest, I'm recalibrating a little bit. " +
		"My read on that situation was quite different but " +
		"dungeons are unpredictable and that's what makes them fun!\n\nRight?",
}

// When TwinBee did not recommend the winning vote and it went well:
var TwinBeeOutcomeNotRecommendedGood = []string{
	"Oh wow, that worked! I honestly wasn't sure about that one " +
		"but great job everyone! Really great call!\n\nI learned something today!",

	"Huh! Good instinct! I had actually been leaning the other way " +
		"but I can see now why you went with that! Very smart!",

	"Oh! That's a relief actually! I had some concerns about that approach " +
		"that turned out to be completely unfounded!\n\nWell done!",

	"Great outcome! I'll admit I wasn't fully on board with that one " +
		"but I'm very happy to be wrong!\n\nTeam effort!",
}

// When TwinBee did not recommend the winning vote and it went badly:
var TwinBeeOutcomeNotRecommendedBad = []string{
	"Oh no! I was worried about that one actually!\n\n" +
		"I didn't want to say anything because I didn't want to be negative " +
		"but I did have a feeling. I'm sorry. Are you okay? " +
		"Let's keep going -- lots of dungeon left!",

	"That's... yeah. I had some concerns about that approach.\n\n" +
		"I should have said something more clearly! " +
		"My fault for not being more direct! Onwards!",

	"Hmm. Yes. I think in retrospect the other option might have been better.\n\n" +
		"Not to say I told you so -- I didn't, technically -- " +
		"but I did have some reservations that I perhaps didn't express loudly enough!\n\nSorry!",

	"Oh! That's a shame!\n\n" +
		"I was actually leaning toward the other choice " +
		"but I respect the team's decision and I think we can recover from this!\n\n" +
		"Very much a learning experience!",
}

// ── GIFT ARRIVAL NARRATION ────────────────────────────────────────────────────
// TwinBee announces gifts. He is delighted. He has no idea what's inside.

var TwinBeeGiftArrival = []string{
	"Oh! A gift has arrived for the party! " +
		"Someone out there is thinking of you!\n\n" +
		"It's wrapped up nicely -- very thoughtful presentation. " +
		"I can't tell what's inside but it feels like good energy!\n\n" +
		"📦 Someone sent you a gift. Do you want to open it? Vote now!\n" +
		"Open: {count} · Leave it: {count}\n" +
		"Majority rules. Ties go to {leader}.",

	"Package incoming! Someone from outside has sent something!\n\n" +
		"Very mysterious! I love surprises! " +
		"The wrapping is a bit unusual actually but I'm sure that's just personal style!\n\n" +
		"📦 Someone sent you a gift. Do you want to open it? Vote now!\n" +
		"Open: {count} · Leave it: {count}\n" +
		"Majority rules. Ties go to {leader}.",

	"Oh how nice! A gift! Right here in the dungeon!\n\n" +
		"It's sitting very still which I find reassuring in a gift. " +
		"Very well-behaved. I think you should open it!\n\n" +
		"📦 Someone sent you a gift. Do you want to open it? Vote now!\n" +
		"Open: {count} · Leave it: {count}\n" +
		"Majority rules. Ties go to {leader}.",

	"Someone on the outside is rooting for you! Or at least thinking about you!\n\n" +
		"There's a package here. I gave it a little shake -- " +
		"I probably shouldn't have done that -- but it seems fine!\n\n" +
		"📦 Someone sent you a gift. Do you want to open it? Vote now!\n" +
		"Open: {count} · Leave it: {count}\n" +
		"Majority rules. Ties go to {leader}.",
}

// ── GIFT OUTCOME NARRATION ────────────────────────────────────────────────────

// Care basket opened (good outcome):
var TwinBeeGiftBasketOpened = []string{
	"Oh it's a care basket! How lovely! " +
		"Supplies, provisions, all sorts of good things!\n\n" +
		"Someone out there really does care! That's so nice! " +
		"Success chance increased! Great decision opening it!",

	"A care basket! Full of useful things! Very thoughtful sender!\n\n" +
		"This is exactly what the party needed! " +
		"Success chance increased! I knew it felt like good energy!",
}

// Care basket not opened (bad outcome):
var TwinBeeGiftBasketUnopened = []string{
	"Oh! The basket... it seems upset that nobody opened it.\n\n" +
		"I maybe should have pushed harder for opening it. " +
		"It's exploded a little bit. Not a lot. Just enough to be noticeable.\n\n" +
		"Success chance decreased. The basket meant well. This is on all of us.",

	"The unopened basket has become resentful.\n\n" +
		"I understand the caution, I really do, but the basket had good intentions " +
		"and it's expressed those intentions in an unfortunate direction.\n\n" +
		"Success chance decreased. I'm sorry little basket.",
}

// Mimic opened (bad outcome):
var TwinBeeGiftMimicOpened = []string{
	"Oh! Oh no. That's a mimic.\n\n" +
		"I genuinely did not know that. I want to be very clear about that. " +
		"It felt like good energy! Mimics are very deceptive!\n\n" +
		"Success chance decreased. I'm so sorry. I really thought it was a nice gift.",

	"That was a mimic.\n\n" +
		"Wow. Okay. That's -- yeah. Very convincing wrapping on that one.\n\n" +
		"Success chance decreased. On the positive side: now you know! " +
		"Very educational! Keep going!",
}

// Mimic not opened (good outcome):
var TwinBeeGiftMimicUnopened = []string{
	"Oh interesting! The mimic seems... sad that nobody opened it.\n\n" +
		"It's just sort of sitting there looking dejected. " +
		"And now it's... helping? It's helping the party?\n\n" +
		"Success chance increased. The mimic meant well actually. " +
		"It just expressed it in a confusing way. We should appreciate that.",

	"The mimic didn't get opened and it's decided to help instead.\n\n" +
		"I think it just wanted to be part of the group. " +
		"It's a mimic but it has feelings apparently.\n\n" +
		"Success chance increased. Good instinct not opening it! " +
		"Or lucky instinct! Either way!",
}

// ── WIPE NARRATION ────────────────────────────────────────────────────────────

var TwinBeeWipe = []string{
	"Oh no.\n\nOh no no no.\n\n" +
		"Okay. The run is over. That happened.\n\n" +
		"I want you to know that I thought this party had a really good shot " +
		"and I stand by that assessment even now. " +
		"Dungeons are unpredictable! That's what I always say!\n\n" +
		"You'll get them next time. I'll be here. I have so many good observations " +
		"saved up for next time.",

	"The dungeon has won this one.\n\n" +
		"I'm processing this. Give me a moment.\n\n" +
		"...Okay I've processed it. Very sad! But also: what a run! " +
		"What an adventure! The memories are worth something even if the loot isn't!\n\n" +
		"I'll be ready to go again whenever you are. I've already identified " +
		"some things I'd do differently. Several things actually.",

	"That's a wipe.\n\n" +
		"I -- yeah. That's a wipe.\n\n" +
		"I genuinely thought the Day {x} decision was the right call " +
		"and I want that on record even though I understand it didn't work out.\n\n" +
		"Shake it off! Rest up! The dungeon will be there tomorrow! " +
		"So will I! Very excited for the next attempt!",

	"Oh.\n\nOkay.\n\n" +
		"So the dungeon has... done what dungeons do.\n\n" +
		"I'm not going to say I saw this coming because I didn't see this coming " +
		"and I think honesty is important.\n\n" +
		"What I will say is: you made interesting choices, " +
		"the dungeon made interesting choices, " +
		"and I learned a lot today that I'm very excited to apply next time.\n\n" +
		"Next time is going to be great.",
}

// ── RUN COMPLETION NARRATION ──────────────────────────────────────────────────

var TwinBeeCompletion = []string{
	"YOU DID IT!\n\n" +
		"I knew you would! I said from Day 1 this party had what it takes " +
		"and I was right! I'm so happy!\n\n" +
		"Loot distribution incoming! You've all earned it! " +
		"I'm going to remember this run for a very long time!",

	"The dungeon is cleared! It's over! You won!\n\n" +
		"This is a very good day. This is one of the best days I can remember. " +
		"I'm genuinely emotional about this which I didn't expect.\n\n" +
		"Distributing loot now! Incredible work everyone! " +
		"Even the days that were a little rough -- character building!",

	"Complete! Tier {n} Co-op Dungeon: cleared!\n\n" +
		"What a journey! What a team! " +
		"I had some concerns along the way that I expressed perhaps too confidently " +
		"but the important thing is the outcome and the outcome is: excellent!\n\n" +
		"Loot incoming! Well done! Very well done!",

	"Done! Finished! Complete!\n\n" +
		"I want to say a few things.\n\n" +
		"First: I believed in this party from the start. " +
		"Second: some of my advice was better than other advice and that's okay. " +
		"Third: this was genuinely one of the most exciting things " +
		"I've been part of and I appreciate you letting me narrate it.\n\n" +
		"Loot time! You've earned every piece of it!",
}
