package plugin

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
)

// Tabletop conveniences — !roll, !stats, !level. None of these mutate
// state; they're pure display/RNG commands.

// ── !roll <dice> ─────────────────────────────────────────────────────────────

var diceNotation = regexp.MustCompile(`^(\d*)d(\d+)([+-]\d+)?$`)

const (
	dndRollMaxCount = 100
	dndRollMaxSides = 1000
)

// parseDice parses standard tabletop notation: "2d6+3", "d20", "4d6-1".
// Returns (count, sides, modifier, ok).
func parseDice(s string) (int, int, int, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	m := diceNotation.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, 0, false
	}
	count := 1
	if m[1] != "" {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, 0, 0, false
		}
		count = n
	}
	sides, err := strconv.Atoi(m[2])
	if err != nil || sides < 2 {
		return 0, 0, 0, false
	}
	mod := 0
	if m[3] != "" {
		n, err := strconv.Atoi(m[3])
		if err != nil {
			return 0, 0, 0, false
		}
		mod = n
	}
	if count < 1 || count > dndRollMaxCount || sides > dndRollMaxSides {
		return 0, 0, 0, false
	}
	return count, sides, mod, true
}

// rollDice returns the individual rolls and the total.
func rollDice(count, sides, mod int) ([]int, int) {
	rolls := make([]int, count)
	total := mod
	for i := 0; i < count; i++ {
		r := 1 + rand.IntN(sides)
		rolls[i] = r
		total += r
	}
	return rolls, total
}

func (p *AdventurePlugin) handleDnDRollCmd(ctx MessageContext, args string) error {
	args = strings.TrimSpace(args)
	if args == "" {
		return p.SendDM(ctx.Sender,
			"`!roll <dice>` — example: `!roll 2d6+3`, `!roll d20`, `!roll 4d6-1`. Max 100 dice, max 1000 sides.")
	}
	count, sides, mod, ok := parseDice(args)
	if !ok {
		return p.SendDM(ctx.Sender,
			"Couldn't parse dice. Use NdN+M format (e.g. `2d6+3`, `d20`, `4d6-1`).")
	}
	rolls, total := rollDice(count, sides, mod)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("🎲 **%dd%d", count, sides))
	if mod > 0 {
		b.WriteString(fmt.Sprintf("+%d", mod))
	} else if mod < 0 {
		b.WriteString(fmt.Sprintf("%d", mod))
	}
	b.WriteString("**\n")

	if count <= 20 {
		strs := make([]string, len(rolls))
		for i, r := range rolls {
			strs[i] = strconv.Itoa(r)
		}
		b.WriteString("Rolls: " + strings.Join(strs, ", "))
		if mod != 0 {
			if mod > 0 {
				b.WriteString(fmt.Sprintf(" (+%d)", mod))
			} else {
				b.WriteString(fmt.Sprintf(" (%d)", mod))
			}
		}
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("**Total: %d**", total))

	// Highlight nat-20 / nat-1 on a single d20
	if count == 1 && sides == 20 {
		switch rolls[0] {
		case 20:
			b.WriteString(" ✨ _critical!_")
		case 1:
			b.WriteString(" 💀 _fumble!_")
		}
	}

	return p.SendDM(ctx.Sender, b.String())
}

// ── !stats ───────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleDnDStatsCmd(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	advChar, _ := loadAdvCharacter(ctx.Sender)
	c, err := p.ensureCharForDnDCmd(ctx.Sender, advChar)
	if err != nil || c == nil {
		return p.SendDM(ctx.Sender, "Couldn't load your character.")
	}
	mods := c.Modifiers()
	ri, _ := raceInfo(c.Race)
	ci, _ := classInfo(c.Class)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**%s %s — Ability Scores**\n", ri.Display, ci.Display))
	b.WriteString(fmt.Sprintf("STR %2d (%+d)   DEX %2d (%+d)   CON %2d (%+d)\n",
		c.STR, mods[0], c.DEX, mods[1], c.CON, mods[2]))
	b.WriteString(fmt.Sprintf("INT %2d (%+d)   WIS %2d (%+d)   CHA %2d (%+d)\n",
		c.INT, mods[3], c.WIS, mods[4], c.CHA, mods[5]))
	b.WriteString(fmt.Sprintf("\nProficiency: +%d   AC: %d", proficiencyBonus(c.Level), c.ArmorClass))
	return p.SendDM(ctx.Sender, b.String())
}

// ── !level ───────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleDnDLevelCmd(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	advChar, _ := loadAdvCharacter(ctx.Sender)
	c, err := p.ensureCharForDnDCmd(ctx.Sender, advChar)
	if err != nil || c == nil {
		return p.SendDM(ctx.Sender, "Couldn't load your character.")
	}
	ri, _ := raceInfo(c.Race)
	ci, _ := classInfo(c.Class)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("⚔️ Level **%d** %s %s\n", c.Level, ri.Display, ci.Display))
	if c.Level >= dndMaxLevel {
		b.WriteString("XP: capped at L20.")
	} else {
		next := dndXPToNextLevel(c.Level)
		pct := int(100.0 * float64(c.XP) / float64(next))
		b.WriteString(fmt.Sprintf("XP: %d / %d (%d%% to L%d)", c.XP, next, pct, c.Level+1))
	}
	return p.SendDM(ctx.Sender, b.String())
}
