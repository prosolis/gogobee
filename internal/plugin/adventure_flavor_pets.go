package plugin

// ── IMPORTANT: DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE ──
//
// This file contains hand-written flavor text. Every entry is the length it is
// on purpose. The voice, pacing, and specific wording are intentional.
// If an entry seems too long: it isn't. Leave it alone.
// If you are tempted to "clean up" or "condense" entries: don't.
// If your context window is too small to fit this file: that is your problem, not ours.
//
// ── PET FLAVOR TEXT ───────────────────────────────────────────────────────────
//
// Two pets. One register each.
//
// The Dog: enthusiastic, physical, zero technique, completely committed.
//           Loves you unconditionally and expresses this through violence on your behalf.
// The Cat: massive, precise, indifferent. Helps occasionally. Does not explain why.
//           The enemy should feel honored and also concerned.
//
// Both: never die, never leave, have opinions about dinner.

// ── DOG ATTACK ────────────────────────────────────────────────────────────────

var PetDogAttack = []string{
	"Your dog has decided to get involved.\n\n" +
		"Not strategically. Not tactically. Just fully and immediately.\n\n" +
		"{enemy} takes {damage} damage and a lot of general chaos.",

	"Your dog launches itself at {enemy} with the energy of something " +
		"that has never once considered consequences.\n\n" +
		"{damage} damage. Your dog lands, shakes itself off, and looks at you " +
		"like it just did the most normal thing in the world.\n\nIt did not.",

	"Your dog has joined the fight.\n\n" +
		"Your dog did not ask permission.\n\n" +
		"Your dog never asks permission.\n\n" +
		"{enemy} takes {damage} damage. Your dog takes a victory lap.",

	"Without warning, without strategy, and without any apparent plan beyond " +
		"extreme commitment, your dog hits {enemy} for {damage} damage.\n\n" +
		"The crowd goes absolutely feral.",

	"Your dog sees an opening that you did not see, will never see, " +
		"and could not explain if asked.\n\n" +
		"It takes it. {damage} damage to {enemy}.\n\n" +
		"The dog returns to your side looking very pleased with itself.\n\n" +
		"It should be.",

	"Your dog barrels into {enemy} at full speed with its entire body weight, " +
		"which is considerable, because it is a massive dog.\n\n" +
		"{damage} damage.\n\nPhysics was involved.",

	"Your dog bites {enemy}.\n\nDeliberately.\n\nRepeatedly.\n\n" +
		"{damage} damage total.\n\nYour dog lets go when it feels like it.",

	"Your dog has entered the arena.\n\nYour dog was not supposed to enter the arena.\n\n" +
		"Your dog did not read the rules.\n\nYour dog cannot read.\n\n" +
		"{enemy} takes {damage} damage. The rules remain technically intact.",
}

// ── CAT ATTACK ────────────────────────────────────────────────────────────────

var PetCatAttack = []string{
	"Your cat intervenes.\n\n" +
		"Once. Precisely. Without looking at you afterward.\n\n" +
		"{enemy} takes {damage} damage and is left with the unsettling feeling " +
		"that this was a favor they did not earn.",

	"Your cat has briefly decided you are worth defending.\n\n" +
		"It acts on this decision with surgical efficiency.\n\n" +
		"{damage} damage to {enemy}.\n\n" +
		"The decision has already been rescinded. Don't read into it.",

	"Your cat moves.\n\nThat's all.\n\nYour cat just moves.\n\n" +
		"{enemy} takes {damage} damage and spends the rest of the round " +
		"trying to understand what happened.",

	"Your cat looks at {enemy}.\n\nThen at you.\n\nThen back at {enemy}.\n\n" +
		"Then it handles it.\n\n{damage} damage.\n\nYour cat sits back down.",

	"Your cat has, for reasons it will not share, chosen this moment.\n\n" +
		"{damage} damage. Clean. Efficient. Completely without ego.\n\n" +
		"Your cat is already elsewhere.",

	"The cat moves through the fight like it owns the arena.\n\n" +
		"It does not own the arena.\n\nThe arena is reconsidering its position on this.\n\n" +
		"{enemy} takes {damage} damage. The cat does not acknowledge the applause.",

	"Your cat strikes {enemy} once.\n\nIt does not strike twice.\n\n" +
		"It did not need to.\n\n{damage} damage.\n\n" +
		"Your cat has returned to doing whatever it was doing before, " +
		"which was nothing, and also everything.",

	"Your massive cat drops from somewhere it wasn't a moment ago " +
		"and lands directly on {enemy}'s immediate future.\n\n" +
		"{damage} damage.\n\nYour cat leaves without comment.\n\n" +
		"{enemy} has questions. The cat will not be answering them.",
}

// ── DOG DEATH REACTION ────────────────────────────────────────────────────────

var PetDogDeath = []string{
	"Your dog was napping as your foe finished you off.\n\n" +
		"They're upset now.\n\nNot because of what happened to you.\n\n" +
		"Because dinner won't be arriving on time today.",

	"Your dog watched the whole thing.\n\nDid nothing.\n\nIs now sitting by the door.\n\n" +
		"Not waiting for you to come back.\n\nWaiting for whoever brings the food to come back.\n\n" +
		"These are different.",

	"Your dog has located your empty bowl and is moving it around the kitchen floor " +
		"to let someone know there's been some kind of administrative error.",

	"Your dog is fine.\n\nYour dog is great, actually.\n\n" +
		"Your dog found something on the floor and ate it and is having a wonderful afternoon.\n\n" +
		"You were gone, they noted. Briefly. Then the floor thing happened.",

	"Your dog is sitting in front of your equipment and looking at it.\n\n" +
		"Not mournfully.\n\nSpeculatively.\n\n" +
		"Your dog has never fully ruled out that the equipment is food.",

	"Your dog is howling.\n\nNot in grief.\n\nIn the specific register of a dog " +
		"who has been waiting an unreasonable amount of time for their walk.\n\n" +
		"The neighbors are aware.",

	"Your dog is absolutely fine and has already made several new friends " +
		"who came to check on the situation.\n\n" +
		"Your dog showed them around.\n\nYour dog let them pet it.\n\n" +
		"Your dog has no concept of what just happened to you and is having the best day.",
}

// ── CAT DEATH REACTION ────────────────────────────────────────────────────────

var PetCatDeath = []string{
	"Your cat is aware you died.\n\n" +
		"Your cat has elected not to comment at this time.\n\n" +
		"Dinner, however, is now late, and your cat has many comments about that.",

	"Your cat watched you lose from across the arena.\n\n" +
		"Its expression did not change.\n\n" +
		"Its expression never changes.\n\n" +
		"Dinner is late. Your cat's expression has changed.",

	"Your cat is sitting on your things.\n\nNot to mourn.\n\n" +
		"Your cat sits on things. That's just what it does.\n\n" +
		"Dinner being late is a separate and more pressing issue " +
		"that your cat would like addressed immediately.",

	"Your cat has knocked something off a surface.\n\n" +
		"Not in grief.\n\nJust because it was there.\n\n" +
		"Dinner is late. Your cat will continue knocking things off surfaces " +
		"until this is corrected.",

	"Your cat is fine.\n\nYour cat is always fine.\n\n" +
		"Your cat was fine before you and will be fine after.\n\n" +
		"Dinner is late and your cat is making it everyone's problem.",

	"Your cat has sat down directly in the center of the room and is staring at the wall.\n\n" +
		"This could mean anything.\n\nIt means dinner is late.",
}

// ── DOG VICTORY REACTION ──────────────────────────────────────────────────────
// Fires occasionally. Not every win. The dog has other things going on.

var PetDogVictory = []string{
	"Your dog is aware you won.\n\nYour dog is losing its mind about this.\n\n" +
		"Your dog has won every fight you have ever been in, " +
		"in the sense that it loves you and you came home, " +
		"and it is treating this occasion with the same energy as all the others, " +
		"which is: maximum.",

	"Your dog is running in circles.\n\nNot for any reason.\n\n" +
		"Just because you won and it's a good day and " +
		"running in circles is how your dog processes good days.",

	"Your dog has brought you something.\n\nYou don't know what it is.\n\n" +
		"Your dog is very proud of it.\n\nYou accept it.\n\n" +
		"This is what winning looks like.",

	"Your dog is pressed against your leg.\n\nHard.\n\n" +
		"Your dog does not fully understand what the arena is.\n\n" +
		"Your dog understands that you left and came back.\n\n" +
		"Your dog finds this extremely good news every single time.",

	"Your dog has decided that the victory belongs to both of you equally " +
		"and is accepting congratulations from nearby strangers on this basis.\n\n" +
		"The strangers are obliging.\n\nThe dog deserves it.",
}

// ── CAT VICTORY REACTION ──────────────────────────────────────────────────────
// Fires rarely. The cat has acknowledged maybe three of your wins. Total.

var PetCatVictory = []string{
	"Your cat glances at you.\n\nOnce.\n\nThen away.\n\n" +
		"This is the cat equivalent of a standing ovation.\n\n" +
		"You will not receive another one this week.",

	"Your cat is sitting near you.\n\nNot with you.\n\nNear you.\n\n" +
		"This is as close as it gets to celebration.\n\n" +
		"Take it.",

	"Your cat blinks slowly in your direction.\n\n" +
		"You have been approved of.\n\n" +
		"Briefly.\n\nConditionally.\n\nDon't push it.",

	"Your cat has moved to a slightly closer position than usual.\n\n" +
		"It will not explain this.\n\nIt may move back.\n\n" +
		"For now: proximity. That's the win.",
}

// ── DOG DEFLECT ───────────────────────────────────────────────────────────────

var PetDogDeflect = []string{
	"Your dog steps in front of {enemy}'s attack.\n\n" +
		"Not strategically.\n\nJust because you were there and the thing coming at you " +
		"was also coming at you and your dog had opinions about that.\n\n" +
		"{enemy} misses.\n\nYour dog looks very pleased with itself.",

	"Your dog saw it coming before you did.\n\n" +
		"Your dog always sees it coming before you do.\n\n" +
		"You've never fully processed this.\n\n" +
		"{enemy}'s strike lands on nothing.\n\nYour dog is already back at your side.",

	"{enemy} swings.\n\nYour dog moves.\n\n" +
		"These two things happened in the wrong order for {enemy}.\n\n" +
		"The attack misses and causes a dog treat to fall out of its pocket.\n\n" +
		"Mission accomplished.. treat acquired! ..Oh and you weren't hit.. I guess. ...yay.",

	"Your dog body-checks {enemy}'s weapon arm at full speed.\n\n" +
		"This was not a trained technique.\n\n" +
		"This was a massive dog moving very fast toward something it didn't like.\n\n" +
		"The attack goes wide.\n\nPhysics handled it.",

	"{enemy} commits to the strike.\n\nYour dog doesn't like what it sees and barks loudly enough to startle {enemy} so strongly that it yelps.\n\n" +
		"This would have been awesome if you hadn't also yelped as well.\n\n" +
		"Either way.. {enemy} misses!\n\n",

	"Your dog throws itself between you and {enemy}'s attack " +
		"with the energy of something that has never once considered " +
		"that this might not work.\n\n" +
		"It works.\n\n{enemy} misses.\n\nYour dog shakes itself off and keeps going.",
}

// ── CAT DEFLECT ───────────────────────────────────────────────────────────────

var PetCatDeflect = []string{
	"Your cat moves once.\n\n" +
		"{enemy}'s attack finds nothing.\n\n" +
		"Your cat does not look at {enemy}.\n\n" +
		"Your cat does not look at you.\n\n" +
		"Your cat has already moved on.",

	"{enemy} strikes.\n\nYour cat was there.\n\nThen your cat wasn't.\n\n" +
		"The attack misses.\n\nYour cat is now somewhere else entirely.\n\n" +
		"Nobody saw it move.",

	"Your cat steps into the path of {enemy}'s attack " +
		"and then out of it in a single motion that takes approximately no time.\n\n" +
		"{enemy} misses.\n\nYour cat sits down.\n\n" +
		"The crowd roars excitedly at the spectacle which you foolishly believe is for you " +
		"until the announcer \"helpfully\" corrects you.",

	"Your cat redirects {enemy}'s strike with one paw.\n\n" +
		"Casually.\n\nLike it had something else to do and this was briefly in the way.\n\n" +
		"The attack goes wide.\n\nYour cat returns to having something else to do.",

	"{enemy} swings.\n\nYour cat looks at the swing.\n\n" +
		"Your cat decides against it.\n\n" +
		"The swing finds nothing.\n\n" +
		"Your cat had already made its decision before {enemy} had finished committing.\n\n" +
		"That's the difference.",

	"Your cat inserts itself into the situation briefly and then removes itself.\n\n" +
		"{enemy}'s attack lands on the space your cat just vacated.\n\n" +
		"Your cat is already somewhere else.\n\nIt blinks.\n\nNot at you.\n\nAt nothing.\n\n" +
		"The way cats do.",
}
// Fires randomly in the morning DM. Cat has left something.
// Results in a defense boost for the day.
// The cat is proud. The cat will never stop doing this.

var PetCatOffering = []string{
	"There is half a rat on your doorstep.\n\n" +
		"You notice that your cat is looking at you from a distance.\n\n" +
		"You pretend to take a bite.\n\nYour cat looks genuinely upset.\n\n" +
		"Defense increased for today.",

	"Something has been left at your door.\n\nSomething that was recently alive.\n\n" +
		"Your cat has arranged it thoughtfully.\n\nThis took effort.\n\n" +
		"You are moved in a way you did not expect and cannot fully explain.\n\n" +
		"Defense increased for today.",

	"Your cat has brought you a bird.\n\nMost of a bird.\n\n" +
		"Your cat is extremely pleased with itself.\n\n" +
		"You look at the bird.\n\nYou look at your cat.\n\n" +
		"Your cat has never once doubted you (in a way that *you* would notice anyway).\n\n" +
		"You head out with that energy.\n\nDefense increased for today.",

	"There is something on your doorstep that you will not be describing in detail.\n\n" +
		"Your cat is sitting beside it with the composure of someone " +
		"who has provided and would like acknowledgement.\n\n" +
		"Your horrified expression pleases the cat and you are now ready for anything.\n\nDefense increased for today.",

	"Your cat hunted last night.\n\nYour cat hunted successfully.\n\n" +
		"Your cat brought the evidence to your door because you are its person " +
		"and it wanted you to know that things have been handled.\n\n" +
		"Things have been handled.\n\nDefense increased for today.",

	"Half a mouse.\n\nYour cat.\n\nThe specific expression of an animal that loves you " +
		"in the only language it fully trusts.\n\n" +
		"You stand there for a moment.\n\nYou go inside.\n\nYou come back with a treat.\n\n" +
		"Your cat sniffs it and eats the rest of the mouse instead.\n\n" +
		"Defense increased for today.",
}

// ── DOG MORNING SMOTHERING ────────────────────────────────────────────────────
// Fires randomly in the morning DM. Dog has slept on top of the player.
// Results in a defense boost for the day.
// The dog has no idea. The dog never has any idea.

var PetDogSmothering = []string{
	"You woke up this morning unable to breathe.\n\n" +
		"This was because your dog was on top of you.\n\nAll of your dog.\n\n" +
		"Your dog was asleep.\n\nYour dog is still asleep.\n\n" +
		"You moved your dog.\n\nYour dog made a noise.\n\n" +
		"You lay there for a moment, alive, aware of it.\n\n" +
		"Defense increased for today.",

	"At some point last night your dog relocated from its bed to your chest.\n\n" +
		"Your dog weighs a considerable amount because it is a massive dog.\n\n" +
		"You slept under this weight for several hours.\n\n" +
		"You feel like you have survived something.\n\nYou have survived something.\n\n" +
		"Defense increased for today.",

	"Your dog was on your legs when you woke up.\n\n" +
		"Your dog was also somehow on your chest.\n\n" +
		"The physics of this are unclear.\n\n" +
		"You have no feeling in your lower half.\n\n" +
		"You have never felt more alive.\n\nDefense increased for today.",

	"You woke up with your dog's full weight distributed across your torso " +
		"in a way that suggests your dog spent the night actively trying to become part of you.\n\n" +
		"Your dog succeeded, spiritually.\n\n" +
		"You are a unit now.\n\nDefense increased for today.",

	"Your dog smothered you last night.\n\nNot with malice.\n\nWith love.\n\n" +
		"There is no meaningful difference in outcome but the intent matters " +
		"and the intent was pure.\n\n" +
		"You made it.\n\nDefense increased for today.",

	"You nearly died in your own bed.\n\n" +
		"Your dog doesn't know this.\n\nYour dog is wagging its tail.\n\n" +
		"Your dog slept better than it ever has.\n\n" +
		"You look at your dog.\n\nYour dog is wagging its tail.\n\n" +
		"Despite the near death experience, you feel all is right with the world at that moment.\n\n" +
		"Defense increased for today.",

	"Your dog is 'little spoon' in a technical sense only.\n\n" +
		"In practice your dog is the entire bed and you are whatever " +
		"fits in the remaining space, which last night was: not much.\n\n" +
		"You woke up on the edge.\n\nYou did not fall.\n\n" +
		"You are tougher than you thought.\n\nDefense increased for today.",
}
