// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_underdark_flavor.go
// Tier 4 zone flavor — The Underdark. Additive only. Pools sampled by
// internal/plugin via deterministic per-run, per-room hashing.
//
// Voice rules (from gogobee_dungeon_zones.md §3.3):
//   • Third person for description; second person for outcomes.
//   • Boss callouts get a beat of cinema. Don't overrun.
//   • TwinBee references the right era — NES, SNES, arcade. Not modern.
//
// The canonical twinbee_gm_flavor.go ships RoomEntryUnderdark and that
// pool is wired in dnd_zone_narration.go. This file adds the boss-entry
// pool, the elite-room intros (Roper), boss ability callouts for Ilvaras
// Xunyl, and zone-specific lore.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ENTRY — Ilvaras Xunyl, Drow High Priestess
// Defined here because no entry exists in twinbee_gm_flavor.go for this boss.
// ─────────────────────────────────────────────────────────────────────────────

var BossEntryIlvaras = []string{
	"The cathedral is round and the round is wrong — drow architecture leans, drow architecture rises, drow architecture does not gather like this. The room was built for one purpose and the purpose is standing at the altar with her back to you. She does not turn when you enter. The trophies on the wall turn first — four sets of weapons, hung at four heights, each set arranged with the care of someone who learned the names of their previous owners and kept the names. 'Five,' Ilvaras Xunyl says, without turning, in a voice that is making itself heard in your head and not in the air. 'I had run out of wall.' She turns. The smile is the smile of someone who has been waiting for the inconvenience of a fifth wall. I say, very low: 'Drow High Priestess. Lolth's favour. Don't accept the offer. There will be an offer.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// ELITE ROOM ENTRY — The Underdark (Roper)
// ─────────────────────────────────────────────────────────────────────────────

var EliteRoomEntryUnderdark = []string{
	"The cavern ahead is a forest of stalagmites — dozens of them, calf-thick to wagon-thick, irregular spacing, the kind of natural architecture that gives an ambush every advantage. I count them, then stop counting, then note that one of them has been counted twice from different angles and that one is not a stalagmite. 'Roper,' I say. 'Six tendrils. Reels you in. Pick the wrong stalagmite and the room becomes one fight at five different ranges.'",
	"You enter a chamber where the ceiling drips minerals that have built columns down to the floor — pillars of pale stone, formed over thousands of years, perfectly natural, all of them. Almost all of them. I identify the off-color column near the back of the room and say: 'False appearance until it isn't. Stay out of grapple range until the tendrils show.'",
	"A grotto. Calm water. A field of stalagmites breaking the surface. Beautiful in the way the deep places are beautiful, which is to say it photographs well and houses something terrible. The stalagmite nearest the path has not been there in any other map I have consulted. I note the inconsistency and recommend ranged options.",
	"The corridor widens into a cavern with twelve pillars holding the ceiling — six of them are dwarven masonry, six of them are natural stone, and one of the natural ones is breathing. The breath is shallow. The patience is the giveaway. I say: 'Roper. Engage the column you can see, not the columns you assume.'",
	"You step into a room where the stalagmites have been arranged — almost arranged — into something like a dining hall. The arrangement is not deliberate. The arrangement is the result of one of the stalagmites occasionally moving in its sleep. I file this under 'apex ambush predator with tenure' and recommend not waking it gently.",
	"A toll-bridge cavern: a narrow path through a field of standing stones, each one twice your height, none of them moving. The toll is paid by whichever party member walks last. I track the sightlines, names the column with the wrong shadow, and say: 'Don't be the last one.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ABILITY CALLOUTS — Ilvaras Xunyl
// Used as a one-line cinematic suffix to BossEntryIlvaras when combat starts.
// ─────────────────────────────────────────────────────────────────────────────

// Spells: Flame Strike / Dispel Magic / Divine Word (CHA DC 18) / Insect Plague.
var IlvarasSpellLines = []string{
	"She has the priest list and she leads with Flame Strike and Insect Plague — both AoE, both back-to-back if the initiative is unkind. I say: 'Spread out. Don't bunch under a column. Insect Plague is a movement tax that compounds.'",
	"Divine Word, CHA DC 18, and the failure tree is brutal — Deafened, Blinded, Stunned, or killed depending on remaining HP. I file this under 'never fight a priest below half HP' and ask who has the best CHA save.",
	"Dispel Magic on your buffs as soon as she rolls initiative. I track which concentration spells went up and recommend shielding the controller until the dispel is spent.",
}

// Lolth's Favour: 1/combat auto-succeed a failed save.
var IlvarasFavourLines = []string{
	"Once per fight she takes a save you thought you'd won and converts the failure into a success. I say: 'Time your hard CCs after the favour is spent — don't waste a Hold Person on the round she's still got it.'",
	"Lolth's Favour. One free re-pass on a save. I note the timing matters more than the damage — the spell you wanted to land is the spell she wanted to skip.",
	"She converts a failed save into a success without rolling. The favour is single-use. I file this under 'plan two openings' and remind the party that the second one is the real one.",
}

// Summon Spiders (1/combat): 2d6 Giant Spiders fill the room.
var IlvarasSpidersLines = []string{
	"She raises both hands and the room fills. Two-d-six Giant Spiders, all at once, all over the floor. I say: 'AoE clears the carpet. Don't get webbed before you cast it.'",
	"Summon Spiders. The carpet is no longer the carpet. I track the count and note that 2d6 in a confined cathedral is a different math than 2d6 in an open hallway.",
	"The spiders come from the cracks the way the rats came in Castlevania, except these are big enough to count individually and small enough to overwhelm. I recommend an early sweep and a clear line of retreat.",
}

// Legendary Actions: Melee (1), Cast Cantrip (1), Drain Life (3 LA, 4d10 necrotic).
var IlvarasLegendaryLines = []string{
	"She takes legendary actions at the end of every other turn. Melee for one, cantrip for one, Drain Life for three — and Drain Life is 4d10 necrotic that heals her. I say: 'Let her spend on the cheap ones. Bait the three-cost.'",
	"Three legendary points per round. The Drain Life is the one that makes the fight last longer than it should. I track her actions and remind the party that her healing comes out of yours.",
	"Legendary Actions: the always-on tax that punishes your turn order. I file this under 'priest economy' and note the cantrip option is what kills your back line if you let her keep three points unspent.",
}

// Phase 2 (<35% HP): Lolth's Avatar overlay — +3 AC; spells cast at +1 slot level.
var IlvarasPhaseTwoLines = []string{
	"Below thirty-five percent the avatar overlay drops on her. AC up three, every spell up one slot. I say: 'Phase shift. The damage breakpoint just moved. Don't ration your hard hits past this point.'",
	"Phase two: Lolth answers the prayer. The room dims. Her shadow is wrong now — too many limbs, too little symmetry. AC bumps, slot levels bump, the whole encounter tier goes up a half-step. I track the new numbers and recommend burning every save-or-suck you've still got.",
	"Avatar overlay. The priest is still there but the priest is also a delivery vehicle now. I file this under 'don't survive into phase three — there isn't one, but it'll feel like one' and ask who has the burst.",
}

// IlvarasSignatureCallouts — combined pool for boss-entry suffix.
// Phase-two lines stay separate (surfaced via dedicated phase-two helper).
var IlvarasSignatureCallouts = func() []string {
	out := make([]string, 0,
		len(IlvarasSpellLines)+
			len(IlvarasFavourLines)+
			len(IlvarasSpidersLines)+
			len(IlvarasLegendaryLines))
	out = append(out, IlvarasSpellLines...)
	out = append(out, IlvarasFavourLines...)
	out = append(out, IlvarasSpidersLines...)
	out = append(out, IlvarasLegendaryLines...)
	return out
}()

// ─────────────────────────────────────────────────────────────────────────────
// LORE — The Underdark
// Sampled by !lore inside this zone (zone-specific pool, generic fallback).
// ─────────────────────────────────────────────────────────────────────────────

var LoreLinesUnderdark = []string{
	"The Underdark is not a place you reach. The Underdark is a depth you continue to. I note the distinction matters: the surface dungeons end. This one keeps going past the part you came to fight.",
	"Drow cities are not built — they are negotiated. Each spire, each platform, each corridor is the result of a House agreeing to share a wall with a House it would prefer to murder. I respect the architecture more than the architects.",
	"Ilvaras Xunyl's House fell two centuries ago. She is not running it. She is running what's left. The four trophies on her wall are not from the surface — three are from rival drow priestesses, in chronological order, and one is a self-portrait. I decline to elaborate on what it means that the self-portrait is hung as a trophy.",
	"The mind flayers are not native here. They came down from somewhere further down. The drow tolerate them in the same way someone tolerates a roof leak they cannot reach — present, problematic, easier to live with than to fix. I file this under 'old grievances' and note the leak is getting worse.",
	"The phosphorescent fungi grow in patterns that match the constellations of the surface sky from sixteen thousand years ago. I have checked the pattern. The drow have not. The drow do not know what surface stars are. The pattern is older than the drow.",
	"Lolth is not the only listener down here. She is the loudest. The other listeners are quieter and more patient and I suggest not naming any of them in the cathedral, not even as a warning, not even at low volume.",
	"The Roper that ate the last expedition did not attack them. They walked into it because they were following a Drow scout who knew exactly where to lead them. The scout is named, in drow records, and I decline to repeat the name out of professional courtesy to the dead.",
}
