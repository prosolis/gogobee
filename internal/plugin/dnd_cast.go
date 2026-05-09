package plugin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

// Phase 9 SP2 — !cast / !spells / !prepare commands and out-of-combat
// resolution. Combat-time resolution of pending_cast lives in dnd_combat.go
// (SP3).
//
// Casting model:
//   - Cantrips: no slot cost. Damage cantrips queue as pending_cast for
//     next combat. Utility cantrips (Mending, Message, Minor Illusion,
//     Guidance) resolve immediately with a flavorful response.
//   - Leveled spells: consume a slot at cast time (audit-style save-then-
//     debit). DAMAGE_*, CONTROL, BUFF_SELF/ALLY queue as pending_cast.
//     HEAL and UTILITY resolve immediately.
//   - Reaction spells (Shield, Counterspell, Absorb Elements) refuse with
//     a "Phase 11" note — they require the boss turn-based engine.
//
// Cross-player targeting (--target @user) is deliberately deferred. SP2
// supports self-target only. Heals on self work; ally buffs queue with the
// caster as the target until SP3 wires up the multi-player resolution.

// PendingCast is the shape stored in dnd_character.pending_cast (JSON blob).
// Keep this minimal — the combat layer resolves the actual numbers on fire.
type PendingCast struct {
	SpellID   string `json:"spell"`
	SlotLevel int    `json:"slot"` // 0 for cantrips
	Target    string `json:"target,omitempty"`
}

func encodePendingCast(p PendingCast) string {
	b, _ := json.Marshal(p)
	return string(b)
}

func decodePendingCast(s string) (PendingCast, bool) {
	if s == "" {
		return PendingCast{}, false
	}
	var p PendingCast
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return PendingCast{}, false
	}
	if p.SpellID == "" {
		return PendingCast{}, false
	}
	return p, true
}

// ── !cast command ────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleDnDCastCmd(ctx MessageContext, args string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	args = strings.TrimSpace(args)
	advChar, _ := loadAdvCharacter(ctx.Sender)
	c, err := p.ensureCharForDnDCmd(ctx.Sender, advChar)
	if err != nil || c == nil {
		return p.SendDM(ctx.Sender, "Couldn't load your Adv 2.0 sheet.")
	}
	if !isSpellcaster(c) {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"%s isn't a caster class. `!cast` is for Mage, Cleric, Ranger, and Arcane Trickster Rogues (L5+).",
			titleClass(c.Class)))
	}

	if args == "" {
		return p.SendDM(ctx.Sender, renderCastHelp(c))
	}

	// Parse: --drop is a special form ("!cast --drop" or "!cast drop").
	lower := strings.ToLower(args)
	if lower == "--drop" || lower == "drop" {
		return p.dndCastDrop(ctx, c)
	}

	// Parse spell name + optional --upcast N.
	tokens := strings.Fields(args)
	upcast := 0
	var spellTokens []string
	for i := 0; i < len(tokens); i++ {
		switch tokens[i] {
		case "--upcast":
			if i+1 < len(tokens) {
				n, err := strconv.Atoi(tokens[i+1])
				if err == nil {
					upcast = n
				}
				i++
			}
		case "--target":
			// Reserved for SP3 — accept and ignore for now.
			if i+1 < len(tokens) {
				i++
			}
		default:
			spellTokens = append(spellTokens, tokens[i])
		}
	}
	if len(spellTokens) == 0 {
		return p.SendDM(ctx.Sender, "Usage: `!cast <spell> [--upcast N]`. Run `!spells` to list options.")
	}
	spell, ok := parseSpell(strings.Join(spellTokens, " "))
	if !ok {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"Unknown spell %q. Run `!spells` to list what you know.", strings.Join(spellTokens, " ")))
	}

	// Class gate. Arcane Trickster Rogues use the Mage spell list (Phase 10
	// SUB2-AT), so accept Mage-tagged spells when the player has that subclass.
	effectiveClass := c.Class
	if c.Class == ClassRogue && c.Subclass == SubclassArcaneTrickster {
		effectiveClass = ClassMage
	}
	classOK := false
	for _, cl := range spell.Classes {
		if cl == effectiveClass {
			classOK = true
			break
		}
	}
	if !classOK {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"%s is not on the %s spell list.", spell.Name, titleClass(c.Class)))
	}
	// AT slot ceiling — capped at L1 until L7 (then L2), tracks third-caster
	// progression. Refuse upcasting beyond what the subclass actually has.
	if c.Class == ClassRogue && c.Subclass == SubclassArcaneTrickster {
		if max := highestAvailableSlotForChar(c); spell.Level > max {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"Arcane Trickster L%d only has up to L%d slots.", c.Level, max))
		}
	}

	// Reaction spells deferred — combat is one-shot SimulateCombat, no
	// per-turn windows where a reaction could trigger. Will land alongside
	// turn-based combat when that arrives.
	if spell.Effect == EffectReaction {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"%s is a reaction spell — those aren't usable yet (combat resolves in one beat, no reaction window). Pick a different spell.", spell.Name))
	}

	// Known + prepared check.
	known, prepared, err := playerKnowsSpell(ctx.Sender, spell.ID)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't check your spell list.")
	}
	if !known {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You don't know %s yet.", spell.Name))
	}
	if !prepared {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"%s isn't prepared today. Run `!prepare %s` first.", spell.Name, spell.ID))
	}

	// Determine slot level.
	slotLevel := spell.Level
	if upcast > slotLevel && spell.Level > 0 {
		slotLevel = upcast
	}
	if slotLevel < 0 || slotLevel > 5 {
		return p.SendDM(ctx.Sender, "Slot level out of range (1–5).")
	}

	// Concentration conflict: if spell needs concentration and player has
	// another active, warn (we'll supersede on the actual cast).
	supersedes := ""
	if spell.Concentration {
		if active := concentrationActive(c); active != "" && active != spell.ID {
			supersedes = active
		}
	}

	// Material cost (Revivify, Raise Dead).
	if spell.MaterialCost > 0 {
		if p.euro == nil || !p.euro.Debit(ctx.Sender, float64(spell.MaterialCost), "dnd_spell_component") {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"%s needs a %d-coin diamond component you can't afford.", spell.Name, spell.MaterialCost))
		}
	}

	// Slot consumption (cantrips skip).
	if spell.Level > 0 {
		ok, err := consumeSpellSlot(ctx.Sender, slotLevel)
		if err != nil {
			return p.SendDM(ctx.Sender, "Couldn't consume slot: "+err.Error())
		}
		if !ok {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"No L%d slot available. %s", slotLevel, renderSlotsBrief(ctx.Sender)))
		}
	}

	// Resolve. Audit pattern: for queueing effects, save state BEFORE we
	// would otherwise consider the action committed. The slot is already
	// debited above; if SaveDnDCharacter fails we refund.
	switch spell.Effect {
	case EffectSpellHeal:
		return p.resolveHealOutOfCombat(ctx, c, spell, slotLevel)
	case EffectUtility:
		return p.resolveUtility(ctx, c, spell, slotLevel)
	default:
		// DAMAGE_*, CONTROL, BUFF_SELF/ALLY → queue for next combat.
		return p.queuePendingCast(ctx, c, spell, slotLevel, supersedes)
	}
}

// ── Out-of-combat: HEAL ──────────────────────────────────────────────────────

func (p *AdventurePlugin) resolveHealOutOfCombat(ctx MessageContext, c *DnDCharacter, spell SpellDefinition, slotLevel int) error {
	dice, faces, _ := parseDamageDice(spell.DamageDice)
	if dice == 0 {
		dice, faces = 1, 8 // safety fallback
	}
	// Upcast scales healing. Most heal spells are "+1d8 per slot above 1st".
	extra := slotLevel - spell.Level
	if extra < 0 {
		extra = 0
	}
	totalDice := dice + extra
	heal := 0
	supreme := lifeDomainSupremeHealing(c)
	for i := 0; i < totalDice; i++ {
		if supreme {
			heal += faces
		} else {
			heal += 1 + rand.IntN(faces)
		}
	}
	heal += abilityModifier(c.WIS)
	heal += lifeDomainHealBonus(c, spell, slotLevel)

	before := c.HPCurrent
	c.HPCurrent = min(c.HPMax, c.HPCurrent+heal)
	delta := c.HPCurrent - before

	if err := SaveDnDCharacter(c); err != nil {
		// Refund the slot.
		if spell.Level > 0 {
			_ = refundSpellSlot(ctx.Sender, slotLevel)
		}
		return p.SendDM(ctx.Sender, "Couldn't apply heal: "+err.Error())
	}

	return p.SendDM(ctx.Sender, fmt.Sprintf(
		"🩹 **%s** — restored %d HP (%d → %d / %d). %s",
		spell.Name, delta, before, c.HPCurrent, c.HPMax,
		renderSlotsBrief(ctx.Sender)))
}

// ── Out-of-combat: UTILITY ──────────────────────────────────────────────────

func (p *AdventurePlugin) resolveUtility(ctx MessageContext, c *DnDCharacter, spell SpellDefinition, slotLevel int) error {
	// Most utility spells are pure narrative — they produce a flavor line
	// and a confirmation. A few (Mage Armor, Hunter's Mark, etc.) are
	// concentration buffs that fire as pending_cast in combat. Those are
	// classified as BUFF_* in the registry, not UTILITY, so we don't see
	// them here. Concentration UTILITY spells (Detect Magic, Beast Sense)
	// merely set the concentration flag; combat doesn't care.
	supersedes := ""
	if spell.Concentration {
		supersedes = setConcentration(c, spell.ID, utilityConcentrationDuration(spell))
	}

	if err := SaveDnDCharacter(c); err != nil {
		if spell.Level > 0 {
			_ = refundSpellSlot(ctx.Sender, slotLevel)
		}
		return p.SendDM(ctx.Sender, "Couldn't save state: "+err.Error())
	}

	msg := fmt.Sprintf("✨ **%s** — %s", spell.Name, spell.Description)
	if supersedes != "" {
		msg += fmt.Sprintf("\n_(Concentration shifts: %s ended.)_", displaySpellName(supersedes))
	}
	if spell.Level > 0 {
		msg += "\n" + renderSlotsBrief(ctx.Sender)
	}
	return p.SendDM(ctx.Sender, msg)
}

func utilityConcentrationDuration(spell SpellDefinition) time.Duration {
	// Phase 9 keeps durations narrative — anything that lasts past a long
	// rest is reset by long rest anyway. Use a generous default.
	switch spell.ID {
	case "detect_magic", "beast_sense":
		return 10 * time.Minute
	}
	return 1 * time.Hour
}

// ── Queue for next combat ───────────────────────────────────────────────────

func (p *AdventurePlugin) queuePendingCast(ctx MessageContext, c *DnDCharacter, spell SpellDefinition, slotLevel int, supersedes string) error {
	if c.PendingCast != "" {
		// Refund the slot we just consumed.
		if spell.Level > 0 {
			_ = refundSpellSlot(ctx.Sender, slotLevel)
		}
		prev, _ := decodePendingCast(c.PendingCast)
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"You already have **%s** queued. Cast `!cast --drop` to clear it first.",
			displaySpellName(prev.SpellID)))
	}

	pc := PendingCast{SpellID: spell.ID, SlotLevel: slotLevel}
	c.PendingCast = encodePendingCast(pc)
	if spell.Concentration {
		setConcentration(c, spell.ID, 1*time.Hour)
	}

	// Audit pattern (Fix C): save-then-the-rest. Slot already debited; on
	// save failure, refund slot and concentration.
	if err := SaveDnDCharacter(c); err != nil {
		if spell.Level > 0 {
			_ = refundSpellSlot(ctx.Sender, slotLevel)
		}
		c.ConcentrationSpell = ""
		c.ConcentrationExpiresAt = nil
		return p.SendDM(ctx.Sender, "Couldn't queue spell: "+err.Error())
	}

	msg := fmt.Sprintf("✨ **%s** queued for next combat.", spell.Name)
	if slotLevel > spell.Level {
		msg += fmt.Sprintf(" _(upcast to L%d)_", slotLevel)
	}
	if supersedes != "" {
		msg += fmt.Sprintf("\n_(Concentration shifts: %s ended.)_", displaySpellName(supersedes))
	}
	if spell.Level > 0 {
		msg += "\n" + renderSlotsBrief(ctx.Sender)
	}
	return p.SendDM(ctx.Sender, msg)
}

// ── !cast --drop ─────────────────────────────────────────────────────────────

func (p *AdventurePlugin) dndCastDrop(ctx MessageContext, c *DnDCharacter) error {
	if c.PendingCast == "" && c.ConcentrationSpell == "" {
		return p.SendDM(ctx.Sender, "Nothing queued and no concentration active.")
	}

	// Refund the queued slot (if any) — voluntary drop is the player's
	// choice; restore the slot rather than forfeit it.
	if pc, ok := decodePendingCast(c.PendingCast); ok && pc.SlotLevel > 0 {
		_ = refundSpellSlot(ctx.Sender, pc.SlotLevel)
	}

	dropped := []string{}
	if pc, ok := decodePendingCast(c.PendingCast); ok {
		dropped = append(dropped, displaySpellName(pc.SpellID)+" (queued)")
	}
	if c.ConcentrationSpell != "" {
		dropped = append(dropped, displaySpellName(c.ConcentrationSpell)+" (concentration)")
	}

	c.PendingCast = ""
	c.ConcentrationSpell = ""
	c.ConcentrationExpiresAt = nil
	if err := SaveDnDCharacter(c); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't drop: "+err.Error())
	}
	return p.SendDM(ctx.Sender, "Dropped: "+strings.Join(dropped, ", ")+".")
}

// ── !spells command ─────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleDnDSpellsCmd(ctx MessageContext, args string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	advChar, _ := loadAdvCharacter(ctx.Sender)
	c, err := p.ensureCharForDnDCmd(ctx.Sender, advChar)
	if err != nil || c == nil {
		return p.SendDM(ctx.Sender, "Couldn't load your Adv 2.0 sheet.")
	}
	if !isSpellcaster(c) {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"%s isn't a caster class.", titleClass(c.Class)))
	}

	args = strings.TrimSpace(args)
	if strings.HasPrefix(strings.ToLower(args), "learn ") {
		return p.handleSpellsLearn(ctx, c, strings.TrimSpace(args[6:]))
	}

	return p.SendDM(ctx.Sender, renderSpellsList(c))
}

func renderSpellsList(c *DnDCharacter) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**Spellbook — %s L%d**\n\n",
		titleClass(c.Class), c.Level))

	known, _ := listKnownSpells(c.UserID)
	if len(known) == 0 {
		b.WriteString("_No spells known. Run `!spells learn <name>` to add one._\n")
	} else {
		// Group by spell level.
		byLevel := map[int][]knownSpellRow{}
		for _, k := range known {
			s, ok := lookupSpell(k.SpellID)
			if !ok {
				continue
			}
			byLevel[s.Level] = append(byLevel[s.Level], k)
		}
		levels := []int{}
		for lvl := range byLevel {
			levels = append(levels, lvl)
		}
		sort.Ints(levels)
		for _, lvl := range levels {
			label := fmt.Sprintf("L%d", lvl)
			if lvl == 0 {
				label = "Cantrip"
			}
			b.WriteString(fmt.Sprintf("__%s__\n", label))
			for _, k := range byLevel[lvl] {
				s, _ := lookupSpell(k.SpellID)
				prepStr := ""
				if c.Class == ClassCleric && s.Level > 0 {
					if k.Prepared {
						prepStr = " ✅"
					} else {
						prepStr = " ⬜"
					}
				}
				b.WriteString(fmt.Sprintf("  • **%s**%s — %s\n", s.Name, prepStr, s.Description))
			}
		}
	}

	slots, _ := getSpellSlots(c.UserID)
	b.WriteString("\n**Slots:** " + renderSlotLine(slots) + "\n")
	b.WriteString(fmt.Sprintf("**Spell DC:** %d   **Spell Atk:** +%d\n",
		spellSaveDC(c), spellAttackBonus(c)))

	if c.Class == ClassMage {
		count, _ := mageLeveledKnownCount(c.UserID)
		cap := mageKnownSpellsCap(c.Level)
		b.WriteString(fmt.Sprintf("**Spellbook:** %d/%d leveled spells", count, cap))
		if avail := cap - count; avail > 0 {
			b.WriteString(fmt.Sprintf(" _(can learn %d more)_", avail))
		}
		b.WriteString("\n")
	}

	if pc, ok := decodePendingCast(c.PendingCast); ok {
		b.WriteString(fmt.Sprintf("\n_Queued: **%s**", displaySpellName(pc.SpellID)))
		if pc.SlotLevel > 0 {
			b.WriteString(fmt.Sprintf(" (L%d slot)", pc.SlotLevel))
		}
		b.WriteString(" — fires next combat._\n")
	}
	if c.ConcentrationSpell != "" && concentrationActive(c) != "" {
		b.WriteString(fmt.Sprintf("_Concentrating on **%s**._\n",
			displaySpellName(c.ConcentrationSpell)))
	}
	b.WriteString("\nSlots refresh on long rest.")
	return b.String()
}

// ── !spells learn (Mage spellbook) ───────────────────────────────────────────

func (p *AdventurePlugin) handleSpellsLearn(ctx MessageContext, c *DnDCharacter, raw string) error {
	if c.Class != ClassMage {
		return p.SendDM(ctx.Sender, "`!spells learn` is for Mage. Cleric uses `!prepare`; Ranger spells are auto-known.")
	}
	if raw == "" {
		return p.SendDM(ctx.Sender, "Usage: `!spells learn <spell name>`")
	}
	spell, ok := parseSpell(raw)
	if !ok {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Unknown spell %q.", raw))
	}
	classOK := false
	for _, cl := range spell.Classes {
		if cl == ClassMage {
			classOK = true
		}
	}
	if !classOK {
		return p.SendDM(ctx.Sender, fmt.Sprintf("%s isn't on the Mage spell list.", spell.Name))
	}
	if spell.Level > highestAvailableSlot(ClassMage, c.Level) && spell.Level > 0 {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"L%d spells require Mage level %d+.", spell.Level, requiredMageLevelFor(spell.Level)))
	}
	known, _, err := playerKnowsSpell(ctx.Sender, spell.ID)
	if err == nil && known {
		return p.SendDM(ctx.Sender, "You already know "+spell.Name+".")
	}
	// Cap applies to leveled spells only — cantrips are unbounded.
	if spell.Level > 0 {
		count, err := mageLeveledKnownCount(ctx.Sender)
		if err != nil {
			return p.SendDM(ctx.Sender, "Couldn't check your spellbook.")
		}
		cap := mageKnownSpellsCap(c.Level)
		if count >= cap {
			return p.SendDM(ctx.Sender, fmt.Sprintf(
				"Spellbook is full (%d/%d). Level up to learn more leveled spells.",
				count, cap))
		}
	}
	if err := addKnownSpell(ctx.Sender, spell.ID, "class", true); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't learn: "+err.Error())
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("📖 Learned **%s**.", spell.Name))
}

func requiredMageLevelFor(slotLevel int) int {
	switch slotLevel {
	case 1:
		return 1
	case 2:
		return 3
	case 3:
		return 5
	case 4:
		return 7
	case 5:
		return 9
	}
	return 1
}

// ── !prepare command (Cleric stub — full SP4) ───────────────────────────────

func (p *AdventurePlugin) handleDnDPrepareCmd(ctx MessageContext, args string) error {
	userMu := p.advUserLock(ctx.Sender)
	userMu.Lock()
	defer userMu.Unlock()

	advChar, _ := loadAdvCharacter(ctx.Sender)
	c, err := p.ensureCharForDnDCmd(ctx.Sender, advChar)
	if err != nil || c == nil {
		return p.SendDM(ctx.Sender, "Couldn't load your Adv 2.0 sheet.")
	}
	if c.Class != ClassCleric {
		return p.SendDM(ctx.Sender, "`!prepare` is for Cleric. Mage uses `!spells learn`; Ranger spells are auto-known.")
	}
	args = strings.TrimSpace(args)
	if args == "" {
		return p.SendDM(ctx.Sender, renderClericPrepStatus(c))
	}

	clear := false
	if strings.HasPrefix(strings.ToLower(args), "clear ") {
		clear = true
		args = strings.TrimSpace(args[6:])
	}

	spell, ok := parseSpell(args)
	if !ok {
		return p.SendDM(ctx.Sender, fmt.Sprintf("Unknown spell %q.", args))
	}
	if spell.Level == 0 {
		return p.SendDM(ctx.Sender, "Cantrips are always prepared.")
	}
	known, _, err := playerKnowsSpell(ctx.Sender, spell.ID)
	if err != nil || !known {
		return p.SendDM(ctx.Sender, fmt.Sprintf("%s isn't on your known list.", spell.Name))
	}

	if clear {
		if err := setSpellPrepared(ctx.Sender, spell.ID, false); err != nil {
			return p.SendDM(ctx.Sender, "Couldn't unprepare: "+err.Error())
		}
		return p.SendDM(ctx.Sender, fmt.Sprintf("Unprepared **%s**.", spell.Name))
	}

	// Cap = WIS mod + Cleric level. Cantrips don't count.
	cap := abilityModifier(c.WIS) + c.Level
	if cap < 1 {
		cap = 1
	}
	rows, _ := listKnownSpells(ctx.Sender)
	prepCount := 0
	for _, r := range rows {
		s, ok := lookupSpell(r.SpellID)
		if !ok || s.Level == 0 {
			continue
		}
		if r.Prepared && r.SpellID != spell.ID {
			prepCount++
		}
	}
	if prepCount+1 > cap {
		return p.SendDM(ctx.Sender, fmt.Sprintf(
			"Prep cap is %d. Unprepare a spell first: `!prepare clear <name>`.", cap))
	}
	if err := setSpellPrepared(ctx.Sender, spell.ID, true); err != nil {
		return p.SendDM(ctx.Sender, "Couldn't prepare: "+err.Error())
	}
	return p.SendDM(ctx.Sender, fmt.Sprintf("📖 Prepared **%s** (%d/%d).",
		spell.Name, prepCount+1, cap))
}

func renderClericPrepStatus(c *DnDCharacter) string {
	cap := abilityModifier(c.WIS) + c.Level
	if cap < 1 {
		cap = 1
	}
	rows, _ := listKnownSpells(c.UserID)
	prepCount := 0
	for _, r := range rows {
		s, ok := lookupSpell(r.SpellID)
		if !ok || s.Level == 0 {
			continue
		}
		if r.Prepared {
			prepCount++
		}
	}
	return fmt.Sprintf("**Prepared spells:** %d/%d. Run `!prepare <spell>` to add, `!prepare clear <spell>` to remove. Long rest re-opens choices.",
		prepCount, cap)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func renderCastHelp(c *DnDCharacter) string {
	var b strings.Builder
	b.WriteString("**Cast a spell**\n\n")
	b.WriteString("Usage: `!cast <spell> [--upcast N]`\n")
	b.WriteString("       `!cast --drop` to clear queued/concentration.\n\n")
	b.WriteString("Run `!spells` for your full list.\n\n")
	if pc, ok := decodePendingCast(c.PendingCast); ok {
		b.WriteString(fmt.Sprintf("_Currently queued: **%s**._\n", displaySpellName(pc.SpellID)))
	}
	if c.ConcentrationSpell != "" && concentrationActive(c) != "" {
		b.WriteString(fmt.Sprintf("_Concentrating on **%s**._\n",
			displaySpellName(c.ConcentrationSpell)))
	}
	b.WriteString("\n" + renderSlotsBrief(c.UserID))
	return b.String()
}

func renderSlotsBrief(userID id.UserID) string {
	slots, err := getSpellSlots(userID)
	if err != nil {
		slog.Warn("dnd: getSpellSlots", "user", userID, "err", err)
	}
	return "**Slots:** " + renderSlotLine(slots)
}

// parseDamageDice extracts (count, faces, flatBonus) from "3d6", "1d8",
// "3d4+3" style strings. Returns (0,0,0) on failure.
func parseDamageDice(s string) (int, int, int) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, 0, 0
	}
	// Optional flat bonus: split on +.
	flat := 0
	if i := strings.Index(s, "+"); i > 0 {
		f, err := strconv.Atoi(strings.TrimSpace(s[i+1:]))
		if err == nil {
			flat = f
		}
		s = strings.TrimSpace(s[:i])
	}
	i := strings.Index(s, "d")
	if i <= 0 {
		return 0, 0, flat
	}
	count, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, 0, flat
	}
	faces, err := strconv.Atoi(s[i+1:])
	if err != nil {
		return 0, 0, flat
	}
	return count, faces, flat
}
