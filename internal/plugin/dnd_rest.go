package plugin

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

// !rest short / !rest long.
//
// Short rest: spend 1 hit-dice charge (max charges = character level,
// restored on long rest). Heals 1d6+CON HP, x2 at L5+. Sets a 1h activity
// lockout — the character is *actually resting* for that hour and can't
// !zone enter / !expedition start until the timer expires.
//
// Long rest: 24h cooldown. Full HP recovery, restores all short rest
// charges, sets an 8h activity lockout. Requires housing (HouseTier > 0)
// OR pays the Thom Krooke inn (200 euros). Slot/spell refresh runs here.
//
// These commands operate on the D&D layer only. The legacy `!adventure`
// menu "rest" choice is unchanged — it remains the daily-action narrative
// day-skip.

const (
	dndLongRestCooldown      = 24 * time.Hour
	dndShortRestLockoutHours = 1
	dndLongRestLockoutHours  = 8
	dndInnCost               = 200
)

// restingLockoutRemaining returns the time left on a character's rest
// lockout (zero if not currently resting). Callers gate !zone enter and
// !expedition start on this so a freshly-rested character can't
// immediately jump back into combat.
func restingLockoutRemaining(c *DnDCharacter) time.Duration {
	if c == nil || c.RestingUntil == nil {
		return 0
	}
	remaining := time.Until(*c.RestingUntil)
	if remaining <= 0 {
		return 0
	}
	return remaining
}

// restBlockedReason returns a player-facing message when the character
// cannot rest right now because they're mid-fight or mid-expedition. An
// empty string means rest is allowed. Both !rest short and !rest long
// honor this — the dungeon shouldn't be a campfire.
//
// Both lookups resolve through the party: a seated member owns neither the
// session row nor the expedition row, so the owner-keyed reads would wave them
// through to a full heal mid boss fight.
func restBlockedReason(uid id.UserID) string {
	if sess, _ := activeCombatSessionFor(uid); sess != nil {
		return "You're mid-fight. Finish it (or `!flee`) before resting."
	}
	exp, isLeader, _ := activeExpeditionFor(uid)
	switch {
	case exp == nil:
		return ""
	case !isLeader:
		return "You can't rest while on an expedition. Ask your leader to `!extract`, or `!expedition leave` to walk out alone."
	}
	return "You can't rest while on an expedition. Use `!expedition extract` first."
}

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
		"`!rest short` — spend 1 hit-dice charge. Heals 1d6 + CON HP (x2 at L5+). Locks zone/expedition for **1 hour**.\n" +
		"`!rest long`  — once per 24h. Full HP, restores hit-dice charges. Locks zone/expedition for **8 hours**. Requires housing or pays the inn (€200)."
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

	if msg := restBlockedReason(ctx.Sender); msg != "" {
		return p.SendDM(ctx.Sender, msg)
	}
	if c.ShortRestCharges <= 0 {
		return p.SendDM(ctx.Sender,
			"You're out of short rest charges. Take a `!rest long` to restore them.")
	}
	hpFull := c.HPCurrent >= c.HPMax
	hasRefreshableSlots, _ := casterHasUsedSlots(ctx.Sender)
	if hpFull && !hasRefreshableSlots {
		return p.SendDM(ctx.Sender, "You're at full HP and your slots are full. Save the charge for when you need it.")
	}

	before := c.HPCurrent
	if !hpFull {
		conMod := abilityModifier(c.CON)
		healDie := 1 + simIntN(6) // 1d6
		heal := healDie + conMod
		if heal < 1 {
			heal = 1
		}
		if c.Level >= 5 {
			heal *= 2 // v1.0 §10.1: "x2 at levels 5+"
		}
		c.HPCurrent += heal
		if c.HPCurrent > c.HPMax {
			c.HPCurrent = c.HPMax
		}
	}
	c.ShortRestCharges--
	now := time.Now().UTC()
	c.LastShortRestAt = &now
	lockoutEnd := now.Add(dndShortRestLockoutHours * time.Hour)
	c.RestingUntil = &lockoutEnd

	slotsRestored, _ := partialRefreshSpellSlots(ctx.Sender, c.Level)

	if err := SaveDnDCharacter(c); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save rest state: "+err.Error())
	}
	markActedToday(ctx.Sender)

	var msg string
	if c.HPCurrent > before {
		msg = fmt.Sprintf(
			"🛌 **Short rest.** You recover **%d HP** (%d→%d / %d).\n_Charges remaining: %d. You're resting — `!zone` and `!expedition` locked for 1 hour._",
			c.HPCurrent-before, before, c.HPCurrent, c.HPMax, c.ShortRestCharges)
	} else {
		msg = fmt.Sprintf(
			"🛌 **Short rest.** You catch your breath — HP already full (%d/%d).\n_Charges remaining: %d. You're resting — `!zone` and `!expedition` locked for 1 hour._",
			c.HPCurrent, c.HPMax, c.ShortRestCharges)
	}
	if slotLine := dndShortRestSlotLine(slotsRestored); slotLine != "" {
		msg += "\n_" + slotLine + "_"
	}
	if line := dndRestShortFlavorLine(); line != "" {
		msg += "\n\n_" + line + "_"
	}
	return p.SendDM(ctx.Sender, msg)
}

// dndShortRestSlotLine renders a "Spell slots restored: 2 (L1), 1 (L2)."
// footer. Returns "" if nothing was restored, suppressing the line for
// martials and casters already at full.
func dndShortRestSlotLine(restored map[int]int) string {
	if len(restored) == 0 {
		return ""
	}
	levels := make([]int, 0, len(restored))
	for lvl := range restored {
		levels = append(levels, lvl)
	}
	sort.Ints(levels)
	parts := make([]string, 0, len(levels))
	for _, lvl := range levels {
		parts = append(parts, fmt.Sprintf("%d (L%d)", restored[lvl], lvl))
	}
	return "Spell slots restored: " + strings.Join(parts, ", ") + "."
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

	if msg := restBlockedReason(ctx.Sender); msg != "" {
		return p.SendDM(ctx.Sender, msg)
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
	house, _ := loadHouseState(ctx.Sender)
	hasHousing := house.Tier > 0
	innPaid := false
	if !hasHousing {
		if p.euro == nil || !p.euro.Debit(ctx.Sender, float64(dndInnCost), "dnd_inn_long_rest") {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"You need housing or €%d for the inn at Thom Krooke's. Run `!thom` to see housing options.",
				dndInnCost))
		}
		innPaid = true
	}

	// Well-rested buff: only when resting at your own home (not the inn), and
	// only from tier 2 up — a tier-1 shack and a rented inn room grant nothing.
	restTier := 0
	if hasHousing {
		restTier = house.Tier
	}

	c.HPCurrent = c.HPMax
	c.TempHP = wellRestedTempHP(c.HPMax, restTier)
	c.ShortRestCharges = c.Level
	// Phase 10 SUB2a — long rest clears one level of exhaustion (5e: a
	// long rest clears one). For Berserker who racks up exhaustion via
	// Frenzy, this is the recovery cadence.
	if c.Exhaustion > 0 {
		c.Exhaustion--
	}
	now := time.Now().UTC()
	c.LastLongRestAt = &now
	lockoutEnd := now.Add(dndLongRestLockoutHours * time.Hour)
	c.RestingUntil = &lockoutEnd

	if err := SaveDnDCharacter(c); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save rest state: "+err.Error())
	}
	markActedToday(ctx.Sender)
	_ = refreshAllResources(ctx.Sender)
	// Phase 9: spell slots refresh on long rest. A home rest also folds in the
	// well-rested bonus slots (no-op for non-casters and inn/tier-1 rests).
	slotBonus, _ := applyLongRestSpellSlots(c, restTier)
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
		"🌙 **Long rest** at %s. Full HP recovered (%d/%d). Hit-dice charges restored: **%d**.\n_You're resting — `!zone` and `!expedition` locked for 8 hours. Next long rest in 24 hours._",
		loc, c.HPCurrent, c.HPMax, c.ShortRestCharges)
	// HomeLongRest pool when at home; generic RestLong otherwise.
	if line := dndRestLongFlavorLine(hasHousing); line != "" {
		msg += "\n\n_" + line + "_"
	}
	if bits := wellRestedSummary(c.TempHP, slotBonus); bits != "" {
		homeName := "home"
		if def := houseTierByTier(restTier); def != nil {
			homeName = def.Name
		}
		msg += fmt.Sprintf("\n\n🛏️ _Well-rested at your %s — %s until your next long rest._",
			strings.ToLower(homeName), bits)
	}
	return p.SendDM(ctx.Sender, msg)
}
