package plugin

import (
	"strings"
	"testing"
)

// Pure-logic tests for the Phase 2 auto-harvest helpers. The full
// autoHarvestRoom path needs the prod DB (loads zone runs / expeditions /
// region state) and lives behind the same setupAuditTestDB gate as the
// other expedition tests.

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
	// Plural "fails" when fails>1 — Josie semantics make this common.
	s.Fails = 3
	got = renderAutoHarvestFooter(s)
	if !strings.Contains(got, "3 fails") {
		t.Errorf("expected plural 'fails', got %q", got)
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
