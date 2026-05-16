// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_underforge_flavor.go
// Tier 3 zone flavor — The Underforge. Additive only. Pools sampled by
// internal/plugin via deterministic per-run, per-room hashing.
//
// Voice rules (from gogobee_dungeon_zones.md §3.3):
//   • Third person for description; second person for outcomes.
//   • Boss callouts get a beat of cinema. Don't overrun.
//   • TwinBee references the right era — NES, SNES, arcade. Not modern.
//
// The canonical twinbee_gm_flavor.go does not (yet) ship a RoomEntry pool
// or a BossEntry pool for this zone, so both are defined here. This file
// also adds elite-room intros (Helmed Horror), boss ability callouts for
// Emberlord Thyrak, and zone-specific lore.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// ROOM ENTRY — The Underforge
// Defined here because no entry exists in twinbee_gm_flavor.go for this zone.
// ─────────────────────────────────────────────────────────────────────────────

var RoomEntryUnderforge = []string{
	"The chamber opens onto a foundry the size of a city block. Conveyor channels of slow-cooling iron. Bellows the height of houses. Anvils arranged for workers who haven't been here in three centuries. I note the bellows are still moving. The bellows are moving on their own.",
	"You step out onto a basalt walkway over a river of lava. The heat is impossible. The walkway is engineered to hold. I have respect for dwarven engineering and a healthy suspicion of any walkway suspended over molten rock. The two coexist comfortably.",
	"A casting hall. The molds are still in place — armor pieces, weapon halves, parts of things I cannot identify by silhouette. One mold contains a shape that's still glowing red. Something was cast recently. I file this under 'occupied' and keep moving.",
	"The corridor smells of hot iron and an absence — the absence of beards, of song, of the specific noise dwarven forges make when they're being tended properly. I note the silence has a shape. The shape is not reassuring.",
	"You enter a chamber where the walls are arranged in tiers and each tier is a row of forges and each forge is lit. Nobody is at the forges. The hammers are striking on their own, in tempo, with the regularity of something that has not stopped striking in a very long time.",
	"A vault door, dwarf-made, three feet thick, ajar. Not blown — ajar. Someone opened it from the inside. I note which side the hinges are on and consider which direction the something that opened it was facing. I do not like the answer.",
	"The walkway crosses a chasm of pure heat-shimmer. You can see the far side, more or less. You cannot see what's between, except by what it does to the air. I suggest not lingering on the bridge — lingering bridges are how lava bosses get introduced.",
	"A barracks. Bunks made. Boots paired and lined up. Beards on the floor where their owners stopped being. I note the beards have not been disturbed. Whatever happened, happened cleanly and at speed and I do not want to find out what was in a hurry.",
	"The chamber ahead is a forge gallery — nine forges in a half-circle, all of them lit, all of them tended by armored figures that don't move when you enter. The figures are not waiting to fight. The figures are working. I note which mode you don't want to interrupt and which mode you absolutely cannot afford to interrupt.",
	"Heat radiates from the floor. Not from anything on the floor — from the floor itself. I diagnose the cause as 'a forge directly underneath us, fully active, somehow still operating without dwarves' and find none of those words individually reassuring.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ENTRY — Emberlord Thyrak
// Defined here because no entry exists in twinbee_gm_flavor.go for this boss.
// ─────────────────────────────────────────────────────────────────────────────

var BossEntryEmberlordThyrak = []string{
	"The chamber is the largest forge I have ever narrated, and it has narrated several. Nine furnaces in a ring. A central anvil the size of a wagon. The figure at the anvil is twelve feet of articulated iron and inset rune-stones that pulse to a tempo I can feel in its plating. The hammer comes down. The strike rings the room. The figure does not turn — finishes the strike, then turns. The eyes are not eyes. They are vents. 'Visitor,' Thyrak says, and the word is a furnace door opening. I take a half-step back and say, very evenly, 'Forge-golem. Three centuries of self-improvement. Don't fight him on the lava.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// ELITE ROOM ENTRY — The Underforge (Helmed Horror)
// ─────────────────────────────────────────────────────────────────────────────

var EliteRoomEntryUnderforge = []string{
	"The armory ahead is full. Stands and stands of polished dwarven plate, weapons racked in order, the kind of inventory that takes a lifetime to build. One suit at the far end is wearing itself. It is also holding its weapon at guard. I identify the silhouette and say: 'Helmed Horror. Spell-immune to a list. Hit it with metal, not magic.'",
	"A hall of dwarven banners, each lit by its own brazier. At the far end, a suit of armor stands in a place of honor. The suit pivots when you enter — the helmet first, then the torso, the way an aware thing acquires a target. I note the lack of a body inside and recommend physical damage above all else.",
	"You enter a workshop that is still in service. Hammers strike. Bellows pump. None of the workers are alive and none of them notice you, except the one nearest the door, which sets down its hammer and picks up a sword. I say: 'Helmed Horror. The room is its job. We are its interruption.'",
	"A small chapel, dwarf-made, dedicated to a god of the forge whose statue stands in armor in the alcove. The statue is not a statue. The statue moves first. I file this under 'ambush by liturgy' and note the immunity list does not include warhammers.",
	"The corridor narrows into a guardpost. There is a guard. The guard is articulated steel from a fashion three centuries old, and it has been at this post since the day the dwarves left. It greets you in a voice that is not coming from the mouth-slit — the mouth-slit is for venting heat. I straighten up. 'Sentry duty,' I say, 'taken to its conclusion.'",
	"A vault antechamber. Two suits of armor flank the inner door. They are alike in pattern, alike in posture, alike in the heat coming off them. The left one moves first by half a heartbeat. I track the asymmetry and say: 'One Helmed Horror is a fight. Two is a logistics problem. Pick a target.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ABILITY CALLOUTS — Emberlord Thyrak
// Used as a one-line cinematic suffix to BossEntryEmberlordThyrak when combat starts.
// ─────────────────────────────────────────────────────────────────────────────

// Molten Strike: +4d6 fire on hit; target armor -1 AC until repaired (long rest).
var ThyrakMoltenStrikeLines = []string{
	"Every hit lands +4d6 fire and pits your armor — AC -1 until you can repair, which requires a long rest. I say: 'The damage hurts. The AC penalty compounds. Don't take more than two if you can help it.'",
	"Molten Strike. The hammer doesn't just hit, it brands. Fire damage on top of physical, and your armor gets worse permanently for the encounter. I file this under 'attrition with paperwork.'",
	"Each connecting blow makes the next one easier for him. Your AC goes down. The fire damage goes through. I track the count and recommend taking the hit on something you weren't going to keep.",
}

// Forge Breath: recharge 5–6; 30-ft cone, 10d6 fire, DEX DC 16 half.
var ThyrakForgeBreathLines = []string{
	"The vents on his chest open. Thirty-foot cone, 10d6 fire, DEX DC 16 for half. I say: 'Don't bunch up. He'll get this back on a 5–6.'",
	"Forge Breath. He inhales — visibly — and the next exhale is a furnace. Cone, big DC, big damage. I mark the recharge and shout the spread order.",
	"Like the Bowser fire breath in 8-3 except wider and with a saving throw. Thirty-foot cone, half-damage on save. I note that 'half' of 10d6 is still meaningful and recommend not getting hit twice in a row.",
}

// Living Forge: heals 15 HP/turn while standing on lava/forge tiles.
var ThyrakLivingForgeLines = []string{
	"He heals fifteen a turn while standing on lava or forge tiles. I say: 'Move him off, or fight a boss who repairs faster than you can dent.'",
	"Living Forge. The lava floor is part of his kit. Pull him off it — shove, push, anything — and the regen stops. I file this under 'positioning is damage.'",
	"He's standing on a heat source and the heat source is fueling him. Fifteen HP a round, indefinitely. I track the regen and remind the party that knocking him into the open hallway is worth more than a good attack roll.",
}

// Construct Resilience: immune poison/psychic/charm/exhaustion; resist non-magical physical.
var ThyrakConstructLines = []string{
	"Immune to poison, psychic, charm, exhaustion. Resists non-magical physical. I say: 'Magic weapons earn their keep here. Mundane steel does half.'",
	"He's a construct first and a boss second. The whole control-effect column on your sheet is greyed out. I file this under 'fights you win by hitting harder, not smarter — the smart options have been removed.'",
	"Construct Resilience: the standard package. Magic damage gets through clean. Mundane physical gets cut in half. I suggest checking which weapons are actually magical before assuming.",
}

// Phase 2 (<50% HP): Forge Breath recharge 4–6; spawns 2 Fire Elementals.
var ThyrakPhaseTwoLines = []string{
	"At half HP the vents widen. Forge Breath recharges on 4–6 now and two Fire Elementals come up out of the floor. I say: 'Phase shift. The cone is twice as common and you have new targets. Pick a priority.'",
	"Half-health. The forge wakes the rest of the way up. Two Fire Elementals burst from the lava channels. Breath weapon is half a turn away on every roll now. I track the new add count and remind you that Thyrak is still the win condition.",
	"Phase two: the elementals come up the way bosses summon adds in every arcade beat-em-up — predictable, relentless, the kind of design that turns a fight into a war of attrition. I file the timing and ask who has fire resistance.",
}

// ThyrakSignatureCallouts — combined pool for boss-entry suffix.
// Phase-two lines stay separate (surfaced via dedicated phase-two helper).
var ThyrakSignatureCallouts = func() []string {
	out := make([]string, 0,
		len(ThyrakMoltenStrikeLines)+
			len(ThyrakForgeBreathLines)+
			len(ThyrakLivingForgeLines)+
			len(ThyrakConstructLines))
	out = append(out, ThyrakMoltenStrikeLines...)
	out = append(out, ThyrakForgeBreathLines...)
	out = append(out, ThyrakLivingForgeLines...)
	out = append(out, ThyrakConstructLines...)
	return out
}()

// ─────────────────────────────────────────────────────────────────────────────
// LORE — The Underforge
// Sampled by !lore inside this zone (zone-specific pool, generic fallback).
// ─────────────────────────────────────────────────────────────────────────────

var LoreLinesUnderforge = []string{
	"Kharak Dûn was a forge-city, not a mine. The dwarves here didn't extract — they finished. Raw stock came in from a hundred lesser holds and left as the kind of work the rest of the world named after the holds. I note the distinction matters: this place was a destination, not a quarry.",
	"The dwarves left. They didn't die in place — they left. The boots in the barracks are paired because they were left paired, intentionally, on the last day. Somewhere, a clan-elder gave the order to walk out. I respect the discipline and am curious about the reason. The reason is not in any record I have read.",
	"Thyrak was not built to run the city. Thyrak was built to maintain the furnaces while the city ran itself. Three centuries of self-maintenance turned out to include self-improvement, and Thyrak has been improving the designs. I find this less terrifying than it sounds and more terrifying than it looks.",
	"The seal on the outer doors was set from the outside. The dwarves walked out, then closed the door behind them, then sealed it with the kind of binding that takes a clan to set. I note the bindings were aimed inward. They were sealing something in. The thing inside is the forge or the forge-golem or both — I decline to commit.",
	"The azers in the deeper chambers are not hostile by default. They are loyal to the forge. They will work with anyone the forge accepts, and they will burn alive anyone the forge rejects. I suggest not picking up the hammer at the central anvil unless you mean it.",
	"The flameskulls drifting in the upper galleries were dwarven mages, once. They volunteered. Their final task was to maintain the wards around Thyrak's casting chamber, and they have been at it ever since. I note the wards are still active and that this is, on balance, probably good news.",
	"Thyrak's core — the crystalline heart that drops on his death — is what made the self-improvement possible. The dwarves didn't install it. It was already there when they built the chassis. I find this implication concerning and suggest the core is not, technically, dwarven work.",
	"There is a second forge below this one. The 'bass note of something very large moving below' is not metaphor. The Underforge is the upper level. I will not speculate about the lower level except to note that it has not been opened in this run and would not be a small undertaking to open.",
}
