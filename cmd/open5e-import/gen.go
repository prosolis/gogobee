package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// classEnum maps an Open5e spell_lists entry to the plugin's DnDClass enum
// identifier. Open5e's "wizard" is the plugin's Mage; classes the plugin
// doesn't model (artificer, etc.) are absent and silently dropped.
//
// classOrder fixes the iteration order so generated Classes slices are stable.
var (
	classEnum = map[string]string{
		"wizard":   "ClassMage",
		"cleric":   "ClassCleric",
		"ranger":   "ClassRanger",
		"druid":    "ClassDruid",
		"bard":     "ClassBard",
		"sorcerer": "ClassSorcerer",
		"warlock":  "ClassWarlock",
		"paladin":  "ClassPaladin",
	}
	classOrder = []string{"wizard", "cleric", "ranger", "druid", "bard", "sorcerer", "warlock", "paladin"}
)

// mapClasses turns an Open5e spell's class tags into plugin enum identifiers,
// deduped and in classOrder. It unions the structured spell_lists array with
// the free-text dnd_class field ("Druid, Wizard") — the SRD dump's spell_lists
// omits paladin entirely, so dnd_class is the more complete source. Returns
// nil when the spell belongs to no modelled class (the caller drops those —
// they have no caster to learn them).
func mapClasses(lists []string, dndClass string) []string {
	seen := map[string]bool{}
	for _, l := range lists {
		seen[strings.ToLower(strings.TrimSpace(l))] = true
	}
	for _, l := range strings.Split(dndClass, ",") {
		seen[strings.ToLower(strings.TrimSpace(l))] = true
	}
	var out []string
	for _, c := range classOrder {
		if seen[c] {
			out = append(out, classEnum[c])
		}
	}
	return out
}

// genSpell is a fully classified spell ready for code emission. Fields mirror
// plugin.SpellDefinition; enum-typed fields hold the Go identifier as a string.
type genSpell struct {
	ID            string
	Name          string
	Level         int
	School        string
	Classes       []string
	Effect        string
	CastTime      string
	Concentration bool
	SaveStat      string
	AttackRoll    bool
	DamageDice    string
	DamageType    string
	Description   string
	Upcast        string
	MaterialCost  int
	AOE           bool
}

// genSpells reads the vendored spells.json, classifies every in-scope spell,
// and writes the generated Go data file.
func genSpells() error {
	raw, err := os.ReadFile(spellsJSON)
	if err != nil {
		return fmt.Errorf("read %s (run `fetch spells` first?): %w", spellsJSON, err)
	}
	var src []open5eSpell
	if err := json.Unmarshal(raw, &src); err != nil {
		return err
	}

	var spells []genSpell
	var skippedLevel, skippedNoClass int
	for _, s := range src {
		if s.LevelInt > 5 {
			// Combat caps at L5 spell slots — nothing can ever cast these.
			skippedLevel++
			continue
		}
		classes := mapClasses(s.SpellLists, s.DndClass)
		if len(classes) == 0 {
			skippedNoClass++
			continue
		}
		spells = append(spells, classify(s, classes))
	}
	sort.Slice(spells, func(i, j int) bool {
		if spells[i].Level != spells[j].Level {
			return spells[i].Level < spells[j].Level
		}
		return spells[i].ID < spells[j].ID
	})
	fmt.Fprintf(os.Stderr, "classified %d spells (skipped %d level>5, %d no modelled class)\n",
		len(spells), skippedLevel, skippedNoClass)

	out := emit(spells)
	if err := os.WriteFile(spellsGenGo, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "wrote", spellsGenGo)
	return nil
}

// ── Classifier ───────────────────────────────────────────────────────────────

var (
	reSave      = regexp.MustCompile(`(Strength|Dexterity|Constitution|Intelligence|Wisdom|Charisma) saving throw`)
	reDice      = regexp.MustCompile(`\d+d\d+(?:\s*\+\s*\d+)?`)
	reDmgType   = regexp.MustCompile(`(?i)(acid|bludgeoning|cold|fire|force|lightning|necrotic|piercing|poison|psychic|radiant|slashing|thunder)\s+damage`)
	reMaterial  = regexp.MustCompile(`(?i)worth (?:at least )?([\d,]+)\s*gp`)
	saveStatMap = map[string]string{
		"Strength": "STR", "Dexterity": "DEX", "Constitution": "CON",
		"Intelligence": "INT", "Wisdom": "WIS", "Charisma": "CHA",
	}
)

func classify(s open5eSpell, classes []string) genSpell {
	desc := strings.TrimSpace(s.Desc)
	low := strings.ToLower(desc)

	g := genSpell{
		ID:            strings.ReplaceAll(s.Slug, "-", "_"),
		Name:          s.Name,
		Level:         s.LevelInt,
		School:        strings.ToLower(strings.TrimSpace(s.School)),
		Classes:       classes,
		Concentration: s.RequiresConcentration,
		Description:   spellDescription(strings.ReplaceAll(s.Slug, "-", "_"), desc),
		Upcast:        firstSentence(strings.TrimSpace(s.HigherLevel), 200),
		AOE:           looksAOE(low),
	}

	// Cast time. Reaction/bonus action win over the ritual flag.
	ct := strings.ToLower(s.CastingTime)
	switch {
	case strings.Contains(ct, "bonus action"):
		g.CastTime = "CastBonusAction"
	case strings.Contains(ct, "reaction"):
		g.CastTime = "CastReaction"
	case s.CanBeCastAsRitual:
		g.CastTime = "CastRitual"
	default:
		g.CastTime = "CastAction"
	}

	if m := reSave.FindStringSubmatch(desc); m != nil {
		g.SaveStat = saveStatMap[m[1]]
	}
	g.AttackRoll = strings.Contains(low, "spell attack")
	if m := reDice.FindString(desc); m != "" {
		g.DamageDice = strings.ReplaceAll(m, " ", "")
	}
	if m := reDmgType.FindStringSubmatch(low); m != nil {
		g.DamageType = strings.ToLower(m[1])
	}
	if m := reMaterial.FindStringSubmatch(s.Material); m != nil {
		g.MaterialCost, _ = strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	}

	g.Effect = classifyEffect(g, low)

	// A heal spell's dice describe healing, not damage — keep DamageDice (the
	// hand-authored list does the same) but drop a stray damage type.
	if g.Effect == "EffectSpellHeal" {
		g.DamageType = ""
	}
	// Non-damaging effects shouldn't carry a damage type/dice picked up from
	// incidental wording.
	if g.Effect == "EffectControl" || g.Effect == "EffectUtility" ||
		g.Effect == "EffectReaction" {
		g.DamageDice, g.DamageType = "", ""
	}
	return g
}

// classifyEffect picks the SpellEffectKind. Heuristic and imperfect by design —
// the hand-authored buildSpellList() overlay is the refinement path, so this
// only needs to be a reasonable default.
func classifyEffect(g genSpell, low string) string {
	if g.CastTime == "CastReaction" {
		return "EffectReaction"
	}
	isHeal := strings.Contains(low, "regain") && strings.Contains(low, "hit point") &&
		!strings.Contains(low, "damage")
	if isHeal {
		return "EffectSpellHeal"
	}
	if g.DamageDice != "" && g.DamageType != "" {
		switch {
		case g.AttackRoll:
			return "EffectDamageAttack"
		case g.SaveStat != "":
			return "EffectDamageSave"
		default:
			return "EffectDamageAuto"
		}
	}
	if g.SaveStat != "" {
		return "EffectControl"
	}
	// Light buff detection — anything still unclassified falls through to
	// utility, which is the safe bucket.
	if strings.Contains(low, "bonus to") || strings.Contains(low, "you gain") ||
		strings.Contains(low, "gains a bonus") || strings.Contains(low, "your ac") {
		if strings.Contains(low, "you gain") || strings.Contains(low, "yourself") ||
			strings.Contains(low, "your ac") {
			return "EffectBuffSelf"
		}
		return "EffectBuffAlly"
	}
	return "EffectUtility"
}

func looksAOE(low string) bool {
	for _, kw := range []string{
		"each creature", "all creatures", "-foot radius", "-foot-radius",
		"foot cone", "foot cube", "foot line", "foot-radius sphere",
	} {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

// spellDescription returns the description text emitted for a spell. A
// hand-authored override in spellDescOverride wins outright; otherwise the
// SRD first-sentence is run through cleanDesc to strip jargon clauses.
func spellDescription(id, raw string) string {
	if s, ok := spellDescOverride[id]; ok {
		return s
	}
	return firstSentence(cleanDesc(raw), 200)
}

// firstSentence trims free-text fields down to a single sentence, capped at
// max runes, so generated Description/Upcast stay terminal-friendly.
func firstSentence(s string, max int) string {
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, "\n"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, ". "); i >= 0 {
		s = s[:i+1]
	}
	s = strings.TrimSpace(s)
	if len(s) > max {
		s = strings.TrimSpace(s[:max]) + "…"
	}
	return s
}

// ── Code emission ────────────────────────────────────────────────────────────

func emit(spells []genSpell) []byte {
	var b bytes.Buffer
	b.WriteString(`// Code generated by cmd/open5e-import. DO NOT EDIT.
//
// Source: Open5e SRD spell dump (data/open5e/spells.json), 5e SRD content
// under CC-BY-4.0 — see NOTICE. Regenerate with:
//
//	go run ./cmd/open5e-import fetch spells
//	go run ./cmd/open5e-import gen spells
//
// Effect/CastTime/SaveStat and friends come from a free-text classifier; the
// hand-authored buildSpellList() overlays this slice and wins on ID collision,
// so any misclassification here is correctable there rather than in the dump.

package plugin

func buildSRDSpellList() []SpellDefinition {
	return []SpellDefinition{
`)
	for _, s := range spells {
		b.WriteString("\t\t{")
		fmt.Fprintf(&b, "ID: %q, Name: %q, Level: %d", s.ID, s.Name, s.Level)
		if s.School != "" {
			fmt.Fprintf(&b, ", School: %q", s.School)
		}
		b.WriteString(", Classes: []DnDClass{")
		for i, c := range s.Classes {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(c)
		}
		b.WriteString("}")
		fmt.Fprintf(&b, ", Effect: %s, CastTime: %s", s.Effect, s.CastTime)
		if s.Concentration {
			b.WriteString(", Concentration: true")
		}
		if s.SaveStat != "" {
			fmt.Fprintf(&b, ", SaveStat: %q", s.SaveStat)
		}
		if s.AttackRoll {
			b.WriteString(", AttackRoll: true")
		}
		if s.DamageDice != "" {
			fmt.Fprintf(&b, ", DamageDice: %q", s.DamageDice)
		}
		if s.DamageType != "" {
			fmt.Fprintf(&b, ", DamageType: %q", s.DamageType)
		}
		if s.Description != "" {
			fmt.Fprintf(&b, ", Description: %q", s.Description)
		}
		if s.Upcast != "" {
			fmt.Fprintf(&b, ", Upcast: %q", s.Upcast)
		}
		if s.MaterialCost != 0 {
			fmt.Fprintf(&b, ", MaterialCost: %d", s.MaterialCost)
		}
		if s.AOE {
			b.WriteString(", AOE: true")
		}
		b.WriteString("},\n")
	}
	b.WriteString("\t}\n}\n")
	return b.Bytes()
}
