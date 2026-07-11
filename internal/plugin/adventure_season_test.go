package plugin

import (
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// TestSeasonForDate_ActiveOnAnchor — every season resolves on its own anchor day
// and both window edges (±seasonHalfWidth), and is dark one day past each edge.
func TestSeasonForDate_ActiveOnAnchor(t *testing.T) {
	for _, s := range seasonTable {
		a := s.Anchor(2026)
		for _, off := range []int{-seasonHalfWidth, 0, seasonHalfWidth} {
			got, ok := seasonForDate(a.AddDate(0, 0, off))
			if !ok || got.Key != s.Key {
				t.Errorf("%s: day offset %d → (%v, %q), want (true, %q)",
					s.Key, off, ok, got.Key, s.Key)
			}
		}
		for _, off := range []int{-seasonHalfWidth - 1, seasonHalfWidth + 1} {
			if got, ok := seasonForDate(a.AddDate(0, 0, off)); ok {
				t.Errorf("%s: day offset %d should be dark, got %q", s.Key, off, got.Key)
			}
		}
	}
}

// TestSeasonForDate_WindowsDisjoint — no calendar day resolves to two seasons, so
// the first-match order in seasonForDate never hides overlapping content.
func TestSeasonForDate_WindowsDisjoint(t *testing.T) {
	for y := 2024; y <= 2030; y++ {
		day := date(y, time.January, 1)
		end := date(y+1, time.January, 1)
		for day.Before(end) {
			matches := 0
			for _, s := range seasonTable {
				for _, yr := range []int{day.Year() - 1, day.Year(), day.Year() + 1} {
					a := s.Anchor(yr)
					if !day.Before(a.AddDate(0, 0, -seasonHalfWidth)) &&
						!day.After(a.AddDate(0, 0, seasonHalfWidth)) {
						matches++
						break
					}
				}
			}
			if matches > 1 {
				t.Fatalf("%s resolves to %d seasons (windows must be disjoint)",
					day.Format("2006-01-02"), matches)
			}
			day = day.AddDate(0, 0, 1)
		}
	}
}

// TestSeasonTable_CuratedItemsExist — every curated curio ID is a real registry
// item, so a season's shelf never lists a phantom the buyer can't purchase. Guards
// against typos and registry edits (mirrors TestTemperMaterialsAreObtainable).
func TestSeasonTable_CuratedItemsExist(t *testing.T) {
	for _, s := range seasonTable {
		if len(s.CurioIDs) == 0 {
			t.Errorf("%s has no curated curios", s.Key)
		}
		for _, id := range s.CurioIDs {
			if _, ok := magicItemRegistry[id]; !ok {
				t.Errorf("%s curio %q is not in magicItemRegistry", s.Key, id)
			}
		}
	}
}

// TestSeasonTable_OmenIsNonCombat — a season's themed omen carries an effect and
// only touches the non-combat omen levers, so overriding the weekly rotation with
// it can never move the combat golden or the balance corpus. §E4/§B3.
func TestSeasonTable_OmenIsNonCombat(t *testing.T) {
	for _, s := range seasonTable {
		o := s.Omen
		hasEffect := o.HarvestYieldBonus > 0 || o.SupplyFreebiePacks > 0 ||
			o.StartMoodBonus > 0 || o.ArenaPayoutMult > 1.0 ||
			o.ConsumableChanceMult > 1.0 || o.ThreatDriftReduce > 0
		if !hasEffect {
			t.Errorf("%s omen has no active effect", s.Key)
		}
		if o.Name == "" || o.TwinBee == "" {
			t.Errorf("%s omen missing player-facing copy", s.Key)
		}
	}
}

// TestSeasonTable_VisitorRewardComplete — every season's road visitor has flavor,
// a named keepsake, and a positive keepsake value, so applyAmbientEffect always
// has a real reward to grant.
func TestSeasonTable_VisitorRewardComplete(t *testing.T) {
	for _, s := range seasonTable {
		v := s.Visitor
		if len(v.FlavorPool) == 0 {
			t.Errorf("%s visitor has no flavor", s.Key)
		}
		if v.Keepsake == "" || v.KeepsakeValue <= 0 {
			t.Errorf("%s visitor keepsake incomplete: %q value %d", s.Key, v.Keepsake, v.KeepsakeValue)
		}
		if v.Weight <= 0 {
			t.Errorf("%s visitor has non-positive weight %d", s.Key, v.Weight)
		}
	}
}

// TestActiveSeason_NeutralizedInSim — the balance sim must never observe a season
// through any path; activeSeason honours simOmenDisabled exactly as activeOmen does.
func TestActiveSeason_NeutralizedInSim(t *testing.T) {
	prev := simOmenDisabled
	simOmenDisabled = true
	defer func() { simOmenDisabled = prev }()
	if s, ok := activeSeason(); ok {
		t.Fatalf("activeSeason returned %q while simOmenDisabled", s.Key)
	}
}
