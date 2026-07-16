// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_ossuary_ascendant_flavor.go
// Tier 6 post-game zone flavor — The Ossuary Ascendant. Additive only. Pools
// sampled by internal/plugin via deterministic per-run, per-room hashing.
//
// Voice rules (from gogobee_dungeon_zones.md §3.3):
//   • Third person for description; second person for outcomes.
//   • Boss callouts get a beat of cinema. Don't overrun.
//   • TwinBee references the right era — NES, SNES, arcade. Not modern.
//
// The zone is the return of Valdris — the lich the party killed in the
// Tier-1 Crypt years ago. Dying there was step one; the phylactery shard
// they've been looting since was bait. He has rebuilt himself as a true lich
// inside an inverted bone cathedral hung over the old Crypt, and this file
// carries the room-entry pool, boss-entry beats, lore, and Valdris's
// ability callouts.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — The Ossuary Ascendant
// Generic entries for stepping into a fresh non-boss, non-elite room across the
// four regions: Bonefall Steps → Cathedral of Marrow → Reliquary Vaults →
// The Apotheosis Engine.
var RoomEntryOssuaryAscendant = []string{
	"You climb a stair carved from a single fallen spine, each step one vertebra wider than the last. It rises toward a ceiling that is also a floor, because the whole cathedral hangs upside down over the old Crypt you cleared years ago. I file this under 'debt, collected with interest.'",
	"The Bonefall Steps drop bone-dust like snow that falls upward. It settles on the underside of the arches above you, packing into new masonry as you watch. He is building the place while you walk through it. I track the rate and note it is not slowing.",
	"A hall of femurs stacked to the vaulting, every one filed smooth and labeled in a hand you've seen before — on a shard in your pack you've carried since the Crypt. I recommend not reading your own name off the wall. It's here somewhere and finding it changes nothing tactical.",
	"The Cathedral of Marrow opens ahead, a nave the size of an arcade cabinet room and lit by no fire you can find. The light comes from the bones. They glow the pale green of a CRT left on too long. I mark two exits and one thing pretending to be a pew.",
	"Grave-smoke pools ankle-deep here and rolls away from your boots like it's shy. It isn't shy. The Grave Cardinal walked this stretch and blessed it, and the blessing is patient. Move through; don't breathe deep. I file the smell under 'incense, weaponized.'",
	"Choir stalls line both walls, each one holding a robe with no one in it, all of them turned to face you. When you pass, the heads that aren't there turn to keep watching. Third person for the description; second person for the part where the hair on your neck stands up.",
	"The Reliquary Vaults: rank on rank of glass coffins, each holding a relic instead of a body — a broken sword, a child's shoe, a cracked phylactery twin to yours. I catalog forty before I stop. Every one of these was somebody's oldest quest item. He kept them all.",
	"A donor hall, and I use the medical word on purpose. The Reliquary Knight was riveted together somewhere near here from adventurers who died in the Crypt below, and the walls still hold the ones that didn't make the cut. You are walking through the spare-parts bin. Keep walking.",
	"The corridor tightens toward the Apotheosis Engine and the air starts to hum on a note just under hearing — the same note an arcade monitor makes right before the picture comes up. Something large is powering on ahead of you. I recommend arriving before it finishes.",
	"Verses are carved into the marrow here, three lines of a hymn broken across three far rooms, and the stanza in front of you stops mid-word. I log the gap. A patient player reads the whole song. A fast one reads none of it. I decline to say which one Valdris is hoping for.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ENTRY — "Valdris, At Last"
// The dramatic beat on reaching the boss room and the boss appearing. A combat
// callout from ValdrisAscendantSignatureCallouts is appended at combat start.
var BossEntryValdrisAscendant = []string{
	"The Apotheosis Engine is a throne built of every spine in the cathedral, and Valdris is finishing his climb up it one vertebra at a time. He sets the last bone in place, turns, and smiles like a man who mailed a letter years ago and just heard the knock. 'You brought my shard home,' he says. 'Thank you. I planned on it.' I file this under 'ambush, incubated.'",
	"He was never a boss you beat in the Crypt. He was a save-state, and this is the continue screen. Valdris rises complete now — robe of donor-bone, eyes two green pilot-lights — and inclines his head at you with real courtesy. 'A good plan deserves respect,' he says. 'Show me yours.' I mark the room. There is no second exit.",
	"You killed this man on the first floor of the first dungeon, back when you couldn't spell your own class. He remembers. He remembers being step one of his own scheme, and he looks delighted that you came all this way to be step last. 'The shard was bait,' Valdris says, gentle as a tutorial. 'You were the fisherman I hired without asking.'",
	"The lich stands at the top of the Engine and the whole inverted cathedral leans toward him like iron filings toward a magnet. Whatever you carried out of the Crypt all those years ago, he is reeling it back in, and it hums in your pack in answer. 'Home,' he says to it, not to you. Then, to you: 'You may begin.'",
	"He does not attack. He waits, hands folded, while the bone-light gathers behind his ribs into something I don't have a category for and file under 'sunrise, indoors.' Valdris is patient, and patience is the tell — he has done arithmetic on this fight that you haven't. 'Let's see what you learned since the Crypt,' he says. The Engine begins to turn.",
}

// ─────────────────────────────────────────────────────────────────────────────
// LORE — sampled by !lore inside this zone
var LoreLinesOssuaryAscendant = []string{
	"Valdris did not lose in the Crypt. Valdris filed a form. Dying there scattered him into the phylactery shards, and every party that looted a shard carried a piece of him one step closer to here. I have run the ledger. You were couriers. Unpaid.",
	"A true lich, the texts say, ascends 'one vertebra at a time,' and I took that for poetry until I saw the throne. It is literal. Every spine in this cathedral is a rung, and he has been climbing for as long as you've been adventuring.",
	"The cathedral hangs inverted over the old Tier-1 Crypt on purpose. He built his heaven directly above his first grave so the fall, if there is one, is short and he lands somewhere familiar. I note the man plans for defeat too. That should worry you.",
	"The Grave Cardinal 'pre-blesses your corpse' with its censer — that is not a threat, it is a courtesy in Valdris's theology. The dead here are congregation, not casualties. The smoke just processes your paperwork early.",
	"The Reliquary Knight is riveted from the donor-bone of every adventurer who died in the Crypt below, and its shield is a coffin lid because Valdris is thrifty and sentimental at once. When it blocks, it is a hundred dead heroes deciding you don't pass. I respect the craftsmanship and dislike the wall.",
	"The Chorister sings your name in nine throats sewn to one column because Valdris kept a guest list. It is not guessing. It read your name off a shard the day you first picked one up, and it has been rehearsing. Hearing yourself sung in rounds is meant to freeze you. Don't let it.",
	"There is a hymn carved through this dungeon in three broken verses, and the shard in your pack hums when you're near one. The texts call them 'Phylactery Verses.' I won't tell you what reading all three does. I'll only note that a lich who respects a good plan built a way to reward one, and hid it where a hurry would miss it.",
	"He respects a good plan the way an old arcade respects a quarter — it will take yours all day and still tip its cabinet toward the player who actually learned the pattern. Valdris is patient because patience already won once. Whether it wins twice is, for the first time in years, genuinely not settled.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ABILITY CALLOUTS — Valdris, At Last (one-line cinematic suffix at combat
// start). Names his threats without tutorializing the hidden Verses.
var ValdrisAscendantSignatureCallouts = []string{
	"He gathers the bone-light behind his ribs — Apotheosis Nova, charging. I say: 'That's the laser-eye boss's tell. When the room goes bright, be behind cover or be a memory.'",
	"His robe of donor-bone knits shut every wound as fast as you open it. I track the healing and say: 'He's soaking hits like a Contra tank. Burst him, don't chip him.'",
	"The throne feeds him. Every spine in the Engine leans his way and closes his cuts. I recommend you fight the room, not just the man.",
	"He answers your best combo without flinching, like he's seen the input before. I file this under 'boss reads your buttons.' Vary the rhythm.",
	"Green pilot-lights flare in his sockets and the temperature drops a screen's worth. Cold snap incoming. I call it: 'Move on the shimmer, not the flash.'",
	"He resists nearly everything you throw — unless you walked in already knowing the words. I note his armor of resistances and say only: 'A fuller road here hits harder. That's all I'll say.'",
	"He raises the fallen off the Bonefall floor to buy himself a turn. Adds incoming. Clear the small ones or the big one clears you.",
	"He does not rush. He waits for your cooldowns like a grappler waiting out your jump. I say: 'Don't spend everything early. He's counting.'",
	"The bone-light peaks white — decisive-phase Nova, the arena-clearer. I call it flat: 'This is the screen-filler. Survive it and the fight is yours to lose.'",
	"He inclines his head, courteous, and the Engine's hum climbs an octave. That courtesy is a countdown. I recommend you act inside it.",
}

// ─────────────────────────────────────────────────────────────────────────────
// PHASE TWO — surfaced when Valdris crosses below 50% HP mid-fight.
var ValdrisAscendantPhaseTwoLines = []string{
	"Below half, he stops climbing and lets go of the throne entirely — floats free, robe unspooling into a corona of donor-bone. 'Now the courtesy ends,' Valdris says, still pleasant. I say: 'Phase two. He's off the rails. Everything's live now.'",
	"At fifty percent the Apotheosis Engine reverses and starts pouring its light into him instead of the walls. He is spending the whole cathedral to stay up. I call it: 'He's cashed in his reserves — hit him while the account's open.'",
	"Half down, and the green in his sockets flares to arcade white. This is the true-lich phase the Crypt never showed you. 'You've earned the real fight,' he says. I file that under 'promotion I did not want,' and recommend you close it fast.",
}
