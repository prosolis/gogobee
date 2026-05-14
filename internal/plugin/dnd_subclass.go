package plugin

import (
	"sort"
	"strings"
)

// Phase 10 — subclass registry. Implements `gogobee_subclass_system.md`:
// 15 subclasses (3 per class × 5 classes), unlocked at L5 via !subclass.
//
// SUB1 (this file + dnd_subclass_cmd.go) covers identity only — selection,
// storage, !respec, sheet display. Mechanical effects of L5/7/10/15
// abilities arrive in SUB2/SUB3.
//
// Forward-deps that aren't built yet are deliberately stubbed: Beast
// Master picks the subclass even though the Phase 15 pet rework hasn't
// landed; Necromancer Animate Dead waits on an undead-summon engine.

type DnDSubclass string

const (
	// Fighter
	SubclassChampion     DnDSubclass = "champion"
	SubclassBattleMaster DnDSubclass = "battle_master"
	SubclassBerserker    DnDSubclass = "berserker"
	// Rogue
	SubclassThief           DnDSubclass = "thief"
	SubclassAssassin        DnDSubclass = "assassin"
	SubclassArcaneTrickster DnDSubclass = "arcane_trickster"
	// Mage
	SubclassEvocation  DnDSubclass = "evocation"
	SubclassAbjuration DnDSubclass = "abjuration"
	SubclassNecromancy DnDSubclass = "necromancy"
	// Cleric
	SubclassLifeDomain     DnDSubclass = "life_domain"
	SubclassWarDomain      DnDSubclass = "war_domain"
	SubclassTrickeryDomain DnDSubclass = "trickery_domain"
	// Ranger
	SubclassHunter       DnDSubclass = "hunter"
	SubclassBeastMaster  DnDSubclass = "beast_master"
	SubclassGloomStalker DnDSubclass = "gloom_stalker"
	// Druid — Open5e caster scaffold
	SubclassCircleLand     DnDSubclass = "circle_land"
	SubclassCircleMoon     DnDSubclass = "circle_moon"
	SubclassCircleShepherd DnDSubclass = "circle_shepherd"
	// Bard
	SubclassCollegeLore    DnDSubclass = "college_lore"
	SubclassCollegeValor   DnDSubclass = "college_valor"
	SubclassCollegeGlamour DnDSubclass = "college_glamour"
	// Sorcerer
	SubclassDraconicBloodline DnDSubclass = "draconic_bloodline"
	SubclassWildMagic         DnDSubclass = "wild_magic"
	SubclassStormSorcery      DnDSubclass = "storm_sorcery"
	// Warlock
	SubclassFiendPatron   DnDSubclass = "fiend_patron"
	SubclassArchfeyPatron DnDSubclass = "archfey_patron"
	SubclassGreatOldOne   DnDSubclass = "great_old_one"
	// Paladin
	SubclassOathDevotion  DnDSubclass = "oath_devotion"
	SubclassOathVengeance DnDSubclass = "oath_vengeance"
	SubclassOathAncients  DnDSubclass = "oath_ancients"
)

// DnDSubclassInfo — registry row. Tagline summarizes identity in one line
// (used in selection prompt). Flavor is the TwinBee narration line shown
// on selection (per design doc §7).
type DnDSubclassInfo struct {
	ID      DnDSubclass
	Class   DnDClass
	Display string
	Tagline string
	Flavor  string
}

// dndSubclassRegistry — 30 entries (3 per class × 10 classes), ordered
// first by class (Fighter, Rogue, Mage, Cleric, Ranger, then the Open5e
// caster scaffold: Druid, Bard, Sorcerer, Warlock, Paladin) then by the
// design doc's listed order within each class. The selection prompt
// presents them in this order. The caster subclasses use canonical 5e
// archetype names, consistent with the original fifteen — see the
// avoid-dnd-naming note: the naming concern is code identifiers and
// marketing, not in-game content.
var dndSubclassRegistry = []DnDSubclassInfo{
	// Fighter
	{SubclassChampion, ClassFighter, "Champion",
		"Raw power, brutal efficiency.",
		"You've chosen the old way. No tricks, no systems. Just you, and the blade, and the next hit. TwinBee approves of this directness."},
	{SubclassBattleMaster, ClassFighter, "Battle Master",
		"Tactical superiority, maneuver control.",
		"Maneuvers, dice, control of the engagement. TwinBee notes that the smartest fighter in the room rarely throws the first punch."},
	{SubclassBerserker, ClassFighter, "Berserker",
		"Rage-fueled destruction, high risk/reward.",
		"Rage as a tool, exhaustion as a tax. TwinBee files this under 'load-bearing recklessness'."},

	// Rogue
	{SubclassThief, ClassRogue, "Thief",
		"Speed, opportunism, fast hands.",
		"Quick fingers, quicker exits. TwinBee suspects the locks of the world have been put on notice."},
	{SubclassAssassin, ClassRogue, "Assassin",
		"Setup and execution; devastating openers.",
		"Patience, then violence, then nothing. TwinBee respects the economy of motion."},
	{SubclassArcaneTrickster, ClassRogue, "Arcane Trickster",
		"Stealth plus a slice of Mage spellcasting.",
		"Magic in the hands of a Rogue. TwinBee considers this the universe's way of apologizing for giving you low HP."},

	// Mage
	{SubclassEvocation, ClassMage, "Evocation",
		"Pure damage output. The glass cannon perfected.",
		"You picked the path of the louder spell. TwinBee finds this aesthetically pleasing and structurally alarming."},
	{SubclassAbjuration, ClassMage, "Abjuration",
		"Wards, shields, counterspells.",
		"You'd rather not get hit, and you've made it your specialty. TwinBee approves of this kind of foresight."},
	{SubclassNecromancy, ClassMage, "Necromancy",
		"Control undead and drain life.",
		"Undeath as a tool rather than a state. TwinBee has opinions about this. TwinBee is keeping them to itself."},

	// Cleric
	{SubclassLifeDomain, ClassCleric, "Life Domain",
		"Maximum healing output. Party support anchor.",
		"Healing as discipline, not afterthought. TwinBee notes the party will live longer because of you, whether they thank you or not."},
	{SubclassWarDomain, ClassCleric, "War Domain",
		"Combat Cleric — extra attacks, martial weapons.",
		"You came to fight, not to mediate. TwinBee finds this clarifying."},
	{SubclassTrickeryDomain, ClassCleric, "Trickery Domain",
		"Illusion, misdirection, chaos.",
		"A Cleric who lies for a living. TwinBee delights in the implied theological complications."},

	// Ranger
	{SubclassHunter, ClassRanger, "Hunter",
		"Zone specialist; bonuses against enemy types.",
		"You read the terrain before you draw the bow. TwinBee finds this professionalism rare and welcome."},
	{SubclassBeastMaster, ClassRanger, "Beast Master",
		"Deep Ranger–pet bond. Pet as combat partner.",
		"You and your pet, all the way down. TwinBee finds this genuinely moving and declines to be embarrassed about that."},
	{SubclassGloomStalker, ClassRanger, "Gloom Stalker",
		"Darkness specialist. Exceptional underground.",
		"You were already good in the dark. Now you're better. TwinBee notes: the Underdark is waiting."},

	// Druid — Open5e caster scaffold
	{SubclassCircleLand, ClassDruid, "Circle of the Land",
		"Versatile caster; recovery and warding.",
		"The land remembers you, and answers when you call. A measured path — and a durable one."},
	{SubclassCircleMoon, ClassDruid, "Circle of the Moon",
		"Combat shapeshifter; tanky beast forms.",
		"You don't cast at the fight so much as become it. Tooth, claw, and a great deal of hide."},
	{SubclassCircleShepherd, ClassDruid, "Circle of the Shepherd",
		"Summoner; spirit totems and conjured allies.",
		"You never walk into a fight alone — the wild walks in with you."},

	// Bard
	{SubclassCollegeLore, ClassBard, "College of Lore",
		"Knowledge and control; turn the enemy's edge dull.",
		"You've read the room, the foe, and three books about both. The fight is half-won before it starts."},
	{SubclassCollegeValor, ClassBard, "College of Valor",
		"Combat bard; songs that sharpen sword and shield.",
		"A song with a blade in it. The old kind of bard — the kind that came back from the war."},
	{SubclassCollegeGlamour, ClassBard, "College of Glamour",
		"Charm and majesty; enthrall and disarm the foe.",
		"You wear the Feywild's glamour like a borrowed coat. Few can bear to strike at something so dazzling."},

	// Sorcerer
	{SubclassDraconicBloodline, ClassSorcerer, "Draconic Bloodline",
		"Durable blaster; draconic scales and elemental punch.",
		"There's a dragon somewhere back down your line, and it left the door unlocked."},
	{SubclassWildMagic, ClassSorcerer, "Wild Magic",
		"Chaotic burst; luck bent hard in your favor.",
		"Your magic doesn't take orders so much as suggestions. Loud, unpredictable, occasionally glorious."},
	{SubclassStormSorcery, ClassSorcerer, "Storm Sorcery",
		"Mobile elemental caster; wind, speed, and thunder.",
		"You move first because the storm always does. Everything after is just weather."},

	// Warlock
	{SubclassFiendPatron, ClassWarlock, "The Fiend",
		"Damage and grit; power borrowed from the lower planes.",
		"The bargain was steep, the firepower is steeper. Try not to read the fine print mid-fight."},
	{SubclassArchfeyPatron, ClassWarlock, "The Archfey",
		"Charm and escape; fey tricks that unmake the attack.",
		"Your patron deals in glamour and misdirection — and so, now, do you. Hard to hit what won't hold still."},
	{SubclassGreatOldOne, ClassWarlock, "The Great Old One",
		"Psychic control; minds bent, foes unmade from within.",
		"Something vast and distant is half-listening through you. Best not to think too hard about which half."},

	// Paladin
	{SubclassOathDevotion, ClassPaladin, "Oath of Devotion",
		"Holy warrior; sacred weapon and protective auras.",
		"You took the oath and meant every word of it. The light leans your way for it."},
	{SubclassOathVengeance, ClassPaladin, "Oath of Vengeance",
		"Relentless attacker; mark a foe and bring it down.",
		"Someone, somewhere, has earned this. You've simply volunteered to deliver it."},
	{SubclassOathAncients, ClassPaladin, "Oath of the Ancients",
		"Resilient nature-paladin; warding light, hard to fell.",
		"You guard the green and growing things, and the old light guards you back. Stubbornly."},
}

// subclassInfo returns the registry row for id, or (zero, false).
func subclassInfo(id DnDSubclass) (DnDSubclassInfo, bool) {
	for _, s := range dndSubclassRegistry {
		if s.ID == id {
			return s, true
		}
	}
	return DnDSubclassInfo{}, false
}

// subclassesForClass returns the 3 subclasses available to a given class,
// in registry/design-doc order.
func subclassesForClass(class DnDClass) []DnDSubclassInfo {
	out := make([]DnDSubclassInfo, 0, 3)
	for _, s := range dndSubclassRegistry {
		if s.Class == class {
			out = append(out, s)
		}
	}
	return out
}

// resolveSubclassInput maps user-supplied input ("1", "champion",
// "battle master", "BattleMaster") to a subclass id valid for the given
// class. Returns ("", false) if no match.
func resolveSubclassInput(class DnDClass, input string) (DnDSubclass, bool) {
	subs := subclassesForClass(class)
	if len(subs) == 0 {
		return "", false
	}
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}
	// Numeric: "1", "2", "3".
	if len(trimmed) == 1 && trimmed[0] >= '1' && trimmed[0] <= '9' {
		idx := int(trimmed[0] - '1')
		if idx >= 0 && idx < len(subs) {
			return subs[idx].ID, true
		}
	}
	// Name match: case-insensitive, ignore spaces/underscores.
	norm := normalizeSubclassName(trimmed)
	for _, s := range subs {
		if normalizeSubclassName(string(s.ID)) == norm ||
			normalizeSubclassName(s.Display) == norm {
			return s.ID, true
		}
	}
	return "", false
}

func normalizeSubclassName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

// renderSubclassPrompt builds the "Choose your path" message for the
// player's class. Used by !subclass with no args and by the L5 level-up
// DM cue.
func renderSubclassPrompt(class DnDClass) string {
	subs := subclassesForClass(class)
	if len(subs) == 0 {
		return "" // unreachable for any registered class
	}
	ci, _ := classInfo(class)
	var b strings.Builder
	b.WriteString("⚔️ **Choose your path** — ")
	b.WriteString(ci.Display)
	b.WriteString(" subclass\n\n")
	for i, s := range subs {
		b.WriteString("  ")
		b.WriteString(string(rune('1' + i)))
		b.WriteString(". **")
		b.WriteString(s.Display)
		b.WriteString("** — ")
		b.WriteString(s.Tagline)
		b.WriteString("\n")
	}
	b.WriteString("\nReply: `!subclass <name or number>` (e.g. `!subclass 1` or `!subclass champion`).")
	return b.String()
}

// allSubclassIDs — used by tests/audits to walk every subclass.
func allSubclassIDs() []DnDSubclass {
	ids := make([]DnDSubclass, 0, len(dndSubclassRegistry))
	for _, s := range dndSubclassRegistry {
		ids = append(ids, s.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
