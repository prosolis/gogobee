// twinbee_ambient_flavor.go
// TwinBee GM Dialogue — Ambient between-day micro-events. Fired by the
// expedition ambient ticker (Phase 12 E7) every few hours while the player
// is offline or idle. Mostly flavor; occasional micro-mechanical nudges.
//
// Voice convention (Phase B2): TwinBee speaks in first-person or implicit
// subject. No third-person "TwinBee [verb]" lines. Add freely.
// Don't shorten existing entries.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// AMBIENT — Pure flavor (no mechanical effect)
// ─────────────────────────────────────────────────────────────────────────────

var AmbientMonologue = []string{
	"I've been counting the dripping sound. The dripping sound has not been keeping pace. I suspect the dripping sound is doing it on purpose.",
	"A draft moves through the corridor in a way that suggests the dungeon is breathing, then in a way that suggests it isn't, then back. I decline to call it.",
	"Somewhere far off, a door closes. Nothing has opened a door. Filed under 'noted, not investigated.'",
	"Your bedroll has shifted approximately two inches to the left over the course of the morning. I did not move it. You did not move it. I am going to stop watching the bedroll now.",
	"A small stone has rolled across the floor with no visible cause. It came to rest in a position I can only describe as 'mildly conversational.'",
	"I tried to think of a song and forgot the words. The dungeon offered a few. I politely declined.",
	"A spiderweb in the corner has rearranged itself overnight into a shape that's almost a sentence. I can read about three letters of it before deciding it's safer to look at the floor.",
	"The torch is burning at exactly the same height it was an hour ago. No wax has accumulated. I considered asking the torch about this and decided against. The torch has not earned an interview.",
	"You have not moved in some time. The dungeon also has not moved. I'm calling this an even draw and declaring the round complete.",
	"A faint smell of bread has drifted through. There is no bread within several days of you. I'm not mentioning it, because mentioning it would make it worse.",
	"I've been alphabetizing your inventory in my head. I got as far as 'flask, empty' before getting distracted by the question of whether 'empty' counts.",
	"The shadow in the far corner has been the same shape for three hours. Shadows are generally not committed to one shape. I've stopped looking at it.",
	"You realize you've been holding your breath. You weren't doing it on purpose. I was, and I'm quietly resuming.",
	"A pebble on the floor has, by all appearances, blinked. I reviewed the definition of 'blink' and found it unhelpful here.",
	"The dungeon is quiet in the specific way that means something is being quiet *at* you. I acknowledge the distinction and move on.",
	"I've been composing an imaginary letter home. The letter is mostly about lichen. The lichen is, frankly, fascinating.",
	"You hear what is almost certainly a cough. It is unclear which of you it came from. I'm assuming responsibility and apologizing.",
	"The map in your pack has folded itself one extra time while you weren't looking. I decline to unfold it for science.",
	"Your own footsteps echo back to you a half-second late, in slightly the wrong order. Noted. I'm choosing to walk in silence for a bit.",
	"A long, slow scrape happens two rooms over. Then nothing. I waited for a second scrape. There wasn't one. The first scrape was the whole sentence.",
}

// ─────────────────────────────────────────────────────────────────────────────
// AMBIENT — Noises (+1 threat)
// ─────────────────────────────────────────────────────────────────────────────

var AmbientNoise = []string{
	"Something far off makes a wet, considered noise — the kind of noise that's been thinking about itself. Marked on the imaginary map. Threat ticks up.",
	"A door slams somewhere it shouldn't. 'Shouldn't' is doing heroic work in that sentence. The dungeon is more aware of you now.",
	"A patrol passes a corridor or two over — boots, low voices, the particular silence of people who hunt in shifts. They didn't find you. They know the shape of where you might be.",
	"The dungeon hums for a moment, like a refrigerator coming on. There is no refrigerator. There is also, distinctly, a humming. The faction's pulse is faster today.",
	"Stones shift overhead in a way that is not settling and not random. Something walked across the ceiling. I'm choosing not to elaborate on what 'across the ceiling' implies.",
	"A horn sounds, very far away — the kind of horn that's a signal to a thing that signals to more things. I acknowledge the chain.",
	"You hear someone whistling your name. You don't have that name. The whistler is workshopping options.",
}

// ─────────────────────────────────────────────────────────────────────────────
// AMBIENT — Pack Rat / Critter ate supplies (−0.2 SU)
// ─────────────────────────────────────────────────────────────────────────────

var AmbientPackRat = []string{
	"A rat has formed strong opinions about your trail mix. The opinions are now inside the rat. Some supplies were lost in transit.",
	"You catch a small mammal mid-burglary. It looks at you with the unbothered confidence of a creature that has read the lease and concluded it has rights. You lose a little supply.",
	"Something with too many legs has been at the rations. I won't say how many legs. I will say: fewer rations now.",
	"A bird the size of a thumbnail has stolen a piece of jerky three times its size, by a process best described as 'unclear but enviable.' Supplies tick down.",
	"Mold. Just normal, ambitious mold. It has made itself at home in a corner of the pack. You evict it; some supplies go with it.",
	"You discover a small hole in the side of your pack and a smaller, more confident animal asleep beside it, full. Supplies were the rent.",
}

// ─────────────────────────────────────────────────────────────────────────────
// AMBIENT — Lucky Find (+1d6 coins)
// ─────────────────────────────────────────────────────────────────────────────

var AmbientLuckyFind = []string{
	"I found a copper piece in your boot. You did not put it there. I did not put it there. I elect not to investigate further and pocket it.",
	"You notice a coin wedged in a wall crack at exactly eye height, as though placed for the next passerby. You are the next passerby. The coin is yours now.",
	"A small pouch is lying in the middle of the floor. I inspect it from a polite distance — no trap, no glyph, just coins. The dungeon occasionally tips.",
	"You step on something flat. It is a coin. It is also two more coins under the first coin. I suspect a coin-laying creature and choose not to share the theory.",
	"A skeleton you walked past three days ago has, on review, a small purse you missed. I retrieve it with the discretion of a librarian recovering an overdue book.",
	"You find coins in the lining of your own cloak. They were always there. I gently suggest counting your pockets more often.",
}

// ─────────────────────────────────────────────────────────────────────────────
// AMBIENT — Camp Critter (camped only; tiny HP +1 or supply nibble)
// ─────────────────────────────────────────────────────────────────────────────

var AmbientCampVisitor = []string{
	"A possum has joined the camp. It seems diplomatic — it brought a leaf. I accept the leaf. Morale ticks up.",
	"A small luminous moth lands on your hand for a full minute, then leaves, taking with it a portion of whatever was weighing on you. You can't explain it. You feel slightly better.",
	"A cat. There should not be a cat down here. There is a cat. It sits at the edge of the firelight and watches you sleep, professionally. You rest a little better.",
	"A frog finds the camp acceptable and gives no further reviews. Its presence is, somehow, soothing. You feel marginally more whole.",
	"A small fox eats some of your jerky and gives you a look afterwards that seems to be gratitude or possibly contempt. Hard to read. The jerky is gone either way.",
	"A field mouse studies your bedroll, judges it, and leaves. Nothing is missing. You feel oddly honored.",
}

// ─────────────────────────────────────────────────────────────────────────────
// AMBIENT — Threat-aware whisper (only when threat ≥ 30; +2 threat)
// ─────────────────────────────────────────────────────────────────────────────

var AmbientFactionWhisper = []string{
	"You overhear two voices a corridor away, going over a list. The list is short. One of the items on it is the corridor you're standing in. They move on. So should you.",
	"A piece of paper is pinned to a wall you haven't reached yet. I can see the corner of it from here. It is the right shape and size to have your description on it.",
	"Something taps three times on stone, pauses, taps three times again. Five seconds later, the same sequence answers it from a different direction. You are between the two.",
	"A scent of lamp oil that isn't yours drifts through. Someone is reading by it. Nearby. They have not moved for some time. I adjust the route.",
	"A bell rings exactly once, somewhere structural. I have learned what one bell means in this zone. I decline to share but increase pace.",
}

// ─────────────────────────────────────────────────────────────────────────────
// AMBIENT — Small item / minor stash hint (flavor only)
// ─────────────────────────────────────────────────────────────────────────────

var AmbientSmallFind = []string{
	"I notice a loose stone you walked past earlier. Under it: a button. Just a button. I pocket it anyway, because of course I do.",
	"A bird's nest in an unlikely place yields a single bead, blue. I like it. It goes in the bag of things I like.",
	"A scrap of fabric has caught on a nail at exactly knee height. I save it without explanation. There may be an explanation later. There probably won't.",
	"You find a single playing card on the floor. The four of cups. Neither omen nor coincidence. Pocketed neutrally.",
	"A small key. To a small door. Neither of which is currently relevant. Filed under 'eventually.'",
}
