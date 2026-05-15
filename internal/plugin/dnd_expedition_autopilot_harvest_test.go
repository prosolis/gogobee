package plugin

import (
	"strings"
	"testing"
)

// Pure-logic tests for the Phase 2 auto-harvest helpers. The full
// autoHarvestRoom path needs the prod DB (loads zone runs / expeditions /
// region state) and lives behind the same setupAuditTestDB gate as the
// other expedition tests.

func TestIsRarePlus(t *testing.T) {
	cases := []struct {
		in   DnDRarity
		want bool
	}{
		{RarityCommon, false},
		{RarityUncommon, false},
		{RarityRare, true},
		{RarityEpic, true},
		{RarityVeryRare, true},
		{RarityLegendary, true},
		{"", false},
	}
	for _, c := range cases {
		if got := isRarePlus(c.in); got != c.want {
			t.Errorf("isRarePlus(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRenderAutoHarvestFooter_EmptyIsEmpty(t *testing.T) {
	s := autoHarvestSummary{Yields: map[string]int{}, Names: map[string]string{}}
	if got := renderAutoHarvestFooter(s); got != "" {
		t.Errorf("expected empty footer for empty summary, got %q", got)
	}
}

func TestRenderAutoHarvestFooter_YieldsAndFailsAndNoise(t *testing.T) {
	s := autoHarvestSummary{
		Yields:    map[string]int{"iron": 2, "herb": 1},
		Names:     map[string]string{"iron": "Scrap Iron", "herb": "Shadow Herb"},
		Fails:     1,
		NoiseInts: 2,
	}
	got := renderAutoHarvestFooter(s)
	// Deterministic ordering: yield keys sorted alphabetically.
	for _, want := range []string{"+1 Shadow Herb", "+2 Scrap Iron", "1 fail", "2 close calls"} {
		if !strings.Contains(got, want) {
			t.Errorf("footer %q missing %q", got, want)
		}
	}
	// Singular "close call" when noise=1.
	s.NoiseInts = 1
	got = renderAutoHarvestFooter(s)
	if !strings.Contains(got, "1 close call") || strings.Contains(got, "close calls") {
		t.Errorf("expected singular 'close call', got %q", got)
	}
}

func TestRenderWalkTally_EmptyIsEmpty(t *testing.T) {
	if got := renderWalkTally(map[string]int{}, map[string]string{}); got != "" {
		t.Errorf("expected empty tally, got %q", got)
	}
}

func TestRenderWalkTally_SortsByID(t *testing.T) {
	got := renderWalkTally(
		map[string]int{"zinc": 1, "alpha": 2, "mid": 3},
		map[string]string{"zinc": "Zinc", "alpha": "Alphite", "mid": "Mid Ore"},
	)
	// "alpha" < "mid" < "zinc" so the rendered order is Alphite, Mid Ore, Zinc.
	if !strings.HasPrefix(got, "**Walk haul:** 2 Alphite") {
		t.Errorf("tally not sorted by id: %q", got)
	}
	if iAlpha, iZinc := strings.Index(got, "Alphite"), strings.Index(got, "Zinc"); iAlpha >= iZinc {
		t.Errorf("Alphite should precede Zinc: %q", got)
	}
}

func TestRenderRarePendingFooter_EmptyIsEmpty(t *testing.T) {
	if got := renderRarePendingFooter(nil); got != "" {
		t.Errorf("expected empty rare-pending footer, got %q", got)
	}
}

func TestRenderRarePendingFooter_DedupsByID(t *testing.T) {
	r := ZoneResource{ID: "war_standard", Name: "Hobgoblin War Standard", Rarity: RarityRare, Action: HarvestScavenge}
	got := renderRarePendingFooter([]ZoneResource{r, r, r})
	// Single mention even with 3 copies in the pending slice.
	if strings.Count(got, "Hobgoblin War Standard") != 1 {
		t.Errorf("expected single mention, got %q", got)
	}
	if !strings.Contains(got, "!scavenge") {
		t.Errorf("expected action hint '!scavenge' in footer, got %q", got)
	}
	if !strings.Contains(got, "Rare") {
		t.Errorf("expected rarity label in footer, got %q", got)
	}
}

func TestRenderRarePendingFooter_MultiplePlural(t *testing.T) {
	a := ZoneResource{ID: "a", Name: "Aaa", Rarity: RarityRare, Action: HarvestForage}
	b := ZoneResource{ID: "b", Name: "Bbb", Rarity: RarityVeryRare, Action: HarvestForage}
	got := renderRarePendingFooter([]ZoneResource{a, b})
	if !strings.Contains(got, "are sitting in this room") {
		t.Errorf("expected plural verb for two items, got %q", got)
	}
}
