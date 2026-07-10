package plugin

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// !setup flow. Player runs subcommands; draft persists in dnd_character with
// pending_setup=1 until !setup confirm finalizes.
//
//   !setup                — show current draft + next step
//   !setup race <name>    — pick race
//   !setup class <name>   — pick class
//   !setup stats <s> <d> <c> <i> <w> <h>   — assign standard array
//   !setup confirm        — finalize, compute HP/AC, clear pending_setup
//   !setup cancel         — wipe the draft

// ── Archetype → Race/Class inference ─────────────────────────────────────────

type dndSuggestion struct {
	Race  DnDRace
	Class DnDClass
	Why   string // archetype name that drove the suggestion
}

// inferDnDFromArchetypes returns a (race, class) suggestion based on the
// player's highest-signal archetype, or the Human Fighter fallback if no
// archetype gives a clear signal.
//
// Map keyed to the *real* archetype names in archetype.go (v1.1 §3.2).
func inferDnDFromArchetypes(userID id.UserID) dndSuggestion {
	results := GetUserArchetypes(string(userID))

	// archetype name → (class, race). First match wins; results are already
	// sorted by signal_score desc.
	mapping := map[string]struct {
		Class DnDClass
		Race  DnDRace
	}{
		"Arena Champion":  {ClassFighter, RaceOrc},
		"Dungeon Crawler": {ClassFighter, RaceHuman},
		"The Adventurer":  {ClassRanger, RaceHuman},
		"The Angler":      {ClassRanger, RaceElf},
		"The Forager":     {ClassRanger, RaceHalfling},
		"The Miner":       {ClassFighter, RaceDwarf},
		"The Merchant":    {ClassRogue, RaceHalfling},
		"Whale":           {ClassRogue, RaceHuman},
		"Degenerate":      {ClassRogue, RaceTiefling},
		"Novelist":        {ClassMage, RaceElf},
		"Philosopher":     {ClassMage, RaceTiefling},
		"Wordsmith":       {ClassMage, RaceElf},
		"Linkmaster":      {ClassMage, RaceHuman},
		"Inquisitor":      {ClassMage, RaceTiefling},
		"Cheerleader":     {ClassCleric, RaceHalfElf},
		"Hype Machine":    {ClassCleric, RaceHalfElf},
		"Patron":          {ClassCleric, RaceDwarf},
		"Reactor":         {ClassCleric, RaceHalfling},
		"Gearhead":        {ClassFighter, RaceDwarf},
		"Shark":           {ClassRogue, RaceHalfElf},
		"Trivia Nerd":     {ClassMage, RaceHalfElf},
	}

	for _, r := range results {
		if m, ok := mapping[r.Name]; ok {
			return dndSuggestion{Race: m.Race, Class: m.Class, Why: r.Name}
		}
	}
	return dndSuggestion{Race: RaceHuman, Class: ClassFighter, Why: "default"}
}

// ── Command handler ──────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleDnDSetupCmd(ctx MessageContext, args string) error {
	// Audit fix A: serialize per-user state mutations. Reuses the existing
	// adventure lock so !setup also serializes against !arena, !adventure
	// dungeon, etc. — all of which can write to dnd_character via auto-migration.
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	args = strings.TrimSpace(args)
	lower := strings.ToLower(args)

	// Bare !setup → show status / instructions.
	if args == "" {
		return p.dndSetupStatus(ctx)
	}

	switch {
	case strings.HasPrefix(lower, "race "):
		return p.dndSetupRace(ctx, strings.TrimSpace(args[5:]))
	case strings.HasPrefix(lower, "class "):
		return p.dndSetupClass(ctx, strings.TrimSpace(args[6:]))
	case strings.HasPrefix(lower, "stats "):
		return p.dndSetupStats(ctx, strings.TrimSpace(args[6:]))
	case lower == "confirm":
		return p.dndSetupConfirm(ctx)
	case lower == "cancel":
		return p.dndSetupCancel(ctx)
	}
	return p.SendDM(ctx.Sender, "Unknown !setup subcommand. Try !setup with no args for instructions.")
}

func (p *AdventurePlugin) dndSetupStatus(ctx MessageContext) error {
	c, err := LoadDnDCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load your Adv 2.0 draft: "+err.Error())
	}

	// No row yet → fresh setup. Show suggestion + race menu.
	if c == nil {
		sug := inferDnDFromArchetypes(ctx.Sender)
		var b strings.Builder
		b.WriteString("⚔️ **Adv 2.0 Setup** — let's build your character.\n\n")
		b.WriteString(fmt.Sprintf("Based on your play style we'd suggest a **%s %s** "+
			"_(driven by your %s archetype)_.\n\n",
			titleRace(sug.Race), titleClass(sug.Class), sug.Why))
		b.WriteString("**Step 1 — Race.** Pick one of:\n")
		b.WriteString(renderRaceMenu())
		b.WriteString("\nReply: `!setup race <name>` (e.g. `!setup race elf`)\n")
		return p.SendDM(ctx.Sender, b.String())
	}

	// Confirmed already → reroute to !sheet, unless this is an auto-migrated
	// character, in which case we let the player rebuild freely.
	if !c.PendingSetup {
		if c.AutoMigrated {
			ri, _ := raceInfo(c.Race)
			ci, _ := classInfo(c.Class)
			var b strings.Builder
			b.WriteString(fmt.Sprintf("⚔️ **Adv 2.0 Setup**\n\nWe auto-built you a **%s %s** based on your play style when you first fought, "+
				"so you'd never miss a battle. You can rebuild freely — no cooldown.\n\n", ri.Display, ci.Display))
			b.WriteString("**Step 1 — Race.** Pick one of:\n")
			b.WriteString(renderRaceMenu())
			b.WriteString("\nReply: `!setup race <name>` (or `!sheet` to keep what you have).\n")
			return p.SendDM(ctx.Sender, b.String())
		}
		return p.SendDM(ctx.Sender, "You've already set up your character. Use `!sheet` to view it, or `!respec` to start over (cooldown applies).")
	}

	// Draft in progress — show what's set and what's next.
	var b strings.Builder
	b.WriteString("⚔️ **Adv 2.0 Setup** — draft in progress.\n\n")
	if c.Race == "" {
		b.WriteString("**Next: Step 1 — Race.**\n")
		b.WriteString(renderRaceMenu())
		b.WriteString("\nReply: `!setup race <name>`\n")
		return p.SendDM(ctx.Sender, b.String())
	}
	b.WriteString(fmt.Sprintf("Race: **%s** ✓\n", titleRace(c.Race)))
	if c.Class == "" {
		b.WriteString("\n**Next: Step 2 — Class.**\n")
		b.WriteString(renderClassMenu())
		b.WriteString("\nReply: `!setup class <name>`\n")
		return p.SendDM(ctx.Sender, b.String())
	}
	b.WriteString(fmt.Sprintf("Class: **%s** ✓\n", titleClass(c.Class)))
	if !statsAssigned(c) {
		b.WriteString("\n**Next: Step 3 — Ability Scores.**\n")
		b.WriteString("Assign these six values — **15 14 13 12 10 8** — to STR, DEX, CON, INT, WIS, CHA. Each value used exactly once.\n\n")
		b.WriteString("Reply: `!setup stats 15 14 13 12 10 8` (rearrange to taste).\n")
		return p.SendDM(ctx.Sender, b.String())
	}
	b.WriteString(fmt.Sprintf("Stats (pre-racial): STR %d  DEX %d  CON %d  INT %d  WIS %d  CHA %d ✓\n",
		c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA))
	b.WriteString("\n**Step 4 — Confirm.** Reply `!setup confirm` to finalize, or `!setup cancel` to scrap and start over.\n")
	b.WriteString(renderConfirmPreview(c))
	return p.SendDM(ctx.Sender, b.String())
}

func (p *AdventurePlugin) dndSetupRace(ctx MessageContext, raceArg string) error {
	r, ok := parseRace(raceArg)
	if !ok {
		return p.SendDM(ctx.Sender, "Unknown race. Options: human, elf, dwarf, halfling, orc, tiefling, half-elf")
	}
	c, err := loadOrInitDraft(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't open your draft: "+err.Error())
	}
	c.Race = r
	if err := SaveDnDCharacter(c); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save: "+err.Error())
	}
	ri, _ := raceInfo(r)
	return p.SendDM(ctx.Sender, fmt.Sprintf("Race set: **%s**. _%s_\n\nNext: `!setup class <name>` — see options with `!setup`.",
		ri.Display, ri.Passive))
}

func (p *AdventurePlugin) dndSetupClass(ctx MessageContext, classArg string) error {
	cl, ok := parseClass(classArg)
	if !ok {
		return p.SendDM(ctx.Sender, "Unknown class. Options: fighter, rogue, mage, cleric, ranger")
	}
	c, err := loadOrInitDraft(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't open your draft: "+err.Error())
	}
	c.Class = cl
	if err := SaveDnDCharacter(c); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save: "+err.Error())
	}
	ci, _ := classInfo(cl)
	return p.SendDM(ctx.Sender, fmt.Sprintf("Class set: **%s** — leans on %s & %s.\n\n"+
		"Next: assign your stats. The standard array is **15 14 13 12 10 8** — six numbers, each used once.\n"+
		"Order: STR DEX CON INT WIS CHA.\n\n"+
		"Example: `!setup stats 15 14 13 12 10 8` (all rolled into STR-first; rearrange to taste).",
		ci.Display, ci.PrimaryA, ci.PrimaryB))
}

func (p *AdventurePlugin) dndSetupStats(ctx MessageContext, statsArg string) error {
	scores, err := parseStatsArg(statsArg)
	if err != nil {
		return p.SendDM(ctx.Sender, err.Error()+
			"\n\nFormat: `!setup stats 15 14 13 12 10 8` (in STR DEX CON INT WIS CHA order)."+
			"\nCommas and parentheses are fine too — `!setup stats 15, 14, 13, 12, 10, 8` works.")
	}
	if !isStandardArray(scores) {
		return p.SendDM(ctx.Sender, "Stats must be a permutation of the standard array {15, 14, 13, 12, 10, 8} — each value used exactly once.\n\n"+
			"Try: `!setup stats 15 14 13 12 10 8` and rearrange to taste.")
	}
	c, err := loadOrInitDraft(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't open your draft: "+err.Error())
	}
	c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA = scores[0], scores[1], scores[2], scores[3], scores[4], scores[5]
	if err := SaveDnDCharacter(c); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save: "+err.Error())
	}
	var b strings.Builder
	b.WriteString("Stats saved (pre-racial bonuses).\n")
	b.WriteString(renderConfirmPreview(c))
	b.WriteString("\nReply `!setup confirm` to finalize.")
	return p.SendDM(ctx.Sender, b.String())
}

func (p *AdventurePlugin) dndSetupConfirm(ctx MessageContext) error {
	c, err := LoadDnDCharacter(ctx.Sender)
	if err != nil || c == nil {
		return p.SendDM(ctx.Sender, "No setup draft found. Run `!setup` to start.")
	}
	if !c.PendingSetup {
		return p.SendDM(ctx.Sender, "Already confirmed. Use `!sheet` to view your character.")
	}
	if c.Race == "" || c.Class == "" || !statsAssigned(c) {
		return p.SendDM(ctx.Sender, "Draft incomplete. Run `!setup` to see what's missing.")
	}

	// Apply racial modifiers to the assigned scores.
	final := applyRaceMods(c.Race, [6]int{c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA})
	c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA = final[0], final[1], final[2], final[3], final[4], final[5]

	// Initial D&D level seeded from existing combat_level (v1.1 §4.1).
	// combat_level "freezes" thereafter — dnd_level is canonical.
	//
	// ensureCharacter (not a bare loadAdvCharacter) so the canonical
	// player_meta seed row + tier-0 equipment exist for D&D-only players
	// who reach !setup confirm without ever touching the legacy adventure
	// path. Without this seed, loadAdvCharacter returns "sql: no rows in
	// result set" on every legacy-layer command (arena, npcs, events, …).
	advChar, _, _ := p.ensureCharacter(ctx.Sender)
	startLevel := 1
	if advChar != nil {
		startLevel = dndLevelFromCombatLevel(advChar.CombatLevel)
	}
	c.Level = startLevel

	conMod := abilityModifier(c.CON)
	dexMod := abilityModifier(c.DEX)
	c.HPMax = computeMaxHP(c.Class, conMod, c.Level)
	c.HPCurrent = c.HPMax
	c.TempHP = 0
	c.ArmorClass = computeAC(c.Class, dexMod)
	c.ShortRestCharges = c.Level
	c.PendingSetup = false
	c.AutoMigrated = false // manually confirmed — no longer an auto-migration
	c.UpdatedAt = time.Now().UTC()

	if err := SaveDnDCharacter(c); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save: "+err.Error())
	}
	_ = initResources(ctx.Sender, c.Class)
	// Phase 9: caster classes get a default known-spell list and slot pool.
	// Idempotent — !setup confirm after a respec wipe will repopulate.
	_ = ensureSpellsForCharacter(c)

	return p.SendDM(ctx.Sender, renderSetupComplete(c))
}

func (p *AdventurePlugin) dndSetupCancel(ctx MessageContext) error {
	c, err := LoadDnDCharacter(ctx.Sender)
	if err != nil || c == nil {
		return p.SendDM(ctx.Sender, "No draft to cancel.")
	}
	if !c.PendingSetup {
		return p.SendDM(ctx.Sender, "Your character is already finalized — use `!respec` instead.")
	}
	if _, err := dbExecCancel(ctx.Sender); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't cancel: "+err.Error())
	}
	return p.SendDM(ctx.Sender, "Draft scrapped. Run `!setup` to start over.")
}

// ── !respec ──────────────────────────────────────────────────────────────────

const (
	dndRespecCost     = 5000
	dndRespecCooldown = 7 * 24 * time.Hour
)

func (p *AdventurePlugin) handleDnDRespecCmd(ctx MessageContext) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	c, err := LoadDnDCharacter(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't load your character: "+err.Error())
	}
	if c == nil {
		return p.SendDM(ctx.Sender, "You don't have a character yet — run `!setup` instead.")
	}
	if c.PendingSetup {
		return p.SendDM(ctx.Sender, "You already have a setup draft in progress. Run `!setup` to continue or `!setup cancel` to scrap it.")
	}
	if c.AutoMigrated {
		return p.SendDM(ctx.Sender, "Your character was auto-built — `!setup` lets you rebuild for free, no respec needed.")
	}

	if c.LastRespecAt != nil {
		elapsed := time.Since(*c.LastRespecAt)
		if elapsed < dndRespecCooldown {
			remaining := dndRespecCooldown - elapsed
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"`!respec` is on cooldown — %s remaining.", formatRespecDuration(remaining)))
		}
	}

	// Pre-check balance so insufficient-funds players get the right error
	// without any state mutation.
	if p.euro == nil || p.euro.GetBalance(ctx.Sender) < float64(dndRespecCost) {
		return p.SendDM(ctx.Sender, fmt.Sprintf("`!respec` costs %d euros — you don't have enough.", dndRespecCost))
	}

	now := time.Now().UTC()
	c.Race = ""
	c.Class = ""
	c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA = 8, 8, 8, 8, 8, 8
	c.HPMax = 0
	c.HPCurrent = 0
	c.TempHP = 0
	c.ArmorClass = 10
	c.Level = 1
	c.XP = 0
	c.ArmedAbility = ""
	c.PendingSetup = true
	c.AutoMigrated = false
	c.LastRespecAt = &now
	// Phase 10: full wipe also resets subclass state. Subclass-respec
	// cooldown is dropped — at L1 with no class there's nothing to gate.
	c.Subclass = ""
	c.LastSubclassRespecAt = nil
	c.Exhaustion = 0
	// Save the wipe BEFORE debiting euros (audit fix B). If save fails, the
	// player's old state survives and they keep their euros — better than
	// a destructive debit-without-wipe.
	if err := SaveDnDCharacter(c); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't save respec state: "+err.Error())
	}
	// Wipe old class's resource pool so respec'd Mages don't carry Fighter
	// stamina rows etc. (audit fix E).
	if _, err := db.Get().Exec(
		`DELETE FROM dnd_resources WHERE user_id = ?`, string(ctx.Sender),
	); err != nil {
		slog.Error("dnd: respec resource wipe", "user", ctx.Sender, "err", err)
	}
	// Phase 9: also wipe known spells + slot pool so respec from caster →
	// non-caster (or caster → caster of another class) starts clean.
	if err := wipeSpellsForUser(ctx.Sender); err != nil {
		slog.Error("dnd: respec spell wipe", "user", ctx.Sender, "err", err)
	}
	// Phase R hardening: a respec also abandons any active zone-run /
	// expedition keyed to the wiped character so they don't orphan with
	// pointers to a stat sheet that no longer exists.
	if err := abandonZoneRun(ctx.Sender); err != nil && err != ErrNoActiveRun {
		slog.Warn("dnd: respec zone-run cleanup", "user", ctx.Sender, "err", err)
	}
	if err := abandonExpedition(ctx.Sender); err != nil && err != ErrNoActiveExpedition {
		slog.Warn("dnd: respec expedition cleanup", "user", ctx.Sender, "err", err)
	}
	// Debit last. If this fails (rare race — euros spent elsewhere between
	// the pre-check and now), the player got a free respec. Strictly better
	// than the alternative of euros-lost-with-wipe-not-applied.
	if !p.euro.Debit(ctx.Sender, float64(dndRespecCost), "dnd respec") {
		slog.Warn("dnd: respec wipe completed but debit failed (race vs. balance change)",
			"user", ctx.Sender)
	}

	return p.SendDM(ctx.Sender, fmt.Sprintf(
		"💸 %d euros spent. Your character is wiped. Run `!setup` to build a new one.\n\n"+
			"Cooldown: %s before next respec.",
		dndRespecCost, formatRespecDuration(dndRespecCooldown)))
}

func formatRespecDuration(d time.Duration) string {
	hours := int(d.Hours())
	if hours >= 24 {
		days := hours / 24
		hours = hours % 24
		if hours == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	mins := int(d.Minutes())
	return fmt.Sprintf("%dm", mins)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func loadOrInitDraft(userID id.UserID) (*DnDCharacter, error) {
	c, err := LoadDnDCharacter(userID)
	if err != nil {
		return nil, err
	}
	if c != nil {
		// Auto-migrated chars can be freely rebuilt — wipe and start fresh.
		// If the player cancels, their next combat will auto-migrate again.
		if c.AutoMigrated && !c.PendingSetup {
			if _, err := db.Get().Exec(
				`DELETE FROM dnd_character WHERE user_id = ?`, string(userID),
			); err != nil {
				return nil, fmt.Errorf("clear auto-migrated row: %w", err)
			}
			// Auto-migrated wipe: also abandon active zone-run /
			// expedition so they don't orphan against the deleted row.
			if err := abandonZoneRun(userID); err != nil && err != ErrNoActiveRun {
				slog.Warn("dnd: auto-migrated wipe zone-run cleanup", "user", userID, "err", err)
			}
			if err := abandonExpedition(userID); err != nil && err != ErrNoActiveExpedition {
				slog.Warn("dnd: auto-migrated wipe expedition cleanup", "user", userID, "err", err)
			}
			// fall through to fresh draft
		} else if !c.PendingSetup {
			return nil, fmt.Errorf("character already finalized — use !respec")
		} else {
			return c, nil
		}
	}
	// Fresh draft.
	now := time.Now().UTC()
	return &DnDCharacter{
		UserID:       userID,
		Level:        1,
		PendingSetup: true,
		ArmorClass:   10,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func dbExecCancel(userID id.UserID) (int64, error) {
	res, err := db.Get().Exec(`DELETE FROM dnd_character WHERE user_id = ? AND pending_setup = 1`, string(userID))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// parseStatsArg accepts the six standard-array values in any of several
// natural forms. All of these work:
//
//	15 14 13 12 10 8
//	15, 14, 13, 12, 10, 8
//	(15, 14, 13, 12, 10, 8)
//	{15 14 13 12 10 8}
//	[15,14,13,12,10,8]
//
// Returns the scores in input order. Validation that they're a permutation
// of the standard array happens in the caller via isStandardArray.
func parseStatsArg(s string) ([6]int, error) {
	var out [6]int
	// Normalize separators: turn commas and bracket-pair characters into
	// spaces, then split on whitespace.
	cleaned := s
	for _, ch := range []string{",", "(", ")", "[", "]", "{", "}"} {
		cleaned = strings.ReplaceAll(cleaned, ch, " ")
	}
	parts := strings.Fields(cleaned)
	if len(parts) != 6 {
		return out, fmt.Errorf("Need exactly 6 numbers, got %d.", len(parts))
	}
	for i, tok := range parts {
		n, err := strconv.Atoi(tok)
		if err != nil {
			return out, fmt.Errorf("Stat %d isn't a number: %q.", i+1, tok)
		}
		out[i] = n
	}
	return out, nil
}

func parseRace(s string) (DnDRace, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "_")
	for _, ri := range dndRaces {
		if string(ri.Key) == s || strings.EqualFold(ri.Display, s) {
			return ri.Key, true
		}
	}
	return "", false
}

func parseClass(s string) (DnDClass, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, ci := range dndClasses {
		if string(ci.Key) == s || strings.EqualFold(ci.Display, s) {
			if !ci.Playable {
				// Recognized class, but not yet selectable (Open5e
				// caster scaffold — no spell list). Treat as no match.
				return "", false
			}
			return ci.Key, true
		}
	}
	return "", false
}

func isStandardArray(scores [6]int) bool {
	a := scores
	sort.Sort(sort.Reverse(sort.IntSlice(a[:])))
	want := standardArray
	return a == want
}

func statsAssigned(c *DnDCharacter) bool {
	// All-8 means default-init; treat as not yet assigned.
	if c.STR == 8 && c.DEX == 8 && c.CON == 8 && c.INT == 8 && c.WIS == 8 && c.CHA == 8 {
		return false
	}
	return true
}

func titleRace(r DnDRace) string {
	if ri, ok := raceInfo(r); ok {
		return ri.Display
	}
	return string(r)
}

func titleClass(c DnDClass) string {
	if ci, ok := classInfo(c); ok {
		return ci.Display
	}
	return string(c)
}

func renderRaceMenu() string {
	var b strings.Builder
	for _, ri := range dndRaces {
		b.WriteString(fmt.Sprintf("  • **%s** — %s\n", ri.Display, ri.Passive))
		if ri.BestFit != "" {
			b.WriteString(fmt.Sprintf("    _best with: %s_\n", ri.BestFit))
		}
	}
	return b.String()
}

func renderClassMenu() string {
	var b strings.Builder
	for _, ci := range dndClasses {
		if !ci.Playable {
			continue // Open5e caster scaffold — hidden until spell lists land.
		}
		b.WriteString(fmt.Sprintf("  • **%s** — %s & %s\n", ci.Display, ci.PrimaryA, ci.PrimaryB))
	}
	return b.String()
}

func renderConfirmPreview(c *DnDCharacter) string {
	final := applyRaceMods(c.Race, [6]int{c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA})
	mods := [6]int{
		abilityModifier(final[0]), abilityModifier(final[1]), abilityModifier(final[2]),
		abilityModifier(final[3]), abilityModifier(final[4]), abilityModifier(final[5]),
	}
	conMod := mods[2]
	dexMod := mods[1]
	advChar, _ := loadAdvCharacter(c.UserID)
	lvl := 1
	if advChar != nil {
		lvl = dndLevelFromCombatLevel(advChar.CombatLevel)
	}
	hp := computeMaxHP(c.Class, conMod, lvl)
	ac := computeAC(c.Class, dexMod)
	return fmt.Sprintf(
		"\n**Preview** (post-racial):\n"+
			"  STR %d (%+d)  DEX %d (%+d)  CON %d (%+d)\n"+
			"  INT %d (%+d)  WIS %d (%+d)  CHA %d (%+d)\n"+
			"  HP %d  AC %d  Level %d\n",
		final[0], mods[0], final[1], mods[1], final[2], mods[2],
		final[3], mods[3], final[4], mods[4], final[5], mods[5],
		hp, ac, lvl,
	)
}

func renderSetupComplete(c *DnDCharacter) string {
	ri, _ := raceInfo(c.Race)
	ci, _ := classInfo(c.Class)
	nextLine := "Use `!sheet` anytime to review. `!zone list` to head out, or `!expedition list` for a longer run."
	if isSpellcaster(c) {
		// Casters get auto-granted spells on character creation but no
		// in-game discovery prompt for them. Surface !spells / !cast so
		// they can actually find their kit. Long-rest refresh is the
		// other piece they need to know, but that becomes obvious the
		// moment they look at !spells.
		nextLine = "Use `!sheet` anytime to review. `!spells` lists what you can cast and `!cast <spell>` does the deed. `!zone list` heads out; `!expedition list` is the longer run."
	}
	return fmt.Sprintf(
		"⚔️ **Character Sheet Forged**\n\n"+
			"You are a **Level %d %s %s**.\n"+
			"  HP %d/%d   AC %d\n"+
			"  STR %d  DEX %d  CON %d  INT %d  WIS %d  CHA %d\n\n"+
			"_%s_\n\n"+
			"%s",
		c.Level, ri.Display, ci.Display,
		c.HPCurrent, c.HPMax, c.ArmorClass,
		c.STR, c.DEX, c.CON, c.INT, c.WIS, c.CHA,
		ri.Passive,
		nextLine,
	)
}
