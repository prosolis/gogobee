// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_last_meridian_flavor.go
// Tier 6 post-game zone flavor — The Last Meridian. Additive only.

package flavor

// ROOM ENTRY
var RoomEntryLastMeridian = []string{
	"A colonnade of brass sundials, every one of them stopped at a different dusk. The shadows point every direction at once, which means none of them are lying and all of them are useless. I file this under 'decommissioned' and mark the exits before the light changes.",
	"The floor is inlaid with a calendar nobody kept. Months have been pried up and carried off — you can see the empty sockets where a season used to be. I recommend not standing on the gaps; things that have been removed do not always agree that they are gone.",
	"A gallery of water clocks, drained. The basins hold a fine grey dust instead of water, and the dust is still trying to drip. It counts nothing at a steady rate. I track the rhythm anyway, out of habit, and out of the suspicion that something here is listening to it.",
	"Candles line the wall, each one a different height, each burning at the same rate. Read left to right they tell you how long you have. Read right to left they tell you how long everyone before you had. Neither reading is encouraging.",
	"The room was a workshop for winding the world. Bench after bench of half-finished hours, key still in the mechanism, the winder gone to lunch and never returned. You touch nothing. I approve of this discipline. Cranking an unfinished hour is exactly the kind of thing that ends a save file.",
	"An observatory floor, the dome above cracked to let one thin blade of starlight through. The star it points at set a long time ago; the light is just the paperwork catching up. I note the angle. When it moves, the room has moved with it, and you will want to know which of you did the traveling.",
	"A hall of stopped pendulums, hung like coats on a rack. They do not swing. They lean, very slightly, all in the same direction — toward the far door, toward midnight, toward the thing that is turning the lights off on its way out. You walk the way they lean. It is the only honest signpost in the building.",
	"Ledgers, floor to ceiling, every page a receipt for one spent hour. Somebody has been going through with a red pen, marking hours 'returned.' The ink is still wet three shelves in. I recommend a quiet pace. Whatever is doing the auditing is only a few rows ahead of you.",
	"The architecture here is the same you passed an hour ago, but the dusk has drained out of it and left midnight in the joints. Same room, later. That is the trick of this place — it does not move you forward, it just keeps taking the daylight until forward is the only thing left.",
	"A waiting room. Chairs bolted in rows, all facing a door, a number-board above it frozen mid-count. Nobody is waiting. Everybody has been served. You take the number anyway, because I told you to, and because the board flickers once when you do — the first thing in this wing that has admitted you exist.",
}

// BOSS ENTRY
var BossEntryCustodian = []string{
	"The corridor opens into the movement-room of the whole cathedral, and standing in the center of it, wound into the orrery like it was born there, is the thing that has been shutting off the lights. It turns to face you without hurry. It has the rest of time, and it intends to spend all of it on this.",
	"Verdigris and brass, orrery rings turning slow around a body the size of a bell tower. It regards you the way a closing shift regards a customer who came in one minute before the doors lock — not unkindly, but with a schedule that will not be moved. I set the timer. It has already set its own.",
	"It raises one great hand, and every stopped clock in the building starts again at once, all of them wrong, all of them counting down to the same number. The Custodian of the Last Hour has decided your visit is the last item on its list. It would like to finish. I would like you to make finishing difficult.",
	"'You are early,' it says, and it means it as a courtesy. The pendulums behind it fall into step. The candles halve. Somewhere a chime is being wound to strike. I read the whole room going onto one clock — its clock — and I tell you plainly: everything in here now keeps the boss's time. Break the clock and you break the rest.",
	"It steps down off the orrery dais and the floor accepts the weight like it has been waiting centuries to. Titanic, polite, terminal. It apologizes before it has even swung — I hear the words scheduled a half-second ahead of the blow. Good. If it announces its manners on a rhythm, then it announces everything on a rhythm, and a rhythm is a thing you and I can learn.",
}

// LORE
var LoreLinesLastMeridian = []string{
	"Time was not discovered here. It was invented here, on commission, to a spec. The Last Meridian is the office that drafted the hour and sold it to the world. What you are walking through is that office, closing.",
	"The Custodian is not a monster. It is an employee. Its contract read: keep the hours until the world no longer needs them kept. It has concluded, on evidence it will not share, that the term has been met. It is not attacking you. It is filing you.",
	"Every region you crossed was the same architecture at a different hour — dusk, then spent, then the escapement, then the minute before. You did not descend. You waited, and the building took the daylight out from under you one floor at a time.",
	"The Amendment is a clause, not a spell. Once per closing, the Custodian is permitted to revert itself to an earlier reading — to strike out damage the way the red pen struck out hours. It cannot do it twice. Whatever you spend before it invokes the clause is spent against a page that will be torn out.",
	"'Closing time' is in the contract too. Past the twentieth stroke the Custodian is obliged to hurry — the world is not owed a slow ending, only a punctual one. Each round after midnight it hits harder, because lingering is a breach and it will not breach.",
	"The Hour Thief was the night watchman. It stole minutes to stay awake through its shift and never stopped once the shift ended. The satchel of ticking is every minute it ever pocketed, and it is still, four hundred years later, trying not to fall asleep at its post.",
	"The Wardens-in-Waiting guard a door with nothing behind it because the room the door led to was decommissioned first. They were never told. They still keep the shift — one on, one resting — waiting for a relief that was returned to the ledger centuries ago.",
	"There is a rewind in the machinery of this whole place, a great key nobody ever turned back. The Custodian could, in principle, wind the world to any hour it chose. It chose to stop the clock instead. I file that under mercy, or under exhaustion. On the evidence I cannot tell them apart.",
}

// CUSTODIAN SIGNATURE CALLOUTS
var CustodianSignatureCallouts = []string{
	"It apologizes — that means the swing is already scheduled. Move on the apology, not the blow.",
	"Chime! The whole room rings at once. I say get low and get spread, this one collects everybody standing together.",
	"The orrery rings speed up. It is winding toward a strike — you have exactly until they align.",
	"It reaches for the red pen. Anything you did in the last few seconds, it is about to un-do.",
	"Rings settling into a slow arc. Decisive phase inbound — this is the swing it has been apologizing for.",
	"The candles just halved. It is buying speed with your daylight. Hurry, or it hurries first.",
	"Midnight is close. Every stroke from here lands heavier — I recommend you end this before it has to.",
	"It bows before the blow. Polite as a closing bell, and about as final. Step off the mark.",
	"The pendulums fall into its rhythm. Now everything in the room keeps the boss's time — count with me.",
	"It resets its own hands to an earlier hour. Front-loaded damage just went on the ledger as 'returned.'",
}

// BOSS PHASE TWO
var CustodianPhaseTwoLines = []string{
	"Under forty-five percent it invokes the Amendment — winds itself back to its round-three reading, once. Everything you front-loaded gets refunded to the house. I say switch to the long game; a steady build survives a rewind, a burst does not.",
	"Phase two, and the clock behind it starts striking toward midnight. This is 'closing time' — every round from here it hits harder, on schedule, no appeals. Sustained pressure now, and finish before the last stroke lands.",
	"It has spent its one rewind. There is no second Amendment in the contract — from here the ledger stays written. Whatever you do to it now, it keeps. Make all of it count.",
}
