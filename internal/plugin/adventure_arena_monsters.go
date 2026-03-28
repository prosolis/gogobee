package plugin

// ── Arena Tier & Monster Definitions ────────────────────────────────────────
//
// Five tiers, four monsters each. Death chance, rewards, and XP scale with tier.
// Monster data from gogobee-arena.md spec.

type ArenaTier struct {
	Number          int
	Name            string
	MinLevel        int
	BasePayout      int64
	SkillMultiplier float64
	CompletionBonus int64
	BattleXP        int
	Monsters        [4]ArenaMonster
}

type ArenaMonster struct {
	Name          string
	Flavor        string
	BaseLethality float64
	ThreatLevel   int
}

var arenaTiers = [5]ArenaTier{
	// Tier 1 — Scrubs
	{
		Number: 1, Name: "Scrubs", MinLevel: 1,
		BasePayout: 150, SkillMultiplier: 1.0, CompletionBonus: 2500, BattleXP: 10,
		Monsters: [4]ArenaMonster{
			{
				Name:          "Gelatinous Homunculus",
				Flavor:        "A translucent blob of vague menace. Has been in every dungeon you've ever visited. Doesn't know why it's here.",
				BaseLethality: 0.10, ThreatLevel: 2,
			},
			{
				Name:          "Ratticus the Persistent",
				Flavor:        "A rat of unusual confidence. Has survived seventeen previous adventurers through sheer stubbornness.",
				BaseLethality: 0.18, ThreatLevel: 5,
			},
			{
				Name:          "Roadside Entrepreneur",
				Flavor:        "A bandit who has been robbing travelers at this exact location since approximately forever. Very committed to the bit.",
				BaseLethality: 0.28, ThreatLevel: 8,
			},
			{
				Name:          "Leafhopper Exemplar",
				Flavor:        "An insect of improbable aggression. Scientifically should not be a threat. Is one anyway. Earthbound energy.",
				BaseLethality: 0.38, ThreatLevel: 12,
			},
		},
	},
	// Tier 2 — Thugs
	{
		Number: 2, Name: "Thugs", MinLevel: 10,
		BasePayout: 500, SkillMultiplier: 2.5, CompletionBonus: 10000, BattleXP: 25,
		Monsters: [4]ArenaMonster{
			{
				Name:          "The Maestro of the Underchamber",
				Flavor:        "An operatic entity of considerable mass and questionable composition who rules his domain with a baritone and an inexhaustible supply of sweet corn. Not technically an RPG villain. Here anyway.",
				BaseLethality: 0.35, ThreatLevel: 16,
			},
			{
				Name:          "Lycanthropic Freelancer",
				Flavor:        "A werewolf between contracts. Takes the full moon very seriously. Will eat you on principle.",
				BaseLethality: 0.45, ThreatLevel: 20,
			},
			{
				Name:          "The Impersonator",
				Flavor:        "A chest. Definitely a chest. Please open it. (Do not open it.)",
				BaseLethality: 0.55, ThreatLevel: 25,
			},
			{
				Name:          "Armored Disagreement",
				Flavor:        "A dark knight who has made peace with violence as a communication strategy. Extensively equipped.",
				BaseLethality: 0.65, ThreatLevel: 30,
			},
		},
	},
	// Tier 3 — Brutes
	{
		Number: 3, Name: "Brutes", MinLevel: 25,
		BasePayout: 1500, SkillMultiplier: 6.0, CompletionBonus: 30000, BattleXP: 60,
		Monsters: [4]ArenaMonster{
			{
				Name:          "Stonefist the Unconvinced",
				Flavor:        "A golem who has heard every argument for sparing adventurers and found all of them unpersuasive.",
				BaseLethality: 0.55, ThreatLevel: 35,
			},
			{
				Name:          "Wyrm of Moderate Ambition",
				Flavor:        "Aspires to be a world-ending dragon. Currently a regional threat at best. Very sensitive about this.",
				BaseLethality: 0.65, ThreatLevel: 42,
			},
			{
				Name:          "Behemoth Adjacent",
				Flavor:        "Not quite a Behemoth. Close enough to ruin your day. Appears to have escaped from a larger game.",
				BaseLethality: 0.73, ThreatLevel: 48,
			},
			{
				Name:          "The Inevitable",
				Flavor:        "A reaper-class entity. No grievances. No agenda. Simply the direction all things are heading.",
				BaseLethality: 0.80, ThreatLevel: 55,
			},
		},
	},
	// Tier 4 — Horrors
	{
		Number: 4, Name: "Horrors", MinLevel: 45,
		BasePayout: 5000, SkillMultiplier: 12.0, CompletionBonus: 100000, BattleXP: 120,
		Monsters: [4]ArenaMonster{
			{
				Name:          "The Watcher in the Peripheral",
				Flavor:        "Seventeen eyes. None of them blink at the same time. Has been observing you specifically for longer than you've been alive.",
				BaseLethality: 0.72, ThreatLevel: 62,
			},
			{
				Name:          "Herald of the Outer Dark",
				Flavor:        "A daedric-adjacent entity that crossed over from a realm adjacent to yours and immediately became your problem. Very well dressed.",
				BaseLethality: 0.80, ThreatLevel: 70,
			},
			{
				Name:          "Lich Adjacent",
				Flavor:        "Not the Lich King. Definitely not. Unrelated individual. Happens to be a skeletal sorcerer of immense power. Coincidence.",
				BaseLethality: 0.87, ThreatLevel: 78,
			},
			{
				Name:          "The Collector of Faces",
				Flavor:        "It has yours already. Has had it for some time. The fight is a formality at this point.",
				BaseLethality: 0.92, ThreatLevel: 88,
			},
		},
	},
	// Tier 5 — World Eaters
	{
		Number: 5, Name: "World Eaters", MinLevel: 70,
		BasePayout: 15000, SkillMultiplier: 25.0, CompletionBonus: 500000, BattleXP: 250,
		Monsters: [4]ArenaMonster{
			{
				Name:          "Omega Mk. Zero",
				Flavor:        "A machine built to be the final test. Has never lost. Is aware of this.",
				BaseLethality: 0.85, ThreatLevel: 95,
			},
			{
				Name:          "The Calamity That Dreamed It Was Sleeping",
				Flavor:        "An ancient parasitic entity that fell from the sky an indeterminate number of years ago and has been ending timelines since. Definitely not Lavos.",
				BaseLethality: 0.90, ThreatLevel: 105,
			},
			{
				Name:          "The Architect of Endings",
				Flavor:        "A god who decided the world was a failed experiment and appointed itself project manager of its destruction. Silver hair. Long coat. Personal.",
				BaseLethality: 0.95, ThreatLevel: 115,
			},
			{
				Name:          "That Which Has Always Been",
				Flavor:        "Pre-dates language. Pre-dates light. The Arena was built around it, not the other way around. Winning this fight is not something the game's designers fully accounted for.",
				BaseLethality: 0.98, ThreatLevel: 130,
			},
		},
	},
}

// arenaGetTier returns the tier definition for a 1-indexed tier number.
// Returns nil if the tier number is out of range.
func arenaGetTier(tier int) *ArenaTier {
	if tier < 1 || tier > 5 {
		return nil
	}
	return &arenaTiers[tier-1]
}

// arenaGetMonster returns the monster for the given tier/round (both 1-indexed).
func arenaGetMonster(tier, round int) *ArenaMonster {
	t := arenaGetTier(tier)
	if t == nil || round < 1 || round > 4 {
		return nil
	}
	return &t.Monsters[round-1]
}

// ── Arena Death Flavor Text ─────────────────────────────────────────────────

var arenaDeathMessages = []string{
	"The Arena has collected its fee.",
	"GogoBee notes your performance for the record. The record is not flattering.",
	"Your earnings have been redistributed to the house. The house always wins.",
	"The crowd has already forgotten your name.",
	"A brief silence. Then the next challenger is called.",
	"The Arena floor is cleaned. It takes less time than expected.",
	"Your equipment will be returned to your next of kin. They will be disappointed by it.",
	"Statistics updated. Morale not applicable.",
}

// arenaMonsterDeathMessages are templates with {monster}, {round}, {tier} placeholders.
var arenaMonsterDeathMessages = []string{
	"{monster} has ended your run. Your earnings have been redistributed to the house.",
	"You made it to Round {round} of Tier {tier}. {monster} was not impressed.",
	"{monster} didn't even use their best material. Round {round}. Tier {tier}. Done.",
	"The last thing you see is {monster}. The last thing they see is lunch.",
}
