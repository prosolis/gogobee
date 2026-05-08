package plugin

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"maunium.net/go/mautrix/id"
)

// Phase 5 — D&D skill check resolver and !check command.
//
// Skill checks are d20 + ability modifier + race bonus vs. a target DC.
// Nat 20 auto-succeeds, nat 1 auto-fails (matches D&D 5e). Used by !check
// for ad-hoc rolls and by NPC handlers (Thom Krooke / Misty / Arina) for
// gated bonuses.

// ── Skill table ──────────────────────────────────────────────────────────────

type DnDSkill string

const (
	SkillAthletics      DnDSkill = "athletics"
	SkillAcrobatics     DnDSkill = "acrobatics"
	SkillStealth        DnDSkill = "stealth"
	SkillSleightOfHand  DnDSkill = "sleight_of_hand"
	SkillArcana         DnDSkill = "arcana"
	SkillInvestigation  DnDSkill = "investigation"
	SkillPerception     DnDSkill = "perception"
	SkillInsight        DnDSkill = "insight"
	SkillPersuasion     DnDSkill = "persuasion"
	SkillIntimidation   DnDSkill = "intimidation"
	SkillDeception      DnDSkill = "deception"
)

type dndSkillInfo struct {
	Key     DnDSkill
	Display string
	Stat    string // "str", "dex", "con", "int", "wis", "cha"
}

var dndSkillTable = []dndSkillInfo{
	{SkillAthletics, "Athletics", "str"},
	{SkillAcrobatics, "Acrobatics", "dex"},
	{SkillStealth, "Stealth", "dex"},
	{SkillSleightOfHand, "Sleight of Hand", "dex"},
	{SkillArcana, "Arcana", "int"},
	{SkillInvestigation, "Investigation", "int"},
	{SkillPerception, "Perception", "wis"},
	{SkillInsight, "Insight", "wis"},
	{SkillPersuasion, "Persuasion", "cha"},
	{SkillIntimidation, "Intimidation", "cha"},
	{SkillDeception, "Deception", "cha"},
}

func skillInfo(s DnDSkill) (dndSkillInfo, bool) {
	for _, si := range dndSkillTable {
		if si.Key == s {
			return si, true
		}
	}
	return dndSkillInfo{}, false
}

func parseSkill(s string) (DnDSkill, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, si := range dndSkillTable {
		if string(si.Key) == s || strings.EqualFold(si.Display, s) {
			return si.Key, true
		}
	}
	return "", false
}

// ── DC table ─────────────────────────────────────────────────────────────────

const (
	DCTrivial    = 5
	DCEasy       = 10
	DCMedium     = 15
	DCHard       = 20
	DCVeryHard   = 25
	DCImpossible = 30
)

func parseDC(s string) (int, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "trivial":
		return DCTrivial, true
	case "easy":
		return DCEasy, true
	case "medium", "med":
		return DCMedium, true
	case "hard":
		return DCHard, true
	case "veryhard", "very_hard", "very-hard":
		return DCVeryHard, true
	case "impossible":
		return DCImpossible, true
	}
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n, true
	}
	return 0, false
}

// ── Resolver ─────────────────────────────────────────────────────────────────

// SkillCheckResult is the outcome of a single d20 check.
type SkillCheckResult struct {
	Skill   DnDSkill
	DC      int
	Roll    int // raw d20 [1, 20]
	Mod     int // stat mod + race bonus
	Total   int // roll + mod (or auto-{success,fail} flag)
	Success bool
	Auto    bool // true if nat 20 / nat 1 short-circuited
}

// statValue extracts the named ability score from a DnDCharacter.
func statValue(c *DnDCharacter, stat string) int {
	switch stat {
	case "str":
		return c.STR
	case "dex":
		return c.DEX
	case "con":
		return c.CON
	case "int":
		return c.INT
	case "wis":
		return c.WIS
	case "cha":
		return c.CHA
	}
	return 10
}

// subclassSkillBonus returns the additive bonus from a subclass ability
// for a given skill. Phase 10 SUB2a:
//
//	Champion (L7+) Remarkable Athlete: +½ proficiency bonus (rounded down)
//	  to STR/DEX/CON skill checks. Approximation of 5e's "+½ prof to
//	  checks not already proficient"; we don't track per-skill prof yet.
//	Thief (L5+) Fast Hands: +5 to Sleight of Hand. Stand-in for 5e's
//	  "bonus action: SoH check" — out-of-combat utility flattened into a
//	  raw skill bonus.
func subclassSkillBonus(c *DnDCharacter, info dndSkillInfo) int {
	if c == nil || c.Subclass == "" {
		return 0
	}
	switch c.Subclass {
	case SubclassChampion:
		if c.Level >= 7 {
			switch info.Stat {
			case "str", "dex", "con":
				return proficiencyBonus(c.Level) / 2
			}
		}
	case SubclassThief:
		if c.Level >= 5 && info.Key == SkillSleightOfHand {
			return 5
		}
	case SubclassAssassin:
		// L5 Infiltration Expertise: crafting a false identity is a
		// Deception skill workout. L7 Impostor — perfect mimicry of voice
		// and manner — bumps the bonus further.
		if info.Key == SkillDeception {
			if c.Level >= 7 {
				return 10
			}
			if c.Level >= 5 {
				return 5
			}
		}
	}
	return 0
}

// subclassSkillAdvantage reports whether the player rolls with advantage
// (best of two d20s) on this skill check. Phase 10 SUB2a:
//
//	Thief (L7+) Supreme Sneak: advantage on Stealth (5e gates this on
//	  half-speed movement; we don't track movement speed, so we apply
//	  unconditionally — approachability cut).
func subclassSkillAdvantage(c *DnDCharacter, info dndSkillInfo) bool {
	if c == nil || c.Subclass == "" {
		return false
	}
	switch c.Subclass {
	case SubclassThief:
		if c.Level >= 7 && info.Key == SkillStealth {
			return true
		}
	}
	return false
}

// raceSkillBonus returns the per-skill bonus from a player's race.
// Half-Elf gets +1 to every skill (rough mapping of "two bonus skill profs").
// Tiefling gets +2 to CHA-based skills (matches the doc's "+bonus on CHA checks").
func raceSkillBonus(c *DnDCharacter, info dndSkillInfo) int {
	switch c.Race {
	case RaceHalfElf:
		return 1
	case RaceTiefling:
		if info.Stat == "cha" {
			return 2
		}
	}
	return 0
}

// performSkillCheck rolls the d20 and computes the result. Subclass
// effects (Phase 10 SUB2a) apply additive bonuses and may grant advantage
// (best of two d20s).
func performSkillCheck(c *DnDCharacter, skill DnDSkill, dc int) SkillCheckResult {
	info, _ := skillInfo(skill)
	mod := abilityModifier(statValue(c, info.Stat)) +
		raceSkillBonus(c, info) +
		subclassSkillBonus(c, info)
	roll := 1 + rand.IntN(20)
	if subclassSkillAdvantage(c, info) {
		if alt := 1 + rand.IntN(20); alt > roll {
			roll = alt
		}
	}
	res := SkillCheckResult{Skill: skill, DC: dc, Roll: roll, Mod: mod}

	switch roll {
	case 20:
		res.Success = true
		res.Auto = true
		res.Total = roll + mod
	case 1:
		res.Success = false
		res.Auto = true
		res.Total = roll + mod
	default:
		res.Total = roll + mod
		res.Success = res.Total >= dc
	}
	return res
}

// ── !check command ──────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleDnDCheckCmd(ctx MessageContext, args string) error {
	// !check can trigger auto-migration which writes to dnd_character.
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	args = strings.TrimSpace(args)
	if args == "" {
		return p.SendDM(ctx.Sender, dndCheckHelpText())
	}

	fields := strings.Fields(args)
	skill, ok := parseSkill(fields[0])
	if !ok {
		return p.SendDM(ctx.Sender,
			"Unknown skill. Try: "+strings.Join(dndSkillNames(), ", "))
	}

	dc := DCMedium
	if len(fields) >= 2 {
		if parsed, ok := parseDC(fields[1]); ok {
			dc = parsed
		} else {
			return p.SendDM(ctx.Sender,
				"DC must be a number or one of: trivial, easy, medium, hard, veryhard, impossible")
		}
	}

	advChar, _ := loadAdvCharacter(ctx.Sender)
	c, err := p.ensureCharForDnDCmd(ctx.Sender, advChar)
	if err != nil || c == nil {
		return p.SendDM(ctx.Sender, "Couldn't load your character.")
	}

	res := performSkillCheck(c, skill, dc)
	return p.SendDM(ctx.Sender, renderSkillCheck(res))
}

func renderSkillCheck(res SkillCheckResult) string {
	info, _ := skillInfo(res.Skill)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("🎲 **%s** check (DC %d)\n", info.Display, res.DC))
	b.WriteString(fmt.Sprintf("  d20 = %d  +  %s mod %+d  =  **%d**\n",
		res.Roll, strings.ToUpper(info.Stat), res.Mod, res.Total))
	if res.Auto && res.Roll == 20 {
		b.WriteString("  ✅ **Critical success** (nat 20)\n")
	} else if res.Auto && res.Roll == 1 {
		b.WriteString("  ❌ **Critical failure** (nat 1)\n")
	} else if res.Success {
		b.WriteString("  ✅ Success\n")
	} else {
		b.WriteString(fmt.Sprintf("  ❌ Failed (needed %d)\n", res.DC))
	}
	return b.String()
}

func dndCheckHelpText() string {
	var b strings.Builder
	b.WriteString("**Skill Check** — `!check <skill> [dc]`\n\n")
	b.WriteString("Skills: " + strings.Join(dndSkillNames(), ", ") + "\n\n")
	b.WriteString("DC: a number, or `trivial` (5), `easy` (10), `medium` (15, default), ")
	b.WriteString("`hard` (20), `veryhard` (25), `impossible` (30)\n\n")
	b.WriteString("Example: `!check athletics hard`")
	return b.String()
}

// ── NPC skill-check hooks ────────────────────────────────────────────────────
//
// These are silent upside helpers: if the player has a confirmed D&D
// character and rolls well on the relevant skill, the NPC interaction's
// cost is refunded. Legacy (no D&D character) players see no change.
//
// The result type distinguishes "no check attempted" (no D&D char) from
// "checked, failed" — so callers can surface different flavor for each.

// NPCSkillCheckResult tells the caller whether a D&D skill check was
// even applicable (Attempted=false → no D&D char) and, if so, whether it
// succeeded.
type NPCSkillCheckResult struct {
	Attempted bool
	Succeeded bool
}

func dndNPCInsightRefund(userID id.UserID, euro *EuroPlugin, cost int) NPCSkillCheckResult {
	return dndNPCRefundCheck(userID, euro, SkillInsight, 12, cost, "misty_insight_refund")
}

func dndNPCArcanaRefund(userID id.UserID, euro *EuroPlugin, cost int) NPCSkillCheckResult {
	return dndNPCRefundCheck(userID, euro, SkillArcana, 14, cost, "arina_arcana_refund")
}

// dndNPCPersuasionDiscount: returns true if the player passed a Persuasion
// check (DC 15). Caller applies the 10% discount. No euro side effect here.
func dndNPCPersuasionDiscount(userID id.UserID) bool {
	c, err := LoadDnDCharacter(userID)
	if err != nil || c == nil || c.PendingSetup {
		return false
	}
	return performSkillCheck(c, SkillPersuasion, 15).Success
}

func dndNPCRefundCheck(userID id.UserID, euro *EuroPlugin, skill DnDSkill, dc int, cost int, reason string) NPCSkillCheckResult {
	c, err := LoadDnDCharacter(userID)
	if err != nil || c == nil || c.PendingSetup {
		return NPCSkillCheckResult{}
	}
	res := performSkillCheck(c, skill, dc)
	if !res.Success {
		return NPCSkillCheckResult{Attempted: true, Succeeded: false}
	}
	if euro != nil {
		euro.Credit(userID, float64(cost), reason)
	}
	return NPCSkillCheckResult{Attempted: true, Succeeded: true}
}

func dndSkillNames() []string {
	out := make([]string, 0, len(dndSkillTable))
	for _, si := range dndSkillTable {
		out = append(out, string(si.Key))
	}
	return out
}
