// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_abyss_portal_flavor.go
// Tier 5 zone flavor — The Abyss Portal. Additive only. Pools sampled by
// internal/plugin via deterministic per-run, per-room hashing.
//
// Voice rules (from gogobee_dungeon_zones.md §3.3):
//   • Third person for description; second person for outcomes.
//   • Boss callouts get a beat of cinema. Don't overrun.
//   • TwinBee references the right era — NES, SNES, arcade. Not modern.
//
// The canonical twinbee_gm_flavor.go ships BossEntryBelaxath (wired in
// dnd_zone_narration.go) but not RoomEntryAbyssPortal, so the room-entry
// pool lives here. This file also adds elite-room intros (Marilith), boss
// ability callouts for Belaxath the Undivided, and zone-specific lore.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — The Abyss Portal
// Defined here because no entry exists in twinbee_gm_flavor.go for this zone.
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryAbyssPortal = []string{
	"You step through what was a doorway and into a chamber where the geometry has stopped agreeing with itself. The far wall is closer than the near wall. The ceiling is in two places. TwinBee files this under 'Euclidean violation' and recommends not lingering on the question of which floor is the real floor.",
	"The corridor breathes. Not the wind — the corridor. The walls expand and contract on a cycle that is approximately, but not exactly, a heartbeat. TwinBee matches the rhythm involuntarily and dislikes the experience.",
	"A chamber where reality has a seam. The seam is visible. It runs floor-to-ceiling along the north wall and what is visible through the seam is not the next room. TwinBee does not look directly at it and recommends you adopt the same policy.",
	"You enter what was once a chapel — to which god, you cannot tell, because the iconography has been overwritten in a script that hurts to read. The altar is intact. The thing on the altar is not what was placed there. TwinBee says nothing and moves you past it.",
	"The air pressure is wrong. Not low — wrong. There is a pressure that is not physical and it is leaning on you with the patience of a mountain. TwinBee identifies it as psychic ambient and recommends moving briskly.",
	"A staircase. The stairs go down on the way up and up on the way down. TwinBee does not pause to verify this. Takes the stairs in the intended direction and the direction works, mostly.",
	"The corridor ahead is lit by something that is not fire. The light is red but the wrong red — the red of an alarm in a system that has never been fully implemented. TwinBee files this under 'demonic ambient' and shortens its stride.",
	"You enter a room that has been used as a portal anchor. The anchor is still in place. The portal is not, but the place where it was is still warm. TwinBee tracks the residual energy and notes the portal is not the only one — there are other warm spots, in other rooms, in a pattern.",
	"A garden. Indoors. The plants are not plants. TwinBee identifies them as 'demonic ornamentals — mid-tier, decorative, will react to perfumes' and recommends not perspiring near them.",
	"The chamber ahead has been a courtroom and is now a kitchen. The transition was not deliberate. The transition was the result of someone with insufficient training opening a portal and not closing it. TwinBee notes the timing must have been comedic and is not laughing.",
}

// ─────────────────────────────────────────────────────────────────────────────
// ELITE ROOM ENTRY — The Abyss Portal (Marilith)
// ─────────────────────────────────────────────────────────────────────────────

var EliteRoomEntryAbyssPortal = []string{
	"The chamber ahead is a barracks. Six racks of weapons along one wall, six saddles along another — except the saddles are wrong shape, and the weapons are six matched longswords, identical in length, identical in grip, identical in the subtle rune-work along the hilts. TwinBee identifies the silhouette in the room before TwinBee identifies the species and says: 'Marilith. Six longswords. Seven attacks a turn. Never stand directly in front.'",
	"You enter a sparring gallery. The dummies are arranged in the kind of asymmetric pattern that suggests a six-armed practitioner. The practitioner is at the far end, mid-routine. She does not stop the routine. She incorporates you into it. TwinBee files this under 'Marilith — demonic captain, treats encounters as drills.'",
	"A war room. Maps on the table, troop positions marked, a planning session in progress. The general at the head of the table has six arms and a snake's lower body and is not surprised by your arrival — was, in fact, expecting you on a different day, with different reinforcements, and is mildly disappointed by both. TwinBee says: 'Marilith. Parry on every attack. The fight is decided by who has more reactions.'",
	"The corridor opens into a colonnade — twelve columns, six on each side, perfect symmetry. The figure between the central columns is a Marilith and she is using the symmetry — three swords engaged with the columns, three free. TwinBee notes the geometry favors her and recommends fighting in the next room.",
	"A throne room, smaller than the Belaxath one. The throne is occupied by a Marilith who is not sitting — she is coiled, three feet of snake below the seat, the rest of her vertical and ready. The longswords are all drawn. None of them are pointed at you. TwinBee tracks the angles and says: 'Reactive trait. She gets two reactions. Don't open with a counterspell-bait.'",
	"You enter what is clearly a guard post. The Marilith on duty is not bored. She is patient in the particular way that someone who has been a soldier for fifteen hundred years is patient. The first three swords are already raised. The other three are coming up as you cross the threshold. TwinBee files this under 'professional courtesy' and asks who has Misty Step.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ABILITY CALLOUTS — Belaxath the Undivided
// Used as a one-line cinematic suffix to BossEntryBelaxath when combat starts.
// ─────────────────────────────────────────────────────────────────────────────

// Multiattack: Longsword + Whip.
var BelaxathMultiattackLines = []string{
	"Longsword and whip every turn. The whip pulls. The longsword finishes. TwinBee says: 'Don't let the whip set the position. Reaction-cancel the pull if you've got it.'",
	"Two attacks: one ranged-pull, one melee-finish. TwinBee files this under 'fighting-game grappler' and notes the combo is fixed in order — survive the whip, survive the round.",
	"He swings the longsword while the whip is already coiled around your front line. The bite is the second hit, every time, and it's the one calibrated for kills. TwinBee tracks the order and reminds the party that the whip's not the kill — the whip's the setup.",
}

// Fire Aura: 10d6 fire to all melee attackers each turn; weapon attacks +3d6 fire.
var BelaxathAuraLines = []string{
	"Stand within five feet and take ten-d-six fire at the start of his turn. His weapons add three-d-six fire on top. TwinBee says: 'Melee tax. Plan rotations. Don't camp a tile near him.'",
	"Fire Aura. The room near him is a damage zone. TwinBee files this under 'positioning is HP' and recommends the back line do their job from the back line.",
	"Anyone in melee takes ten-d-six fire on his upkeep regardless of whether he hits them. The fire ignores resistance later. TwinBee tracks the timer and reminds the tank that being the wall is a budget item, not a strategy.",
}

// Lightning Discharge (recharge 5–6): 120-ft line, 12d6 lightning, DEX DC 20 half.
var BelaxathLightningLines = []string{
	"One-twenty-foot line, twelve-d-six lightning, DEX DC 20 for half. Recharge five-or-six. TwinBee says: 'Don't line up. The line is the whole room if you're sloppy.'",
	"Lightning Discharge. The line is long enough to clip the back rank from where the front rank is standing. TwinBee tracks the recharge die and shouts the angle when the discharge points the wrong way.",
	"Like the laser-eye boss in Contra, except the boss has six arms and the laser is on a recharge counter and the laser is sometimes lightning. TwinBee files this under 'know where the line is' and notes the line is wherever you're easiest to hit.",
}

// Death Throes: on death, 30-ft radius, 20d6 fire, DEX DC 20 half — destroys non-legendary equipment on failed save.
var BelaxathDeathThroesLines = []string{
	"He explodes when he dies. Twenty-d-six fire in a thirty-foot radius. DEX DC 20 for half. Failure also destroys non-legendary equipment on the wearer. TwinBee says: 'Drop him with range. Save your gear.'",
	"Death Throes. The kill shot is the easy part. The Death Throes are the hard part. TwinBee files this under 'cost-of-victory mechanics' and recommends positioning before the kill, not after.",
	"He goes out the way some bosses do — taking the room with him. Twenty-d-six on a save you might fail with disadvantage from the previous round's Frightened. TwinBee tracks the radius and reminds the party that 'half damage' of twenty-d-six is still 'roll a new character.'",
}

// Demonic Resilience: resist cold/fire/lightning; immune to poison; immune to non-magical physical.
var BelaxathResilienceLines = []string{
	"Resists cold, fire, lightning. Immune to poison. Immune to non-magical physical. TwinBee says: 'Magic damage only. Force, radiant, psychic — those carry. Mundane mace doesn't.'",
	"Demonic Resilience. The standard demonic-lord package. TwinBee files this under 'check your damage type before you commit a slot' and notes the Magic Resistance trait stacks on top — advantage on saves vs spells.",
	"Three resistances and two immunities. The window for damage is narrow. TwinBee tracks party loadouts and reminds the casters that radiant damage is not optional in this fight — it is the answer.",
}

// Phase 2 (<40% HP): grows to Huge size; advantage on all attacks; Death Throes recharge 4–6.
var BelaxathPhaseTwoLines = []string{
	"Below forty percent he grows. Huge size, advantage on every attack, and Death Throes is now on a four-or-six recharge — meaning if you don't kill him cleanly, he might detonate in your face mid-fight. TwinBee says: 'Phase shift. Don't graze him below forty.'",
	"Phase two: bigger, faster, and the explosion is no longer reserved for the kill. TwinBee tracks the new recharge and recommends every burst window the party can buy.",
	"He grows. The room shrinks. Advantage on swings. The Death Throes timer becomes a real timer — if a recharge lands at the wrong moment, the room ends. TwinBee files this under 'fights you have to finish in two rounds or accept the consequences.'",
}

// BelaxathSignatureCallouts — combined pool for boss-entry suffix.
// Phase-two lines stay separate (surfaced via dedicated phase-two helper).
var BelaxathSignatureCallouts = func() []string {
	out := make([]string, 0,
		len(BelaxathMultiattackLines)+
			len(BelaxathAuraLines)+
			len(BelaxathLightningLines)+
			len(BelaxathDeathThroesLines)+
			len(BelaxathResilienceLines))
	out = append(out, BelaxathMultiattackLines...)
	out = append(out, BelaxathAuraLines...)
	out = append(out, BelaxathLightningLines...)
	out = append(out, BelaxathDeathThroesLines...)
	out = append(out, BelaxathResilienceLines...)
	return out
}()

// ─────────────────────────────────────────────────────────────────────────────
// LORE — The Abyss Portal
// Sampled by !lore inside this zone (zone-specific pool, generic fallback).
// ─────────────────────────────────────────────────────────────────────────────

var LoreLinesAbyssPortal = []string{
	"The portal was not summoned. The portal was torn, from the other side, by Belaxath, with purpose, over thirty years. TwinBee notes the duration matters: this was not a moment of carelessness on the part of a surface mage. This was a project, on the demonic side, executed with patience.",
	"The site was a temple, before. Then a fortress, before. Then a meadow, before. The meadow is the original use of the land — the temple was built to seal something the meadow had been quiet over. TwinBee files this under 'land memory' and notes the seal worked for fourteen centuries.",
	"The Shard of the Abyss is the closing key. It is not optional. The portal cannot be closed without it, and the shard cannot be replaced — there is exactly one. TwinBee notes the always-drop guarantee on the loot table is a story commitment, not a generosity.",
	"The Marilith captains in the outer chambers are veterans of wars that did not happen on this plane. The wars happened in the Abyss, between demonic factions, over scheduling — which Layer got which incursion priority. TwinBee finds this both bureaucratic and terrifying.",
	"Belaxath is named 'the Undivided' because he refused to take a side in the Layer-Seven schism. The other Balors took sides. The other Balors are mostly dead. Belaxath remained intact and inherited their soldiers, in stages, over the schism's two-decade arc. TwinBee files this under 'patience as competitive advantage.'",
	"The Vrock spores last for twenty-four hours after exposure. The save is the start of the timer, not the end. TwinBee suggests the party not assume the encounter is over when the room is clear — the spores keep ticking on the way to the next.",
	"The Quasits in this zone are not random encounters. Each one has been individually deployed as a scout, with instructions, by a specific Hezrou. TwinBee notes the chain of command is real and that killing a Quasit causes the Hezrou it reports to to know exactly where you are.",
	"The portal is not the only opening. There are seven smaller openings in this zone — pinholes, nothing more — that Belaxath has been using to bring through reinforcements one at a time. The pinholes close when Belaxath dies. TwinBee files this under 'killing the boss is also a public service' and notes the surface clergy will be relieved.",
}
