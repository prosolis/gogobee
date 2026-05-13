// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_sunken_temple_flavor.go
// Tier 2 zone flavor — Sunken Temple of Dar'eth. Additive only. Pools
// sampled by internal/plugin via deterministic per-run, per-room hashing.
//
// Voice rules (from gogobee_dungeon_zones.md §3.3):
//   • Third person for description; second person for outcomes.
//   • Boss callouts get a beat of cinema. Don't overrun.
//   • TwinBee references the right era — NES, SNES, arcade. Not modern.
//
// The canonical twinbee_gm_flavor.go does not (yet) ship a RoomEntry pool
// or a BossEntry pool for this zone, so both are defined here. Elsewhere
// (Goblin Warrens, Crypt of Valdris, Forest of Shadows), zone files only
// add elite/signature/lore overlays — the canonical file owns RoomEntry.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — Sunken Temple of Dar'eth
// Defined here because no entry exists in twinbee_gm_flavor.go for this zone.
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntrySunkenTemple = []string{
	"You step into water that's been in this room for thirty years. The floor is tiled and slick. The pillars are barnacled at chest height — that's the old waterline. The new waterline is at your ankles. TwinBee notes the temple is partway through deciding which it prefers.",
	"The chamber is half-flooded and half-lit. Salt in the air, salt on the walls, salt in places nothing oceanic should be reaching. TwinBee files this under 'unwell' and proceeds.",
	"Glyphs cover the far wall in a script that doesn't match anything TwinBee recognizes. The angles are wrong on purpose. It keeps reading anyway, because it always tries, and stops when the reading starts to feel reciprocal.",
	"The water in this room is not moving. Not the way still water doesn't move — the way a held breath doesn't move. TwinBee suggests not disturbing it more than necessary.",
	"Pillars rise from water that goes deeper than the room should allow. TwinBee tests the depth with the haft of a polearm and stops at the point where the haft stops finding bottom.",
	"A vaulted ceiling that's mostly intact. Water pools in places it shouldn't, drips from places it can't be coming from. TwinBee identifies the temple as 'wet on principle' and leaves it at that.",
	"The mosaics on the floor are still legible under the water. They show a procession toward something that is decidedly not the god the temple was built for. TwinBee notes the mosaics are facing inward — they were laid for the thing inside, not for visitors.",
	"You hear something move in the water. Not a splash — a displacement. The kind of displacement that happens when a large thing decides to be in a different part of the room than it was a moment ago. TwinBee acts unbothered and is bothered.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ENTRY — The Dreaming Aboleth
// Defined here because no entry exists in twinbee_gm_flavor.go for this boss.
// ─────────────────────────────────────────────────────────────────────────────

var BossEntryDreamingAboleth = []string{
	"The chamber opens onto a pool that has no business being this large in a building this size. Something underneath the surface shifts, and the surface is suddenly the smallest part of what you're looking at. The Aboleth does not surface. It does not need to. Its mind arrives in your mind first, polite, ancient, deeply uninterested in your comfort. 'Welcome,' it says, and the word is in your voice. TwinBee says, very evenly, 'Don't agree to anything. Don't answer questions. Roll initiative.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// ELITE ROOM ENTRY — Sunken Temple (Water Elemental)
// ─────────────────────────────────────────────────────────────────────────────

var EliteRoomEntrySunkenTemple = []string{
	"The water in the chamber ahead is moving. Not currents — purpose. It's gathering toward a center, taking on a shape, deciding what it wants its arms to look like this time. TwinBee says: 'Water elemental. The room is the enemy.'",
	"You enter a hall where the floor is six inches of standing water and the standing water is, on closer inspection, watching you. A column of it rises and walks toward you with the deliberate gait of something that did not learn to walk from a creature with legs. TwinBee respects the originality.",
	"The room hums at a frequency you feel in your teeth. The water rises into a vaguely humanoid pillar and the pillar steps forward. TwinBee notes this is the kind of fight where 'fall back to higher ground' is not a useful idea — there is no higher ground. The room is the ground.",
	"A column of water becomes a hand becomes a torso becomes a thing intent on contact. TwinBee files water elementals under 'fights where lightning damage is fun' and asks who has lightning damage today.",
	"The temple's oldest mechanism activates as you enter — a font in the ceiling drains, the floor floods another inch, and the new water gathers itself into a sentry. TwinBee says: 'It's been waiting since the day they built this. Be brief.'",
	"Water rises in a vortex in the chamber's center. The vortex slows. The vortex opens what passes for eyes. TwinBee respects the staging and is not entertained by it.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ABILITY CALLOUTS — The Dreaming Aboleth
// Used as a one-line cinematic suffix to BossEntryDreamingAboleth.
// ─────────────────────────────────────────────────────────────────────────────

// Tentacle Multiattack: 3 hits; on-hit Diseased (no magical healing 24h until cured).
var AbolethTentacleMultiattackLines = []string{
	"Three tentacles, three rolls. Each on-hit risks Diseased — no magical healing for 24 hours until cured. TwinBee says: 'Cleric's tools come back online tomorrow. Survive today.'",
	"The Aboleth's tentacles arrive in sequence, three of them, the hits compounding. The disease isn't the damage — the disease is the design. Magical healing fails until you cleanse. TwinBee files this under 'durability problem.'",
	"Three attacks, one turn. Any landing tentacle leaves a mark that locks out magical healing. TwinBee suggests potions, rest, and the kind of patience that pretends to be patience but is mostly grim arithmetic.",
}

// Enslave: recharge 6; WIS DC 14 or Charmed; player skips turn, drifts toward Aboleth.
var AbolethEnslaveLines = []string{
	"The Aboleth speaks in your head and the speech is a question. WIS DC 14 or you spend your turn walking, peacefully, toward the water. TwinBee says: 'Recharge 6. It will try this again. Make the save.'",
	"Enslave. The word in your head sounds like your own thought. It isn't. WIS save now. On a fail, your turn becomes the Aboleth's turn and you become the wrong piece on the wrong side of the board.",
	"Charm-class effect, but worse than charm — it's a directional pull. You drift toward the pool whether you want to or not. TwinBee shouts your save aloud so you don't mistake it for an idea you had.",
}

// Mucus Cloud: melee attackers CON DC 14 or skin→membrane (6d6 acid if not submerged).
var AbolethMucusCloudLines = []string{
	"Anyone in melee range gets a CON DC 14. Fail and your skin starts converting to a membrane that needs water to stay viable. Out of water it's 6d6 acid per turn. TwinBee says: 'Don't punch the fish.'",
	"The Aboleth's mucus does something to skin that skin should not do. Membrane formation. Submerged is fine. Dry is the problem — 6d6 acid until it ends. TwinBee revises the engagement plan: ranged only.",
	"Like the slime debuff in old console RPGs — except this one wants you to stay in the water and dissolves you if you don't. TwinBee respects the elegance of the threat and dislikes everything else about it.",
}

// Legendary Actions (3/round): Detect / Tail Swipe (2 LA) / Psychic Drain (3 LA, max-HP cut).
var AbolethLegendaryActionLines = []string{
	"The Aboleth gets three legendary actions per round. Detect (1), Tail Swipe (2), Psychic Drain (3, cuts max HP). TwinBee tracks the budget and reminds you when it spends.",
	"Legendary Actions: 3 budget. Detect is cheap and reads your hand. Tail Swipe is medium and hurts. Psychic Drain is expensive and lasting — your max HP is the price. TwinBee suggests bursting it before it banks.",
	"Three LA per round. The Aboleth will spend them on you and TwinBee will narrate each one with the specific resigned cadence of a GM who would prefer the boss had two LA.",
}

// AbolethSignatureCallouts — combined pool for boss-entry suffix.
// Legendary action lines stay in the pool; Aboleth has no phase-two split.
var AbolethSignatureCallouts = func() []string {
	out := make([]string, 0,
		len(AbolethTentacleMultiattackLines)+
			len(AbolethEnslaveLines)+
			len(AbolethMucusCloudLines)+
			len(AbolethLegendaryActionLines))
	out = append(out, AbolethTentacleMultiattackLines...)
	out = append(out, AbolethEnslaveLines...)
	out = append(out, AbolethMucusCloudLines...)
	out = append(out, AbolethLegendaryActionLines...)
	return out
}()

// ─────────────────────────────────────────────────────────────────────────────
// LORE — Sunken Temple of Dar'eth
// Sampled by !lore inside this zone (zone-specific pool, generic fallback).
// ─────────────────────────────────────────────────────────────────────────────

var LoreLinesSunkenTemple = []string{
	"Dar'eth was a sea god before the cult that built the temple decided he wasn't sea god enough and started worshipping something else under his roof. The temple kept the name. The thing being worshipped did not. TwinBee notes the bait-and-switch with professional appreciation.",
	"The tide that withdrew thirty years ago didn't withdraw on its own. It was held back. By what — by whom — is not advertised, but TwinBee has seen the kind of pressure-line on the outer walls that suggests a decision was made and something is still making it.",
	"The Aboleth has been here longer than the temple has been a temple. The temple was built around the Aboleth. The cult thought they were containing it. The Aboleth thought they were furnishing the room. Both parties remained satisfied with the arrangement until the cult ran out of cultists.",
	"The Kuo-toa here are not the local stock. They were brought. They are loyal in the way that a mind not entirely their own can be loyal. TwinBee considers their loyalty a symptom and the Aboleth the cause.",
	"Aboleth memory is collective and ancient — it remembers things from before there were people to remember things. TwinBee finds this genuinely unsettling because the Aboleth's memory of you is identical to its memory of the cult and the temple's first architect: 'a thing that was here.' Past tense, even when you're standing right there.",
	"The mucus residue in the upper chambers is recent. The Aboleth has been moving through the temple in the last few weeks — not just sitting in the central pool. Something has been making it restless. TwinBee suggests not being the thing that has been making it restless.",
	"The phylactery shard you'll find here is not Valdris's. It belongs to a different lich entirely. TwinBee notes there are now two phylacteries in your itinerary and refuses to draw connecting lines on the map yet.",
}
