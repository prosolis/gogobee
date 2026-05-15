package plugin

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase 9 — spell system.
//
// Implements the registry side of gogobee_spell_system.md: 76 in-scope spells
// (3 reaction spells deferred to Phase 11), three casters (Mage/Cleric/Ranger),
// spell slots, spell save DC, spell attack bonus, and the migration helper
// that auto-grants a sensible known list to existing players.
//
// SP2/SP3 (in dnd_spells_cast.go and dnd_combat.go) handle !cast and the
// pending-cast resolution at combat time.

// ── Effect categories ────────────────────────────────────────────────────────

type SpellEffectKind string

const (
	// In-combat / pre-combat (queued via pending_cast)
	EffectDamageAttack SpellEffectKind = "damage_attack" // spell attack roll vs AC
	EffectDamageSave   SpellEffectKind = "damage_save"   // target saves; half on success
	EffectDamageAuto   SpellEffectKind = "damage_auto"   // no save, no attack (Magic Missile)
	EffectControl      SpellEffectKind = "control"       // save vs DC; failure → condition
	EffectBuffSelf     SpellEffectKind = "buff_self"     // AC/attack/HP self-buff for next fight
	EffectBuffAlly     SpellEffectKind = "buff_ally"     // same, on a target player
	// Out-of-combat / immediate-resolution
	EffectSpellHeal SpellEffectKind = "spell_heal"
	EffectUtility   SpellEffectKind = "utility"
	// Deferred to Phase 11 (turn-based bosses)
	EffectReaction SpellEffectKind = "reaction"
)

// ── Spell timing & damage type ───────────────────────────────────────────────

type SpellCastTime string

const (
	CastAction      SpellCastTime = "action"
	CastBonusAction SpellCastTime = "bonus_action"
	CastReaction    SpellCastTime = "reaction"
	CastRitual      SpellCastTime = "ritual" // 10 min, no slot
)

// SpellDefinition is the static description of a spell. Not stored per-player;
// known spells are tracked in dnd_known_spells, slot pool in dnd_spell_slots,
// and the queued cast lives in dnd_character.pending_cast as a JSON blob.
type SpellDefinition struct {
	ID            string
	Name          string
	Level         int // 0 = cantrip
	School        string
	Classes       []DnDClass
	Effect        SpellEffectKind
	CastTime      SpellCastTime
	Concentration bool
	// SaveStat — empty for attack-roll or auto-effect spells.
	SaveStat string // "STR" | "DEX" | "CON" | "INT" | "WIS" | "CHA"
	// AttackRoll — true for spell attack rolls (Fire Bolt, Inflict Wounds).
	AttackRoll bool
	// DamageDice — descriptive only ("3d6", "1d10"). Roll bounds tested in
	// dnd_spells_test.go via simple regex on this field.
	DamageDice  string
	DamageType  string
	Description string
	// Upcast describes scaling at higher slots ("+1d6 per slot above 3rd").
	Upcast string
	// MaterialCost is in coins; non-zero spells (Revivify, Raise Dead) debit
	// at cast time and refuse if balance is short.
	MaterialCost int
	// AOE — true when the spell hits all enemies in the encounter.
	AOE bool
}

// ── Registry ─────────────────────────────────────────────────────────────────

// dndSpellRegistry merges the vendored Open5e SRD dump with the hand-authored
// spell list. SRD loads first as the broad baseline; buildSpellList() overlays
// it and wins on any ID collision — the hand-authored entries carry the tuned
// Effect/DamageDice/Upcast values, the SRD entry is the fallback shape.
//
// One special-case on collision: the Classes slice is *unioned* across both
// sources rather than replaced. The hand-authored tables were written before
// the Open5e caster scaffolds (Druid/Bard/Sorcerer/Warlock/Paladin) landed,
// so most entries are tagged Mage-only; the SRD knows the broader 5e class
// list. Unioning means a Sorcerer can actually cast a `magic_missile` they
// were auto-granted, without us having to hand-fix every overlay entry.
var dndSpellRegistry = func() map[string]SpellDefinition {
	out := make(map[string]SpellDefinition, 320)
	for _, s := range buildSRDSpellList() {
		out[s.ID] = s
	}
	for _, s := range buildSpellList() {
		if prev, ok := out[s.ID]; ok {
			s.Classes = mergeClassList(prev.Classes, s.Classes)
		}
		out[s.ID] = s
	}
	return out
}()

// mergeClassList returns the union of two class slices, preserving the order
// of the first (SRD baseline) and appending any classes only in the second.
// Order-stability matters for any callers that iterate Classes deterministically.
func mergeClassList(a, b []DnDClass) []DnDClass {
	seen := make(map[DnDClass]bool, len(a)+len(b))
	out := make([]DnDClass, 0, len(a)+len(b))
	for _, c := range a {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	for _, c := range b {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

func lookupSpell(id string) (SpellDefinition, bool) {
	s, ok := dndSpellRegistry[strings.ToLower(strings.TrimSpace(id))]
	return s, ok
}

// parseSpell accepts loose user input ("Fire Bolt", "fire-bolt", "fire_bolt").
func parseSpell(s string) (SpellDefinition, bool) {
	key := strings.ToLower(strings.TrimSpace(s))
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, "-", "_")
	if def, ok := dndSpellRegistry[key]; ok {
		return def, true
	}
	for _, def := range dndSpellRegistry {
		if strings.EqualFold(def.Name, s) {
			return def, true
		}
	}
	return SpellDefinition{}, false
}

func spellsForClass(class DnDClass, levelMax int) []SpellDefinition {
	var out []SpellDefinition
	for _, s := range dndSpellRegistry {
		if s.Level > levelMax {
			continue
		}
		for _, c := range s.Classes {
			if c == class {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// ── Slot tables ──────────────────────────────────────────────────────────────

// slotsForClassLevel returns the spell slot pool a caster of this class+level
// should have at full rest. Returned map is slot_level → total. Non-casters
// return an empty map.
func slotsForClassLevel(class DnDClass, level int) map[int]int {
	if level < 1 {
		return nil
	}
	switch class {
	case ClassMage, ClassDruid, ClassBard, ClassSorcerer:
		// All 5e full casters share one slot progression.
		return fullCasterSlots(level)
	case ClassCleric:
		return clericSlots(level)
	case ClassRanger, ClassPaladin:
		// Half-casters share the Ranger progression: no slots until L2.
		return rangerSlots(level)
	case ClassWarlock:
		return warlockSlots(level)
	}
	return nil
}

// subclassSpellSlots returns a slot pool granted by a subclass on top of the
// class baseline. Phase 10 SUB2-AT: Arcane Trickster is a third-caster that
// unlocks at Rogue L5 (3 L1 slots), expanding modestly with level. Returns
// nil for subclasses without spellcasting.
func subclassSpellSlots(sub DnDSubclass, level int) map[int]int {
	if sub == SubclassArcaneTrickster && level >= 5 {
		// 5e third-caster table, simplified to the levels the engine cares
		// about: 3 L1 at L5–6, 4 L1 + 2 L2 at L7+, expanding into L3 at
		// rogue L13. Matches the design doc's "L5 → 3 L1 slots" anchor.
		switch {
		case level >= 13:
			return map[int]int{1: 4, 2: 3, 3: 2}
		case level >= 7:
			return map[int]int{1: 4, 2: 2}
		default:
			return map[int]int{1: 3}
		}
	}
	return nil
}

// slotsForCharacter returns the combined class+subclass slot pool. Use this
// for any character-level decision (display, slot reset, !cast gating);
// slotsForClassLevel is the class-only base.
func slotsForCharacter(c *DnDCharacter) map[int]int {
	if c == nil {
		return nil
	}
	out := map[int]int{}
	for lvl, n := range slotsForClassLevel(c.Class, c.Level) {
		out[lvl] += n
	}
	for lvl, n := range subclassSpellSlots(c.Subclass, c.Level) {
		out[lvl] += n
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Tables transcribed from gogobee_spell_system.md §1. We interpolate
// between the doc's milestone rows so every level 1..20 has a defined pool.
// fullCasterSlots is the standard 5e full-caster slot progression, shared by
// Mage, Druid, Bard and Sorcerer. (Was mageSlots — generalized when the
// Open5e caster classes landed; output is byte-identical for Mage.)
func fullCasterSlots(level int) map[int]int {
	rows := [][6]int{
		// L1, L2, L3, L4, L5
		{2, 0, 0, 0, 0}, // 1
		{3, 0, 0, 0, 0}, // 2
		{4, 2, 0, 0, 0}, // 3
		{4, 3, 0, 0, 0}, // 4
		{4, 3, 2, 0, 0}, // 5
		{4, 3, 3, 0, 0}, // 6
		{4, 3, 3, 1, 0}, // 7
		{4, 3, 3, 2, 0}, // 8
		{4, 3, 3, 3, 1}, // 9
		{4, 3, 3, 3, 2}, // 10
		{4, 3, 3, 3, 2}, // 11
		{4, 3, 3, 3, 2}, // 12
		{4, 3, 3, 3, 2}, // 13
		{4, 3, 3, 3, 2}, // 14
		{4, 3, 3, 3, 2}, // 15
		{4, 3, 3, 3, 2}, // 16
		{4, 3, 3, 3, 2}, // 17
		{4, 3, 3, 3, 3}, // 18
		{4, 3, 3, 3, 3}, // 19
		{4, 3, 3, 3, 3}, // 20
	}
	if level > len(rows) {
		level = len(rows)
	}
	r := rows[level-1]
	return packSlots(r[0], r[1], r[2], r[3], r[4])
}

func clericSlots(level int) map[int]int {
	rows := [][6]int{
		{2, 0, 0, 0, 0}, // 1
		{3, 0, 0, 0, 0}, // 2
		{4, 2, 0, 0, 0}, // 3
		{4, 3, 0, 0, 0}, // 4
		{4, 3, 2, 0, 0}, // 5
		{4, 3, 3, 0, 0}, // 6
		{4, 3, 3, 1, 0}, // 7
		{4, 3, 3, 2, 0}, // 8
		{4, 3, 3, 3, 1}, // 9
		{4, 3, 3, 3, 2}, // 10
		{4, 3, 3, 3, 2}, // 11
		{4, 3, 3, 3, 2}, // 12
		{4, 3, 3, 3, 2}, // 13
		{4, 3, 3, 3, 2}, // 14
		{4, 3, 3, 3, 2}, // 15
		{4, 3, 3, 3, 2}, // 16
		{4, 3, 3, 3, 3}, // 17
		{4, 3, 3, 3, 3}, // 18
		{4, 3, 3, 3, 3}, // 19
		{4, 3, 3, 3, 3}, // 20
	}
	if level > len(rows) {
		level = len(rows)
	}
	r := rows[level-1]
	return packSlots(r[0], r[1], r[2], r[3], r[4])
}

func rangerSlots(level int) map[int]int {
	// Half-caster — no slots until L2, max 3rd-level.
	rows := [][6]int{
		{0, 0, 0, 0, 0}, // 1
		{2, 0, 0, 0, 0}, // 2
		{3, 0, 0, 0, 0}, // 3
		{3, 0, 0, 0, 0}, // 4
		{3, 0, 0, 0, 0}, // 5
		{3, 0, 0, 0, 0}, // 6
		{3, 1, 0, 0, 0}, // 7  (doc table)
		{3, 2, 0, 0, 0}, // 8
		{3, 2, 0, 0, 0}, // 9  (doc has 3/2 known; slots step at 9 too)
		{3, 2, 0, 0, 0}, // 10
		{3, 2, 0, 0, 0}, // 11
		{3, 2, 0, 0, 0}, // 12
		{3, 2, 1, 0, 0}, // 13
		{3, 2, 1, 0, 0}, // 14
		{3, 2, 1, 0, 0}, // 15
		{3, 2, 1, 0, 0}, // 16
		{3, 2, 2, 0, 0}, // 17
		{3, 2, 2, 0, 0}, // 18
		{3, 3, 2, 0, 0}, // 19
		{3, 3, 2, 0, 0}, // 20
	}
	if level > len(rows) {
		level = len(rows)
	}
	r := rows[level-1]
	return packSlots(r[0], r[1], r[2], r[3], r[4])
}

// warlockSlots — simplified pool. Real 5e Pact Magic gives a tiny number of
// slots that all sit at the highest level and recharge on a short rest; we
// deliberately don't model that (see the Open5e class-scaffold decision —
// "simplified pool, long-rest reset like everyone else"). Instead the Warlock
// gets a modest long-rest pool that tops out at 4th-level slots.
func warlockSlots(level int) map[int]int {
	rows := [][6]int{
		{1, 0, 0, 0, 0}, // 1
		{2, 0, 0, 0, 0}, // 2
		{2, 2, 0, 0, 0}, // 3
		{2, 2, 0, 0, 0}, // 4
		{2, 2, 2, 0, 0}, // 5
		{2, 2, 2, 0, 0}, // 6
		{2, 2, 2, 1, 0}, // 7
		{2, 2, 2, 1, 0}, // 8
		{2, 2, 2, 2, 0}, // 9
		{2, 2, 2, 2, 0}, // 10
		{3, 2, 2, 2, 0}, // 11
		{3, 2, 2, 2, 0}, // 12
		{3, 3, 2, 2, 0}, // 13
		{3, 3, 2, 2, 0}, // 14
		{3, 3, 3, 2, 0}, // 15
		{3, 3, 3, 2, 0}, // 16
		{4, 3, 3, 2, 0}, // 17
		{4, 3, 3, 2, 0}, // 18
		{4, 3, 3, 3, 0}, // 19
		{4, 3, 3, 3, 0}, // 20
	}
	if level > len(rows) {
		level = len(rows)
	}
	r := rows[level-1]
	return packSlots(r[0], r[1], r[2], r[3], r[4])
}

func packSlots(l1, l2, l3, l4, l5 int) map[int]int {
	out := map[int]int{}
	for i, n := range []int{l1, l2, l3, l4, l5} {
		if n > 0 {
			out[i+1] = n
		}
	}
	return out
}

// ── DC math ──────────────────────────────────────────────────────────────────

// spellcastingMod returns the ability modifier used for spell DCs and attacks.
func spellcastingMod(c *DnDCharacter) int {
	if c == nil {
		return 0
	}
	switch c.Class {
	case ClassMage:
		return abilityModifier(c.INT)
	case ClassCleric, ClassRanger, ClassDruid:
		return abilityModifier(c.WIS)
	case ClassBard, ClassSorcerer, ClassWarlock, ClassPaladin:
		return abilityModifier(c.CHA)
	case ClassRogue:
		// Phase 10 SUB2-AT: Arcane Trickster is INT-based.
		if c.Subclass == SubclassArcaneTrickster {
			return abilityModifier(c.INT)
		}
	}
	return 0
}

// spellSaveDC = 8 + proficiency bonus + spellcasting modifier.
//
// Phase 10 SUB2-AT: Arcane Trickster L7 Magical Ambush adds +2. 5e gives
// targets disadvantage on the save when the AT casts while hidden — we
// don't model "hidden" in the one-shot combat layer, so the bump bakes the
// expected-value effect in as a flat DC nudge whenever the AT casts.
func spellSaveDC(c *DnDCharacter) int {
	dc := 8 + proficiencyBonus(c.Level) + spellcastingMod(c)
	if c != nil && c.Class == ClassRogue && c.Subclass == SubclassArcaneTrickster && c.Level >= 7 {
		dc += 2
	}
	return dc
}

// spellAttackBonus = proficiency bonus + spellcasting modifier.
func spellAttackBonus(c *DnDCharacter) int {
	return proficiencyBonus(c.Level) + spellcastingMod(c)
}

// classIsCaster returns true for every spellcasting class. The Open5e
// scaffold classes (Druid/Bard/Sorcerer/Warlock/Paladin) are casters by the
// engine's reckoning even while Playable=false — they have slot tables and a
// spellcasting ability; they just have no spell list to know yet.
func classIsCaster(class DnDClass) bool {
	switch class {
	case ClassMage, ClassCleric, ClassRanger,
		ClassDruid, ClassBard, ClassSorcerer, ClassWarlock, ClassPaladin:
		return true
	}
	return false
}

// isSpellcaster is the character-level gate: full casters by class plus the
// Arcane Trickster subclass once it unlocks at Rogue L5. Use this for !cast,
// !spells and any spellbook bootstrap path.
func isSpellcaster(c *DnDCharacter) bool {
	if c == nil {
		return false
	}
	if classIsCaster(c.Class) {
		return true
	}
	if c.Class == ClassRogue && c.Subclass == SubclassArcaneTrickster && c.Level >= 5 {
		return true
	}
	return false
}

// ── Slot persistence ─────────────────────────────────────────────────────────

// setSpellSlotsForCharacter writes the combined class+subclass slot pool
// (Phase 10 SUB2-AT covers Arcane Trickster's third-caster pool). Mirrors
// setSpellSlotsForLevel's reset-and-insert flow.
func setSpellSlotsForCharacter(c *DnDCharacter) error {
	if c == nil {
		return nil
	}
	pool := slotsForCharacter(c)
	tx, err := db.Get().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM dnd_spell_slots WHERE user_id = ?`, string(c.UserID)); err != nil {
		return err
	}
	for lvl, total := range pool {
		if _, err := tx.Exec(`
			INSERT INTO dnd_spell_slots (user_id, slot_level, total, used)
			VALUES (?, ?, ?, 0)`,
			string(c.UserID), lvl, total); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// setSpellSlotsForLevel writes the slot pool for a class+level, fully
// resetting any prior pool. Used by setup/migration/respec.
func setSpellSlotsForLevel(userID id.UserID, class DnDClass, level int) error {
	pool := slotsForClassLevel(class, level)
	tx, err := db.Get().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM dnd_spell_slots WHERE user_id = ?`, string(userID)); err != nil {
		return err
	}
	for lvl, total := range pool {
		if _, err := tx.Exec(`
			INSERT INTO dnd_spell_slots (user_id, slot_level, total, used)
			VALUES (?, ?, ?, 0)`,
			string(userID), lvl, total); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// getSpellSlots returns slot_level → (total, used) for a player.
func getSpellSlots(userID id.UserID) (map[int][2]int, error) {
	rows, err := db.Get().Query(
		`SELECT slot_level, total, used FROM dnd_spell_slots WHERE user_id = ? ORDER BY slot_level`,
		string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int][2]int{}
	for rows.Next() {
		var lvl, total, used int
		if err := rows.Scan(&lvl, &total, &used); err != nil {
			return nil, err
		}
		out[lvl] = [2]int{total, used}
	}
	return out, rows.Err()
}

// consumeSpellSlot decrements used+1 if a slot is available at slotLevel.
// Returns true on success.
func consumeSpellSlot(userID id.UserID, slotLevel int) (bool, error) {
	res, err := db.Get().Exec(`
		UPDATE dnd_spell_slots
		   SET used = used + 1
		 WHERE user_id = ? AND slot_level = ? AND used < total`,
		string(userID), slotLevel)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// refundSpellSlot re-credits one slot at slotLevel (used in audit-style
// rollback paths and voluntary `!cast --drop`).
func refundSpellSlot(userID id.UserID, slotLevel int) error {
	_, err := db.Get().Exec(`
		UPDATE dnd_spell_slots
		   SET used = MAX(0, used - 1)
		 WHERE user_id = ? AND slot_level = ?`,
		string(userID), slotLevel)
	return err
}

// refreshSpellSlots resets used=0 across all of a player's slots. Called
// on long rest.
func refreshSpellSlots(userID id.UserID) error {
	_, err := db.Get().Exec(
		`UPDATE dnd_spell_slots SET used = 0 WHERE user_id = ?`,
		string(userID))
	return err
}

// ── Known spells ─────────────────────────────────────────────────────────────

func addKnownSpell(userID id.UserID, spellID string, source string, prepared bool) error {
	prep := 1
	if !prepared {
		prep = 0
	}
	_, err := db.Get().Exec(`
		INSERT INTO dnd_known_spells (user_id, spell_id, source, prepared)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, spell_id) DO UPDATE SET source=excluded.source`,
		string(userID), spellID, source, prep)
	return err
}

func setSpellPrepared(userID id.UserID, spellID string, prepared bool) error {
	prep := 0
	if prepared {
		prep = 1
	}
	_, err := db.Get().Exec(`
		UPDATE dnd_known_spells SET prepared = ?
		WHERE user_id = ? AND spell_id = ?`,
		prep, string(userID), spellID)
	return err
}

type knownSpellRow struct {
	SpellID  string
	Source   string
	Prepared bool
}

func listKnownSpells(userID id.UserID) ([]knownSpellRow, error) {
	rows, err := db.Get().Query(
		`SELECT spell_id, source, prepared FROM dnd_known_spells
		 WHERE user_id = ? ORDER BY spell_id`,
		string(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []knownSpellRow
	for rows.Next() {
		var r knownSpellRow
		var prep int
		if err := rows.Scan(&r.SpellID, &r.Source, &prep); err != nil {
			return nil, err
		}
		r.Prepared = prep == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func playerKnowsSpell(userID id.UserID, spellID string) (bool, bool, error) {
	var prep int
	err := db.Get().QueryRow(
		`SELECT prepared FROM dnd_known_spells WHERE user_id = ? AND spell_id = ?`,
		string(userID), spellID).Scan(&prep)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, prep == 1, nil
}

// wipeSpellsForUser clears known + slot rows. Used by !respec.
func wipeSpellsForUser(userID id.UserID) error {
	if _, err := db.Get().Exec(`DELETE FROM dnd_known_spells WHERE user_id = ?`, string(userID)); err != nil {
		return err
	}
	if _, err := db.Get().Exec(`DELETE FROM dnd_spell_slots WHERE user_id = ?`, string(userID)); err != nil {
		return err
	}
	return nil
}

// ── Concentration ────────────────────────────────────────────────────────────

// setConcentration stamps a new concentration spell on the character. Returns
// the previous spell id (empty if none) so the caller can narrate the
// supersession. Caller is responsible for SaveDnDCharacter.
func setConcentration(c *DnDCharacter, spellID string, duration time.Duration) string {
	prev := c.ConcentrationSpell
	c.ConcentrationSpell = spellID
	if duration > 0 {
		exp := time.Now().Add(duration)
		c.ConcentrationExpiresAt = &exp
	} else {
		c.ConcentrationExpiresAt = nil
	}
	return prev
}

// concentrationActive returns the active concentration spell id, or "" if
// none / expired. Mutates the caller's DnDCharacter copy when expiry is hit
// (caller should Save afterwards if it cares).
func concentrationActive(c *DnDCharacter) string {
	if c == nil || c.ConcentrationSpell == "" {
		return ""
	}
	if c.ConcentrationExpiresAt != nil && time.Now().After(*c.ConcentrationExpiresAt) {
		c.ConcentrationSpell = ""
		c.ConcentrationExpiresAt = nil
		return ""
	}
	return c.ConcentrationSpell
}

// ── Migration / auto-grant ───────────────────────────────────────────────────

// ensureSpellsForCharacter populates a sensible known-spell list and slot
// pool for a caster who has none. Idempotent: skips if any spells are
// already known for the user. Called from !setup confirm and from
// ensureDnDCharacterForCombat (auto-migration path).
//
// Defaults are conservative — a caster always has at least cantrips and
// enough leveled options to be functional. Players can swap via
// !spells learn (Mage) or !prepare (Cleric) once SP4 lands.
func ensureSpellsForCharacter(c *DnDCharacter) error {
	if !isSpellcaster(c) {
		return nil
	}
	existing, err := listKnownSpells(c.UserID)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		// Refresh the slot pool in case level/subclass has changed since the
		// prior grant (covers level-up between sessions and AT subclass pick).
		return setSpellSlotsForCharacter(c)
	}
	defaults := defaultKnownSpells(c.Class, c.Level)
	if c.Class == ClassRogue && c.Subclass == SubclassArcaneTrickster {
		defaults = arcaneTricksterDefaultSpells(c.Level)
	}
	for _, sid := range defaults {
		// Cleric "prepares" daily — but prep system is SP4. Until then,
		// every default cleric spell is auto-prepared so !cast works.
		if err := addKnownSpell(c.UserID, sid, "class", true); err != nil {
			return err
		}
	}
	return setSpellSlotsForCharacter(c)
}

// arcaneTricksterDefaultSpells returns the Mage-list starter spells granted
// when a Rogue picks Arcane Trickster. Picks lean on illusion + control +
// single-target combat utility from spells already in the registry; mage_hand
// and disguise_self/invisibility are flavored AT staples but not modelled
// yet, so we substitute the closest available analogues.
func arcaneTricksterDefaultSpells(level int) []string {
	out := []string{"minor_illusion", "shocking_grasp"}
	if level >= 5 {
		out = append(out, "magic_missile", "shield", "sleep")
	}
	if level >= 7 {
		out = append(out, "mirror_image", "misty_step")
	}
	if level >= 13 {
		out = append(out, "hypnotic_pattern")
	}
	return out
}

// defaultKnownSpells returns a sensible starter list for class+level. Tuned
// to give each caster at least a damage option, a buff/utility, and a heal
// where applicable. Cantrips first, then leveled spells up to the player's
// max slot level. Picks are deterministic so tests can lock them down.
func defaultKnownSpells(class DnDClass, level int) []string {
	maxSlot := highestAvailableSlot(class, level)
	switch class {
	case ClassMage:
		out := []string{"fire_bolt", "minor_illusion", "mending"}
		if level >= 1 {
			// No `shield` here — it's a reaction (EffectReaction) and combat
			// has no reaction window yet, so an auto-granted shield is just
			// a dead entry in the player's spellbook. Reinstated when SP-Reactions ships.
			out = append(out, "magic_missile", "mage_armor", "burning_hands", "detect_magic", "sleep")
		}
		if maxSlot >= 2 {
			out = append(out, "scorching_ray", "misty_step", "mirror_image")
		}
		if maxSlot >= 3 {
			// `counterspell` deferred for the same reason as `shield`.
			out = append(out, "fireball", "fly")
		}
		if maxSlot >= 4 {
			out = append(out, "ice_storm", "greater_invisibility")
		}
		if maxSlot >= 5 {
			out = append(out, "cone_of_cold", "wall_of_force")
		}
		return out
	case ClassCleric:
		out := []string{"sacred_flame", "guidance", "mending"}
		if level >= 1 {
			out = append(out, "cure_wounds", "healing_word", "bless", "guiding_bolt", "shield_of_faith")
		}
		if maxSlot >= 2 {
			out = append(out, "spiritual_weapon", "lesser_restoration", "aid")
		}
		if maxSlot >= 3 {
			out = append(out, "spirit_guardians", "revivify", "mass_healing_word")
		}
		if maxSlot >= 4 {
			out = append(out, "guardian_of_faith", "death_ward")
		}
		if maxSlot >= 5 {
			out = append(out, "mass_cure_wounds", "flame_strike")
		}
		return out
	case ClassRanger:
		// Rangers have no spells until level 2.
		if level < 2 {
			return []string{"guidance", "thorn_whip"}
		}
		out := []string{"guidance", "thorn_whip"}
		out = append(out, "hunters_mark", "cure_wounds")
		if maxSlot >= 2 {
			out = append(out, "pass_without_trace", "spike_growth")
		}
		if maxSlot >= 3 {
			out = append(out, "lightning_arrow", "conjure_barrage")
		}
		return out

	// ── Open5e caster scaffold classes ───────────────────────────────────
	// Picks are SRD spell IDs (dnd_spells_srd_data.go); a handful resolve to
	// the hand-authored overlay on ID collision, which is fine. Each list
	// gives the class a damage option, control, and a heal/buff where the
	// class fantasy expects one.
	case ClassDruid:
		// `shillelagh` deferred — the overlay flags it ranger/cleric only and
		// it has no real attack profile in the engine yet (BuffSelf, no damage
		// dice). Reinstated when weapon-bonus casts wire up.
		out := []string{"produce_flame", "guidance", "mending"}
		if level >= 1 {
			out = append(out, "cure_wounds", "healing_word", "faerie_fire", "thunderwave", "entangle")
		}
		if maxSlot >= 2 {
			out = append(out, "flaming_sphere", "heat_metal", "hold_person", "lesser_restoration")
		}
		if maxSlot >= 3 {
			out = append(out, "call_lightning", "conjure_animals", "dispel_magic")
		}
		if maxSlot >= 4 {
			out = append(out, "ice_storm", "polymorph")
		}
		if maxSlot >= 5 {
			out = append(out, "insect_plague", "mass_cure_wounds")
		}
		return out
	case ClassBard:
		out := []string{"vicious_mockery", "minor_illusion", "message"}
		if level >= 1 {
			out = append(out, "cure_wounds", "healing_word", "heroism", "hideous_laughter", "faerie_fire")
		}
		if maxSlot >= 2 {
			out = append(out, "hold_person", "shatter", "invisibility", "lesser_restoration")
		}
		if maxSlot >= 3 {
			out = append(out, "hypnotic_pattern", "dispel_magic", "fear")
		}
		if maxSlot >= 4 {
			out = append(out, "greater_invisibility", "polymorph")
		}
		if maxSlot >= 5 {
			out = append(out, "hold_monster", "mass_cure_wounds")
		}
		return out
	case ClassSorcerer:
		out := []string{"fire_bolt", "ray_of_frost", "shocking_grasp", "mending"}
		if level >= 1 {
			// `shield` skipped — reaction spell, no usable window in current combat.
			out = append(out, "magic_missile", "burning_hands", "mage_armor", "thunderwave", "sleep")
		}
		if maxSlot >= 2 {
			out = append(out, "scorching_ray", "mirror_image", "misty_step", "hold_person")
		}
		if maxSlot >= 3 {
			// `counterspell` skipped — reaction.
			out = append(out, "fireball", "lightning_bolt", "haste", "fly")
		}
		if maxSlot >= 4 {
			out = append(out, "ice_storm", "greater_invisibility")
		}
		if maxSlot >= 5 {
			out = append(out, "cone_of_cold", "hold_monster")
		}
		return out
	case ClassWarlock:
		out := []string{"eldritch_blast", "chill_touch", "minor_illusion"}
		if level >= 1 {
			// `hellish_rebuke` skipped — reaction spell, no usable window
			// in current combat. Substituting `burning_hands` as the L1
			// damage staple (SRD: Mage/Cleric/Sorcerer/Warlock).
			out = append(out, "burning_hands", "command", "charm_person", "hideous_laughter")
		}
		if maxSlot >= 2 {
			out = append(out, "scorching_ray", "misty_step", "hold_person", "invisibility")
		}
		if maxSlot >= 3 {
			// `counterspell` skipped — reaction.
			out = append(out, "fly", "vampiric_touch", "hypnotic_pattern")
		}
		if maxSlot >= 4 {
			out = append(out, "banishment", "greater_invisibility")
		}
		if maxSlot >= 5 {
			out = append(out, "hold_monster", "scrying")
		}
		return out
	case ClassPaladin:
		// Half-caster — no cantrips, no spells until level 2.
		if level < 2 {
			return nil
		}
		out := []string{"bless", "cure_wounds", "divine_favor", "shield_of_faith", "command"}
		if maxSlot >= 2 {
			out = append(out, "aid", "find_steed", "magic_weapon", "lesser_restoration")
		}
		if maxSlot >= 3 {
			out = append(out, "revivify", "dispel_magic", "haste")
		}
		return out
	}
	return nil
}

// mageKnownSpellsCap returns the maximum number of *leveled* spells a Mage
// of the given level may have in their spellbook. 5e standard: 6 starting
// spells + 2 per level after L1 (so 6 at L1, 8 at L2, …, 44 at L20).
// Cantrips don't count against this cap.
func mageKnownSpellsCap(level int) int {
	if level < 1 {
		level = 1
	}
	return 6 + 2*(level-1)
}

// mageLeveledKnownCount counts leveled (Level ≥ 1) spells in the player's
// spellbook. Cantrips and unknown registry entries are skipped.
func mageLeveledKnownCount(userID id.UserID) (int, error) {
	rows, err := listKnownSpells(userID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		s, ok := lookupSpell(r.SpellID)
		if !ok || s.Level == 0 {
			continue
		}
		n++
	}
	return n, nil
}

func highestAvailableSlot(class DnDClass, level int) int {
	pool := slotsForClassLevel(class, level)
	high := 0
	for lvl := range pool {
		if lvl > high {
			high = lvl
		}
	}
	return high
}

// highestAvailableSlotForChar — like highestAvailableSlot but consults the
// combined class+subclass pool. Phase 10 SUB2-AT uses this to clamp Arcane
// Trickster upcasts to their actual subclass slot ceiling.
func highestAvailableSlotForChar(c *DnDCharacter) int {
	high := 0
	for lvl := range slotsForCharacter(c) {
		if lvl > high {
			high = lvl
		}
	}
	return high
}

// ── Display helpers ──────────────────────────────────────────────────────────

func displaySpellName(spellID string) string {
	if s, ok := lookupSpell(spellID); ok {
		return s.Name
	}
	return spellID
}

func renderSlotLine(slots map[int][2]int) string {
	if len(slots) == 0 {
		return "_(no spell slots)_"
	}
	var parts []string
	for lvl := 1; lvl <= 5; lvl++ {
		if pair, ok := slots[lvl]; ok {
			parts = append(parts, fmt.Sprintf("L%d %d/%d", lvl, pair[0]-pair[1], pair[0]))
		}
	}
	return strings.Join(parts, " · ")
}
