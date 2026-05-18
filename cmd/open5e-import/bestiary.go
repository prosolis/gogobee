package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// open5eMonster is the subset of the Open5e v1 monster schema we consume. The
// vendored JSON keeps the full payload; unknown fields are ignored here.
type open5eMonster struct {
	Slug             string             `json:"slug"`
	Name             string             `json:"name"`
	Size             string             `json:"size"`
	Type             string             `json:"type"`
	CR               float64            `json:"cr"`
	ArmorClass       int                `json:"armor_class"`
	HitPoints        int                `json:"hit_points"`
	Speed            map[string]any     `json:"speed"`
	Strength         int                `json:"strength"`
	Dexterity        int                `json:"dexterity"`
	Constitution     int                `json:"constitution"`
	Intelligence     int                `json:"intelligence"`
	Wisdom           int                `json:"wisdom"`
	Charisma         int                `json:"charisma"`
	Actions          []open5eAction     `json:"actions"`
	LegendaryActions []open5eAction     `json:"legendary_actions"`
	SpecialAbilities []open5eNamedBlock `json:"special_abilities"`
}

// open5eAction is one entry in a monster's actions list. An entry is a weapon
// attack iff DamageDice is non-empty; Multiattack and utility actions leave it
// blank. AttackBonus/DamageBonus arrive as JSON null on non-attacks, which
// unmarshals to 0 — harmless, since DamageDice is the attack discriminator.
type open5eAction struct {
	Name        string `json:"name"`
	Desc        string `json:"desc"`
	AttackBonus int    `json:"attack_bonus"`
	DamageDice  string `json:"damage_dice"`
	DamageBonus int    `json:"damage_bonus"`
}

type open5eNamedBlock struct {
	Name string `json:"name"`
}

type monstersPage struct {
	Next    string          `json:"next"`
	Results []open5eMonster `json:"results"`
}

// fetchMonsters pages through the SRD monster list and writes the full result
// set to data/open5e/monsters.json, pretty-printed for a reviewable diff.
func fetchMonsters() error {
	client := &http.Client{Timeout: 30 * time.Second}
	var all []open5eMonster
	url := monstersAPIBase
	for url != "" {
		fmt.Fprintln(os.Stderr, "GET", url)
		page, err := getMonstersPage(client, url)
		if err != nil {
			return err
		}
		all = append(all, page.Results...)
		url = page.Next
	}
	fmt.Fprintf(os.Stderr, "fetched %d monsters\n", len(all))

	if err := os.MkdirAll(filepath.Dir(monstersJSON), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(monstersJSON, out, 0o644)
}

func getMonstersPage(client *http.Client, url string) (*monstersPage, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var page monstersPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// ── Staging-table classifier ─────────────────────────────────────────────────

// genStatBlock mirrors plugin.SRDStatBlock — a raw SRD stat block. These values
// are NOT engine-tuned: raw SRD damage one-shots gogobee's solo player (see
// bestiary_srd.go). The staging table is a balance baseline the hand-authored
// dndBestiary / srdProfiles tuning pass reads against, not a drop-in roster.
type genStatBlock struct {
	Slug, Name  string
	Size, Type  string
	CR          float64
	XP          int
	HP, AC      int
	SpeedWalk   int
	STR, DEX    int
	CON, INT    int
	WIS, CHA    int
	Multiattack string
	Attacks     []genStatAttack
	Traits      []string
	Legendary   bool
}

type genStatAttack struct {
	Name        string
	AttackBonus int
	DamageDice  string
	DamageBonus int
	AvgDamage   int
	DamageType  string
}

// xpByCR is the SRD experience-by-challenge-rating table. Open5e exposes no XP
// field, but the tuning pass wants it, so it is derived here from CR.
var xpByCR = map[float64]int{
	0: 10, 0.125: 25, 0.25: 50, 0.5: 100,
	1: 200, 2: 450, 3: 700, 4: 1100, 5: 1800, 6: 2300, 7: 2900, 8: 3900,
	9: 5000, 10: 5900, 11: 7200, 12: 8400, 13: 10000, 14: 11500, 15: 13000,
	16: 15000, 17: 18000, 18: 20000, 19: 22000, 20: 25000, 21: 33000,
	22: 41000, 23: 50000, 24: 62000, 25: 75000, 26: 90000, 27: 105000,
	28: 120000, 29: 135000, 30: 155000,
}

// genBestiary reads the vendored monsters.json, classifies every SRD monster
// into a raw staging stat block, and writes the generated Go data file.
func genBestiary() error {
	raw, err := os.ReadFile(monstersJSON)
	if err != nil {
		return fmt.Errorf("read %s (run `fetch bestiary` first?): %w", monstersJSON, err)
	}
	var src []open5eMonster
	if err := json.Unmarshal(raw, &src); err != nil {
		return err
	}

	blocks := make([]genStatBlock, 0, len(src))
	var attacks int
	for _, m := range src {
		b := classifyMonster(m)
		attacks += len(b.Attacks)
		blocks = append(blocks, b)
	}
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].CR != blocks[j].CR {
			return blocks[i].CR < blocks[j].CR
		}
		return blocks[i].Slug < blocks[j].Slug
	})
	fmt.Fprintf(os.Stderr, "classified %d monsters (%d parsed attacks)\n", len(blocks), attacks)

	out := emitBestiary(blocks)
	if err := os.WriteFile(bestiaryGenGo, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "wrote", bestiaryGenGo)
	return nil
}

func classifyMonster(m open5eMonster) genStatBlock {
	b := genStatBlock{
		Slug: strings.ReplaceAll(m.Slug, "-", "_"),
		Name: m.Name,
		Size: strings.ToLower(strings.TrimSpace(m.Size)),
		Type: strings.ToLower(strings.TrimSpace(m.Type)),
		CR:   m.CR,
		XP:   xpByCR[m.CR],
		HP:   m.HitPoints,
		AC:   m.ArmorClass,
		STR:  m.Strength, DEX: m.Dexterity, CON: m.Constitution,
		INT: m.Intelligence, WIS: m.Wisdom, CHA: m.Charisma,
		SpeedWalk: speedWalk(m.Speed),
		Legendary: len(m.LegendaryActions) > 0,
	}
	for _, a := range m.Actions {
		if strings.EqualFold(strings.TrimSpace(a.Name), "Multiattack") {
			b.Multiattack = firstSentence(strings.TrimSpace(a.Desc), 200)
			continue
		}
		if a.DamageDice == "" {
			continue
		}
		atk := genStatAttack{
			Name:        a.Name,
			AttackBonus: a.AttackBonus,
			DamageDice:  strings.ReplaceAll(a.DamageDice, " ", ""),
			DamageBonus: a.DamageBonus,
		}
		atk.AvgDamage = int(avgDice(atk.DamageDice)) + a.DamageBonus
		if mt := reDmgType.FindStringSubmatch(strings.ToLower(a.Desc)); mt != nil {
			atk.DamageType = strings.ToLower(mt[1])
		}
		b.Attacks = append(b.Attacks, atk)
	}
	for _, sa := range m.SpecialAbilities {
		if n := strings.TrimSpace(sa.Name); n != "" {
			b.Traits = append(b.Traits, n)
		}
	}
	return b
}

// speedWalk pulls the walking speed (in feet) out of Open5e's speed object,
// falling back to the fastest other movement mode for creatures with no walk
// speed (flyers, swimmers). Returns 0 when nothing parses.
func speedWalk(speed map[string]any) int {
	asInt := func(v any) int {
		if f, ok := v.(float64); ok {
			return int(f)
		}
		return 0
	}
	if w := asInt(speed["walk"]); w > 0 {
		return w
	}
	best := 0
	for mode, v := range speed {
		if mode == "walk" {
			continue
		}
		if n := asInt(v); n > best {
			best = n
		}
	}
	return best
}

// avgDice returns the statistical average of an NdM dice expression. A bare or
// unparseable expression yields 0 — the staging table records what it can and
// leaves the rest for the tuning pass.
func avgDice(dice string) float64 {
	i := strings.IndexByte(dice, 'd')
	if i <= 0 {
		return 0
	}
	n, err1 := strconv.Atoi(dice[:i])
	m, err2 := strconv.Atoi(dice[i+1:])
	if err1 != nil || err2 != nil || n <= 0 || m <= 0 {
		return 0
	}
	return float64(n) * (float64(m) + 1) / 2
}

// ── Code emission ────────────────────────────────────────────────────────────

func emitBestiary(blocks []genStatBlock) []byte {
	var b bytes.Buffer
	b.WriteString(`// Code generated by cmd/open5e-import. DO NOT EDIT.
//
// Source: Open5e SRD monster dump (data/open5e/monsters.json), 5e SRD content
// under CC-BY-4.0 — see NOTICE. Regenerate with:
//
//	go run ./cmd/open5e-import fetch bestiary
//	go run ./cmd/open5e-import gen bestiary
//
// This is the RAW SRD staging table — see bestiary_srd_staging.go for why these
// values are a balance baseline and not a drop-in engine roster.

package plugin

func buildSRDStagingBestiary() map[string]SRDStatBlock {
	return map[string]SRDStatBlock{
`)
	for _, m := range blocks {
		fmt.Fprintf(&b, "\t\t%q: {Slug: %q, Name: %q", m.Slug, m.Slug, m.Name)
		if m.Size != "" {
			fmt.Fprintf(&b, ", Size: %q", m.Size)
		}
		if m.Type != "" {
			fmt.Fprintf(&b, ", Type: %q", m.Type)
		}
		fmt.Fprintf(&b, ", CR: %s", strconv.FormatFloat(m.CR, 'g', -1, 64))
		if m.XP != 0 {
			fmt.Fprintf(&b, ", XP: %d", m.XP)
		}
		fmt.Fprintf(&b, ", HP: %d, AC: %d", m.HP, m.AC)
		if m.SpeedWalk != 0 {
			fmt.Fprintf(&b, ", SpeedWalk: %d", m.SpeedWalk)
		}
		fmt.Fprintf(&b, ", STR: %d, DEX: %d, CON: %d, INT: %d, WIS: %d, CHA: %d",
			m.STR, m.DEX, m.CON, m.INT, m.WIS, m.CHA)
		if m.Multiattack != "" {
			fmt.Fprintf(&b, ", Multiattack: %q", m.Multiattack)
		}
		if len(m.Attacks) > 0 {
			b.WriteString(", Attacks: []SRDStatAttack{")
			for i, a := range m.Attacks {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "{Name: %q, AttackBonus: %d, DamageDice: %q, DamageBonus: %d, AvgDamage: %d",
					a.Name, a.AttackBonus, a.DamageDice, a.DamageBonus, a.AvgDamage)
				if a.DamageType != "" {
					fmt.Fprintf(&b, ", DamageType: %q", a.DamageType)
				}
				b.WriteString("}")
			}
			b.WriteString("}")
		}
		if len(m.Traits) > 0 {
			b.WriteString(", Traits: []string{")
			for i, t := range m.Traits {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "%q", t)
			}
			b.WriteString("}")
		}
		if m.Legendary {
			b.WriteString(", Legendary: true")
		}
		b.WriteString("},\n")
	}
	b.WriteString("\t}\n}\n")
	return b.Bytes()
}
