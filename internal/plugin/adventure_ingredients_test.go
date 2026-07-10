package plugin

import (
	"fmt"
	"testing"
)

// N1/A3 — every crafting ingredient must be obtainable from some live drop
// source. Phase R retired the legacy activity loop, which orphaned
// generateAdvLoot (reachable only from the now-uncalled resolveDungeonAction)
// and with it every ingredient in the game. rollZoneIngredient reconnects the
// legacy tables to zone combat; this test is the guard that keeps them
// reachable.

// liveIngredientSources returns every item name a player can obtain from the
// loot tables zone combat draws on, keyed to the tier it drops at.
func liveIngredientSources() map[string][]int {
	sources := map[string][]int{}
	for _, act := range advIngredientActivities {
		table := advLootTable(act)
		for tier, defs := range table {
			for _, d := range defs {
				sources[d.Name] = append(sources[d.Name], tier)
			}
		}
	}
	return sources
}

func TestEveryRecipeIngredientHasALiveSource(t *testing.T) {
	sources := liveIngredientSources()
	for _, recipe := range craftingRecipes {
		for _, ing := range recipe.Ingredients {
			if tiers, ok := sources[ing]; !ok || len(tiers) == 0 {
				t.Errorf("recipe %q needs %q, which no live loot table drops",
					recipe.Result, ing)
			}
		}
	}
}

// The drop is keyed by zone tier, so an ingredient that only exists at
// legacy tier N is only reachable in a tier-N zone. Assert each recipe is
// completable by someone — i.e. every ingredient sits in tier 1..5.
func TestIngredientTiersAreReachableByZoneTier(t *testing.T) {
	sources := liveIngredientSources()
	for _, recipe := range craftingRecipes {
		for _, ing := range recipe.Ingredients {
			for _, tier := range sources[ing] {
				if tier < 1 || tier > 5 {
					t.Errorf("%q drops at tier %d, outside the zone tier range",
						ing, tier)
				}
			}
		}
	}
}

// Every zone tier must be able to produce an ingredient, or that tier's
// players are cut out of crafting entirely.
func TestRollZoneIngredient_EveryTierCanDrop(t *testing.T) {
	for tier := 1; tier <= 5; tier++ {
		t.Run(fmt.Sprintf("tier%d", tier), func(t *testing.T) {
			got := false
			for i := 0; i < 500 && !got; i++ {
				if item := rollZoneIngredient(tier); item != nil {
					got = true
					if item.Tier != tier {
						t.Errorf("tier %d drop carried tier %d", tier, item.Tier)
					}
					if item.Value <= 0 {
						t.Errorf("%q dropped with value %d", item.Name, item.Value)
					}
				}
			}
			if !got {
				t.Errorf("tier %d never dropped an ingredient in 500 rolls", tier)
			}
		})
	}
}

func TestJoinLootLines_SkipsEmpties(t *testing.T) {
	if got := joinLootLines("", ""); got != "" {
		t.Errorf("all-empty = %q, want empty", got)
	}
	if got := joinLootLines("a", ""); got != "a" {
		t.Errorf("trailing empty = %q, want %q", got, "a")
	}
	if got := joinLootLines("", "b"); got != "b" {
		t.Errorf("leading empty = %q, want %q", got, "b")
	}
	if got := joinLootLines("a", "b"); got != "a\nb" {
		t.Errorf("both = %q, want %q", got, "a\nb")
	}
}
