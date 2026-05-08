// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_goblin_warrens_flavor.go
// Tier 1 zone flavor — Goblin Warrens. Additive only. Pools sampled by
// internal/plugin via deterministic per-run, per-room hashing.
//
// Voice rules (from gogobee_dungeon_zones.md §3.3):
//   • Third person for description; second person for outcomes.
//   • Boss callouts get a beat of cinema. Don't overrun.
//   • TwinBee references the right era — NES, SNES, arcade. Not modern.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// ELITE ROOM ENTRY — Goblin Warrens (Hobgoblin Warchief)
// ─────────────────────────────────────────────────────────────────────────────

var EliteRoomEntryWarrens = []string{
	"The chamber widens. A hobgoblin in lacquered scale stands at the center, arms folded, watching the door. The lesser goblins go quiet. TwinBee notes the silence is the part you should be paying attention to.",
	"A war-banner hangs from the ceiling — three clans stitched together with rough thread. Beneath it, a Warchief turns to face you with the slow patience of someone who has done this part a hundred times. TwinBee straightens up out of habit.",
	"The corridor opens into a drilling ground. Goblins in formation. A Hobgoblin barking orders that stop the moment your boot hits the threshold. TwinBee has seen this exact composition in Shining Force and finds the parallel unhelpful.",
	"You smell the polish before you see the armor. The Warchief's blade is oiled, his straps are tight, his stance is correct. TwinBee acknowledges, with reluctance, that this one was trained.",
	"A circle of torches. A throne of stacked shields. The hobgoblin seated on it does not stand. He gestures, single-finger, in a way TwinBee universally translates as 'come here.' You go there.",
	"The room has been cleared for combat. Rugs rolled, braziers spaced evenly, sand on the floor for grip. TwinBee respects the preparation and is professionally annoyed by it.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ABILITY CALLOUTS — Grol the Unbroken
// Used as a one-line cinematic suffix to BossEntryGrol when combat starts.
// ─────────────────────────────────────────────────────────────────────────────

// Surprise Attack: +2d6 if player has not acted this combat.
var GrolSurpriseAttackLines = []string{
	"Grol moves before you do. He always moves before you do. TwinBee notes — too late — that the first hit in this fight is his.",
	"The cleaver comes around in an arc that started before you were in the room. TwinBee winces in advance.",
	"He's not waiting for the fight to start. The fight starts when he says it does. TwinBee marks the lesson and files it under 'next time.'",
}

// Heart of Hruggek: crits deal max damage (no roll).
var GrolHeartOfHruggekLines = []string{
	"Grol crits and the dice don't roll. TwinBee has seen this Bugbear blessing once before. The number is the maximum number, every time, no negotiation.",
	"His critical lands flat-max. No spread, no luck — just Hruggek's blessing collecting on a long-overdue debt. TwinBee notes the damage with grim respect.",
	"The crit hits like a Final Fantasy 'Berserk' status — capped, deterministic, unkind. TwinBee skips the roll and writes the maximum.",
}

// Terrifying Roar: 1/combat; allies +2 to hit for 2 turns; player WIS DC 13 or Frightened.
var GrolTerrifyingRoarLines = []string{
	"Grol throws his head back and roars. The room shakes. The goblins around him stand straighter. TwinBee says: 'WIS save. Now.'",
	"The roar isn't a sound — it's a pressure change. Your knees know about it before your ears do. TwinBee respects the technique and asks you to roll Wisdom.",
	"He bellows once. The torchlight bends. The goblins answer with cheers that sound rehearsed. TwinBee notes the buff is up; you have two turns to outlast it.",
}

// GrolSignatureCallouts — combined pool the boss-entry composer samples
// from. Concrete ability pools above are kept distinct so future per-turn
// ability hooks (D6) can wire to a specific trigger.
var GrolSignatureCallouts = func() []string {
	out := make([]string, 0,
		len(GrolSurpriseAttackLines)+len(GrolHeartOfHruggekLines)+len(GrolTerrifyingRoarLines))
	out = append(out, GrolSurpriseAttackLines...)
	out = append(out, GrolHeartOfHruggekLines...)
	out = append(out, GrolTerrifyingRoarLines...)
	return out
}()

// ─────────────────────────────────────────────────────────────────────────────
// LORE — Goblin Warrens
// Sampled by !lore inside this zone (zone-specific pool, generic fallback).
// ─────────────────────────────────────────────────────────────────────────────

var LoreLinesWarrens = []string{
	"The Warrens are old. Not goblin-old — older. The tunnels predate the goblins and the goblins know it. They've widened doorways meant for something taller and braced ceilings meant for something heavier. TwinBee has theories about what was here first. None of them are reassuring.",
	"Grol united the three clans by killing each of their war-chiefs in single combat, in order, in one afternoon. The fourth clan declined to send a chief and pledged loyalty by letter. TwinBee finds the letter, professionally, very funny.",
	"The graffiti on the walls dates the occupation. Goblins date by leaders and the names go back six generations. TwinBee can read goblin script when pressed and notes that the third-oldest name is one a paladin order would still recognize.",
	"The Merchant's Road above doesn't know about the Warrens. The Merchant's Road above used to know about the Warrens. TwinBee was here when it stopped knowing, and notes that the people in charge of Knowing About Things were the first to disappear.",
	"Hobgoblins hold formation; goblins hold grudges; bugbears hold pretty much anything they pick up. TwinBee offers this not as a joke but as a tactical primer that will save you a lot of HP if you remember it.",
	"The belt Grol wears is not a trophy. It is a contract. Each tooth on it is a debt some troll forgave when the wearing of the belt began, and the debts run in both directions. TwinBee strongly suggests not breaking the belt unless you mean to.",
}
