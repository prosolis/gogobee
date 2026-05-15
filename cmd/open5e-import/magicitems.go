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
	"strings"
	"time"
)

// open5eMagicItem is the subset of the Open5e v1 magicitems schema we consume.
// The vendored JSON keeps the full payload; unknown fields are ignored here.
type open5eMagicItem struct {
	Slug               string `json:"slug"`
	Name               string `json:"name"`
	Type               string `json:"type"`
	Desc               string `json:"desc"`
	Rarity             string `json:"rarity"`
	RequiresAttunement string `json:"requires_attunement"`
}

type magicItemsPage struct {
	Next    string            `json:"next"`
	Results []open5eMagicItem `json:"results"`
}

// fetchMagicItems pages through the SRD magic-item list and writes the full
// result set to data/open5e/magicitems.json, pretty-printed for a reviewable
// diff.
func fetchMagicItems() error {
	client := &http.Client{Timeout: 30 * time.Second}
	var all []open5eMagicItem
	url := magicItemsAPIBase
	for url != "" {
		fmt.Fprintln(os.Stderr, "GET", url)
		page, err := getMagicItemsPage(client, url)
		if err != nil {
			return err
		}
		all = append(all, page.Results...)
		url = page.Next
	}
	fmt.Fprintf(os.Stderr, "fetched %d magic items\n", len(all))

	if err := os.MkdirAll(filepath.Dir(magicItemsJSON), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(magicItemsJSON, out, 0o644)
}

func getMagicItemsPage(client *http.Client, url string) (*magicItemsPage, error) {
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
	var page magicItemsPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// ── Classifier ───────────────────────────────────────────────────────────────

// genMagicItem is a fully classified magic item ready for code emission. Fields
// mirror plugin.MagicItem; enum-typed fields hold the Go identifier as a string.
type genMagicItem struct {
	ID         string
	Name       string
	Kind       string // plugin.MagicItemKind identifier
	Rarity     string // plugin.DnDRarity identifier
	Attunement bool
	Slot       string // plugin.DnDSlot identifier; "" = not equippable / ambiguous
	Value      int
	Desc       string
}

// rarityEnum maps an Open5e rarity string to the plugin's DnDRarity identifier.
// "artifact" folds into Legendary (the plugin has no artifact tier); "varies"
// and unknown/blank values fall back to Common — the conservative bucket.
var rarityEnum = map[string]string{
	"common":    "RarityCommon",
	"uncommon":  "RarityUncommon",
	"rare":      "RarityRare",
	"very rare": "RarityVeryRare",
	"legendary": "RarityLegendary",
	"artifact":  "RarityLegendary",
}

// rarityValue is the sell-value midpoint per rarity, aligned with the §8.1
// rarity brackets the zone-loot tables already use. Keyed by the Open5e rarity
// string so the classifier can read it directly.
var rarityValue = map[string]int{
	"common":    50,
	"uncommon":  150,
	"rare":      750,
	"very rare": 2500,
	"legendary": 8000,
	"artifact":  8000,
}

// kindByPrefix maps the leading word of Open5e's free-text Type field
// ("Wondrous item", "Weapon (any sword)", "Armor (medium or heavy)") to a
// plugin.MagicItemKind identifier.
var kindByPrefix = map[string]string{
	"wondrous": "MagicItemWondrous",
	"weapon":   "MagicItemWeapon",
	"armor":    "MagicItemArmor",
	"ring":     "MagicItemRing",
	"wand":     "MagicItemWand",
	"rod":      "MagicItemRod",
	"staff":    "MagicItemStaff",
	"potion":   "MagicItemPotion",
	"scroll":   "MagicItemScroll",
}

// genMagicItems reads the vendored magicitems.json, classifies every SRD magic
// item, and writes the generated Go data file.
func genMagicItems() error {
	raw, err := os.ReadFile(magicItemsJSON)
	if err != nil {
		return fmt.Errorf("read %s (run `fetch magicitems` first?): %w", magicItemsJSON, err)
	}
	var src []open5eMagicItem
	if err := json.Unmarshal(raw, &src); err != nil {
		return err
	}

	items := make([]genMagicItem, 0, len(src))
	var attunement, slotted int
	for _, m := range src {
		g := classifyMagicItem(m)
		if g.Attunement {
			attunement++
		}
		if g.Slot != "" {
			slotted++
		}
		items = append(items, g)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	fmt.Fprintf(os.Stderr, "classified %d magic items (%d need attunement, %d slotted)\n",
		len(items), attunement, slotted)

	out := emitMagicItems(items)
	if err := os.WriteFile(magicItemsGenGo, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "wrote", magicItemsGenGo)
	return nil
}

func classifyMagicItem(m open5eMagicItem) genMagicItem {
	g := genMagicItem{
		ID:         strings.ReplaceAll(m.Slug, "-", "_"),
		Name:       stripNameParenthetical(m.Name),
		Attunement: strings.TrimSpace(m.RequiresAttunement) != "",
		Desc:       magicItemDescription(strings.ReplaceAll(m.Slug, "-", "_"), strings.TrimSpace(m.Desc)),
	}

	rarity := strings.ToLower(strings.TrimSpace(m.Rarity))
	if enum, ok := rarityEnum[rarity]; ok {
		g.Rarity = enum
	} else {
		g.Rarity = "RarityCommon"
	}
	if v, ok := rarityValue[rarity]; ok {
		g.Value = v
	} else {
		g.Value = rarityValue["common"]
	}

	// Kind: first word of the Type field. Anything unrecognised (the SRD has a
	// handful of bare or oddly-prefixed entries) falls back to Wondrous.
	kindWord := ""
	if fields := strings.Fields(strings.ToLower(m.Type)); len(fields) > 0 {
		kindWord = fields[0]
	}
	if enum, ok := kindByPrefix[kindWord]; ok {
		g.Kind = enum
	} else {
		g.Kind = "MagicItemWondrous"
	}

	g.Slot = inferSlot(g.Kind, m.Name)
	return g
}

// inferSlot makes a best-effort guess at the equip slot. Weapons/staves go to
// the main hand, wands/rods to the off hand, armor to the chest, rings to a
// ring slot. Wondrous items are name-sniffed (cloak, boots, amulet, …) and
// left blank when nothing matches. Potions and scrolls are consumables and
// never carry a slot.
func inferSlot(kind, name string) string {
	switch kind {
	case "MagicItemWeapon", "MagicItemStaff":
		return "DnDSlotMainHand"
	case "MagicItemWand", "MagicItemRod":
		return "DnDSlotOffHand"
	case "MagicItemArmor":
		return "DnDSlotChest"
	case "MagicItemRing":
		return "DnDSlotRing1"
	case "MagicItemPotion", "MagicItemScroll":
		return ""
	}
	// Wondrous item: sniff the name for a wearable noun.
	low := strings.ToLower(name)
	for _, m := range []struct {
		kw   string
		slot string
	}{
		{"amulet", "DnDSlotAmulet"}, {"necklace", "DnDSlotAmulet"},
		{"medallion", "DnDSlotAmulet"}, {"periapt", "DnDSlotAmulet"},
		{"brooch", "DnDSlotAmulet"}, {"pendant", "DnDSlotAmulet"},
		{"ring", "DnDSlotRing1"},
		{"cloak", "DnDSlotChest"}, {"cape", "DnDSlotChest"},
		{"mantle", "DnDSlotChest"}, {"robe", "DnDSlotChest"},
		{"armor", "DnDSlotChest"}, {"vestments", "DnDSlotChest"},
		{"boots", "DnDSlotFeet"}, {"slippers", "DnDSlotFeet"},
		{"gauntlet", "DnDSlotHands"}, {"gloves", "DnDSlotHands"},
		{"bracers", "DnDSlotHands"},
		{"helm", "DnDSlotHead"}, {"hat", "DnDSlotHead"},
		{"crown", "DnDSlotHead"}, {"circlet", "DnDSlotHead"},
		{"goggles", "DnDSlotHead"}, {"eyes", "DnDSlotHead"},
		{"headband", "DnDSlotHead"},
	} {
		if strings.Contains(low, m.kw) {
			return m.slot
		}
	}
	return ""
}

// magicItemDescription returns the Desc text emitted for an item. Override
// wins; otherwise the SRD first-sentence runs through cleanDesc.
func magicItemDescription(id, raw string) string {
	if s, ok := magicItemDescOverride[id]; ok {
		return s
	}
	return firstSentence(cleanDesc(raw), 200)
}

// ── Code emission ────────────────────────────────────────────────────────────

func emitMagicItems(items []genMagicItem) []byte {
	var b bytes.Buffer
	b.WriteString(`// Code generated by cmd/open5e-import. DO NOT EDIT.
//
// Source: Open5e SRD magic-item dump (data/open5e/magicitems.json), 5e SRD
// content under CC-BY-4.0 — see NOTICE. Regenerate with:
//
//	go run ./cmd/open5e-import fetch magicitems
//	go run ./cmd/open5e-import gen magicitems
//
// Kind/Rarity/Slot/Value come from a free-text classifier; the hand-authored
// magicItemOverlay overlays this slice and wins on ID collision, so any
// misclassification here is correctable there rather than in the dump.

package plugin

func buildSRDMagicItems() []MagicItem {
	return []MagicItem{
`)
	for _, m := range items {
		b.WriteString("\t\t{")
		fmt.Fprintf(&b, "ID: %q, Name: %q, Kind: %s, Rarity: %s", m.ID, m.Name, m.Kind, m.Rarity)
		if m.Attunement {
			b.WriteString(", Attunement: true")
		}
		if m.Slot != "" {
			fmt.Fprintf(&b, ", Slot: %s", m.Slot)
		}
		fmt.Fprintf(&b, ", Value: %d", m.Value)
		if m.Desc != "" {
			fmt.Fprintf(&b, ", Desc: %q", m.Desc)
		}
		b.WriteString("},\n")
	}
	b.WriteString("\t}\n}\n")
	return b.Bytes()
}
