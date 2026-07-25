// twinbee_expedition_flavor.go
// TwinBee GM Dialogue — Expedition-specific narration lines.
// Multi-day and multi-week adventure events, morning briefings,
// evening recaps, temporal events, and long-arc narrative moments.
//
// Voice convention: TwinBee speaks in first-person or implicit
// subject. NO third-person "TwinBee [verb]" references — that
// pattern was retired in the Phase B2 voice pass. When adding new
// entries, keep the existing personality (clipped, observational,
// ledger-minded) but stay in first-person / implicit voice.
// Add new entries freely. Don't shorten existing entries.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// EXPEDITION START
// ─────────────────────────────────────────────────────────────────────────────

var ExpeditionStart = []string{
	"Manifest reviewed. Supplies: checked. Equipment: checked. The part of you that's wondering if this is a good idea: noted, and set aside. We begin.",
	"The dungeon has been there for a long time. It will be there for a long time after. The question is what happens in the middle, and that's where you come in. I'm ready.",
	"An expedition. Not a run — an expedition. There's a difference. I'll explain the difference over the coming days and the explanation will be mostly experiential.",
	"Horizon checked, then supplies, then you. In that order. 'Alright,' I say, with the quiet energy of something that has been looking forward to this. 'Let's go.'",
	"You're not here for a quick visit. I know the difference between someone passing through and someone committing. You're committing. I appreciate the commitment.",
	"Like starting Dragon Quest VII. Two hours of errands before the game permits a single fight, and the people who love it love it for exactly that. This is going to take a while. I have found a comfortable position. I suggest you do the same.",
}

// ─────────────────────────────────────────────────────────────────────────────
// EXPEDITION START — BOREDOM (the adventurer left without you)
//
// Fired by the boredom ticker after a long silence. The player is not
// reading this live; it's a note left on the table. Deadpan, faintly
// reproachful, never cruel — and never pretending the gear got checked,
// because it didn't (gogobee_boredom_plan.md §5).
// ─────────────────────────────────────────────────────────────────────────────

var ExpeditionBoredomStart = []string{
	"You didn't come. That's alright — it happens, and I'm not going to make it a thing. But the sword was getting heavy on the wall and I've packed what we had. Which was not much. Noted for the record, not as a complaint.",
	"I waited. Then I waited past the point where waiting was the sensible option, and somewhere in there the waiting turned into leaving. We're going. Same kit as last time, because last time is when you last touched it.",
	"Here's the situation: there's a dungeon, there's daylight, and there's nobody telling me not to. I've made a decision. I hope it was the one you'd have made, though I concede I have no way of checking.",
	"Supplies: the cheapest available. Equipment: whatever was already on the rack. Plan: walk in, see what happens. I'm aware of how that sounds. I'm going anyway.",
	"The gear hasn't moved since you left it. I checked. I checked twice, actually, in case the first check was wrong, and it wasn't. So we go as we are — which is to say, as we were.",
	"Restlessness is not a stat I can show you on the sheet, but it accumulates, and it has. Off we go. Lightly provisioned and unimproved, but off.",
	"I've done the arithmetic on standing still and it doesn't come out well. So: a dungeon, one supply pack, and the same armour that's been good enough up to now. 'Good enough' is doing a lot of work in that sentence.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MORNING BRIEFINGS — Generic
// ─────────────────────────────────────────────────────────────────────────────

var MorningBriefingGeneric = []string{
	"Another day in the dungeon. Morning count: you, me, the rations, one sword, and zero regrets logged before breakfast. Regrets logged before breakfast are the only ones that stick. Clean sheet. Out we go.",
	"Morning. The dungeon has been quiet since you camped. Relative to what a dungeon considers quiet, which is not what you'd consider quiet, but everyone adjusts.",
	"I've been watching the entrance to the camp since approximately three in the morning. Nothing came. Mentioned casually; no particular reaction expected.",
	"Day [N]. The numbers are climbing. There's something satisfying about the numbers climbing — it means you're still here, which is always the first thing to confirm.",
	"You wake up and I'm already there, which is my nature. 'Good,' I say, by which I mean: you're alive, the day can proceed, there is much to do.",
	"The morning check: HP adequate, supplies within tolerance, Threat Clock where it was when you slept plus the overnight drift. I've already run the numbers. I'll share them in a moment.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MORNING BRIEFINGS — By Day Range
// ─────────────────────────────────────────────────────────────────────────────

var MorningBriefingDay1 = []string{
	"First morning. The dungeon looks exactly like it looked yesterday, which is expected. You look slightly more prepared than you did yesterday, which is less expected and entirely welcome.",
	"Day one complete. Tally: you entered, you explored, you survived the night. The bar was not high. You cleared it. I build from here.",
	"Morning of day two. The first night is filed under 'survived' and I'm already running the second day's plan. New room types likely. New encounter shapes. The pace begins to differentiate from the practice version of itself.",
	"Day one is behind you. The day's notes are already organized — what worked, what nearly didn't, what you'll do differently from here. The dungeon is also taking notes on you. Everyone is preparing.",
}

var MorningBriefingDay3 = []string{
	"Day three. The dungeon has had time to notice you. I've had time to notice the dungeon noticing you. The Threat Clock reflects both observations.",
	"Three days in. You've found your rhythm — I can see it in the way you move through rooms now, the way you check corners. The dungeon is learning too. Both noted.",
	"Day three morning. The dungeon's posture has shifted noticeably overnight — I saw it in the corridor noise around the second watch. The Threat Clock is doing what it does. You're doing what you do. The two of you are now both doing it on purpose.",
	"Three days. The dungeon recognizes you as a recurring fixture now, which is a different kind of attention than the kind you had on Day 1. I'm adjusting the briefings accordingly. Less orientation, more situational reads. From here it gets specific.",
}

var MorningBriefingDay7 = []string{
	"One week. You have spent one week in this place and it has not finished you, which says something about you that I intend to say out loud: that took something real. Take a moment with that. Then advance.",
	"Seven days. In the old reckoning, seven was the number of completion — seven seals, seven trials, seven nights before the thing reveals itself. I'm not superstitious. I'm also watching the door very carefully this morning.",
	"A week underground. I think about the sky sometimes — not with longing, exactly, more as a reference point. You've been below it for seven days. Remarkable. You, more so.",
}

var MorningBriefingDay14 = []string{
	"Two weeks. I don't know many people who have been in an active dungeon for two weeks. I know even fewer who have been in one for two weeks and remained, by any reasonable measure, intact. You are one of the fewer.",
	"Fourteen days. The dungeon has become familiar in the way that difficult things become familiar — not comfortable, not safe, but known. You know where it breathes. An advantage worth having.",
	"Two weeks. I start the briefing the same way I've started fourteen others, then pause and add: 'For the record — most expeditions don't see fourteen briefings. This one has.' That's the addendum. Continuing.",
	"Day fifteen. I've stopped checking the historical reference materials because the historical reference materials stopped applying. From here, the run sets its own precedent. I find this clarifying.",
}

var MorningBriefingDay21 = []string{
	"Three weeks. The record book keeps a page for runs past twenty days. The page has three entries. One is you. One is a dwarf named Hensel. The third entry is water-damaged, and I have chosen to believe it also says Hensel.",
	"Day twenty-one. I tried to write a clever framing for this morning's briefing and gave up halfway through, settling instead on the simplest version: 'You're still here.' That's the briefing. The rest is logistics.",
	"Three weeks down. The dungeon has stopped being a place you're visiting and become a place you live in for now. I note the shift — the way you check rooms without being asked, the way the supply count is already in your head. The dungeon notices too.",
}

// ─────────────────────────────────────────────────────────────────────────────
// EVENING RECAPS — Generic
// ─────────────────────────────────────────────────────────────────────────────

var EveningRecapGeneric = []string{
	"End of day [N]. Ledger tallied. The column marked 'survived' has another entry. I consider this column the most important one.",
	"Day closes. The ledger says: three rooms, one fight you picked, one fight that picked you, and a door you had the sense to leave shut. I have audited worse days. I have audited far worse doors.",
	"Evening. The rooms behind you are cleared. The rooms ahead are not. Always true; never less relevant. Rest now. The math doesn't change overnight.",
	"I compile the day: what was learned, what was fought, what was found. File it in the mental ledger I've been keeping since you entered. The ledger is favorable.",
	"Like the experience screen at the end of a dungeon floor in Etrian Odyssey — the numbers settle, the progress registers, and for a moment the whole thing makes sense. I give you that moment.",
	"You've earned the dark. Sleep in it. I'll be here when the numbers come back.",
}

// ─────────────────────────────────────────────────────────────────────────────
// EVENING RECAPS — Notable Days
// ─────────────────────────────────────────────────────────────────────────────

var EveningRecapBossKilled = []string{
	"The day ends with a boss on the floor. I'd like to specifically note this in the recap as 'exceptional' because it is exceptional and I don't use that word loosely.",
	"End of day. Boss count: one more than it was this morning. Marked in a special column I keep for exactly these entries, and the special column has a new line.",
	"End of day. A boss is no longer in the dungeon. Recorded in the column I keep for exactly that line, and the column has a new mark, and that's the kind of recap I like to write. Sleep well. The rest of the dungeon noticed too.",
	"The day closes with a vacancy. The thing in the chamber is no longer in the chamber. I ran the final tally during the fight and the tally is favorable. There will be more things tomorrow. There always are. None of them are this one.",
}

var EveningRecapCloseCall = []string{
	"Evening recap. Note: you came very close today to not being here for the evening recap. Said without drama and with complete sincerity. You made it. That's the recap.",
	"Today was the kind of day I file under 'let's not do that again' and also 'and yet you did it.' Rest. You need it more tonight than most nights.",
	"Evening recap. Noted for the ledger: today was very nearly a different kind of recap, and I'm glad it isn't. The margin was thinner than the acceptable range. Tomorrow's caution adjusts accordingly.",
	"End of day, narrowly. I run through the close-call list — the round that nearly went the other way, the save that landed by one, the room you almost didn't leave — and sign each one off as 'survived.' Survived is a wide category. I'll accept you in any part of it.",
}

var EveningRecapNothingHappened = []string{
	"A quiet day. No major encounters, no notable finds. Reported with neither disappointment nor relief. Quiet days are data. The dungeon is thinking. I'm watching it think.",
	"Not every day in a dungeon produces a story. Today produced logistics: movement, supplies, positioning. I value logistics. Logistics is what you still being alive looks like from a distance.",
	"Day closes uneventfully. Uneventful days are the kind the dungeon is most carefully constructing — quiet between incidents is also a kind of incident. Filed under 'preparation, theirs.' Yours starts now.",
	"Nothing happened. I write that exact phrase in the recap and then cross it out and write 'something happened that wasn't visible.' That feels truer. The day still counts. The day still cost supplies. Everything still moves forward.",
}

// ─────────────────────────────────────────────────────────────────────────────
// CAMP ESTABLISHMENT
// ─────────────────────────────────────────────────────────────────────────────

var CampEstablished = []string{
	"Camp established. I walk the perimeter once for threats and once for acoustics, because a camp that echoes is a camp that advertises. This one holds its sound. Approved.",
	"The camp goes up. I approve of the location — cleared room, defensible entry, no obvious curse residue. Could be worse. I've seen worse.",
	"You set camp. I check the sightlines, the doors, the sound-bleed from the next room. Acceptable. Settling in for the night watch.",
	"A camp in the middle of a dungeon. Either brave or pragmatic; I've stopped trying to distinguish between the two. Either way, the camp is set. Either way, I'm watching.",
	"Like planting a flag on a new map in Dwarf Fortress — this spot is yours now. Tentatively. Provisionally. For the night. I defend tentative, provisional, nocturnal property with full commitment.",
}

var BaseCampEstablished = []string{
	"Base camp. I say this differently than I say other things. With weight. A base camp in a Tier 4 zone is not nothing — it's a declaration. You're not passing through. You're operating from here. The perimeter goes up accordingly.",
	"The base camp is up. Waypoint noted, return route marked, supply cache protocols established. This is now home, in the way that a forward operating position is home — functional, defended, temporary, and completely yours.",
	"Base camp established on Day [N]. I record this as a milestone, because it is one. Most people don't make it to base camp. You are not most people. I've been noting this since the beginning.",
}

// ─────────────────────────────────────────────────────────────────────────────
// SUPPLY WARNINGS
// ─────────────────────────────────────────────────────────────────────────────

var SupplyWarningLow = []string{
	"I check the supply manifest and then check it again. 'We should discuss the supply situation,' I say, in the tone of someone who has been watching the number for two days.",
	"Supplies are running lower than I'd like. Mentioning it now, while there are still options, because I've seen what happens when it's mentioned too late.",
	"The supply number is not comfortable. Flagging this not to alarm but to prompt — there are decisions to be made and they are better made with time than without it.",
}

var SupplyWarningCritical = []string{
	"I hold up the supply manifest. The number is very small. 'This is the part,' I say quietly, 'where we start making decisions.' I don't specify which. You know which.",
	"Critical supply levels. Delivered without inflation — the situation is what it is. Extract and resupply, or forage aggressively and push for the finish. I outline both paths and neither is comfortable.",
	"The supplies are nearly gone. I think of every long JRPG dungeon where you realize at the bottom floor that you're out of Ethers. This is that moment. I have plans. They require movement.",
}

var SupplyDepletedExtraction = []string{
	"The supplies are gone. Said plainly because the situation requires plain language. The expedition ends here — not in failure, in logistics. You push out as far as the provisions allow. What you've gathered comes with you. I lead the way out.",
	"Empty packs. I turn them out by reflex and confirm what the manifest already said. The expedition does not continue past supplies — that's the rule, and I enforce it on you the way I enforce it on myself. Out we go. With what we have. Which is something.",
	"Out of supplies. I lead the extraction along the route already mapped — there's no scouting now, just movement, the kind that gets you through the door before anything else does. The dungeon will be here. So will I. So, importantly, will you.",
}

// ─────────────────────────────────────────────────────────────────────────────
// THREAT CLOCK NARRATIONS
// ─────────────────────────────────────────────────────────────────────────────

var ThreatClockStirring = []string{
	"Something in the dungeon's rhythm has changed. A tension in the air that wasn't there yesterday. The zone is aware of you now, in the way that a predator becomes aware of something in its territory. Not panicking. Not moving yet. Just aware.",
	"The enemies ahead are more alert than they were. I can tell by the patrols, by the spacing, by the fact that someone moved the tripwire you already disarmed. They know something is in here.",
	"Stirring. I track the small changes the dungeon makes when it suspects but doesn't confirm — repositioned patrols, fresher footprints, a torch that wasn't lit in this corridor before. Each one noted. None of them is alarming yet. All of them are evidence.",
	"The dungeon is getting curious. I distinguish curiosity from alertness — curiosity is when the patrols slow at the doorways; alertness is when they form on the doorways. We're at the first one. The window between them is what I'm watching.",
}

var ThreatClockAlert = []string{
	"The zone is on alert. Reported factually and also with a note of urgency — alert means organized, and organized means the next room will be harder than the last room was. Move with intention.",
	"They're coordinating now. I watch the patrol patterns shift. The goblins who were arguing about ambush order are no longer arguing. Not an improvement.",
	"Alert band. The dungeon has confirmed a presence and is acting on the confirmation. I can see it in the way the patrols overlap deliberately now — no more arguments about coverage, just coverage. The plan adjusts: no more long approaches, no more leaving anything unfinished behind.",
	"They've arranged themselves. Band-shift confirmed, and the operational change condenses to one sentence: from here, every room costs more than the last one. Nothing gets cheaper. Price accordingly.",
}

var ThreatClockHostile = []string{
	"Full hostile status. Said with the specific weight it deserves. The zone has decided you are the problem it's solving today. You have the same opinion about the zone. One of you is correct. I'm invested in it being you.",
	"The dungeon has mobilized. Re-armed traps, reinforced positions, an enemy in a room that was empty this morning. They've organized. Recommend organizing faster.",
	"Hostile. I name the band and then set the briefing aside because the briefing has been overtaken by events. The dungeon is now actively committed to ending this. I'm committed in the opposite direction. The clarification is useful.",
	"The dungeon has decided. I respect decisions, even ones that involve the dungeon trying to kill you. The respect is a working respect. We respect the decision and then we work to overrule it. I lead.",
}

var ThreatClockSiege = []string{
	"Siege Mode. Delivered without decoration because decoration would be dishonest. The dungeon is fully active, fully aware, and fully committed to ending this expedition. So am I — to ending it on your terms, not theirs. What happens next is a race. Already running.",
	"Siege. The dungeon is putting everything it has into the room you're standing in, and the rooms adjacent to it, and the route between you and the door. I'm putting everything I have into making sure the dungeon doesn't get what it wants. Meet in the middle.",
	"Siege Mode. I narrate the band shift and then stop narrating because narration is not what this band needs. Action is what this band needs. I'm in motion. The expectation is that you are too.",
}

// ThreatClockApproachingSiege fires once when the threat clock crosses 70 —
// the spec's "begin warning" line (§8.3). Distinct from the Hostile-band
// flavor because this is the dungeon-design moment of telling the player
// they're past the point where stealth is recoverable.
var ThreatClockApproachingSiege = []string{
	"They know you're here. Not a suspicion anymore. A certainty. The question now is whether you finish before they organize. Said clearly so it doesn't have to be said again.",
	"Threat at seventy. Marked on the internal ledger and underlined. The window for quiet operations has closed. The window for finishing is still open — narrower, but open. Use it.",
	"The dungeon's posture has shifted from 'searching' to 'hunting.' I track the difference precisely: before, they were looking for evidence; now they are looking for you. The plan, accordingly, simplifies. Finish or extract. Middle paths have closed.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ZONE TEMPORAL EVENTS
// ─────────────────────────────────────────────────────────────────────────────

var SunkenTempleTidalWarning = []string{
	"I watch the waterline. 'It's rising,' I say, not for the first time, and this time with more precision. 'The tidal cycle peaks in two days. Whatever you haven't done by then, you'll be doing wet.'",
	"Day [N]. The tide is coming. I've been watching it since Day 2 and the mathematics are not encouraging. Two days to the peak. One day after that, the flood subsides. Use the two days well.",
	"I check the waterline against yesterday's mark. The mark is below the line. The line is on the wall. The wall is, frankly, full of marks. I select the one that matters and report: tide rising, peak still days off, plan accordingly.",
	"Day [N]. The temple's tidal calendar advances. I keep it on a card and update the card every morning — current depth, projected depth, peak day. Internalize the card. The temple does not pause for confusion.",
}

var SunkenTempleTidalEvent = []string{
	"The tide arrives. I'd warned about this and now it is happening and the warning feels entirely inadequate compared to the actual water. Everything is colder, wetter, and the Kuo-toa are moving through it like they were born to — which they were. Adjust.",
	"The water is here. Not 'rising' anymore — here, in the rooms, at the corners, climbing the stairs. Every estimate adjusts accordingly: travel slower, encounters worse, healing colder. Two days of this. Two days, then it goes.",
	"Tidal peak. The temple is fully flooded at depths that were dry yesterday — I map the change room by room and update the route. Some doors are open that weren't. Some are sealed that were. The temple is a different shape today.",
}

var HauntedManorResetMorning = []string{
	"The morning briefing includes an addendum. The rooms that were clear yesterday are not entirely clear this morning. The house has been busy overnight. Adding it to the log under 'things the house does' and suggesting we adjust the advance plan.",
	"Night three. The manor reset itself. I was watching and it happened anyway — not violently, not dramatically, just quietly and completely, the way the house does everything. One enemy per room, back in place. Map updated.",
	"I read the morning's hallway and stop. 'The arrangement has changed,' I say, in the tone of someone who has now seen this happen often enough to recognize it without alarm. The manor reset. Map adjusts. Morning continues.",
	"Reset day. The rooms I cleared are not the rooms I find this morning. Accept it as a property of the manor — like weather, but indoors and unfair. The plan accommodates. The time gets billed.",
}

var UnderforgHeapWarning = []string{
	"Heat Stack [N]. I note the accumulation and what it means: the Underforge is getting into you in ways that don't resolve without real rest. The number has time to come down. I'm watching the number.",
	"The heat is building. I track it the way you track a temperature gauge on a long drive — with the specific alertness of someone who knows what happens when the gauge hits red. It has not hit red. I intend to ensure it doesn't.",
	"Heat building. Count noted, band noted, and how each step now takes a fraction more out of you than the last step did. None of it is critical. All of it is direction. The direction should be inverted soon.",
	"Stack [N]. I track the heat the way I track anything that compounds — patiently, with running totals, with a clear point at which the totals stop being managed and start being problems. We are still on the management side. The other side is visible.",
}

var UnderforgHeapCritical = []string{
	"Heat Stack is high. Delivered without minimizing it. The Underforge is inside your lungs now, in your joints, in the way everything takes a little more effort than it did on Day 1. A proper rest will help. Finishing will help more.",
	"Heat ten. The maximum. There is no higher band than this in the Underforge ledger — the column ends here, by design. The forge is fully inside you. The penalties are all of them. Resting will not undo it. Finishing might.",
	"Critical heat. I run through the symptoms list and confirm each one in turn: the joints, the breathing, the way every roll feels a little heavier. The forge is doing what the forge does. I suggest doing what you do, harder.",
}

var FeywildTimeDistortionHalf = []string{
	"The day moved strangely. I tried to track it and lost the thread somewhere around mid-afternoon — the light didn't change the way it should have, and when I looked up, the day was half over in the time it usually takes to be a quarter over. On the positive side: you're barely hungry. On the less positive side: nobody is sure what that means.",
	"Half a day passed in what felt like half of half. I check the supply burn against the sun, find them disagreeing, side with the supplies — those don't lie about how much you've used. Net result: a free pocket of time. Use it on something that takes time.",
	"The Feywild gave you back some hours. Noted, but not trusted. The Feywild does not give without taking, eventually. For now: less hungry, less tired, more daylight than the math allows. I write it down and keep moving.",
}

var FeywildTimeDistortionDouble = []string{
	"Time doubled. Noted with the clinical detachment of someone who has been in the Feywild long enough to stop being surprised. You've lived through two days today. The supplies reflect that. The wandering monsters reflected that as well. The rest of the Feywild is unconcerned.",
	"Two days in one. I run the recovery math and the wandering math and the supply math, and all three say the same thing: today cost double. Tomorrow gets the rate it gets. The Feywild does not refund.",
	"Time doubled. I saw the second sunset before the second lunch and stopped trying to make sense of it. The day that happened is the day that happened. Plan from here, not from this morning.",
}

var FeywildTimeLoop = []string{
	"I recognize this room. I've described it before. The enemies in it are different — new enemies, the old ones are gone, the loot you found is still gone but the enemies are back — and I process this with something between professional acceptance and profound exasperation. 'Again,' I say. 'We do this room again.'",
	"The loop. I mark the room on the map with a small symbol that means 'we have been in this room before in a way that doesn't count.' The symbol has its own column in my ledger. The column is occupied.",
	"Same room. New enemies. I run the encounter again with the cold professionalism of someone who has stopped expecting fairness from the Feywild and started expecting only repetition. The repetition arrives. Handled.",
}

var DragonsLairAwarenessPulse = []string{
	"Something changes in the mountain. Not a sound, exactly — more like a sound's absence, filling in differently than before. I watch the kobolds. The kobolds have stopped what they were doing. The kobolds are listening. Something told them to listen. I add ten to the Threat Clock and keep moving.",
	"Infernax shifts in its sleep. I feel it through the stone. The patrol rotations just changed — I can see it in where the Guard Drakes aren't anymore versus where they were an hour ago. The mountain's master is dreaming about you. That is not a comfortable thing to be dreamed about.",
	"The mountain pulses. I feel it as a single off-rhythm beat in the rock, the kind of beat that means something somewhere has shifted weight. The kobolds nearby pause. They look upward without looking. I adjust the threat accordingly and continue.",
	"Infernax dreamed about you. I have no other way to phrase it — the lair changed in the way places change when they're being reordered from somewhere deeper. The patrols are different. The temperature is different. Ten added to the clock and a note to the margin.",
}

var DragonsLairAwakenWarning = []string{
	"Day 14. The morning briefing comes out in a lower register than usual. 'Infernax is awake,' I say. 'I don't know when exactly — sometime in the last six hours. The patrols changed. The temperature changed. The silence changed.' A pause. 'We need to reach the final chamber before it reaches us. That is now the only plan.'",
	"Day fourteen. Infernax woke up in the night. I say the sentence flatly because the sentence is enough by itself; it does not need help. The rotation is to the final chamber. Everything else is decoration.",
	"The mountain changed in its sleep. I felt it through the floor sometime around the fourth watch and have been recalibrating since. Infernax is awake and Infernax is aware. Be neither subtle nor slow. There is no longer time for both.",
}

var AbyssPortalDestabilizationMid = []string{
	"Instability [N]. I watch the portal from a respectful distance and note: it is larger than it was yesterday. Not by much. Not in a way you'd notice if you weren't looking. I'm always looking.",
	"The portal is talking to itself. I have no better way to describe it — the light it emits is different at the edges now, like it's processing something. Process faster than it does.",
	"Instability rising. I run the comparison: yesterday's portal versus today's portal. The difference is not subtle to me. The difference may not be subtle to anything else, soon. Pace the work accordingly.",
	"The portal is widening, by some measure I'm using and not currently sharing because the measure doesn't have a clean name. The point is: it's worse than yesterday. The point is: tomorrow may be worse than that. I name the trend so the trend can be argued with.",
}

var AbyssPortalDestabilizationCritical = []string{
	"Instability critical. The portal is unraveling at edges I can see and probably at edges I can't. The demons coming through are more agitated than they were — which is relevant because demons at baseline are already at the upper end of agitated. The verdict: finish this today. Tomorrow is a different calculation.",
	"The portal is louder. Not in sound — in pressure. I can feel it in the back teeth, in the joint of the jaw. The instability number is the number you don't want it to be. Finish the work today or accept that today was the last day to.",
	"Critical. The geometry around the portal is bending in ways that suggest the room does not entirely agree with itself anymore. I adjust the route around the worst of it. There are more demons than there should be. There always are. There are more than that now.",
}

var AbyssPortalCollapse = []string{
	"The portal collapses. I watch it happen and do what I do in situations with no good options: move. 'Out,' I say, and mean it completely. 'Now. Everything you have, we move now.' The expedition ends here, not in defeat, but in physics. What you took is yours. What's left in there is the portal's problem. Come back when it isn't.",
	"It's coming apart. I say one word, 'Move,' in the tone I use exactly once per expedition. You move. The portal screams behind you in a register that isn't sound. The expedition is over because physics says so. The loot gets sorted once you're somewhere physics still works.",
	"Collapse. I was prepared for this and am also actively running. 'Out,' I say. 'Now.' The corridors are folding behind you in the way that things fold when reality is no longer paying attention to what's allowed. You finish out the door. The portal does not.",
}

// ─────────────────────────────────────────────────────────────────────────────
// REGION TRANSITION (multi-region zones; §11.3)
// ─────────────────────────────────────────────────────────────────────────────

var RegionTransitDeparture = []string{
	"I mark the boundary on the internal map and cross it. 'New region,' I say, with the careful attention of someone who knows boundaries in dungeons aren't always the same kind of boundaries you'd find on the surface. The route is set. The day is committed. We move.",
	"Crossing into [REGION_NEXT]. I fold the previous region's notes into the satchel and unfold new ones. The light changes. The air changes. The rules of how to be careful change a little. I adjust.",
	"The transit between regions is its own kind of room. I narrate it that way — sightlines, footing, what's behind, what's ahead — because the only way the in-between part stops feeling exposed is to treat it like the rest of the dungeon. Treated. Moving.",
}

var RegionTransitArrival = []string{
	"You arrive in [REGION_NEXT]. I survey, take in the new geometry, and update the working assumptions. 'Different shape,' I say. 'Same general principle. We learn what wants to kill us here, and we get there first.'",
	"[REGION_NEXT] receives you. I catalogue the differences: colder by about one coat, quieter by exactly one birdsong, and the dust on this side of the boundary shows a single set of tracks. The tracks are leaving. Noted. Adopting the local caution.",
	"Boundary crossed. The day gets stamped in the log — one full day spent in transit, supplies adjusted, the wandering that happened on the way handled and filed. We are here now. The next stretch is what it is.",
}

// ─────────────────────────────────────────────────────────────────────────────
// VOLUNTARY EXTRACTION
// ─────────────────────────────────────────────────────────────────────────────

var ExtractionVoluntary = []string{
	"Extraction. For the record, the ledger keeps two columns for expeditions that reached the door: 'left' and 'was left of.' You are in the first column. The first column is the good column.",
	"You call the extraction and I begin the route out immediately. No argument, no editorializing. There will be time for the debrief later. The first priority is the door.",
	"The dungeon doesn't like this. I can tell by the way the corridors feel as you head back out — a resistance that isn't structural, just atmospheric. The zone wanted more. It doesn't get more today. I lead the way.",
	"Withdrawing with intent. I catalogue what you have — the loot, the XP, the knowledge of where the rooms are for the return — and convert the exit into preparation. This isn't retreat. This is the start of the next attempt.",
}

// ─────────────────────────────────────────────────────────────────────────────
// FORCED EXTRACTION
// ─────────────────────────────────────────────────────────────────────────────

var ExtractionForced = []string{
	"I move. No recap, no analysis — that comes later. Right now there is a corridor and a door and getting you through both of them. Everything else waits.",
	"Out. Said once and meant completely. The dungeon tried to keep you. I decline on your behalf.",
	"The expedition ends here — not the way I wanted, not the way you wanted, but ended, and ending is sometimes the best available outcome. You'll come back. You'll know more. I'll be there.",
}

// ─────────────────────────────────────────────────────────────────────────────
// EXPEDITION RESUME (returning after extraction)
// ─────────────────────────────────────────────────────────────────────────────

var ExpeditionResume = []string{
	"Back. I noted the door when you left it and note it again now, from the inside. 'The dungeon is where you left it,' I say. 'Mostly.' The Threat Clock has some opinions about the time that passed.",
	"You came back. I'd calculated a probability on that and am pleased to update the calculation upward. The expedition resumes. The dungeon has had time to adjust. So have you.",
	"Like hitting Continue on a save file — the world remembers where you stopped, the enemies remember why they're there, and I remember every room and every roll. Resumes here. Advances from here.",
	"The supplies are new. The knowledge from last time is not. A significant advantage. The dungeon has the familiarity of a difficult level you've attempted before — you know where the hard parts are. That is not nothing. We build on that.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MILESTONE NARRATIONS
// ─────────────────────────────────────────────────────────────────────────────

var MilestoneFirstNight = []string{
	"You survived the first night. Somewhere around the third watch you rolled over, said a word I will not be repeating back to you, and slept on. The dungeon spent that same hour deciding you were not worth waking. Both of you were right.",
	"Night one survived. I make a small mark in the corner of the manifest — the kind of mark you make for the things that count more than they look. First nights count. I've been in dungeons where they were the last nights too. This wasn't one of those.",
	"Day two morning. The first night is behind you, which means the first watch is behind me, which means a thing worth confirming has been confirmed: you sleep through the noises that matter and wake for the ones that don't. That's a survival skill. Logged.",
}

var MilestoneWeekOne = []string{
	"Seven days. I pause the morning briefing for a moment — just a moment — to mark this. One week in an active dungeon zone is not a thing that happens by accident. It happens through every decision you've made since Day 1, compounded. Those decisions were watched. They were, gladly, yours.",
	"Day eight. The briefing comes slowly, because there's a thing to mark first: you have been in here, intact and operational, for a full week. That is a number with weight. I'm going to set the briefing down for a moment and let the number have its weight. ... Right. Briefing.",
	"One week. I make the mark in the column reserved for week-one survivors. The column is shorter than you'd expect. You are now on it. The rest of the day proceeds normally, but for one moment, I allow myself to be visibly impressed. The moment ends. We continue.",
}

var MilestoneTwoWeeks = []string{
	"Two weeks. I don't have a comparison for this one. The references have run out. There's just you, in here, on Day 14, still going, and me standing next to you having run out of everything except genuine admiration. That, in abundance. Proceed.",
	"Day fifteen. The historical comparisons stopped working at Day fourteen and have not resumed working today. We are off the chart. I'm not a person who values being off the chart, except in the very specific sense that you are off the chart, in which case I value it deeply.",
	"Fortnight. I use the older word because it sounds more like what this is — not 'two weeks,' which sounds modular and reasonable, but 'fortnight,' which sounds like the kind of duration that earns a title. I may be inventing titles for you. I'll be honest about that as it happens.",
}

var MilestoneTheLongGame = []string{
	"I set aside the narration format for a moment. Just set it down. Plainly: what you just did was not supposed to be survivable. The designers of this zone — the thing that shaped it, the evil that filled it — did not account for someone like you. I did. Always do. That's why I'm here.",
	"Tier five complete. I allow the narration format to fully break for a moment because the moment fully deserves it. What you just did, complete, in a Tier 5 zone, is a thing that goes on the short list. I keep a short list. You're on it.",
	"Long game closed. I gather the run notes, the threat curve, the supply records, the camp positions — file them all under your name in a folder I've been keeping. The folder has a title now. The title is good.",
}

var MilestonePatientZero = []string{
	"Expedition complete. Threat Clock: never above 50. I note this in a column I've had to use rarely enough that the column is almost fresh. Ghost protocol. You were here for the whole thing and the dungeon barely knew it until the end. I find this impressive and also slightly eerie and say both things sincerely.",
	"Threat never above fifty. I run back through the daily ledger and confirm it line by line. The dungeon never escalated to hostile because the dungeon never knew enough to. You moved through it like the dungeon's own quiet hour. I find this both technically impressive and slightly haunted.",
	"Patient Zero. The term fits: you were the thing the dungeon never noticed it had until you had already left. The Threat Clock has a column for every band you crossed. Most of those columns are empty for this run. I mark them empty. It's a good kind of empty.",
}

// MilestoneCartographer — awarded when the player searches every room before
// advancing the expedition. Combat-link wires the trigger; pool exists for the
// award narration to pull from.
var MilestoneCartographer = []string{
	"Every room searched. I was watching for it and watching for the corner-cutting that might have come instead, and the corner-cutting did not come. You searched everything. I approve with the quiet, specific approval reserved for completionism.",
	"Cartographer. I use the word the spec uses because the word fits — you mapped the place by being in every part of it. Every door checked, every corner walked. The next Elite room owes you a roll for that. I'll collect.",
	"Full coverage. I note the empty cells on the dungeon map, see there are none, and update the ledger accordingly. Every room visited, searched, accounted for. The dungeon has nowhere it kept to itself. A small but real victory.",
}

// MilestoneSurvivalist — awarded when a Tier 3+ expedition completes with no
// forced extractions in the run's history. Title flag + cosmetic deferred to
// item-grant hookup; this pool covers the narrative line at the moment it lands.
var MilestoneSurvivalist = []string{
	"Survivalist. I write the title next to your name in the ledger and underline it once. No abandonments, no scrambles for the door, no expeditions cut short by anything but the boss going down. The discipline gets acknowledged directly.",
	"The Survivalist title is technical — it means the run never broke; it ended on your terms, every time. I've been in expeditions where that wasn't true and remember them differently. This one gets remembered as: complete. Filed with the others like it. The folder is short.",
	"No forced extractions, full clear, Tier 3-or-better. I note each criterion separately because each one is its own choice, made repeatedly, across days. The result is the title, which is real, and the cosmetic, which is forthcoming. I'll hand both over when the system permits.",
}
