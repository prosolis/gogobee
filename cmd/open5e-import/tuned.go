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
//   - Speed, BlockRate     — coarse baselines from SpeedWalk / AC; the ability
//     pass refines them per creature.
//   - XP, CR               — verbatim from the staging table.
//   - Ability              — nil. Trait→MonsterAbility wiring is a follow-up
//     pass; Notes records the SRD multiattack/trait text so that pass has a
//     starting point.
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

// genTunedMonster mirrors plugin.DnDMonsterTemplate, minus the Ability pointer
// (the generator never wires abilities — that is a hand-authored follow-up).
type genTunedMonster struct {
	ID, Name    string
	CR          float64
	HP, AC      int
	Attack      int
	AttackBonus int
	Speed       int
	BlockRate   float64
	XPValue     int
	Notes       string
}

// tuneMonster applies the tuning formula to one raw SRD stat block.
func tuneMonster(b genStatBlock) genTunedMonster {
	ac := b.AC
	if ac < 10 { // engine minimum (see the zombie 8→10 note in dnd_bestiary.go)
		ac = 10
	}
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
		Notes:       tunedNotes(b),
	}
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

// tunedNotes records the SRD multiattack and trait text so the follow-up
// ability-wiring pass has the source material in front of it.
func tunedNotes(b genStatBlock) string {
	const prefix = "SRD-tuned baseline — ability not yet wired."
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
		fmt.Fprintf(&b, "\t\t%q: {ID: %q, Name: %q, CR: %s, HP: %d, AC: %d, Attack: %d, AttackBonus: %d, Speed: %d, BlockRate: %s, XPValue: %d, Notes: %q},\n",
			m.ID, m.ID, m.Name,
			strconv.FormatFloat(m.CR, 'g', -1, 32),
			m.HP, m.AC, m.Attack, m.AttackBonus, m.Speed,
			strconv.FormatFloat(m.BlockRate, 'g', -1, 64),
			m.XPValue, m.Notes)
	}
	b.WriteString("\t}\n}\n")
	return b.Bytes()
}
