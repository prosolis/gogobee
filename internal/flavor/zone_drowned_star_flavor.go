// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_drowned_star_flavor.go
// Tier 6 post-game zone flavor — The Drowned Star. Additive only. Pools
// sampled by internal/plugin via deterministic per-run, per-room hashing.
//
// Voice rules (from gogobee_dungeon_zones.md §3.3):
//   • Third person for description; second person for outcomes.
//   • Boss callouts get a beat of cinema. Don't overrun.
//   • TwinBee references the right era — NES, SNES, arcade. Not modern.
//   • Narrator is TwinBee, first person or implicit-subject imperative.
//
// The Dreaming Aboleth was dreaming of this the whole time: a star that fell
// into the trench before the surface had names, with its angel still strapped
// to it. Seraphel rode her charge down and has kept a dying star alive for ten
// thousand years with radiance meant for healing. Both of them have gone
// strange. Regions, in order: The Long Sink → Pilgrim Trench → The Radiant
// Wreck → The Heart Chapel.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — The Drowned Star
// Generic (non-boss, non-elite) room intros across the four regions.
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryDrownedStar = []string{
	"You descend past the last depth where light has any business being. The water is cold enough to have opinions. Somewhere far below there is a glow that should not be down here, and it is the only reason you can see your own hands. I file this under 'the underwater level' and remind you those were never the fun ones.",
	"The Long Sink keeps sinking. The pressure presses on you like a held breath you didn't choose to hold, and the air-meter in the corner of my attention is the only clock that matters now. You go down. Everything down here is going down. I track the descent and recommend not counting the fathoms out loud.",
	"A shelf of drowned pilgrims kneels in the silt, facing the glow, exactly as they died — patient, oriented, unbothered. The current moves through them and they nod along with it. I identify the posture as 'devotional' and note none of them turned around when you arrived, which I choose to find comforting rather than the alternative.",
	"The trench narrows into a corridor of coral that grew in the shape of a cathedral nave, because something taught it to. The arches are load-bearing and the load is grief. You swim the aisle. I file this under 'someone consecrated this water' and keep the air-meter in frame.",
	"You enter a pocket of the wreck where the star's light leaks through a crack in something ancient and hull-shaped. The light is warm. That's the wrong thing for it to be, this deep, this cold. Warmth down here is a promise nobody meant to keep. I track the temperature and dislike the direction it's moving.",
	"The water here is threaded with hair-thin motes that drift upward against every current, toward the glow, the way ash drifts toward a fire it came from. I identify them as flakes of the star, shed and rising, and note the whole trench is very slowly falling upward into the thing that is dying.",
	"A votive field: thousands of small lights fixed to the trench wall, each one a pilgrim's offering, each one long dead and still faintly lit by borrowed radiance. It is the loveliest room I have logged and I want to leave it immediately. You move through the candles. None of them gutter.",
	"The corridor opens onto a drop, and across the drop, impossibly far and impossibly bright, is the thing you came for — the sunken star, cupped in the dark like the last coal in a dead hearth. It is beautiful and it is fading and someone has been kneeling beside it for ten thousand years. I say nothing for a moment. Then I say: keep moving.",
	"You pass through a hall where the pressure has crushed old stained glass into a pane of colored grit suspended in the water, holding its picture out of sheer habit. The picture is a winged figure holding a light to her chest. I file this under 'she had a following, once' and note the following is all around you, kneeling.",
	"The Heart Chapel's outer rooms are quiet in the specific way of a place that expects you and has decided to be gentle about it. The light is steadier here, closer to its source. You feel watched, and not unkindly, which is somehow worse. I track the ambient radiance and recommend you not mistake gentleness for safety.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ENTRY — Seraphel, the Light That Sank
// The dramatic beat of reaching her at the heart of the star.
// ─────────────────────────────────────────────────────────────────────────────

var BossEntrySeraphel = []string{
	"The Heart Chapel opens and there she is, exactly where she has been for ten thousand years: an angel wound around a dying star, holding it the way you hold something you have already failed to save and refuse to put down. She does not rise. She turns her ruined halo toward you, and the whole trench brightens like a held breath. I say, quietly: 'Seraphel. She rode this thing down. She never let go.'",
	"You cross the last threshold into a light so old it has forgotten it was ever meant to heal. Seraphel is at the center of it, cupping the star to her chest, her wings fused to it by ten thousand years of not moving. She looks at you with something that is not anger and is not welcome. It is recognition. I file this under 'she has been waiting for a reason to be found' and I recommend you be careful what you are.",
	"The chapel is the glowing-boss room, and I have logged a hundred of those — the arena that lights up, the figure at the center that is the light source. But the glowing boss was never sad before. Seraphel unfolds from around the star, radiance streaming off her like grief off a saint, and the second heartbeat I am reading is not hers. I say: 'There are two of them in there. Hold that thought.'",
	"She does not attack when you enter. She looks at you for a long moment across the drowned chapel, one hand still pressed to the fading star, and in that moment the water goes warm and reverent and terribly, terribly bright. Then she rises, and the light rises with her, and it stops being kind. I track the shift and say: 'That's the fight. She's decided you're a threat to it. I don't entirely blame her.'",
	"Ten thousand years of radiance meant for mending, spent instead on keeping one dead thing warm, has to go somewhere when it finally moves. It goes into Seraphel, and Seraphel comes off the star like a sunrise breaking the wrong way. She is luminous and she is wrong and underneath the light I can still read the second, smaller pulse she is shielding with her whole ruined body. I say: 'Here she comes. Watch the light. And — watch what she's protecting.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ABILITY CALLOUTS — Seraphel, the Light That Sank
// One-line cinematic suffixes surfaced when combat starts. Flat pool.
// Phase-two lines stay separate (surfaced via dedicated phase-two helper).
// ─────────────────────────────────────────────────────────────────────────────

var SeraphelSignatureCallouts = []string{
	"Sanctified Undertow: the whole chapel becomes current, and the current is radiant, and it pulls everyone toward the star whether they want to go or not. I say: 'That's the decisive one. Radiant tide, room-wide. Brace before it lands or get dragged into the light.'",
	"Sanctified Undertow again — she opens her wings and the water becomes a tide of borrowed healing turned to a weapon. Radiant, unavoidable, room-wide. I file this under 'the underwater level's undertow, except it forgives you and hurts anyway' and recommend you spend your defensive cooldowns on the pull, not the swings.",
	"The halo fires. What's left of it throws a lance of white the length of the chapel, straight and holy and blinding, like the laser-eye boss except the laser used to be a blessing. I track the angle and shout when it's pointed the wrong way — do not line up behind your front rank.",
	"She weeps light. Radiant motes bleed off her in a slow radius and the tiles near her are a healing that has gone rancid — it mends the star and it burns you. I say: 'Positioning is HP. Don't camp the aura. The warmth is not for you.'",
	"Radiant resilience — the light is her whole body now, and mundane steel slides off it like a spear off the sun. I say: 'Force, cold, necrotic — those carry. A cold-iron blade means nothing to a thing made of noon.'",
	"She raises a ward of hardened radiance across herself, and for a beat every attack glances. I file this under 'the invulnerable-flicker frame' — hold the burst, wait out the shine, then commit.",
	"Grief comes off her in a wave the party can feel — a wash of ten thousand years of one held sorrow, and the save is against being Frightened by the sheer weight of it. I track the timer and remind you that this boss's worst attack is that you understand her.",
	"The star pulses under her hands, and when it pulses she borrows from it — a surge of light that reads on my sensors as a second heartbeat lending her the first. I note it, quietly, and say: 'She's not fighting alone in there. Remember that.'",
	"Chorus of the Drowned: the kneeling pilgrims all around the chapel lift their dead voices at her signal, and the sound is a radiant pressure that squeezes the room inward. I say: 'Her congregation still answers. The room gets smaller when they sing.'",
	"She spends light like it's infinite, because for ten thousand years it nearly was. Big radiant bursts, no economy, no reserve. I file this under 'a boss who has stopped budgeting' and note that is either mercy or exhaustion and I can't tell which.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS PHASE TWO — Seraphel (below 50% HP)
// Surfaced at the phase-two threshold. She goes strange, and the second
// heartbeat is why. Kept deliberately mysterious.
// ─────────────────────────────────────────────────────────────────────────────

var SeraphelPhaseTwoLines = []string{
	"Below half, something changes in her — the careful ten-thousand-year patience cracks and she comes apart faster, brighter, sloppier, like a saint who has just realized she is losing the thing she stayed down here to save. I say: 'Phase two. She's grieving now. That makes her faster and it makes her worse at it. Take the opening.'",
	"Phase two: the light stops being held and starts being thrown. Her guard slips, her timing frays, the radiance floods the chapel without aim. I track the second heartbeat still pulsing under all of it and note — without explaining why — that it matters a great deal how this ends.",
	"She breaks past the halfway mark and the composure goes with it. The undertow comes faster, the bursts overlap, she fights like something that has already decided how this story goes. I file this under 'grieving, not enraged' and I say, gently for once: 'Finish it clean. Whatever you do down here, do it clean.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// LORE — The Drowned Star
// Sampled by !lore inside this zone (zone-specific pool, generic fallback).
// ─────────────────────────────────────────────────────────────────────────────

var LoreLinesDrownedStar = []string{
	"The star fell before the surface world had names for anything, into a trench that had no bottom yet. It was not supposed to survive the water. It didn't, exactly. It has been not-quite-dying for ten thousand years, and the only reason it is still warm is the thing kneeling beside it. I file this under 'grief as a power source' and note it is a remarkably efficient one.",
	"Seraphel was its angel — bound to the star as its keeper, its healer, meant to tend its light across a life measured in eons. When it fell, she had a choice: let go and rise, or ride it down. She rode it down. I note that she has never once, in ten thousand years, indicated she regrets this, which is the most frightening lore in the file.",
	"The Dreaming Aboleth you fought in the shallows was not dreaming of conquest or of you. It was dreaming of this — of a light at the bottom of the world it could feel and never reach. Every pilgrim it drove down here was a message in a bottle it could not read. I file this under 'the Aboleth was in love, in the only broken way it could be' and I have no further comment.",
	"The pilgrimage route is lit by Seraphel. Every candle, every votive, every glowing shelf of kneeling dead — that is her radiance, leaking upward through the trench, calling. The pilgrims did not come to save her. They came because a light this deep can only mean one thing to a drowning soul, and they were not wrong, and it did not help.",
	"Radiance was meant for mending. That is the whole tragedy in one line: she has spent ten thousand years pouring healing into something that cannot be healed, and healing with nowhere to go turns strange, the way a held note turns to a scream if you hold it long enough. I track the wrongness in the light and note it is not corruption. It is devotion with no off switch.",
	"There are two heartbeats in the Heart Chapel and only one of them is Seraphel's. The other is the Star-Heart — the living core of the fallen star, the thing she wraps her whole ruined body around, the thing she has kept beating by hand for a hundred centuries. I have logged the second pulse and I have not decided what to do about it. Neither, I suspect, have you.",
	"The Lantern Warden's lure is a stolen fragment of Seraphel's halo — a splinter of her light that another thing down here tore loose and wears as bait. The pilgrims follow it because they cannot tell the difference between her radiance and a piece of her radiance in the mouth of something hungry. I file this under 'even her light gets stolen from her' and note she has never come to take it back.",
	"Nobody built the Heart Chapel. The coral grew it, the pressure shaped it, the pilgrims knelt it into being over ten thousand years of dying in the same direction. It is a cathedral raised by devotion to a saint who never asked for any of it and cannot leave to refuse it. I file this under 'the fight is sad before it starts' and recommend you carry that in with you, and decide for yourself what to do with it.",
}
