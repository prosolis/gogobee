// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_dragons_lair_flavor.go
// Tier 5 zone flavor — Dragon's Lair (Infernus Peak). Additive only. Pools
// sampled by internal/plugin via deterministic per-run, per-room hashing.
//
// Voice rules (from gogobee_dungeon_zones.md §3.3):
//   • Third person for description; second person for outcomes.
//   • Boss callouts get a beat of cinema. Don't overrun.
//   • TwinBee references the right era — NES, SNES, arcade. Not modern.
//
// The canonical twinbee_gm_flavor.go ships RoomEntryDragonsLair and
// BossEntryInfernax, both wired in dnd_zone_narration.go. This file adds
// the elite-room intros (Young Red Dragon), boss ability callouts for
// Infernax the Undying, and zone-specific lore.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// ELITE ROOM ENTRY — Dragon's Lair (Young Red Dragon)
// ─────────────────────────────────────────────────────────────────────────────

var EliteRoomEntryDragonsLair = []string{
	"The chamber ahead has a hoard. Not Infernax's hoard — a smaller hoard, an apprentice's hoard, the kind of starter pile a dragon builds before it has earned a real one. The dragon on top of the pile is the size of a wagon and the temperament of an only child. I say: 'Young Red. Fire breath, sixteen-d-six, DC 21. Don't bunch.'",
	"You enter what used to be a vault. The vault door is on the floor, peeled. The dragon inside is half-grown and entirely awake. It opens one eye, then the other, then makes a sound that is not a roar — it is a sigh, the sigh of a creature being interrupted at home. I note the eye contact and recommend not breaking it first.",
	"A scorched gallery. The walls are blast-blackened in a pattern that suggests practice — the dragon has been working on its breath weapon in here. I identify the burn pattern as 'cone, sixty feet, recently used' and recommend approaching from a flank that hasn't been zeroed in.",
	"The corridor opens into a smaller chamber where a Young Red is curled around a single piece of treasure — not a hoard, just one item, the kind of thing a dragon would only guard if it had been told to. I file this under 'gift from Infernax' and note the gift is being protected with the seriousness of a final-exam project.",
	"You step into a heat-shimmer cavern that the dragon is using as a forge. There are tools — pincers the size of a person, bellows the size of a house — and the dragon is using them. Badly. With enthusiasm. I identify the silhouette and say: 'Young Red. Frightful Presence on the WIS save. Don't let the room intimidate you twice.'",
	"A roost. Stone shelves carved into the cavern wall, each one a perch, all of them empty except the highest. The dragon on the highest perch is watching the door. It has been watching the door for the last six hours, since whatever roused it. I say: 'Multiattack on engage. Hit it before it lifts off — flying Young Red is a different fight than grounded Young Red.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ABILITY CALLOUTS — Infernax the Undying
// Used as a one-line cinematic suffix to BossEntryInfernax when combat starts.
// ─────────────────────────────────────────────────────────────────────────────

// Multiattack: Bite + 2 Claws.
var InfernaxMultiattackLines = []string{
	"Bite plus two claws every turn. Three swings of ancient-dragon math against your front line. I say: 'Don't stand alone in front. Don't stand alone behind, either. There is no alone in this fight.'",
	"Multiattack: bite, claw, claw. The bite is the largest single hit you'll take in this zone. I track the damage tier and recommend max HP buffs before the encounter, not during.",
	"Three attacks a turn, each one calibrated for a tank. I file this under 'arithmetic problem with teeth' and note the math does not get better at higher levels — the tier scales with you.",
}

// Fire Breath (recharge 5–6): 90-ft cone, 26d6 fire, DEX DC 24 half.
var InfernaxBreathLines = []string{
	"Fire Breath, ninety-foot cone, twenty-six-d-six fire, DEX DC 24 for half. Recharge five-or-six. I say: 'Pre-position. Don't share an angle. Half of twenty-six-d-six is still a TPK.'",
	"The cone covers most of the chamber. The save is high. The damage is the kind that ends fights. I track the recharge die and shout the spread pattern when it shows.",
	"Like the Bowser jump-on-the-platform pattern from World 8, except the platform is a cone of fire and the platform is most of the room. I note there are no axes in this fight. There is only DEX and distance.",
}

// Frightful Presence: WIS DC 21 or Frightened 1 min.
var InfernaxPresenceLines = []string{
	"Frightful Presence on entry. WIS DC 21. Frightened means disadvantage on attacks and you can't move closer. I say: 'Eat the save the first round. Hold initiative for the unfrightened ones.'",
	"WIS save at 21. Frightened for a minute. I file this under 'why we bring high-WIS classes' and ask who has the Wisdom-save proficiency to lead.",
	"He looks at you and the save is rolled before the breath is. Half the party loses a minute. I track the duration and recommend not opening with your highest-investment ability if your character failed.",
}

// Legendary Resistance (3/combat) + Legendary Actions (3): Detect, Tail, Wing.
var InfernaxLegendaryLines = []string{
	"Three Legendary Resistances. Three Legendary Action points per round. He spends them on Tail Attack and Wing Attack — the wing is AoE knockback, DEX DC 22. I say: 'Three saves you'll wish back. Don't waste your dispel-tier spells in the first round.'",
	"Legendary Actions on every other turn. Wing Attack costs two and knocks the room around. I track the spend and remind the party that the action economy is the actual fight — Infernax is the venue.",
	"Three free passes on his saves. Plan two openings. I file this under 'priest economy applied to dragons' and note the third opening is the one that lands.",
}

// Lair Actions (init 20): Magma eruption, Volcanic gases (CON DC 13 Poisoned), Tremor (DEX DC 15 Prone).
var InfernaxLairLines = []string{
	"Lair Actions on initiative count twenty: magma erupts, gases force CON saves, tremors knock you prone. I say: 'The room is a third combatant. Track the initiative. Don't stand on cracks.'",
	"The mountain helps him. Initiative twenty triggers a lair effect every round. I track the rotation and warn the party that the prone effect comes during caster turns, on purpose.",
	"Magma, gas, tremor — the three-card lair rotation. I file this under 'environment as DPS' and recommend fighting from the cleared rim of the room, not the gold-flooded center.",
}

// Phase 2 (<50% HP): Fire Breath recharge 4–6; fire damage ignores resistance.
var InfernaxPhaseTwoLines = []string{
	"Below half HP the breath recharges on a four. Fire resistance no longer applies — your tank's fire-resist gear is now decorative. I say: 'Phase shift. Burn him before the second cone.'",
	"Phase two: the cone comes more often, the damage cuts through every fire-resist resource you brought. I track the breath count and ask who still has movement-class abilities — the second cone wants spread.",
	"Half-HP. The mountain wakes the rest of the way up. Recharge four-six on the cone. Fire damage goes raw. I file this under 'no plan survives second contact with Infernax' and recommend every nova you've still got.",
}

// InfernaxSignatureCallouts — combined pool for boss-entry suffix.
// Phase-two lines stay separate (surfaced via dedicated phase-two helper).
var InfernaxSignatureCallouts = func() []string {
	out := make([]string, 0,
		len(InfernaxMultiattackLines)+
			len(InfernaxBreathLines)+
			len(InfernaxPresenceLines)+
			len(InfernaxLegendaryLines)+
			len(InfernaxLairLines))
	out = append(out, InfernaxMultiattackLines...)
	out = append(out, InfernaxBreathLines...)
	out = append(out, InfernaxPresenceLines...)
	out = append(out, InfernaxLegendaryLines...)
	out = append(out, InfernaxLairLines...)
	return out
}()

// ─────────────────────────────────────────────────────────────────────────────
// LORE — Dragon's Lair (Infernus Peak)
// Sampled by !lore inside this zone (zone-specific pool, generic fallback).
// ─────────────────────────────────────────────────────────────────────────────

var LoreLinesDragonsLair = []string{
	"Infernus Peak has not erupted in forty years and the locals call this dormant. The peak has not erupted in forty years because Infernax has been asleep, and his presence stabilizes the magma chamber. The locals are wrong about which one is causing which. I note the irony and file it under 'killing the dragon may have geological consequences.'",
	"Infernax remembers when the surface civilizations were just fires. The fires he is referring to are the ones the first humans set, which is how I date him — give or take six thousand years on either side of the precise number, which I decline to commit to.",
	"The kobolds are not slaves. The kobolds are clergy. They serve voluntarily, in shifts, with rotation, and the rotation is run by elders who have written sermons. I respect the organization and note the kobold scale-sorcerers are graduates, not recruits.",
	"The hoard is not random. Each piece is catalogued in Infernax's memory, by location and by year of acquisition. He will know if you take a single coin. I suggest not taking a single coin and instead taking the items the design doc expects you to take — those have been pre-cleared.",
	"The Dragon Hoard mechanic exists because Infernax does not lose track of his coins. Killing him releases his hold on the catalogue. The 50d10 × 100 coin drop is the entire pile relaxing for the first time in eight centuries. I respect the math and note the rest of the surface economy will too.",
	"Infernax has had three challengers in the last eight hundred years. Two were heroes. One was a younger dragon. He kept the younger dragon's skull as a paperweight on a treaty desk that has not been used since. I note the paperweight is in the treasury, on the third shelf, and am not worth picking up — picking it up triggers his attention from anywhere on the mountain.",
	"The Young Red dragons in the outer chambers are his children. Or his grandchildren. Or unrelated and tolerated. Infernax does not clarify and I have not asked. They are loyal in the way that loyal works for dragons, which is to say: they will fight you, but they will not die for him, and the distinction matters more than it should.",
	"The kobold scale-sorcerers cast through bloodline. The bloodline traces back to a single clutch laid in the magma chamber three centuries ago. I note the entire sorcerous gene pool of this zone is one extended family and that they all know each other's names.",
}
