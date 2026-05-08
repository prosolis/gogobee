package plugin

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

// Phase 6 — !rest short / !rest long.
//
// Short rest: 1h cooldown, no daily-action cost. Recovers 1d6+CON HP at L1-4
// and 2*(1d6+CON) at L5+ (matching v1.0 §10.1's "x2 at level 5+" line).
//
// Long rest: 24h cooldown. Requires housing (HouseTier > 0) OR pays the
// Thom Krooke inn (200 euros). Full HP recovery; resources reset (none yet
// in Phase 6, but the wiring is in place for Phase 7+).
//
// These commands operate on the D&D layer only. The legacy `!adventure` menu
// "rest" choice is unchanged — it remains the daily-action narrative day-skip.

const (
	dndShortRestCooldown = 1 * time.Hour
	dndLongRestCooldown  = 24 * time.Hour
	dndInnCost           = 200
)

func (p *AdventurePlugin) handleDnDRestCmd(ctx MessageContext, args string) error {
	args = strings.TrimSpace(strings.ToLower(args))
	switch args {
	case "short":
		return p.handleDnDShortRest(ctx)
	case "long":
		return p.handleDnDLongRest(ctx)
	case "":
		return p.SendDM(ctx.Sender, dndRestHelpText())
	}
	return p.SendDM(ctx.Sender, "Unknown rest type. Use `!rest short` or `!rest long`.")
}

func dndRestHelpText() string {
	return "🛌 **Adv 2.0 Rest**\n\n" +
		"`!rest short` — 1h cooldown. Recovers 1d6 + CON HP. No action cost.\n" +
		"`!rest long`  — 24h cooldown. Full HP recovery. Requires housing or pays the inn (€200)."
}

// ── Short rest ───────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleDnDShortRest(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	advChar, _ := loadAdvCharacter(ctx.Sender)
	c, err := p.ensureCharForDnDCmd(ctx.Sender, advChar)
	if err != nil || c == nil {
		return p.SendDM(ctx.Sender, "Couldn't load your character.")
	}

	if c.LastShortRestAt != nil {
		elapsed := time.Since(*c.LastShortRestAt)
		if elapsed < dndShortRestCooldown {
			remaining := dndShortRestCooldown - elapsed
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"Short rest on cooldown — %s remaining.", formatRespecDuration(remaining)))
		}
	}

	if c.HPCurrent >= c.HPMax {
		return p.SendDM(ctx.Sender, "You're already at full HP. Save the rest for when you need it.")
	}

	conMod := abilityModifier(c.CON)
	healDie := 1 + rand.IntN(6) // 1d6
	heal := healDie + conMod
	if heal < 1 {
		heal = 1
	}
	if c.Level >= 5 {
		heal *= 2 // v1.0 §10.1: "x2 at levels 5+"
	}

	before := c.HPCurrent
	c.HPCurrent += heal
	if c.HPCurrent > c.HPMax {
		c.HPCurrent = c.HPMax
	}
	now := time.Now().UTC()
	c.LastShortRestAt = &now

	if err := SaveDnDCharacter(c); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save rest state: "+err.Error())
	}

	msg := fmt.Sprintf(
		"🛌 **Short rest.** You recover **%d HP** (%d→%d / %d).\n_Next short rest available in 1 hour._",
		c.HPCurrent-before, before, c.HPCurrent, c.HPMax)
	if line := dndRestShortFlavorLine(); line != "" {
		msg += "\n\n_" + line + "_"
	}
	return p.SendDM(ctx.Sender, msg)
}

// ── Long rest ────────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleDnDLongRest(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	advChar, err := loadAdvCharacter(ctx.Sender)
	if err != nil || advChar == nil {
		return p.SendDM(ctx.Sender, "Couldn't load your character.")
	}
	c, err := p.ensureCharForDnDCmd(ctx.Sender, advChar)
	if err != nil || c == nil {
		return p.SendDM(ctx.Sender, "Couldn't load your Adv 2.0 sheet.")
	}

	if c.LastLongRestAt != nil {
		elapsed := time.Since(*c.LastLongRestAt)
		if elapsed < dndLongRestCooldown {
			remaining := dndLongRestCooldown - elapsed
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"Long rest on cooldown — %s remaining.", formatRespecDuration(remaining)))
		}
	}

	// Eligibility: housing OR pay inn fee.
	hasHousing := advChar.HouseTier > 0
	innPaid := false
	if !hasHousing {
		if p.euro == nil || !p.euro.Debit(ctx.Sender, float64(dndInnCost), "dnd_inn_long_rest") {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"You need housing or €%d for the inn at Thom Krooke's. Run `!thom` to see housing options.",
				dndInnCost))
		}
		innPaid = true
	}

	c.HPCurrent = c.HPMax
	c.TempHP = 0
	now := time.Now().UTC()
	c.LastLongRestAt = &now

	if err := SaveDnDCharacter(c); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save rest state: "+err.Error())
	}
	_ = refreshAllResources(ctx.Sender)
	// Phase 9: spell slots refresh on long rest.
	_ = refreshSpellSlots(ctx.Sender)
	// Phase 9: Cleric prep flags reset (SP4) — until SP4 ships, default
	// Cleric grants are already prepared=1 so this is a no-op for them.
	// Voluntary concentration ends at long rest (mage_armor's 8h is exactly
	// the rest interval; resetting here keeps state clean).
	c.ConcentrationSpell = ""
	c.ConcentrationExpiresAt = nil
	c.PendingCast = ""
	_ = SaveDnDCharacter(c)

	loc := "your home"
	if innPaid {
		loc = fmt.Sprintf("the inn (€%d spent)", dndInnCost)
	}
	msg := fmt.Sprintf(
		"🌙 **Long rest** at %s. Full HP recovered (%d/%d).\n_Next long rest available in 24 hours._",
		loc, c.HPCurrent, c.HPMax)
	// HomeLongRest pool when at home; generic RestLong otherwise.
	if line := dndRestLongFlavorLine(hasHousing); line != "" {
		msg += "\n\n_" + line + "_"
	}
	return p.SendDM(ctx.Sender, msg)
}
