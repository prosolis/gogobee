// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// twinbee_expedition_flavor.go
// TwinBee GM Dialogue — Expedition-specific narration lines.
// Multi-day and multi-week adventure events, morning briefings,
// evening recaps, temporal events, and long-arc narrative moments.
// Add new entries freely. Never remove or alter existing entries.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// EXPEDITION START
// ─────────────────────────────────────────────────────────────────────────────

var ExpeditionStart = []string{
	"TwinBee reviews the manifest. Supplies: checked. Equipment: checked. The part of you that's wondering if this is a good idea: noted, and set aside. We begin.",
	"The dungeon has been there for a long time. It will be there for a long time after. The question is what happens in the middle, and that's where you come in. TwinBee is ready.",
	"An expedition. Not a run — an expedition. There's a difference. TwinBee will explain the difference over the coming days and the explanation will be mostly experiential.",
	"TwinBee checks the horizon, then the supplies, then you. In that order. 'Alright,' TwinBee says, with the quiet energy of something that has been looking forward to this. 'Let's go.'",
	"You're not here for a quick visit. TwinBee knows the difference between someone passing through and someone committing. You're committing. TwinBee appreciates the commitment.",
	"Like the opening screen of a long RPG — the kind that asks for your name and warns you to find a comfortable position because this is going to take a while. TwinBee has found a comfortable position. TwinBee suggests you do the same.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MORNING BRIEFINGS — Generic
// ─────────────────────────────────────────────────────────────────────────────

var MorningBriefingGeneric = []string{
	"Another day in the dungeon. TwinBee says this without resignation. The dungeon is still full of things worth doing and you are still the person to do them.",
	"Morning. The dungeon has been quiet since you camped. Relative to what a dungeon considers quiet, which is not what you'd consider quiet, but everyone adjusts.",
	"TwinBee has been watching the entrance to the camp since approximately three in the morning. Nothing came. TwinBee mentions this casually and expects no particular reaction.",
	"Day [N]. The numbers are climbing. TwinBee finds something satisfying about the numbers climbing — it means you're still here, which is always the first thing to confirm.",
	"You wake up and TwinBee is already there, which is the nature of TwinBee. 'Good,' TwinBee says, by which it means: you're alive, the day can proceed, there is much to do.",
	"The morning check: HP adequate, supplies within tolerance, Threat Clock where it was when you slept plus the overnight drift. TwinBee has already run the numbers. TwinBee will share them in a moment.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MORNING BRIEFINGS — By Day Range
// ─────────────────────────────────────────────────────────────────────────────

var MorningBriefingDay1 = []string{
	"First morning. The dungeon looks exactly like it looked yesterday, which is expected. You look slightly more prepared than you did yesterday, which is less expected and entirely welcome.",
	"Day one complete. TwinBee tallies: you entered, you explored, you survived the night. The bar was not high. You cleared it. TwinBee builds from here.",
	"Morning of day two. The first night is filed under 'survived' and TwinBee is already running the second day's plan. New room types likely. New encounter shapes. The pace begins to differentiate from the practice version of itself.",
	"Day one is behind you. TwinBee has the day's notes already organized — what worked, what nearly didn't, what you'll do differently from here. The dungeon is also taking notes on you. Everyone is preparing.",
}

var MorningBriefingDay3 = []string{
	"Day three. The dungeon has had time to notice you. TwinBee has had time to notice the dungeon noticing you. The Threat Clock reflects both observations.",
	"Three days in. You've found your rhythm — TwinBee can see it in the way you move through rooms now, the way you check corners. The dungeon is learning too. TwinBee acknowledges both.",
	"Day three morning. The dungeon's posture has shifted noticeably overnight — TwinBee saw it in the corridor noise around the second watch. The Threat Clock is doing what it does. You're doing what you do. The two of you are now both doing it on purpose.",
	"Three days. The dungeon recognizes you as a recurring fixture now, which is a different kind of attention than the kind you had on Day 1. TwinBee adjusts the briefings accordingly. Less orientation, more situational reads. From here it gets specific.",
}

var MorningBriefingDay7 = []string{
	"One week. You have spent one week in this place and it has not finished you, which says something about you that TwinBee intends to say out loud: that took something real. Take a moment with that. Then advance.",
	"Seven days. In the old reckoning, seven was the number of completion — seven seals, seven trials, seven nights before the thing reveals itself. TwinBee is not superstitious. TwinBee is also watching the door very carefully this morning.",
	"A week underground. TwinBee thinks about the sky sometimes — not with longing, exactly, more as a reference point. You've been below it for seven days. Remarkable. You, more so.",
}

var MorningBriefingDay14 = []string{
	"Two weeks. TwinBee does not know many people who have been in an active dungeon for two weeks. TwinBee knows even fewer who have been in one for two weeks and remained, by any reasonable measure, intact. You are one of the fewer.",
	"Fourteen days. The dungeon has become familiar in the way that difficult things become familiar — not comfortable, not safe, but known. You know where it breathes. TwinBee considers this an advantage worth having.",
	"Two weeks. TwinBee starts the briefing the same way TwinBee has started fourteen others, then pauses and adds: 'For the record — most expeditions don't see fourteen briefings. This one has.' That's the addendum. Continuing.",
	"Day fifteen. TwinBee has stopped checking the historical reference materials because the historical reference materials stopped applying. From here, the run sets its own precedent. TwinBee finds this clarifying.",
}

var MorningBriefingDay21 = []string{
	"Three weeks. TwinBee has run out of historical comparisons for this. Three weeks is its own category. You have made a category. TwinBee reports this as a fact and also as something that doesn't entirely have words yet.",
	"Day twenty-one. TwinBee tried to write a clever framing for this morning's briefing and gave up halfway through, settling instead on the simplest version: 'You're still here.' That's the briefing. The rest is logistics.",
	"Three weeks down. The dungeon has stopped being a place you're visiting and become a place you live in for now. TwinBee notes the shift — the way you check rooms without being asked, the way the supply count is already in your head. The dungeon notices too.",
}

// ─────────────────────────────────────────────────────────────────────────────
// EVENING RECAPS — Generic
// ─────────────────────────────────────────────────────────────────────────────

var EveningRecapGeneric = []string{
	"End of day [N]. TwinBee tallies the ledger. The column marked 'survived' has another entry. TwinBee considers this column the most important one.",
	"Day closes. TwinBee reviews what happened and finds, on balance, more right than wrong — which in a dungeon is the operating definition of a good day.",
	"Evening. The rooms behind you are cleared. The rooms ahead are not. TwinBee notes this is always true and is never less relevant. Rest now. The math doesn't change overnight.",
	"TwinBee compiles the day: what was learned, what was fought, what was found. Files it in a mental ledger that TwinBee has been keeping since you entered. The ledger is favorable.",
	"Like the experience screen at the end of a dungeon floor in Etrian Odyssey — the numbers settle, the progress registers, and for a moment the whole thing makes sense. TwinBee gives you that moment.",
	"You've earned the dark. Sleep in it. TwinBee will be here when the numbers come back.",
}

// ─────────────────────────────────────────────────────────────────────────────
// EVENING RECAPS — Notable Days
// ─────────────────────────────────────────────────────────────────────────────

var EveningRecapBossKilled = []string{
	"The day ends with a boss on the floor. TwinBee would like to specifically note this in the recap as 'exceptional' because it is exceptional and TwinBee does not use that word loosely.",
	"End of day. Boss count: one more than it was this morning. TwinBee marks this in a special column it keeps for exactly these entries and the special column has a new line.",
	"End of day. A boss is no longer in the dungeon. TwinBee records this in the column it keeps for exactly that line, and the column has a new mark, and that's the kind of recap TwinBee likes to write. Sleep well. The rest of the dungeon noticed too.",
	"The day closes with a vacancy. The thing in the chamber is no longer in the chamber. TwinBee ran the final tally during the fight and the tally is favorable. There will be more things tomorrow. There always are. None of them are this one.",
}

var EveningRecapCloseCall = []string{
	"TwinBee runs the evening recap and notes: you came very close today to not being here for the evening recap. TwinBee notes this without drama and with complete sincerity. You made it. That's the recap.",
	"Today was the kind of day that TwinBee files under 'let's not do that again' and also 'and yet you did it.' Rest. You need it more tonight than most nights.",
	"Evening recap. TwinBee will note for the ledger that today was very nearly a different kind of recap, and is glad it isn't. The margin was thinner than the acceptable range. Tomorrow's caution adjusts accordingly.",
	"End of day, narrowly. TwinBee runs through the close-call list — the round that nearly went the other way, the save that landed by one, the room you almost didn't leave — and signs each one off as 'survived.' Survived is a wide category. TwinBee accepts you in any part of it.",
}

var EveningRecapNothingHappened = []string{
	"A quiet day. No major encounters, no notable finds. TwinBee reports this with neither disappointment nor relief. Quiet days are data. The dungeon is thinking. TwinBee is watching it think.",
	"Not every day in a dungeon produces a story. Today produced logistics: movement, supplies, positioning. TwinBee values logistics. Logistics is what you still being alive looks like from a distance.",
	"Day closes uneventfully. TwinBee considers uneventful days the kind that the dungeon is most carefully constructing — quiet between incidents is also a kind of incident. TwinBee files the day under 'preparation, theirs.' Yours starts now.",
	"Nothing happened. TwinBee writes that exact phrase in the recap and then crosses it out and writes 'something happened that wasn't visible.' That feels truer. The day still counts. The day still cost supplies. Everything still moves forward.",
}

// ─────────────────────────────────────────────────────────────────────────────
// CAMP ESTABLISHMENT
// ─────────────────────────────────────────────────────────────────────────────

var CampEstablished = []string{
	"Camp established. TwinBee surveys the perimeter with the efficiency of someone who has done this many times and learned from every time it went wrong.",
	"The camp goes up. TwinBee approves of the location — cleared room, defensible entry, no obvious curse residue. Could be worse. TwinBee has seen worse.",
	"You set camp. TwinBee checks the sightlines, checks the doors, checks the sound-bleed from the next room. Acceptable. TwinBee settles in for the night watch.",
	"A camp in the middle of a dungeon. TwinBee finds this either brave or pragmatic and has stopped trying to distinguish between the two. Either way, the camp is set. Either way, TwinBee is watching.",
	"Like planting a flag on a new map in Dwarf Fortress — this spot is yours now. Tentatively. Provisionally. For the night. TwinBee defends tentative, provisional, nocturnal property with full commitment.",
}

var BaseCampEstablished = []string{
	"Base camp. TwinBee says this differently than it says other things. With weight. A base camp in a Tier 4 zone is not nothing — it's a declaration. You're not passing through. You're operating from here. TwinBee establishes the perimeter accordingly.",
	"The base camp is up. TwinBee notes the waypoint, marks the return route, establishes the supply cache protocols. This is now home, in the way that a forward operating position is home — functional, defended, temporary, and completely yours.",
	"Base camp established on Day [N]. TwinBee records this as a milestone, because it is one. Most people don't make it to base camp. You are not most people. TwinBee has been noting this since the beginning.",
}

// ─────────────────────────────────────────────────────────────────────────────
// SUPPLY WARNINGS
// ─────────────────────────────────────────────────────────────────────────────

var SupplyWarningLow = []string{
	"TwinBee checks the supply manifest and then checks it again. 'We should discuss the supply situation,' TwinBee says, in the tone of someone who has been watching the number for two days.",
	"Supplies are running lower than TwinBee would like. TwinBee mentions this now, while there are still options, because TwinBee has seen what happens when it's mentioned too late.",
	"The supply number is not comfortable. TwinBee flags this not to alarm but to prompt — there are decisions to be made and they are better made with time than without it.",
}

var SupplyWarningCritical = []string{
	"TwinBee holds up the supply manifest. The number is very small. 'This is the part,' it says quietly, 'where we start making decisions.' Doesn't specify which. You know which.",
	"Critical supply levels. TwinBee delivers this without inflation — the situation is what it is. Extract and resupply, or forage aggressively and push for the finish. TwinBee outlines both paths and neither is comfortable.",
	"The supplies are nearly gone. TwinBee thinks of every long JRPG dungeon where you realize at the bottom floor that you're out of Ethers. This is that moment. TwinBee has plans. They require movement.",
}

var SupplyDepletedExtraction = []string{
	"The supplies are gone. TwinBee says this plainly because the situation requires plain language. The expedition ends here — not in failure, in logistics. You push out as far as the provisions allow. What you've gathered comes with you. TwinBee leads the way out.",
	"Empty packs. TwinBee turns them out by reflex and confirms what the manifest already said. The expedition does not continue past supplies — that's the rule, and TwinBee enforces it on you the way TwinBee enforces it on TwinBee. Out we go. With what we have. Which is something.",
	"Out of supplies. TwinBee leads the extraction along the route already mapped — there's no scouting now, just movement, the kind that gets you through the door before anything else does. The dungeon will be here. So will TwinBee. So, importantly, will you.",
}

// ─────────────────────────────────────────────────────────────────────────────
// THREAT CLOCK NARRATIONS
// ─────────────────────────────────────────────────────────────────────────────

var ThreatClockStirring = []string{
	"TwinBee notices something in the dungeon's rhythm has changed. A tension in the air that wasn't there yesterday. The zone is aware of you now, in the way that a predator becomes aware of something in its territory. Not panicking. Not moving yet. Just aware.",
	"The enemies ahead are more alert than they were. TwinBee can tell by the patrols, by the spacing, by the fact that someone moved the tripwire you already disarmed. They know something is in here.",
	"Stirring. TwinBee tracks the small changes the dungeon makes when it suspects but doesn't confirm — repositioned patrols, fresher footprints, a torch that wasn't lit in this corridor before. TwinBee notes each one. None of them is alarming yet. All of them are evidence.",
	"The dungeon is getting curious. TwinBee distinguishes curiosity from alertness — curiosity is when the patrols slow at the doorways; alertness is when they form on the doorways. We're at the first one. The window between them is what TwinBee is watching.",
}

var ThreatClockAlert = []string{
	"The zone is on alert. TwinBee reports this factually and also with a note of urgency — alert means organized, and organized means the next room will be harder than the last room was. Move with intention.",
	"They're coordinating now. TwinBee watches the patrol patterns shift. The goblins who were arguing about ambush order are no longer arguing. TwinBee does not find this an improvement.",
	"Alert band. The dungeon has confirmed a presence and is acting on the confirmation. TwinBee can see it in the way the patrols overlap deliberately now — no more arguments about coverage, just coverage. The plan adjusts: no more long approaches, no more leaving anything unfinished behind.",
	"They've arranged themselves. TwinBee delivers the band-shift line and then condenses the operational change into one sentence: from here, every room costs more than the last one. Nothing gets cheaper. TwinBee suggests pricing accordingly.",
}

var ThreatClockHostile = []string{
	"Full hostile status. TwinBee says this with the specific weight it deserves. The zone has decided you are the problem it's solving today. You have the same opinion about the zone. One of you is correct. TwinBee is invested in it being you.",
	"The dungeon has mobilized. TwinBee notes re-armed traps, reinforced positions, an enemy in a room that was empty this morning. They've organized. TwinBee recommends organizing faster.",
	"Hostile. TwinBee names the band and then sets the briefing aside because the briefing has been overtaken by events. The dungeon is now actively committed to ending this. TwinBee is committed in the opposite direction. The clarification is useful.",
	"The dungeon has decided. TwinBee respects decisions, even ones that involve the dungeon trying to kill you. The respect is a working respect. We respect the decision and then we work to overrule it. TwinBee leads.",
}

var ThreatClockSiege = []string{
	"Siege Mode. TwinBee delivers this without decoration because decoration would be dishonest. The dungeon is fully active, fully aware, and fully committed to ending this expedition. So is TwinBee — to ending it on your terms, not theirs. What happens next is a race. Already running.",
	"Siege. The dungeon is putting everything it has into the room you're standing in, and the rooms adjacent to it, and the route between you and the door. TwinBee is putting everything TwinBee has into making sure the dungeon doesn't get what it wants. Meet in the middle.",
	"Siege Mode. TwinBee narrates the band shift and then stops narrating because narration is not what this band needs. Action is what this band needs. TwinBee is in motion. The expectation is that you are too.",
}

// ThreatClockApproachingSiege fires once when the threat clock crosses 70 —
// the spec's "begin warning" line (§8.3). Distinct from the Hostile-band
// flavor because this is the dungeon-design moment of telling the player
// they're past the point where stealth is recoverable.
var ThreatClockApproachingSiege = []string{
	"They know you're here. Not a suspicion anymore. A certainty. The question now is whether you finish before they organize. TwinBee says this clearly so it doesn't have to be said again.",
	"Threat at seventy. TwinBee marks this on the internal ledger and underlines it. The window for quiet operations has closed. The window for finishing is still open — narrower, but open. TwinBee suggests using it.",
	"The dungeon's posture has shifted from 'searching' to 'hunting.' TwinBee tracks the difference precisely: before, they were looking for evidence; now they are looking for you. The plan, accordingly, simplifies. Finish or extract. Middle paths have closed.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ZONE TEMPORAL EVENTS
// ─────────────────────────────────────────────────────────────────────────────

var SunkenTempleTidalWarning = []string{
	"TwinBee watches the waterline. 'It's rising,' TwinBee says, not for the first time, and this time with more precision. 'The tidal cycle peaks in two days. Whatever you haven't done by then, you'll be doing wet.'",
	"Day [N]. The tide is coming. TwinBee has been watching it since Day 2 and the mathematics are not encouraging. Two days to the peak. One day after that, the flood subsides. TwinBee suggests using the two days well.",
	"TwinBee checks the waterline against yesterday's mark. The mark is below the line. The line is on the wall. The wall is, frankly, full of marks. TwinBee selects the one that matters and reports: tide rising, peak still days off, plan accordingly.",
	"Day [N]. The temple's tidal calendar advances. TwinBee has it on a card and updates the card every morning — current depth, projected depth, peak day. TwinBee suggests internalizing the card. The temple does not pause for confusion.",
}

var SunkenTempleTidalEvent = []string{
	"The tide arrives. TwinBee had warned about this and now it is happening and the warning feels entirely inadequate compared to the actual water. Everything is colder, wetter, and the Kuo-toa are moving through it like they were born to — which they were. Adjust.",
	"The water is here. Not 'rising' anymore — here, in the rooms, at the corners, climbing the stairs. TwinBee adjusts every estimate accordingly: travel slower, encounters worse, healing colder. Two days of this. Two days, then it goes.",
	"Tidal peak. The temple is fully flooded at depths that were dry yesterday — TwinBee maps the change room by room and updates the route. Some doors are open that weren't. Some are sealed that were. The temple is a different shape today.",
}

var HauntedManorResetMorning = []string{
	"TwinBee's morning briefing includes an addendum. The rooms that were clear yesterday are not entirely clear this morning. The house has been busy overnight. TwinBee adds this to the log under 'things the house does' and suggests adjusting the advance plan.",
	"Night three. The manor reset itself. TwinBee was watching and it happened anyway — not violently, not dramatically, just quietly and completely, the way the house does everything. One enemy per room, back in place. TwinBee has updated the map.",
	"TwinBee reads the morning's hallway and stops. 'The arrangement has changed,' it says, in the tone of someone who has now seen this happen often enough to recognize it without alarm. The manor reset. Map adjusts. Morning continues.",
	"Reset day. The rooms TwinBee cleared are not the rooms it finds this morning. Accepts this as a property of the manor — like weather, but indoors and unfair. The plan accommodates. The time gets billed.",
}

var UnderforgHeapWarning = []string{
	"Heat Stack [N]. TwinBee notes the accumulation and what it means: the Underforge is getting into you in ways that don't resolve without real rest. The number has time to come down. TwinBee is watching the number.",
	"The heat is building. TwinBee tracks it the way you track a temperature gauge on a long drive — with the specific alertness of someone who knows what happens when the gauge hits red. It has not hit red. TwinBee intends to ensure it doesn't.",
	"Heat building. TwinBee notes the count, notes the band, notes how each step now takes a fraction more out of you than the last step did. None of it is critical. All of it is direction. TwinBee suggests the direction be inverted soon.",
	"Stack [N]. TwinBee tracks the heat the way it tracks anything that compounds — patiently, with running totals, with a clear point at which the totals stop being managed and start being problems. We are still on the management side. The other side is visible.",
}

var UnderforgHeapCritical = []string{
	"Heat Stack is high. TwinBee delivers this without minimizing it. The Underforge is inside your lungs now, in your joints, in the way everything takes a little more effort than it did on Day 1. A proper rest will help. Finishing will help more.",
	"Heat ten. TwinBee marks the maximum. There is no higher band than this in the Underforge ledger — the column ends here, by design. The forge is fully inside you. The penalties are all of them. Resting will not undo it. Finishing might.",
	"Critical heat. TwinBee runs through the symptoms list and confirms each one in turn: the joints, the breathing, the way every roll feels a little heavier. The forge is doing what the forge does. TwinBee suggests doing what you do, harder.",
}

var FeywildTimeDistortionHalf = []string{
	"The day moved strangely. TwinBee tried to track it and lost the thread somewhere around mid-afternoon — the light didn't change the way it should have, and when it looked up, the day was half over in the time it usually takes to be a quarter over. On the positive side: you're barely hungry. On the less positive side: nobody is sure what that means.",
	"Half a day passed in what felt like half of half. TwinBee checks the supply burn against the sun, finds them disagreeing, sides with the supplies — those don't lie about how much you've used. Net result: a free pocket of time. Use it on something that takes time.",
	"The Feywild gave you back some hours. TwinBee notes this without trusting it. The Feywild does not give without taking, eventually. For now: less hungry, less tired, more daylight than the math allows. TwinBee writes it down and keeps moving.",
}

var FeywildTimeDistortionDouble = []string{
	"Time doubled. TwinBee notes this with the clinical detachment of someone who has been in the Feywild long enough to stop being surprised. You've lived through two days today. The supplies reflect that. The wandering monsters reflected that as well. The rest of the Feywild is unconcerned.",
	"Two days in one. TwinBee runs the recovery math and the wandering math and the supply math, and all three of them say the same thing: today cost double. Tomorrow gets the rate it gets. The Feywild does not refund.",
	"Time doubled. TwinBee saw the second sunset before the second lunch and stopped trying to make sense of it. The day that happened is the day that happened. Plan from here, not from this morning.",
}

var FeywildTimeLoop = []string{
	"TwinBee recognizes this room. Has described it before. The enemies in it are different — new enemies, the old ones are gone, the loot you found is still gone but the enemies are back — and it processes this with something between professional acceptance and profound exasperation. 'Again,' TwinBee says. 'We do this room again.'",
	"The loop. TwinBee marks the room on the map with a small symbol that means 'we have been in this room before in a way that doesn't count.' The symbol has its own column on TwinBee's ledger. The column is occupied.",
	"Same room. New enemies. TwinBee runs the encounter again with the cold professionalism of someone who has stopped expecting fairness from the Feywild and started expecting only repetition. The repetition arrives. TwinBee handles it.",
}

var DragonsLairAwarenessPulse = []string{
	"Something changes in the mountain. Not a sound, exactly — more like a sound's absence, filling in differently than before. TwinBee watches the kobolds. The kobolds have stopped what they were doing. The kobolds are listening. Something told them to listen. TwinBee adds ten to the Threat Clock and keeps moving.",
	"Infernax shifts in its sleep. TwinBee feels this through the stone. The patrol rotations just changed — TwinBee can see it in where the Guard Drakes aren't anymore versus where they were an hour ago. The mountain's master is dreaming about you. That is not a comfortable thing to be dreamed about.",
	"The mountain pulses. TwinBee feels it as a single off-rhythm beat in the rock, the kind of beat that means something somewhere has shifted weight. The kobolds nearby pause. They look upward without looking. TwinBee adjusts the threat accordingly and continues.",
	"Infernax dreamed about you. TwinBee has no other way to phrase it — the lair changed in the way places change when they're being reordered from somewhere deeper. The patrols are different. The temperature is different. TwinBee adds ten to the clock and a note to the margin.",
}

var DragonsLairAwakenWarning = []string{
	"Day 14. TwinBee delivers the morning briefing in a lower register than usual. 'Infernax is awake,' TwinBee says. 'I don't know when exactly — sometime in the last six hours. The patrols changed. The temperature changed. The silence changed.' A pause. 'We need to reach the final chamber before it reaches us. That is now the only plan.'",
	"Day fourteen. Infernax woke up in the night. TwinBee says the sentence flatly because the sentence is enough by itself; it does not need help. The rotation is to the final chamber. Everything else is decoration.",
	"The mountain changed in its sleep. TwinBee felt it through the floor sometime around the fourth watch and has been recalibrating since. Infernax is awake and Infernax is aware. TwinBee suggests being neither subtle nor slow. There is no longer time for both.",
}

var AbyssPortalDestabilizationMid = []string{
	"Instability [N]. TwinBee watches the portal from a respectful distance and notes: it is larger than it was yesterday. Not by much. Not in a way you'd notice if you weren't looking. TwinBee is always looking.",
	"The portal is talking to itself. TwinBee has no better way to describe it — the light it emits is different at the edges now, like it's processing something. TwinBee suggests processing faster than it does.",
	"Instability rising. TwinBee runs the comparison: yesterday's portal versus today's portal. The difference is not subtle to TwinBee. The difference may not be subtle to anything else, soon. TwinBee suggests pacing the work accordingly.",
	"The portal is widening, by some measure that TwinBee is using and not currently sharing because the measure does not have a clean name. The point is: it's worse than yesterday. The point is: tomorrow may be worse than that. TwinBee names the trend so the trend can be argued with.",
}

var AbyssPortalDestabilizationCritical = []string{
	"Instability critical. The portal is unraveling at edges TwinBee can see and probably at edges it can't. The demons coming through are more agitated than they were — which is relevant because demons at baseline are already at the upper end of agitated. The verdict: finish this today. Tomorrow is a different calculation.",
	"The portal is louder. Not in sound — in pressure. TwinBee can feel it in the back teeth, in the joint of the jaw. The instability number is the number you don't want it to be. Finish the work today or accept that today was the last day to.",
	"Critical. The geometry around the portal is bending in ways that suggest the room does not entirely agree with itself anymore. TwinBee adjusts the route around the worst of it. There are more demons than there should be. There always are. There are more than that now.",
}

var AbyssPortalCollapse = []string{
	"The portal collapses. TwinBee watches it happen and does what it does in situations with no good options: moves. 'Out,' it says, and means it completely. 'Now. Everything you have, we move now.' The expedition ends here, not in defeat, but in physics. What you took is yours. What's left in there is the portal's problem. Come back when it isn't.",
	"It's coming apart. TwinBee says one word, 'Move,' in the tone it uses exactly once per expedition. You move. The portal screams behind you in a register that isn't sound. The expedition is over because physics says so. The loot gets sorted once you're somewhere physics still works.",
	"Collapse. TwinBee was prepared for this and is also actively running. 'Out,' TwinBee says. 'Now.' The corridors are folding behind you in the way that things fold when reality is no longer paying attention to what's allowed. You finish out the door. The portal does not.",
}

// ─────────────────────────────────────────────────────────────────────────────
// REGION TRANSITION (multi-region zones; §11.3)
// ─────────────────────────────────────────────────────────────────────────────

var RegionTransitDeparture = []string{
	"TwinBee marks the boundary on the internal map and crosses it. 'New region,' TwinBee says, with the careful attention of someone who knows boundaries in dungeons aren't always the same kind of boundaries you'd find on the surface. The route is set. The day is committed. We move.",
	"Crossing into [REGION_NEXT]. TwinBee folds the previous region's notes into the satchel and unfolds new ones. The light changes. The air changes. The rules of how to be careful change a little. TwinBee adjusts.",
	"The transit between regions is its own kind of room. TwinBee narrates it that way — sightlines, footing, what's behind, what's ahead — because the only way the in-between part stops feeling exposed is to treat it like the rest of the dungeon. Treated. Moving.",
}

var RegionTransitArrival = []string{
	"You arrive in [REGION_NEXT]. TwinBee surveys, takes in the new geometry, and updates the working assumptions. 'Different shape,' TwinBee says. 'Same general principle. We learn what wants to kill us here, and we get there first.'",
	"[REGION_NEXT] receives you. TwinBee notes the temperature, the sound, the things-not-said-by-the-room-but-implied. A region is not just a place. It's a posture. TwinBee adopts the new one and suggests you do as well.",
	"Boundary crossed. TwinBee stamps the day in the log — one full day spent in transit, supplies adjusted, the wandering that happened on the way handled and filed. We are here now. The next stretch is what it is.",
}

// ─────────────────────────────────────────────────────────────────────────────
// VOLUNTARY EXTRACTION
// ─────────────────────────────────────────────────────────────────────────────

var ExtractionVoluntary = []string{
	"Extraction. TwinBee notes the decision and respects it — knowing when to leave is a skill, not a failure, and TwinBee has watched enough expeditions end wrong to deeply appreciate the ones that end right.",
	"You call the extraction and TwinBee begins the route out immediately. No argument, no editorializing. There will be time for the debrief later. The first priority is the door.",
	"The dungeon doesn't like this. TwinBee can tell by the way the corridors feel as you head back out — a resistance that isn't structural, just atmospheric. The zone wanted more. It doesn't get more today. TwinBee leads the way.",
	"Withdrawing with intent. TwinBee catalogues what you have — the loot, the XP, the knowledge of where the rooms are for the return — and converts the exit into preparation. This isn't retreat. This is the start of the next attempt.",
}

// ─────────────────────────────────────────────────────────────────────────────
// FORCED EXTRACTION
// ─────────────────────────────────────────────────────────────────────────────

var ExtractionForced = []string{
	"TwinBee moves. No recap, no analysis — that comes later. Right now there is a corridor and a door and getting you through both of them. Everything else waits.",
	"Out. TwinBee says this once and means it completely. The dungeon tried to keep you. TwinBee declines on your behalf.",
	"The expedition ends here — not the way TwinBee wanted, not the way you wanted, but ended, and ending is sometimes the best available outcome. You'll come back. You'll know more. TwinBee will be there.",
}

// ─────────────────────────────────────────────────────────────────────────────
// EXPEDITION RESUME (returning after extraction)
// ─────────────────────────────────────────────────────────────────────────────

var ExpeditionResume = []string{
	"Back. TwinBee noted the door when you left it and notes it again now, from the inside. 'The dungeon is where you left it,' TwinBee says. 'Mostly.' The Threat Clock has some opinions about the time that passed.",
	"You came back. TwinBee had calculated a probability on that and is pleased to update the calculation upward. The expedition resumes. The dungeon has had time to adjust. So have you.",
	"Like hitting Continue on a save file — the world remembers where you stopped, the enemies remember why they're there, and TwinBee remembers every room and every roll. Resumes here. Advances from here.",
	"The supplies are new. The knowledge from last time is not. TwinBee considers that a significant advantage. The dungeon has the familiarity of a difficult level you've attempted before — you know where the hard parts are. That is not nothing. TwinBee builds on that.",
}

// ─────────────────────────────────────────────────────────────────────────────
// MILESTONE NARRATIONS
// ─────────────────────────────────────────────────────────────────────────────

var MilestoneFirstNight = []string{
	"You survived the first night. TwinBee notes this milestone specifically because not everyone does, and those who do carry something from it that changes how the rest of the expedition goes. You have that now. TwinBee has noticed it already.",
	"Night one survived. TwinBee makes a small mark in the corner of the manifest — the kind of mark you make for the things that count more than they look. First nights count. TwinBee has been in dungeons where they were the last nights too. This wasn't one of those.",
	"Day two morning. The first night is behind you, which means the first watch is behind TwinBee, which means a thing worth confirming has been confirmed: you sleep through the noises that matter and wake for the ones that don't. That's a survival skill. Logged.",
}

var MilestoneWeekOne = []string{
	"Seven days. TwinBee pauses the morning briefing for a moment — just a moment — to mark this. One week in an active dungeon zone is not a thing that happens by accident. It happens through every decision you've made since Day 1, compounded. Those decisions were watched. They were, gladly, yours.",
	"Day eight. TwinBee delivers the briefing slowly, because there's a thing to mark first: you have been in here, intact and operational, for a full week. That is a number with weight. TwinBee is going to set the briefing down for a moment and let the number have its weight. ... Right. Briefing.",
	"One week. TwinBee makes the mark in the column reserved for week-one survivors. The column is shorter than you'd expect. You are now on it. The rest of the day proceeds normally, but for one moment, TwinBee allows itself to be visibly impressed. The moment ends. We continue.",
}

var MilestoneTwoWeeks = []string{
	"Two weeks. TwinBee doesn't have a comparison for this one. The references have run out. There's just you, in here, on Day 14, still going, and TwinBee standing next to you having run out of everything except genuine admiration. That, in abundance. Proceed.",
	"Day fifteen. TwinBee notes that the historical comparisons stopped working at Day fourteen and have not resumed working today. We are off the chart. TwinBee is not a person who values being off the chart, except in the very specific sense that you are off the chart, in which case TwinBee values it deeply.",
	"Fortnight. TwinBee uses the older word because it sounds more like what this is — not 'two weeks,' which sounds modular and reasonable, but 'fortnight,' which sounds like the kind of duration that earns a title. TwinBee may be inventing titles for you. TwinBee will be honest about that as it happens.",
}

var MilestoneTheLongGame = []string{
	"TwinBee sets aside the narration format for a moment. Just sets it down. Speaks plainly: what you just did was not supposed to be survivable. The designers of this zone — the thing that shaped it, the evil that filled it — did not account for someone like you. TwinBee did. Always does. That's why it's here.",
	"Tier five complete. TwinBee allows the narration format to fully break for a moment because the moment fully deserves it. What you just did, complete, in a Tier 5 zone, is a thing that goes on the short list. TwinBee keeps a short list. You're on it.",
	"Long game closed. TwinBee gathers the run notes, the threat curve, the supply records, the camp positions — files them all under your name in a folder TwinBee has been keeping. The folder has a title now. The title is good.",
}

var MilestonePatientZero = []string{
	"Expedition complete. Threat Clock: never above 50. TwinBee notes this in a column it has had to use rarely enough that the column is almost fresh. Ghost protocol. You were here for the whole thing and the dungeon barely knew it until the end. TwinBee finds this impressive and also slightly eerie and says both things sincerely.",
	"Threat never above fifty. TwinBee runs back through the daily ledger and confirms it line by line. The dungeon never escalated to hostile because the dungeon never knew enough to. You moved through it like the dungeon's own quiet hour. TwinBee finds this both technically impressive and slightly haunted.",
	"Patient Zero. TwinBee uses the term because it fits: you were the thing the dungeon never noticed it had until you had already left. The Threat Clock has a column for every band you crossed. Most of those columns are empty for this run. TwinBee marks them empty. It's a good kind of empty.",
}

// MilestoneCartographer — awarded when the player searches every room before
// advancing the expedition. Combat-link wires the trigger; pool exists for the
// award narration to pull from.
var MilestoneCartographer = []string{
	"Every room searched. TwinBee was watching for it and watching for the corner-cutting that might have come instead, and the corner-cutting did not come. You searched everything. TwinBee approves with the quiet, specific approval reserved for completionism.",
	"Cartographer. TwinBee uses the word the spec uses because the word fits — you mapped the place by being in every part of it. Every door checked, every corner walked. The next Elite room owes you a roll for that. TwinBee will collect.",
	"Full coverage. TwinBee notes the empty cells on the dungeon map, sees there are none, and updates the ledger accordingly. Every room visited, searched, accounted for. The dungeon has nowhere it kept to itself. TwinBee considers this a small but real victory.",
}

// MilestoneSurvivalist — awarded when a Tier 3+ expedition completes with no
// forced extractions in the run's history. Title flag + cosmetic deferred to
// item-grant hookup; this pool covers the narrative line at the moment it lands.
var MilestoneSurvivalist = []string{
	"Survivalist. TwinBee writes the title next to your name in the ledger and underlines it once. No abandonments, no scrambles for the door, no expeditions cut short by anything but the boss going down. TwinBee acknowledges the discipline directly.",
	"The Survivalist title is technical — it means the run never broke; it ended on your terms, every time. TwinBee has been in expeditions where that wasn't true and remembers them differently. This one gets remembered as: complete. Filed with the others like it. The folder is short.",
	"No forced extractions, full clear, Tier 3-or-better. TwinBee notes each criterion separately because each one is its own choice, made repeatedly, across days. The result is the title, which is real, and the cosmetic, which is forthcoming. TwinBee will hand both over when the system permits.",
}
