// Command open5e-import vendors Open5e SRD data (CC-BY-4.0) into the repo and
// regenerates the Go data files the plugin compiles against.
//
// It is a pull-once / use-many tool — the generated files are committed, so
// the running bot has no runtime dependency on the Open5e API.
//
//	open5e-import fetch spells     # vendor data/open5e/spells.json from the API
//	open5e-import gen spells       # classify → internal/plugin/dnd_spells_srd_data.go
//	open5e-import fetch bestiary   # vendor data/open5e/monsters.json from the API
//	open5e-import gen bestiary     # classify → internal/plugin/bestiary_srd_data.go
//	open5e-import gen tuned        # tuning formula → internal/plugin/bestiary_tuned_data.go
//	open5e-import fetch magicitems # vendor data/open5e/magicitems.json from the API
//	open5e-import gen magicitems   # classify → internal/plugin/magic_items_srd_data.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	spellsAPIBase = "https://api.open5e.com/v1/spells/?document__slug=wotc-srd&limit=50"
	spellsJSON    = "data/open5e/spells.json"
	spellsGenGo   = "internal/plugin/dnd_spells_srd_data.go"

	monstersAPIBase = "https://api.open5e.com/v1/monsters/?document__slug=wotc-srd&limit=50"
	monstersJSON    = "data/open5e/monsters.json"
	bestiaryGenGo   = "internal/plugin/bestiary_srd_data.go"
	tunedGenGo      = "internal/plugin/bestiary_tuned_data.go"

	magicItemsAPIBase = "https://api.open5e.com/v1/magicitems/?document__slug=wotc-srd&limit=50"
	magicItemsJSON    = "data/open5e/magicitems.json"
	magicItemsGenGo   = "internal/plugin/magic_items_srd_data.go"
)

func main() {
	if len(os.Args) < 3 {
		usage()
	}
	verb, noun := os.Args[1], os.Args[2]
	switch verb {
	case "fetch":
		switch noun {
		case "spells":
			must(fetchSpells())
		case "bestiary":
			must(fetchMonsters())
		case "magicitems":
			must(fetchMagicItems())
		default:
			usage()
		}
	case "gen":
		switch noun {
		case "spells":
			must(genSpells())
		case "bestiary":
			must(genBestiary())
		case "tuned":
			must(genTuned())
		case "magicitems":
			must(genMagicItems())
		default:
			usage()
		}
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: open5e-import fetch (spells|bestiary|magicitems) | gen (spells|bestiary|tuned|magicitems)")
	os.Exit(2)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// open5eSpell is the subset of the Open5e v1 spell schema we consume. Unknown
// fields are ignored; the vendored JSON keeps the full payload.
type open5eSpell struct {
	Slug                  string   `json:"slug"`
	Name                  string   `json:"name"`
	Desc                  string   `json:"desc"`
	HigherLevel           string   `json:"higher_level"`
	Range                 string   `json:"range"`
	Material              string   `json:"material"`
	CanBeCastAsRitual     bool     `json:"can_be_cast_as_ritual"`
	Duration              string   `json:"duration"`
	RequiresConcentration bool     `json:"requires_concentration"`
	CastingTime           string   `json:"casting_time"`
	LevelInt              int      `json:"level_int"`
	School                string   `json:"school"`
	// SpellLists is the structured class list, but the SRD dump leaves it
	// incomplete (no paladin entries at all). DndClass is the free-text
	// "Druid, Wizard" field and is the more complete source — mapClasses
	// unions both. Keep both fields vendored.
	SpellLists []string `json:"spell_lists"`
	DndClass   string   `json:"dnd_class"`
}

type spellsPage struct {
	Next    string        `json:"next"`
	Results []open5eSpell `json:"results"`
}

// fetchSpells pages through the SRD spell list and writes the full result set
// to data/open5e/spells.json, pretty-printed for a reviewable diff.
func fetchSpells() error {
	client := &http.Client{Timeout: 30 * time.Second}
	var all []open5eSpell
	url := spellsAPIBase
	for url != "" {
		fmt.Fprintln(os.Stderr, "GET", url)
		page, err := getSpellsPage(client, url)
		if err != nil {
			return err
		}
		all = append(all, page.Results...)
		url = page.Next
	}
	fmt.Fprintf(os.Stderr, "fetched %d spells\n", len(all))

	if err := os.MkdirAll(filepath.Dir(spellsJSON), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(spellsJSON, out, 0o644)
}

func getSpellsPage(client *http.Client, url string) (*spellsPage, error) {
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
	var page spellsPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	return &page, nil
}
