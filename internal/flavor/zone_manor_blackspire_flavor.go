// DO NOT REWRITE, SUMMARIZE, OR SHORTEN ANY ENTRIES IN THIS FILE
// zone_manor_blackspire_flavor.go
// Tier 3 zone flavor — Haunted Manor of Blackspire. Additive only. Pools
// sampled by internal/plugin via deterministic per-run, per-room hashing.
//
// Voice rules (from gogobee_dungeon_zones.md §3.3):
//   • Third person for description; second person for outcomes.
//   • Boss callouts get a beat of cinema. Don't overrun.
//   • TwinBee references the right era — NES, SNES, arcade. Not modern.
//
// The canonical twinbee_gm_flavor.go ships RoomEntryHauntedManor and that
// pool is wired in dnd_zone_narration.go. This file adds the boss-entry
// pool, the elite-room intros (Revenant), boss ability callouts for Lord
// Aldric Blackspire, and zone-specific lore.

package flavor

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ENTRY — Lord Aldric Blackspire
// Defined here because no entry exists in twinbee_gm_flavor.go for this boss.
// ─────────────────────────────────────────────────────────────────────────────

var BossEntryAldricBlackspire = []string{
	"The portrait above the mantel was not finished — the painter clearly stopped partway through the eyes. The eyes in the portrait are now finished. They were finished by being looked through. Lord Aldric Blackspire steps out of the frame the way a man steps out of a coat: practiced, unhurried, the gesture of someone who has done this many times. His clothes are seventy years out of fashion and immaculate. 'You're early,' he says, and the word is polite the way a knife is polite — a tool that has not yet committed to its purpose. TwinBee says, very quietly, 'That's a vampire. The signet on his hand is not jewelry. Don't take what he offers.'",
}

// ─────────────────────────────────────────────────────────────────────────────
// ELITE ROOM ENTRY — Haunted Manor (Revenant)
// ─────────────────────────────────────────────────────────────────────────────

var EliteRoomEntryManorBlackspire = []string{
	"The chamber ahead was a study, once. The desk is still set for work — inkwell uncapped, ledger open, a pen laid down mid-sentence. The man at the desk turns his head with the slow patience of someone who has been waiting at this desk for forty years for one specific person to walk in. He is not looking at you. He is looking past you, at someone who isn't there. Yet. TwinBee identifies the silhouette and says: 'Revenant. He's not here for us. He's here for whoever wronged him. Try not to look like them.'",
	"You enter what appears to be a guest bedroom. The bed is made. The candles are lit. A man sits on the edge of the bed with his back to you, perfectly still. He has been perfectly still for a long time. When he turns, only his head turns, and TwinBee notes the impossible angle and files this under 'revenant — directed undead, single purpose, finite lifespan, very patient.' The patience is the part that worries TwinBee.",
	"The hallway dead-ends in a chapel. There is a kneeling figure at the altar. The candles around him are unlit but somehow casting shadows. He has been praying since before the manor was haunted, and the prayer was not a request — it was a contract. TwinBee says: 'Revenant. The contract is the encounter. Break the contract, end the fight.'",
	"A library. The man at the table is reading a book that has no pages. He is reading it carefully. He looks up when you enter and his expression does something that suggests he was expecting a different visitor and is willing to make do. TwinBee notes the revenant's hands — the bones in the wrong places, the joints set with intent — and recommends ranged engagement until the geometry of the threat is clear.",
	"The ballroom is empty except for one figure standing in the exact center, not dancing, not waiting, simply standing. The chandeliers above him are not moving. They should be moving — they were moving in the last room — and the fact that they have stopped where this figure is standing tells TwinBee everything TwinBee needed to know. 'Revenant in the middle of a stilled room,' TwinBee says. 'He brought the silence with him.'",
	"A nursery. Untouched. The cradle still rocks. The figure beside the cradle was, in life, the sort of man people did not survive being wronged by. In death he has been more thorough. TwinBee notes the revenant's grip on the cradle's edge and suggests, quietly, that whatever happened in this room is not the encounter — the encounter is what walks out of it.",
}

// ─────────────────────────────────────────────────────────────────────────────
// BOSS ABILITY CALLOUTS — Lord Aldric Blackspire
// Used as a one-line cinematic suffix to BossEntryAldricBlackspire when combat starts.
// ─────────────────────────────────────────────────────────────────────────────

// Multiattack: Unarmed Strike + Bite (life drain) every turn.
var AldricMultiattackLines = []string{
	"Aldric multiattacks every turn — Unarmed Strike, then Bite. The bite drains. Whatever HP he takes from you, he keeps. TwinBee says: 'He's a battery with manners. Don't let him charge.'",
	"Two hits a turn, and the second one is the one that funds the rest of the fight. The bite returns HP to him equal to the damage dealt. TwinBee notes that this changes the math on burst-vs-sustain considerably.",
	"Strike, then bite. The bite is the resource transfer. TwinBee files this under 'fights where defense is offense' — every point of damage you avoid is a point he doesn't get to take.",
}

// Charm: WIS DC 17 or Charmed for 24h (broken by damage).
var AldricCharmLines = []string{
	"He looks at one of you and the look is a question and the question is being answered without your permission. WIS DC 17. On a fail: charmed for twenty-four hours, broken by damage. TwinBee says: 'High DC. Have someone designated to slap the charmed party once.'",
	"Charm. WIS DC 17. The duration is twenty-four hours and the only out is taking damage. TwinBee notes the irony — the cure is in the room, applied by your own party, in the hardest possible way.",
	"Aldric's gaze settles. WIS save now. Failure is a long-term problem — a full day of charm — and the only solvent is a friendly hit. TwinBee files this under 'social-coded crowd control with a violent escape clause.'",
}

// Children of the Night (1/combat): summons 2d6 Bat Swarms or 3d6 Rats.
var AldricChildrenLines = []string{
	"Once per fight he calls. Bats from the chimney, rats from the walls — 2d6 swarms or 3d6 rats, GM's choice, both bad. TwinBee says: 'AoE clears the adds. Don't let them stack tokens on whoever's charmed.'",
	"Children of the Night. The walls produce. The ceiling produces. TwinBee tracks the adds and recommends an AoE before the swarms occupy enough squares to be a separate problem.",
	"He raises a hand and the manor answers. The room fills. Bats or rats or both. TwinBee notes this is a one-time effect per combat — survive the wave and the room belongs to you again.",
}

// Mist Form: at 0 HP retreats to coffin; must destroy coffin in 30 turns or fully regenerates.
var AldricMistFormLines = []string{
	"Drop him to zero and he doesn't drop — he goes to mist, retreats to a coffin somewhere in the manor. Thirty turns to find and destroy it. TwinBee says: 'Find the coffin first, fight the count second. The order matters.'",
	"Mist Form. At 0 HP he becomes mist and goes home. Home is a coffin. The coffin is somewhere in the manor and you have thirty turns to break it. TwinBee files this under 'zero is not death, zero is a timer.'",
	"Like the Castlevania bosses that didn't actually die — except those games gave you the next room. This one gives you a thirty-turn coffin hunt. TwinBee respects the design and recommends scouting on the way in.",
}

// Phase 2 (Mist destroyed): all attacks have advantage; AoE Charm.
var AldricPhaseTwoLines = []string{
	"Coffin destroyed. He returns furious. All attacks have advantage now and his Charm goes AoE — every party member, WIS DC 17 each turn. TwinBee says: 'You won the long game. The short game just got shorter.'",
	"Phase two: no more retreat, no more singles. AoE charm, advantage on every swing. TwinBee tracks the saves and reminds you that advantage on the boss means disadvantage on your continuing-to-stand.",
	"Without the coffin, Aldric's restraint is gone. The fight goes loud. Charm sweeps the room each turn. TwinBee stops narrating long enough to breathe and says: 'Burn him.'",
}

// AldricSignatureCallouts — combined pool for boss-entry suffix.
// Phase-two lines stay separate (surfaced via dedicated phase-two helper).
var AldricSignatureCallouts = func() []string {
	out := make([]string, 0,
		len(AldricMultiattackLines)+
			len(AldricCharmLines)+
			len(AldricChildrenLines)+
			len(AldricMistFormLines))
	out = append(out, AldricMultiattackLines...)
	out = append(out, AldricCharmLines...)
	out = append(out, AldricChildrenLines...)
	out = append(out, AldricMistFormLines...)
	return out
}()

// ─────────────────────────────────────────────────────────────────────────────
// LORE — Haunted Manor of Blackspire
// Sampled by !lore inside this zone (zone-specific pool, generic fallback).
// ─────────────────────────────────────────────────────────────────────────────

var LoreLinesManorBlackspire = []string{
	"The Blackspire line was old before it was wealthy and wealthy before it was cursed. The portraits are in chronological order — TwinBee has counted them. The expressions get worse as the years go on. By the seventh portrait the painter has stopped pretending the family is well.",
	"Aldric was the seventh and the last. He was not the first to make the deal — the deal had been made twice already, by his grandmother and by his uncle — but he was the first to honor it past the term of his own life. TwinBee considers this the kind of stubbornness that becomes a curse on its own, before any vampires are involved.",
	"The manor wasn't always haunted. It became haunted on a specific night, in a specific hour, and the clock on the mantel still shows that hour. TwinBee notes the clock isn't broken. The clock is keeping correct time for that one night and refusing to acknowledge any other.",
	"The portraits are not paintings. The pigment is correct, the technique is correct, the canvases are correct — but the things behind the eyes are not. TwinBee has watched a portrait blink. TwinBee declines to elaborate on which one.",
	"The Blackspire signet ring — the one that drops if you beat Aldric — is not enchanted in any conventional sense. It is recognized. The undead in this region have been raised under it, contracted under it, organized under it. They will not attack the wearer unless attacked first. TwinBee finds this both useful and uncomfortable.",
	"The deed to the manor changes hands and the manor refuses each transfer. Eleven owners on paper since the curse settled. Eleven sets of estate records. TwinBee has seen the lawyers' files. The lawyers are no longer in the profession.",
	"The vampire spawn are not converts in the romantic sense — they are the previous owners. Each buyer who didn't leave became staff. TwinBee notes which uniforms match which historical fashions and stops counting at eleven, which is the correct number.",
}
