// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_forest_shadows_flavor.go
// Tier 2 zone flavor — Forest of Shadows. Additive only. Pools sampled by
// internal/plugin via deterministic per-run, per-room hashing.
//
// Voice rules (from gogobee_dungeon_zones.md §3.3):
//   • Third person for description; second person for outcomes.
//   • Boss callouts get a beat of cinema. Don't overrun.
//   • TwinBee references the right era — NES, SNES, arcade. Not modern.
//
// Room-entry and boss-entry pools for this zone live in the canonical
// twinbee_gm_flavor.go (RoomEntryForestShadows, BossEntryHollowKing) and
// are not duplicated here. This file adds elite-room intros, boss
// ability callouts, and zone-specific lore.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// ELITE ROOM ENTRY — Forest of Shadows (Green Hag)
// ─────────────────────────────────────────────────────────────────────────────

var EliteRoomEntryForestShadows = []string{
	"The trees thin into a ring of stones the forest grew around but never claimed. In the center: a hut on chicken-thin legs that has no business being upright. The door is open. The hag inside the door has been expecting you. I was hoping for the other kind of clearing.",
	"The path ends at a pond that shouldn't be a pond — too still, too dark, the wrong kind of reflective. A figure is wading at the far edge, washing something that probably wasn't laundry. She turns. She smiles. I say, very quietly, 'green hag,' and stop there.",
	"You smell the cooking before you see the cook. Sweet, herbal, faintly wrong. The clearing ahead has a fire, a pot, and a woman who is too tall and too thin and whose teeth are arranged with a creativity that a normal mouth could not justify. I suggest not eating anything she offers.",
	"A circle of toadstools, perfectly spaced. In the center, a stump with a teacup on it. Steam still rising. I identify the trap structure on instinct — fey hospitality, hag rules, a rite that asks one question and punishes wrong answers permanently.",
	"The forest gets quieter the way a room gets quieter when the host walks in. She's already in the clearing when you arrive — bone necklace, briar crown, eyes the color of pond-bottom. 'Travelers,' she says, the way someone says 'lunch.' I straighten up.",
	"The trees here are leaning in, not away. That's the tell. I note the change in posture and look for the center the trees are listening to. The center is the hag. The hag has been listening back.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ABILITY CALLOUTS — The Hollow King
// Used as a one-line cinematic suffix to BossEntryHollowKing when combat starts.
// ─────────────────────────────────────────────────────────────────────────────

// Corrupting Aura: melee-range targets WIS DC 14 each turn or lose bonus action.
var HollowKingCorruptingAuraLines = []string{
	"Stepping into melee range puts you in his aura. The forest in your head goes quiet. I say: 'WIS save each turn or lose your bonus action. Adjust your spacing.'",
	"The air around the Hollow King isn't air. It's the absence of something — focus, intent, the part of you that decides the second small action of a turn. I note the WIS DC 14 and recommend ranged.",
	"His aura presses on you the way a closed room presses on a held breath. Bonus actions become a gamble. I file this under 'positional' and recommend accordingly.",
}

// Root Surge: recharge 5–6; Restrain (STR DC 15) + 2d8 bludgeoning.
var HollowKingRootSurgeLines = []string{
	"Roots erupt under you like the floor of the forest decided to participate. STR DC 15 or you don't move next turn. Either way, 2d8 bludgeoning. I say: 'Recharge 5–6. He'll do this again.'",
	"The ground splits. The roots come up coordinated, like fingers — I use the word 'fingers' deliberately and dislike itself for the accuracy. Restrained on a fail. I suggest breaking free as a priority.",
	"Like the vine traps in Castlevania III's stage 5 — except the vine is also a damage source and also a lock on your action economy. I respect the multitasking and resent the design.",
}

// Devour Light: extinguishes magical light for 2 turns; player AC -2.
var HollowKingDevourLightLines = []string{
	"The Hollow King exhales and your magical light goes out. Not flickers — out. I note the AC -2 for two turns and remind you that the dark is also part of the encounter now.",
	"Every magical light in the room dims to nothing. The natural torchlight remains, dimmer than it was. I say: 'AC penalty for two rounds. Fight smarter or fight harder. Both works.'",
	"The forest gets darker in a way that wasn't there a moment ago. I can still see — I always can — but the AC -2 is real and the next round of attacks will feel it.",
}

// Phase 2 (<40% HP): summons 2 Dire Wolves; gains Reckless Attack.
var HollowKingPhaseTwoLines = []string{
	"At 40% HP the Hollow King throws his head back and the forest answers. Two dire wolves come through the brush at full speed. I say: 'Phase shift. He's also reckless now. Use it.'",
	"The antlers bend back, the eyes lose what was left of their mercy, and somewhere in the forest something with paws starts running toward you. Two dire wolves, incoming. Reckless Attack on the boss — advantage to him, advantage to you. I pick a target and wait for you to commit.",
	"Half-health is the line for most bosses. The Hollow King's line is forty percent and he hits it dramatically. Wolves at his flanks, attacks now reckless. I respect the choreography.",
}

// HollowKingSignatureCallouts — combined pool for boss-entry suffix.
// Phase-two lines stay separate (surfaced via dedicated phase-two helper).
var HollowKingSignatureCallouts = func() []string {
	out := make([]string, 0,
		len(HollowKingCorruptingAuraLines)+
			len(HollowKingRootSurgeLines)+
			len(HollowKingDevourLightLines))
	out = append(out, HollowKingCorruptingAuraLines...)
	out = append(out, HollowKingRootSurgeLines...)
	out = append(out, HollowKingDevourLightLines...)
	return out
}()

// ─────────────────────────────────────────────────────────────────────────────
// LORE — Forest of Shadows
// Sampled by !lore inside this zone (zone-specific pool, generic fallback).
// ─────────────────────────────────────────────────────────────────────────────

var LoreLinesForestShadows = []string{
	"The forest used to be a forest. That sentence is doing more work than it looks like it's doing. Something was let in — not summoned, not invaded, let in — and the forest has been a different kind of forest ever since. I have theories about who did the letting in. None of the theories are reassuring.",
	"The Hollow King wasn't always hollow. He was a guardian once, the kind of guardian a forest gets when the forest is doing well. Then the forest started to do less well, and the guardian held on past the point where holding on was the kind thing to do. I find the arc tragic and am not interested in absolving the outcome.",
	"The bandits in the woods aren't local. They came from somewhere else, found the forest in its current state, and decided the cover was worth the risk. I note their cookfires are recent and their numbers are growing. Someone is recruiting.",
	"Owlbears shouldn't pack-hunt. These do. I have logged three separate engagements where two owlbears coordinated in a way that felt taught. Something in the corruption is rewriting their behavior. I doesn't like the implication.",
	"The bioluminescent fungi are not native. They arrived with the corruption and they are part of the corruption — they fluoresce on a frequency that does something to the mood-banding of anyone who sleeps near them too long. I suggest resting somewhere darker if a long rest is on the schedule.",
	"The Hollow Crown — the one the Hollow King wears, the one that drops if you beat him — is not the source of the corruption. It is a symptom. The source is older and is not in this zone. I won't say more, partly because I doesn't fully know.",
	"There used to be a road through this forest. The road is still here. Nobody uses the road. I find the road instructive — it goes exactly where it always went, but the going-where part stopped working when the forest stopped behaving like a forest.",
}
