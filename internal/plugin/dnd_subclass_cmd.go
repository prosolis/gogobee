package plugin

import (
	"fmt"
	"strings"
	"time"
)

// !subclass — Phase 10 SUB1.
//
// Usage:
//
//	!subclass                  → show selection prompt for player's class
//	!subclass <name|number>    → choose / change subclass
//
// Rules:
//   - L<5: blocked with hint message.
//   - First selection (Subclass == ""): free, no cooldown.
//   - Subsequent change: 30-day cooldown + 500-coin cost. Same surface
//     command — no separate `respec` subcommand. The full-character-wipe
//     `!respec` command is unchanged.
//
// Mechanical effects of L5/7/10/15 subclass abilities arrive in SUB2/SUB3.

const (
	dndSubclassMinLevel       = 5
	dndSubclassRespecCost     = 500
	dndSubclassRespecCooldown = 30 * 24 * time.Hour
)

func (p *AdventurePlugin) handleDnDSubclassCmd(ctx MessageContext, args string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	c, err := LoadDnDCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load your character: "+err.Error())
	}
	if c == nil || c.PendingSetup {
		return p.SendDM(ctx.Sender,
			"No Adv 2.0 character yet — run `!setup` (or just enter combat and we'll auto-build one).")
	}
	if c.Level < dndSubclassMinLevel {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"Subclass selection unlocks at **Level %d**. You're currently L%d.",
			dndSubclassMinLevel, c.Level))
	}
	if len(subclassesForClass(c.Class)) == 0 {
		return p.SendDM(ctx.Sender, "Your class has no subclasses registered yet.")
	}

	args = strings.TrimSpace(args)

	// Bare `!subclass` → prompt or current.
	if args == "" {
		if c.Subclass == "" {
			return p.SendDM(ctx.Sender, renderSubclassPrompt(c.Class))
		}
		info, _ := subclassInfo(c.Subclass)
		var b strings.Builder
		b.WriteString(fmt.Sprintf("**Current subclass:** %s — _%s_\n\n", info.Display, info.Tagline))
		b.WriteString(fmt.Sprintf(
			"To change: `!subclass <name>` — costs **%d euros**, **%d-day** cooldown.\n",
			dndSubclassRespecCost, int(dndSubclassRespecCooldown/(24*time.Hour))))
		return p.SendDM(ctx.Sender, b.String())
	}

	// `!subclass <name|number>` → set or change.
	chosen, ok := resolveSubclassInput(c.Class, args)
	if !ok {
		var b strings.Builder
		b.WriteString("Unknown subclass for your class. Options:\n\n")
		b.WriteString(renderSubclassPrompt(c.Class))
		return p.SendDM(ctx.Sender, b.String())
	}

	// First selection — free.
	if c.Subclass == "" {
		return p.applySubclassChoice(ctx, c, chosen, false)
	}

	// Same subclass already chosen — no-op.
	if c.Subclass == chosen {
		info, _ := subclassInfo(chosen)
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You're already a **%s**. No change made.", info.Display))
	}

	// Change — gated by cooldown + cost.
	if c.LastSubclassRespecAt != nil {
		elapsed := time.Since(*c.LastSubclassRespecAt)
		if elapsed < dndSubclassRespecCooldown {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"Subclass change is on cooldown — %s remaining.",
				formatRespecDuration(dndSubclassRespecCooldown-elapsed)))
		}
	}
	if p.euro == nil || p.euro.GetBalance(ctx.Sender) < float64(dndSubclassRespecCost) {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"Changing subclass costs **%d euros** — you don't have enough.",
			dndSubclassRespecCost))
	}
	return p.applySubclassChoice(ctx, c, chosen, true)
}

// applySubclassChoice writes the new subclass to the character and (if
// charged) debits the cost. Saves before debiting (audit pattern from
// !respec): if the save fails the player keeps their euros.
func (p *AdventurePlugin) applySubclassChoice(
	ctx MessageContext, c *DnDCharacter, chosen DnDSubclass, charged bool,
) error {
	prev := c.Subclass
	c.Subclass = chosen
	if charged {
		now := time.Now().UTC()
		c.LastSubclassRespecAt = &now
	}
	if err := SaveDnDCharacter(c); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save subclass choice: "+err.Error())
	}
	// Phase 10 SUB2a-ii — provision subclass-specific resource pools (e.g.
	// Battle Master superiority dice). Idempotent. Failure is non-fatal —
	// the player still gets the subclass; resources can be re-initialized
	// next long rest.
	_ = initSubclassResources(ctx.Sender, chosen, c.Level)
	// Phase 10 SUB2-AT — Arcane Trickster bootstraps a Mage-list spellbook
	// + third-caster slot pool on selection. Idempotent and non-fatal: the
	// next `!cast`/`!spells` call also runs ensureSpellsForCharacter.
	if chosen == SubclassArcaneTrickster {
		_ = ensureSpellsForCharacter(c)
	}
	if charged {
		// Debit last; race vs. balance change is rare and strictly safer
		// to favor the player than risk taking euros without applying the
		// change (matches the !respec ordering).
		// Race vs. balance change is rare; if Debit fails the save
		// already landed. Mirrors the !respec ordering in dnd_setup.go.
		_ = p.euro.Debit(ctx.Sender, float64(dndSubclassRespecCost), "dnd subclass change")
	}

	info, _ := subclassInfo(chosen)
	var b strings.Builder
	if prev == "" {
		b.WriteString(fmt.Sprintf("🎯 **Subclass selected: %s**\n\n", info.Display))
	} else {
		prevInfo, _ := subclassInfo(prev)
		b.WriteString(fmt.Sprintf("🎯 **Subclass changed: %s → %s**\n\n",
			prevInfo.Display, info.Display))
		b.WriteString(fmt.Sprintf("_%d euros spent. Cooldown: %dd before next change._\n\n",
			dndSubclassRespecCost, int(dndSubclassRespecCooldown/(24*time.Hour))))
	}
	b.WriteString("_" + info.Flavor + "_\n\n")
	b.WriteString("_(Mechanical L5/7/10/15 subclass abilities land in upcoming phases.)_")
	return p.SendDM(ctx.Sender, b.String())
}
