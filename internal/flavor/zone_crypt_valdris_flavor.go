// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_crypt_valdris_flavor.go
// Tier 1 zone flavor — The Crypt of Valdris. Additive only. Pools sampled
// by internal/plugin via deterministic per-run, per-room hashing.
//
// Voice rules (from gogobee_dungeon_zones.md §3.3):
//   • Third person for description; second person for outcomes.
//   • Boss callouts get a beat of cinema. Don't overrun.
//   • TwinBee references the right era — NES, SNES, arcade. Not modern.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// ELITE ROOM ENTRY — Crypt of Valdris (Wight / Flameskull)
// ─────────────────────────────────────────────────────────────────────────────

var EliteRoomEntryCrypt = []string{
	"The room is colder than the last one. Not by feeling — by measurement. TwinBee can tell. There is a presence here that does not believe in the ambient temperature of stone.",
	"A figure sits in the cathedra at the chamber's far end. Robes intact. Skin not. The eyes open as you enter and they were already looking at the door. TwinBee draws breath despite not strictly needing to.",
	"Floating above the altar: a skull, lit from within in a color that pretends to be fire. TwinBee recognizes the pretense. The skull is doing math. The math is about where you are.",
	"The torches in this room are out, but the room is lit anyway. TwinBee identifies this as a problem worth naming aloud: 'That light has a source. The source is the encounter.'",
	"You step into a chapel. Pews. Altar. A figure rising from the front pew with the unhurried grace of something that has been waiting in this exact pew for a very long time. TwinBee respects the patience and refuses to be impressed by it.",
	"The mosaics on the walls show a procession. The procession leads to a king. The king is here. TwinBee notes that the mosaics are still being added to in real time — one tile per visitor — and you have not been added yet.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ABILITY CALLOUTS — Valdris the Unburied
// Used as a one-line cinematic suffix to BossEntryValdris when combat starts.
// ─────────────────────────────────────────────────────────────────────────────

// Corrupting Touch: +4d6 necrotic; target max HP reduced by damage dealt.
var ValdrisCorruptingTouchLines = []string{
	"His hand passes through your chestplate without touching it. The damage isn't to the plate. TwinBee watches your max HP tick down and quietly stops watching.",
	"Necrotic damage lands and lingers. The wound doesn't bleed — it forgets how. Your HP ceiling drops with it. TwinBee notes: long rest restores. Survive to long rest.",
	"Where Valdris touches, the body takes a piece of permanent. Not permanent-permanent. Long-rest-permanent. TwinBee considers this distinction the only good news in the sentence.",
}

// Legendary Resistance: 2/combat; auto-succeed one failed save.
var ValdrisLegendaryResistanceLines = []string{
	"Your save would have landed. Valdris decides otherwise. The roll resets to a success on his side of the table. TwinBee notes one charge spent, one remaining.",
	"The spell hits cleanly and Valdris ignores it on principle. TwinBee marks the Legendary Resistance and reminds you: he has two of these. After two, the math changes.",
	"You rolled the number. The number was correct. Valdris waves it away with a gesture that costs him something but not enough. TwinBee keeps count. Keep rolling.",
}

// Call of the Grave: recharge 5–6; summons 1d4 skeletons.
var ValdrisCallOfTheGraveLines = []string{
	"Valdris speaks a word that is not for the living. Bones in the chamber walls remember they were once attached to people. TwinBee counts the new combatants and recommends crowd control.",
	"From the alcoves: the dry rattle of skeletons reassembling on demand. TwinBee notes 1d4 new bodies on the field and quietly hopes for the lower end of that range.",
	"The skeletons stand up like a Castlevania stage hazard — same animation, same timing, same exact bad news. TwinBee has memories of this and they are not warm ones.",
}

// Phase 2 (<50% HP): gains Fly speed; spells deal +1d6 necrotic.
var ValdrisPhaseTwoLines = []string{
	"Valdris drops to half HP and stops walking. Not because he's tired — because he doesn't need to anymore. He rises a foot off the floor and TwinBee files this under 'phase shift.'",
	"The fight changes shape. Valdris hangs in the air now, robes still, very calm, very lethal. His spells gain a necrotic edge. TwinBee says, simply: 'Phase two. Adjust.'",
	"Half-HP is the line. You crossed it and Valdris crossed something else — gravity, mostly. The boss music gets a third instrument. TwinBee braces.",
	"Like Castlevania's Death revealing his real attack pattern at half HP — the fight you started is not the fight you're finishing. TwinBee respects the pivot and dislikes it intensely.",
}

// ValdrisSignatureCallouts — combined pool the boss-entry composer samples
// from at the start of the boss fight. Phase-two lines are surfaced via
// the dedicated phase-two helper rather than the entry suffix.
var ValdrisSignatureCallouts = func() []string {
	out := make([]string, 0,
		len(ValdrisCorruptingTouchLines)+
			len(ValdrisLegendaryResistanceLines)+
			len(ValdrisCallOfTheGraveLines))
	out = append(out, ValdrisCorruptingTouchLines...)
	out = append(out, ValdrisLegendaryResistanceLines...)
	out = append(out, ValdrisCallOfTheGraveLines...)
	return out
}()

// ─────────────────────────────────────────────────────────────────────────────
// LORE — Crypt of Valdris
// Sampled by !lore inside this zone (zone-specific pool, generic fallback).
// ─────────────────────────────────────────────────────────────────────────────

var LoreLinesCrypt = []string{
	"Valdris was a scholar. Then he was an aspirant. Then he was a failed aspirant. The failure is the part that mattered — the lich ritual completed but completed wrong, and the wrongness has had three centuries to compound. TwinBee has read the contemporaneous accounts and finds the original scholar likable, which makes the rest harder.",
	"The phylactery shard you'll find here is one of seven. The other six are not in this crypt. TwinBee will not say where they are, because it doesn't fully know, and the partial knowledge it has is the kind that gets people followed by things that prefer not to be looked for.",
	"The candles in the crypt do not consume wax. They do not consume time, either, in the strict sense. They were lit on the day Valdris was interred and they have been lit ever since. TwinBee respects this kind of consistency in a way that does not extend to approval.",
	"The skeletons here are not all enemies. Some of them are former students, posed in the alcoves where they died, marked with the texts they were translating. TwinBee suggests you don't disturb the marked ones unless you mean to. Some lessons end and some lessons keep going.",
	"The iron gate at the entrance has been opened from the inside three times in the last decade, and from the outside zero times. TwinBee finds the asymmetry instructive. It also finds the pattern instructive: you are walking into the open door, not opening it.",
	"In life, Valdris was patient. In undeath, Valdris is also patient — which is unfair, mathematically, because the same trait scales differently across mortality conditions. TwinBee dislikes the math but respects the consistency.",
}
