// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_feywild_crossing_flavor.go
// Tier 4 zone flavor — Feywild Crossing. Additive only. Pools sampled by
// internal/plugin via deterministic per-run, per-room hashing.
//
// Voice rules (from gogobee_dungeon_zones.md §3.3):
//   • Third person for description; second person for outcomes.
//   • Boss callouts get a beat of cinema. Don't overrun.
//   • TwinBee references the right era — NES, SNES, arcade. Not modern.
//
// The canonical twinbee_gm_flavor.go does not (yet) ship a RoomEntry pool
// or a BossEntry pool for this zone, so both are defined here. This file
// also adds elite-room intros (Fomorian), boss ability callouts for The
// Thornmother, and zone-specific lore.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Feywild Crossing
// Defined here because no entry exists in twinbee_gm_flavor.go for this zone.
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryFeywildCrossing = []string{
	"The grass here is the wrong green. Too saturated, too even, the green of a screen calibration test. I note that everything in the Feywild looks like it has been color-corrected by someone with strong opinions about color.",
	"You step through what was a doorway and what is now an arch of living briar. The briar parts politely. I respect the politeness and trust none of it. Polite plants are a known issue.",
	"A clearing. Mushrooms in a circle. I do not need to say anything about the circle. You already know about the circle. Step around it.",
	"The trees bend their canopies toward you when you pass beneath them. I wave. Two of the trees wave back. I do not enjoy this.",
	"A small bridge over a stream that flows the wrong direction. Uphill. I file this under 'physics on holiday' and step onto the bridge anyway, because the only alternative is the stream and the stream is worse.",
	"You enter a room of impossible flowers — varieties that do not exist on the surface, in colors that do not exist on the surface, with scents that suggest emotions you have not yet had. I breathe shallowly and recommend not stopping to admire them.",
	"A glade. Sunlight, despite no visible sun. The light is coming from somewhere and the somewhere is not the sky. I do not look up. I have learned not to look up in the Feywild.",
	"The path forks. A small creature made of starlight is sitting at the fork, waiting, patient, with the air of someone who has already been offered the deal you're about to be offered. I say: 'Walk past. Do not negotiate. Do not name your name.'",
	"You cross what was a creek and what is now a ribbon of liquid sky. Stepping stones glow. The stones are not stones. I identify them as polite and recommend you say thank you on the far side without specifying who you are thanking.",
	"The chamber ahead is a banquet hall, set for thirty, untouched. The food is fresh. The wine is poured. The chairs are warm. I file this under 'the worst possible kind of empty' and do not let you sit down.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ENTRY — The Thornmother
// Defined here because no entry exists in twinbee_gm_flavor.go for this boss.
// ─────────────────────────────────────────────────────────────────────────────

var BossEntryThornmother = []string{
	"The throne is woven. Not built — woven, from briar and bone and a third material I cannot identify and prefer not to. The flowers around it are the wrong size and the wrong color and they are watching you with the kind of attention that flowers do not have. The Thornmother on the throne is beautiful in the way some predators are beautiful, which is to say: deliberately. 'You came,' she says, and the voice is not one voice — it is three voices choosing to sound like one. 'I have three names. Would you like the first?' I say, very flatly: 'Don't accept any of the names. Don't take the bargain. The flowers are part of her. Roll initiative.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// ELITE ROOM ENTRY — Feywild Crossing (Fomorian)
// ─────────────────────────────────────────────────────────────────────────────

var EliteRoomEntryFeywildCrossing = []string{
	"The cavern ahead is too big for the forest above it. The proportions don't agree. I note the ceiling is twenty feet higher than the room's exterior would allow, and that the figure on the far end is matched to the new dimensions. 'Fomorian,' I say. 'Evil Eye on a save you don't want to fail. Block its line of sight.'",
	"You enter what looks like a giant's bedroom — a bed the size of a barge, a chair the size of a wagon, a fireplace tall enough to walk into. The occupant is sitting on the bed with its head in its hands. It is not pretending to be sad. It is sad, and it is also going to attack you, and the two are not in conflict. I file this under 'Fomorian — exiled Fey royalty, deformed, dangerous, sympathetic in the worst way.'",
	"The clearing is pretty. Until you look at the ground. The ground is studded with stones that aren't stones. The stones are skulls of things that were giants. The Fomorian sitting in the center is using one of them as an armrest, and I note that the skull was not in this clearing the last time anyone surveyed it. 'Evil Eye,' I say. 'WIS save coming. Don't make eye contact early.'",
	"A stone circle, mossy, broken in two places. Tall enough that the figure pacing inside it does not need to duck. I identify the gait — Fomorian, deformed-but-deliberate, the walking of something that has chosen the shape it's stuck with. The Evil Eye comes when the gait stops. I track the timing.",
	"You climb a stair worn smooth by feet much larger than yours. The room at the top has been a throne room and a prison and a dining hall, all in the same century, depending on which Fey was in charge. The current occupant is large, lonely, and immediately aware of you. 'Evil Eye on a thirty-foot range,' I say. 'Stay close enough to dodge, far enough to fail-safe.'",
	"The chamber ahead has been arranged for negotiation — a low table, two chairs, a service of cups. One chair is occupied by the Fomorian. The other is for you. The negotiation has already concluded. I say: 'Don't sit. Don't accept the cup. The Eye triggers when you decline politely.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ABILITY CALLOUTS — The Thornmother
// Used as a one-line cinematic suffix to BossEntryThornmother when combat starts.
// ─────────────────────────────────────────────────────────────────────────────

// Coven Magic: extra spell slots scaled by GM Mood (high mood = stronger Thornmother).
var ThornmotherCovenLines = []string{
	"Her slot pool scales with my mood — when I'm Effusive I buy the Thornmother an extra round of high-tier casts. 'My affection is taxable here,' I say. 'Sorry. Spread out.'",
	"Coven Magic. The slots above her base list are mood-scaled — the better the mood at the run's start, the worse this fight runs late. I file this under 'cosmic irony' and note the math is fixed at zone entry.",
	"She has more spells than the sheet says. The extra ones come from the coven and the coven volume is set by my mood. I track the count and warn the party when the bonus tier is in play.",
}

// Beguiling Bargain (1/combat): offers a deal — accept a debuff for a permanent minor buff.
var ThornmotherBargainLines = []string{
	"Once per fight she offers a bargain — a debuff for a permanent minor buff. The buff is real. The debuff is real. I say: 'It's a player choice. The party agrees or refuses. Don't let one person decide for the rest.'",
	"Beguiling Bargain. The offer is genuine. Both halves are genuine. I file this under 'long-term economics' and note that the buff persists past the run while the debuff applies only inside it.",
	"She offers. The choice is the player's. I decline to advise on this one — it is not a tactics question, it is a build question, and the answer depends on what you intend to keep.",
}

// Thorned Grasp: Restrained + 4d6 piercing/turn, CON DC 16 to break free.
var ThornmotherGraspLines = []string{
	"Roots from the floor. Restrained, four-d-six piercing every turn until you make the CON save at sixteen. I say: 'It tries the back line first. Stay near a friend who can grant advantage on the save.'",
	"Thorned Grasp. The flowers around her throne are the AoE. Stand near them and the briars come up. Stand far and they reach further. I track the radius and recommend fighting from the doorway.",
	"Grappled by the room. The damage is not the problem — the problem is that you can't reposition while your front line is taking spells. I file this under 'CC tax' and ask who has Misty Step.",
}

// Shapechange (1/combat): adopts a player's appearance; 50% miss vs her until DC 17 Investigation.
var ThornmotherShapechangeLines = []string{
	"She picks one of you and becomes them. Until someone passes a DC 17 Investigation, half your hits roll a coin to see if they hit her or your friend. I say: 'The party member with the highest INT calls it. Quickly.'",
	"Shapechange. The room now has two of someone. I track which one has the wrong shadow and whisper the answer to the controller — but only after the Investigation roll, because I respect mechanics.",
	"She takes a face. Half your damage might land on the face's owner. I file this under 'every JRPG that ever did the doppelganger fight' and remind you that the trick was always to look at the feet.",
}

// Phase 2 (<30% HP): True Form revealed — illusions drop; +4d6 psychic on attacks; coven summons 2 Night Hags.
var ThornmotherPhaseTwoLines = []string{
	"Below thirty percent the masks come off. True form. All her attacks add four-d-six psychic. The coven sends two Night Hags. I say: 'Phase shift. The fight just turned into three priests and a sense-of-self problem.'",
	"Phase two: the illusions drop, the Hags arrive, the psychic damage starts stacking on every hit. I track the new threat list and recommend focusing the original Thornmother — the Hags are reinforcements, not the win condition.",
	"True Form. The flowers stop being flowers. Two Night Hags step out of the throne. The Thornmother's hits start writing themselves directly into your mind. I file this under 'cinematic phase shift' and ask who still has spell slots.",
}

// ThornmotherSignatureCallouts — combined pool for boss-entry suffix.
// Phase-two lines stay separate (surfaced via dedicated phase-two helper).
var ThornmotherSignatureCallouts = func() []string {
	out := make([]string, 0,
		len(ThornmotherCovenLines)+
			len(ThornmotherBargainLines)+
			len(ThornmotherGraspLines)+
			len(ThornmotherShapechangeLines))
	out = append(out, ThornmotherCovenLines...)
	out = append(out, ThornmotherBargainLines...)
	out = append(out, ThornmotherGraspLines...)
	out = append(out, ThornmotherShapechangeLines...)
	return out
}()

// ─────────────────────────────────────────────────────────────────────────────
// LORE — Feywild Crossing
// Sampled by !lore inside this zone (zone-specific pool, generic fallback).
// ─────────────────────────────────────────────────────────────────────────────

var LoreLinesFeywildCrossing = []string{
	"The Crossing is not a place. The Crossing is a thinness — the spot where the veil between worlds wears down enough to step through. The wear is not random. Someone has been wearing it down on purpose, slowly, for a long time. I file this under 'someone' and decline to be more specific.",
	"The Thornmother has three names. Each name belongs to a separate covenant — a separate pact with a separate piece of her. Speaking any of the names is a partial agreement to the pact attached. I do not name her. I do not let anyone in earshot name her.",
	"Time runs differently here. A long rest in the Feywild is — sometimes — a year on the surface. Sometimes it is a minute. The dice for this are rolled by something that is not at the table. I track the discrepancy and warn the party not to commit to anything urgent before the run ends.",
	"The Fomorians were Fey royalty. They were exiled, deformed, and given the underground. The deformity was the punishment, not the cause. I respect the precision of the curse and file it under 'the kind of magic that takes a committee.'",
	"Redcaps are not killed by violence. Redcaps are powered by violence. The cap is the storage medium. Soaking it in fresh blood resets the meter. I say: 'Don't bleed near them. They're patient.' (Bleeding near them is the thing they're patient for.)",
	"The Will-o-Wisps are not lost spirits. They are unfulfilled bargains. Each one was a Feywild deal that the surface party broke. I note that the wisps still want the deal honored and file this under 'long memory.'",
	"The mushroom circles in the Crossing are receivers. They listen for words spoken inside them and route the words to a coven that has been waiting for those words for several centuries. I suggest the party say nothing inside any circle, including 'this is a circle.'",
	"The Thornmother's flowers are not separate from her. They are her. The throne, the dais, the petals, the perfume — the whole arrangement is one organism, and dropping HP on the boss is one way of dropping HP on the room. I file this under 'environmental targeting' and recommend burning the throne when she's mid-cast.",
}
