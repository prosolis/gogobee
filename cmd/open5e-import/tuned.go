package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// genTuned derives an engine-ready DnDMonsterTemplate for every SRD monster and
// writes the generated Go data file. It is the codified "tuning pass": the raw
// SRD staging table one-shots gogobee's solo player, so this applies a
// deterministic formula to scale each stat block down to the engine's
// gameplay-tuned shape.
//
// Formula (reverse-engineered from the hand-tuned dndBestiary, 2026-05 passes):
//
//   - HP, AC, AttackBonus  — verbatim SRD (AC clamped to the engine's min 10).
//   - Attack               — the auto-resolve damage stat, driven by CR alone
//     via attackByCR (raw SRD per-hit damage is deliberately ignored — CR is
//     the calibration axis the 2026-05-10 rebalance used).
//   - Speed, BlockRate     — coarse baselines from SpeedWalk / AC; a later hand
//     pass refines them per creature.
//   - XP, CR               — verbatim from the staging table.
//   - Ability              — classified from the SRD trait names by
//     abilityFromTraits: the highest-priority recognised trait maps onto one of
//     the engine's MonsterAbility effects. Creatures whose traits are all
//     non-combat (Keen Smell, Amphibious, …) stay nil. Notes still records the
//     raw SRD multiattack/trait text so a hand pass can refine the pick.
//
// Hand-authored dndBestiary entries WIN: the generated map is merged in only
// for IDs the roster doesn't already define (see bestiary_tuned.go).
func genTuned() error {
	raw, err := os.ReadFile(monstersJSON)
	if err != nil {
		return fmt.Errorf("read %s (run `fetch bestiary` first?): %w", monstersJSON, err)
	}
	var src []open5eMonster
	if err := json.Unmarshal(raw, &src); err != nil {
		return err
	}

	tuned := make([]genTunedMonster, 0, len(src))
	for _, m := range src {
		tuned = append(tuned, tuneMonster(classifyMonster(m)))
	}
	sort.Slice(tuned, func(i, j int) bool {
		if tuned[i].CR != tuned[j].CR {
			return tuned[i].CR < tuned[j].CR
		}
		return tuned[i].ID < tuned[j].ID
	})
	fmt.Fprintf(os.Stderr, "tuned %d monsters\n", len(tuned))

	out := emitTuned(tuned)
	if err := os.WriteFile(tunedGenGo, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "wrote", tunedGenGo)
	return nil
}

// genTunedMonster mirrors plugin.DnDMonsterTemplate. Ability is nil when none
// of the creature's SRD traits map onto an engine effect.
type genTunedMonster struct {
	ID, Name    string
	CR          float64
	HP, AC      int
	Attack      int
	AttackBonus int
	Speed       int
	BlockRate   float64
	XPValue     int
	Ability     *genMonsterAbility
	Notes       string
}

// genMonsterAbility mirrors plugin.MonsterAbility.
type genMonsterAbility struct {
	Name       string
	Phase      string
	ProcChance float64
	Effect     string
}

// tuneMonster applies the tuning formula to one raw SRD stat block.
func tuneMonster(b genStatBlock) genTunedMonster {
	ac := b.AC
	if ac < 10 { // engine minimum (see the zombie 8→10 note in dnd_bestiary.go)
		ac = 10
	}
	ability := abilityFromTraits(b.Traits)
	return genTunedMonster{
		ID:          b.Slug,
		Name:        b.Name,
		CR:          b.CR,
		HP:          b.HP,
		AC:          ac,
		Attack:      attackByCR(b.CR),
		AttackBonus: primaryAttackBonus(b.Attacks),
		Speed:       tunedSpeed(b.SpeedWalk),
		BlockRate:   tunedBlockRate(ac),
		XPValue:     b.XP,
		Ability:     ability,
		Notes:       tunedNotes(b, ability),
	}
}

// traitAbilityRules maps SRD special-ability names onto the engine's
// MonsterAbility effects. Each rule's Match list holds lowercase substrings; a
// creature's trait matches a rule if any substring is contained in the
// (lowercased, paren-stripped) trait name. The list is priority-ordered —
// abilityFromTraits walks it top-down and the first rule that matches any of
// the creature's traits wins, so the most combat-defining mechanic is picked
// when a stat block carries several traits. Traits that name no rule (Keen
// Smell, Amphibious, Hold Breath, telepathy/senses flavor) leave Ability nil.
//
// This is a best-effort baseline, like the rest of the tuning formula: the
// hand-authored dndBestiary entry always wins the merge, so refining a pick is
// a matter of adding the creature to the roster.
var traitAbilityRules = []struct {
	match  []string
	name   string
	phase  string
	proc   float64
	effect string
}{
	// Death-triggered blasts — resolve on the killing-blow phase.
	{[]string{"death burst", "death throes", "elemental demise"}, "Death Burst", "decisive", 1.0, "death_aoe"},
	// Spellcasters open with a magical barrage.
	{[]string{"spellcasting"}, "Spellcasting", "opening", 0.5, "aoe"},
	// Cheats death once.
	{[]string{"undead fortitude"}, "Undead Fortitude", "decisive", 0.5, "survive_at_1"},
	// Self-sustain — trolls, liches clawing back.
	{[]string{"regeneration", "rejuvenation"}, "Regeneration", "any", 0.5, "regenerate"},
	// Shrugs off spells.
	{[]string{"magic resistance", "legendary resistance", "magic immunity", "spell immunity"}, "Magic Resistance", "any", 0.5, "spell_resist"},
	// Ambush predators — a heavy opening strike.
	{[]string{"surprise attack", "assassinate", "ambusher", "shadow stealth"}, "Surprise Attack", "opening", 0.8, "bonus_damage"},
	// Fear and petrification lock the player out of acting.
	{[]string{"petrifying gaze", "fear aura", "horrific appearance"}, "Frightful Presence", "opening", 0.4, "stun"},
	// Frenzy / rage — damage ramps as the fight drags.
	{[]string{"blood frenzy", "rampage", "reckless", "aggressive", "berserk", "brute"}, "Frenzy", "any", 0.4, "enrage"},
	// Damaging auras and spiky hides punish every exchange.
	{[]string{"heated body", "fire aura", "fire form", "stench", "mucous cloud", "barbed hide", "reflective carapace", "heated weapons", "hellish weapons"}, "Damaging Aura", "any", 0.4, "retaliate"},
	// Charges, pounces, leaps — an opening lunge.
	{[]string{"charge", "pounce", "standing leap", "running leap"}, "Charge", "opening", 0.4, "bonus_damage"},
	// Coordinated attackers gain advantage.
	{[]string{"pack tactics"}, "Pack Tactics", "any", 0.3, "advantage"},
	// Riders that pile damage onto a landed hit.
	{[]string{"sneak attack", "martial advantage"}, "Martial Advantage", "any", 0.35, "bonus_damage"},
	// Slippery movement — dodges a blow outright.
	{[]string{"nimble escape", "misty escape", "cunning action", "evasion", "incorporeal movement", "ethereal jaunt", "flyby", "shapechanger"}, "Evasive", "any", 0.3, "evade"},
	// Enchanted weapons — a flat damage edge.
	{[]string{"magic weapons", "angelic weapons"}, "Magic Weapons", "any", 0.3, "bonus_damage"},
}

// abilityFromTraits picks the highest-priority MonsterAbility implied by a
// creature's SRD trait names, or nil when none of them map onto an engine
// effect. See traitAbilityRules for the priority order.
func abilityFromTraits(traits []string) *genMonsterAbility {
	norm := make([]string, len(traits))
	for i, t := range traits {
		// Drop the parenthetical suffix SRD appends to recharge-limited
		// traits ("Legendary Resistance (3/Day)").
		if p := strings.IndexByte(t, '('); p >= 0 {
			t = t[:p]
		}
		norm[i] = strings.ToLower(strings.TrimSpace(t))
	}
	for _, rule := range traitAbilityRules {
		for _, tr := range norm {
			for _, m := range rule.match {
				if strings.Contains(tr, m) {
					return &genMonsterAbility{
						Name:       rule.name,
						Phase:      rule.phase,
						ProcChance: rule.proc,
						Effect:     rule.effect,
					}
				}
			}
		}
	}
	return nil
}

// attackByCRPoints anchors the Attack-stat curve against the hand-tuned
// dndBestiary: each point is an (CR, observed Attack) pair lifted straight from
// a playtested roster entry. attackByCR interpolates between anchors and
// extrapolates past the top one along the final segment's slope.
var attackByCRPoints = []struct {
	cr  float64
	atk int
}{
	{0, 1}, {0.125, 1}, {0.25, 1}, {0.5, 2}, {1, 2}, {2, 4}, {3, 6}, {4, 7},
	{5, 8}, {6, 10}, {7, 12}, {8, 13}, {9, 14}, {10, 16}, {11, 18}, {12, 19},
	{13, 21}, {16, 26}, {19, 31}, {20, 33}, {24, 38},
}

func attackByCR(cr float64) int {
	pts := attackByCRPoints
	if cr <= pts[0].cr {
		return pts[0].atk
	}
	last := pts[len(pts)-1]
	if cr >= last.cr {
		prev := pts[len(pts)-2]
		slope := float64(last.atk-prev.atk) / (last.cr - prev.cr)
		atk := int(math.Round(float64(last.atk) + slope*(cr-last.cr)))
		if atk < 1 {
			atk = 1
		}
		return atk
	}
	for i := 1; i < len(pts); i++ {
		if cr <= pts[i].cr {
			lo, hi := pts[i-1], pts[i]
			t := (cr - lo.cr) / (hi.cr - lo.cr)
			atk := int(math.Round(float64(lo.atk) + t*float64(hi.atk-lo.atk)))
			if atk < 1 {
				atk = 1
			}
			return atk
		}
	}
	return last.atk
}

// primaryAttackBonus returns the d20 to-hit modifier of the creature's
// hardest-hitting attack — the one the engine's single Attack stat stands in
// for. Returns 0 (engine falls back to the tier-scaled bonus) when the stat
// block has no parsed weapon attacks.
func primaryAttackBonus(attacks []genStatAttack) int {
	bonus, best := 0, -1
	for _, a := range attacks {
		if a.AvgDamage > best {
			best, bonus = a.AvgDamage, a.AttackBonus
		}
	}
	return bonus
}

// tunedSpeed maps an SRD walk speed (feet) onto the engine's speed scale. The
// engine treats speed as an initiative-ordering knob, so a coarse divide-and-
// clamp is enough; the ability pass can hand-tune outliers.
func tunedSpeed(walk int) int {
	if walk <= 0 {
		return 10
	}
	s := int(math.Round(float64(walk) / 2.5))
	if s < 6 {
		s = 6
	}
	if s > 18 {
		s = 18
	}
	return s
}

// tunedBlockRate is a baseline damage-halve chance keyed off AC — heavier armor
// reads as a better chance to soak a hit. The ability pass overrides this for
// creatures whose block is a real mechanic (Parry, shields, etc.).
func tunedBlockRate(ac int) float64 {
	switch {
	case ac >= 18:
		return 0.15
	case ac >= 15:
		return 0.10
	case ac >= 13:
		return 0.05
	default:
		return 0.0
	}
}

// tunedNotes records the SRD multiattack and trait text so a hand-tuning pass
// has the source material in front of it, and states which ability (if any)
// the trait classifier wired.
func tunedNotes(b genStatBlock, ability *genMonsterAbility) string {
	prefix := "SRD-tuned baseline — no ability wired (traits are non-combat)."
	if ability != nil {
		prefix = fmt.Sprintf("SRD-tuned baseline — %s (%s) wired from traits.",
			ability.Name, ability.Effect)
	}
	var parts []string
	if b.Multiattack != "" {
		parts = append(parts, "Multiattack: "+b.Multiattack)
	}
	if len(b.Traits) > 0 {
		parts = append(parts, "Traits: "+strings.Join(b.Traits, ", "))
	}
	if len(parts) == 0 {
		return prefix
	}
	return prefix + " " + strings.Join(parts, " ")
}

// emitTuned renders the tuned templates as a generated Go source file.
func emitTuned(monsters []genTunedMonster) []byte {
	var b bytes.Buffer
	b.WriteString(`// Code generated by cmd/open5e-import. DO NOT EDIT.
//
// Source: Open5e SRD monster dump (data/open5e/monsters.json), 5e SRD content
// under CC-BY-4.0 — see NOTICE. Regenerate with:
//
//	go run ./cmd/open5e-import fetch bestiary
//	go run ./cmd/open5e-import gen tuned
//
// Engine-ready DnDMonsterTemplates derived from the raw SRD staging table by
// the tuning formula in cmd/open5e-import/tuned.go. Hand-authored dndBestiary
// entries take precedence — see bestiary_tuned.go for the merge.

package plugin

func buildTunedBestiarySRD() map[string]DnDMonsterTemplate {
	return map[string]DnDMonsterTemplate{
`)
	for _, m := range monsters {
		fmt.Fprintf(&b, "\t\t%q: {ID: %q, Name: %q, CR: %s, HP: %d, AC: %d, Attack: %d, AttackBonus: %d, Speed: %d, BlockRate: %s, XPValue: %d, ",
			m.ID, m.ID, m.Name,
			strconv.FormatFloat(m.CR, 'g', -1, 32),
			m.HP, m.AC, m.Attack, m.AttackBonus, m.Speed,
			strconv.FormatFloat(m.BlockRate, 'g', -1, 64),
			m.XPValue)
		if m.Ability != nil {
			fmt.Fprintf(&b, "Ability: &MonsterAbility{Name: %q, Phase: %q, ProcChance: %s, Effect: %q}, ",
				m.Ability.Name, m.Ability.Phase,
				strconv.FormatFloat(m.Ability.ProcChance, 'g', -1, 64),
				m.Ability.Effect)
		}
		fmt.Fprintf(&b, "Notes: %q},\n", m.Notes)
	}
	b.WriteString("\t}\n}\n")
	return b.Bytes()
}
