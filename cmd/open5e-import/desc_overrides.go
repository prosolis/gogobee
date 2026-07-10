package main

import (
	"regexp"
	"strings"
)

// Description-cleaning hooks shared by the spell and magic-item generators.
//
// The Open5e dump descriptions are SRD prose pulled verbatim — they leak
// "within range," "WIS saving throw," "your Constitution score is 19," and
// other rules-text the game now hides. This file handles two layers:
//
//   - A sanitizer (cleanDesc) strips the worst jargon clauses from the auto
//     first-sentence and tidies the result. Used as the default path.
//   - Per-ID override maps win outright when a description needs more than
//     mechanical scrubbing (e.g. Amulet of Health's "score is 19," all of
//     the Mage default-list cantrips). Hand-author here, not in the
//     generated file — these survive a regen.

// spellDescOverride wins over the auto-derived first-sentence Description
// when the slug matches. Cover anything that surfaces in the player's
// spellbook listing (defaultKnownSpells) or the !spells available command.
//
// Tone target: outcome-first, second person, plain English. No DC, no slot
// jargon, no "within range." Match the voice already in dnd_spells_data.go.
var spellDescOverride = map[string]string{
	// SRD-only spells that appear in defaultKnownSpells (no hand-authored
	// overlay in dnd_spells_data.go). Sourced from the S3 surfaced-spells
	// audit; each gives the player a one-line read of what it does, with
	// just enough bite that the spellbook isn't a phone book.
	"call_lightning":   "Summon a sulking thundercloud, then ask it — politely — to drop a bolt on someone you dislike.",
	"charm_person":     "Sweet-talk a humanoid into treating you as their new favourite acquaintance. Wears off; they may notice.",
	"conjure_animals":  "Call up a posse of summoned beasts. They are very enthusiastic and only mostly trained.",
	"eldritch_blast":   "A beam of crackling otherworldly Whatever streaks at a target. Your patron's preferred greeting.",
	"entangle":         "The ground sprouts rude, grabby roots in a patch. Anyone caught looks undignified at best.",
	"faerie_fire":      "Outline targets in glittery light. They can't hide; you hit them more easily; they hate it.",
	"fear":             "Project a cone of pure dread. Brave folk turn and run; less brave folk were already running.",
	"flaming_sphere":   "A rolling ball of fire shows up and goes wherever you point. Carpet not included.",
	"haste":            "An ally surges with speed — extra ground, sharper defences, an extra swing. Brief, glorious, exhausting.",
	"healing_word":     "Bark a healing syllable across the battlefield. An ally finds themselves slightly less dead.",
	"heat_metal":       "Their weapon, their armour, their belt buckle — glowing red-hot. Hard to grip. Worse to wear.",
	"heroism":          "An ally takes courage. No fear, more grit, slightly insufferable battle cries.",
	"hideous_laughter": "Target collapses in a fit of unstoppable giggling. Awkward for them, very funny for you.",
	"insect_plague":    "A swarm of biting locusts boils up in an area. Everyone inside has Opinions about it.",
	"invisibility":     "An ally fades from sight. Lasts until they attack, cast, or yell something dramatic.",
	"produce_flame":    "A flame springs to your palm. Use it as a torch, or sling it at someone. Multipurpose.",
	"ray_of_frost":     "A freezing beam catches a target and leaves them sluggish, frosted, and visibly cross.",
	"vampiric_touch":   "Drain life from a target. The damage you deal patches you up. Frowned upon at parties.",
	"vicious_mockery":  "Hurl an insult so cutting it physically hurts. Their next swing wobbles in shame.",

	// Hand-authored overlay descriptions in dnd_spells_data.go are the
	// canonical text and win at registry merge time, so they don't need
	// entries here. If a spell ever moves out of buildSpellList(), add it
	// above.
}

// magicItemDescOverride wins over the auto-derived Desc when the slug
// matches. Priority targets: items the plan called out by name (Amulet of
// Health, Belt of Giant Strength) and any item whose SRD blurb leaks rules
// math or names a spell the player can't cast.
var magicItemDescOverride = map[string]string{
	"amulet_of_health":                "Hums against your collarbone and quietly upgrades your constitution to 'bothersome to kill.'",
	"belt_of_giant_strength_hill":     "Buckle it on; suddenly you can deadlift the bartender. Hill-giant grade.",
	"belt_of_giant_strength_stone":    "Buckle it on; the floor creaks where you weren't standing yesterday. Stone-giant grade.",
	"belt_of_giant_strength_frost":    "Buckle it on; doors come off their hinges when you knock politely. Frost-giant grade.",
	"belt_of_giant_strength_fire":     "Buckle it on; furniture rearranges itself to be elsewhere. Fire-giant grade.",
	"belt_of_giant_strength_cloud":    "Buckle it on; you arm-wrestle siege engines for fun. Cloud-giant grade.",
	"belt_of_giant_strength_storm":    "Buckle it on; the world feels strangely flimsy. Storm-giant grade.",
	"belt_of_hill_giant_strength":     "Buckle it on; suddenly you can deadlift the bartender. Hill-giant grade.",
	"belt_of_stone_giant_strength":    "Buckle it on; the floor creaks where you weren't standing yesterday. Stone-giant grade.",
	"belt_of_frost_giant_strength":    "Buckle it on; doors come off their hinges when you knock politely. Frost-giant grade.",
	"belt_of_fire_giant_strength":     "Buckle it on; furniture rearranges itself to be elsewhere. Fire-giant grade.",
	"belt_of_cloud_giant_strength":    "Buckle it on; you arm-wrestle siege engines for fun. Cloud-giant grade.",
	"belt_of_storm_giant_strength":    "Buckle it on; the world feels strangely flimsy. Storm-giant grade.",
	"belt_of_dwarvenkind":             "A stout dwarven belt — beard not included, hardiness very much included.",
	"boots_of_levitation":             "Float upward at will. Down is a separate problem you'll figure out later.",
	"boots_of_speed":                  "Click the heels together; the world starts moving at a polite walking pace.",
	"boots_of_striding_and_springing": "Tireless stride and effortless hops. Ceilings: now optional.",
	"boots_of_the_winterlands":        "Snow doesn't slow you, cold doesn't bite, and your tracks tactfully erase themselves.",
	"bracers_of_archery":              "Steady your bow arm — your arrows hit harder and find seams in armour they shouldn't.",
	"bracers_of_defense":              "Light bracers that quietly turn aside blows. No armour required, no questions asked.",
	"brooch_of_shielding":             "Pinned to your cloak. Eats incoming bolts of force like they're hors d'oeuvres.",
	"cloak_of_displacement":           "Bends light around you. Attackers keep swinging at where you obviously are, and missing.",
	"cloak_of_elvenkind":              "Pulls shadows around you whether you asked or not. Watchful eyes slide right off.",
	"cloak_of_protection":             "A perfectly ordinary travelling cloak that perfectly extraordinarily refuses to let you get hit.",
	"cloak_of_resistance":             "Wards off one kind of harm — fire, frost, lightning, poison, or some other unpleasant Tuesday.",
	"cloak_of_the_bat":                "By night you glide instead of fall; on a really good night, you fly. Daytime: just a cloak.",
	"cloak_of_the_manta_ray":          "Underwater, you swim like a manta and breathe like you're on land. On land, you look damp.",
	"gauntlets_of_ogre_power":         "Heavy gauntlets that lend you an ogre's strength and an ogre's complete disregard for door frames.",
	"headband_of_intellect":           "Slips on, makes you think clever thoughts you have no business thinking. People notice.",
	"ring_of_protection":              "A plain ring. Quietly turns aside blows you'd otherwise take. Refuses to explain how.",
	"ring_of_regeneration":            "Wounds knit themselves back together while you wear it. Slowly. Smugly.",
	"ring_of_resistance":              "Wards off one kind of harm — fire, frost, lightning, poison, or another flavour of bad day.",
	"ring_of_swimming":                "Slip through water faster than any swimmer should. Dolphins file complaints.",
	"ring_of_warmth":                  "Cold weather no longer applies to you. Chill damage softens; you become unbearable in winter.",
	"slippers_of_spider_climbing":     "Walk up walls and across ceilings as if the floor were merely a suggestion.",
}

// cleanDesc strips the worst SRD jargon from a description before it lands in
// the generated file. It is intentionally conservative: drop offending phrases,
// collapse the resulting whitespace, leave the rest of the sentence alone. Any
// description that still reads poorly after this pass should get a hand-authored
// entry in spellDescOverride / magicItemDescOverride above.
func cleanDesc(s string) string {
	if s == "" {
		return s
	}
	for _, pat := range descScrub {
		s = pat.ReplaceAllString(s, "")
	}
	// Collapse runs of whitespace and tidy stray punctuation orphans left
	// behind by deletions ("a creature  and ", " , ", " .").
	s = reSpaces.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, " ,", ",")
	s = strings.ReplaceAll(s, " .", ".")
	s = strings.ReplaceAll(s, "( )", "")
	// Repair sentence stubs left by clause-strippers — "...must make." after
	// the saving-throw object is gone, or trailing " and." where one half of
	// a coordinated noun phrase was removed. These are conservative: they
	// only fire when the orphan sits at end-of-sentence.
	s = reOrphanMustMake.ReplaceAllString(s, "$1")
	s = reOrphanTrailingAnd.ReplaceAllString(s, ".")
	s = strings.TrimSpace(s)
	return s
}

var (
	reSpaces            = regexp.MustCompile(`\s+`)
	reOrphanMustMake    = regexp.MustCompile(`(?i)\s+must\s+(?:make|succeed on|attempt)([.,;])`)
	reOrphanTrailingAnd = regexp.MustCompile(`(?i),?\s+and\.`)
	descScrub           = []*regexp.Regexp{
		// Range / distance jargon.
		regexp.MustCompile(`(?i)\s*\bwithin range\b`),
		regexp.MustCompile(`(?i)\s*\bwithin \d+ feet(?: of [a-z ]+?)?`),
		regexp.MustCompile(`(?i)\s+\d+-foot[- ](?:radius|cone|cube|line|sphere|square)`),
		regexp.MustCompile(`(?i)\s+(?:out to a range of\s*)?\d+ feet\b`),
		regexp.MustCompile(`(?i)\s+out to a range of\b`),
		// Save / DC / slot jargon.
		regexp.MustCompile(`(?i)\s*\b(?:Strength|Dexterity|Constitution|Intelligence|Wisdom|Charisma)\s+saving throws?\b`),
		regexp.MustCompile(`(?i)\s*\bsaving throws?\b`),
		regexp.MustCompile(`(?i)\s*\(save(?:\s+DC\s*\d+)?\)`),
		regexp.MustCompile(`(?i)\s*\bDC\s*\d+\b`),
		regexp.MustCompile(`(?i)\s*\b(?:using|cast(?:ing)?(?: it)? with) a spell slot of \d+(?:st|nd|rd|th) level or higher\b`),
		regexp.MustCompile(`(?i)\s*\bspell slot\b`),
		// Cross-references and stat-block leakage.
		regexp.MustCompile(`(?i)\s*\bsee the spell\b`),
		regexp.MustCompile(`(?i)\s*\b(?:wisdom|strength|dexterity|constitution|intelligence|charisma)\s+save\b`),
		regexp.MustCompile(`(?i)\s*\b(?:wisdom|strength|dexterity|constitution|intelligence|charisma)\s+score\s+is\s+\d+\b`),
		regexp.MustCompile(`(?i)\s*\bconstitution score\b`),
	}
)
