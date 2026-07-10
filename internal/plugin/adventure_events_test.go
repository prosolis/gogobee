package plugin

import (
	"math/rand/v2"
	"testing"

	"maunium.net/go/mautrix/id"
)

// TestAnchoredEventWeeklyRate pins the A6 rebalance: a player who reads an
// end-of-day digest, sells at Thom's, and cashes out of the arena every day
// should see roughly one mid-day event per week. The old 0.5%/day roll put
// that at one per ~200 days.
//
// The simulation drives its own seeded RNG rather than the package-global
// one, so it measures the policy — the chance constants and the one-event-
// per-day cap — and never flakes on RNG ordering elsewhere in the suite.
func TestAnchoredEventWeeklyRate(t *testing.T) {
	anchors := []float64{advEventChanceDigest, advEventChanceSell, advEventChanceArena}

	const weeks = 20000
	rng := rand.New(rand.NewPCG(0x5EED, 0xA6))
	events := 0
	for range weeks {
		for range 7 {
			firedToday := false
			for _, chance := range anchors {
				if firedToday {
					break // one event per player per UTC day
				}
				if rng.Float64() < chance {
					firedToday = true
				}
			}
			if firedToday {
				events++
			}
		}
	}

	perWeek := float64(events) / weeks
	if perWeek < 0.8 || perWeek > 1.5 {
		t.Errorf("anchored events = %.3f/week, want ~1 (0.8–1.5)", perWeek)
	}
}

// TestClaimDailyEventSlot_OnePerDay guards the cap the rate test assumes.
func TestClaimDailyEventSlot_OnePerDay(t *testing.T) {
	uid := id.UserID("@events-slot:example")
	const day = "2026-07-09"

	advEventFiredMu.Lock()
	advEventFired = nil
	advEventFiredDay = ""
	advEventFiredMu.Unlock()

	if !claimDailyEventSlot(uid, day) {
		t.Fatal("first claim of the day should succeed")
	}
	if claimDailyEventSlot(uid, day) {
		t.Error("second claim on the same day should be refused")
	}
	// A trigger that bailed before reaching the player hands the slot back.
	releaseDailyEventSlot(uid)
	if !claimDailyEventSlot(uid, day) {
		t.Error("claim after release should succeed")
	}
	// A new UTC day resets everyone.
	if !claimDailyEventSlot(uid, "2026-07-10") {
		t.Error("claim on a new day should succeed")
	}
}

// TestClaimDailyEventSlot_PerUser — one player's event doesn't consume another's.
func TestClaimDailyEventSlot_PerUser(t *testing.T) {
	const day = "2026-07-11"
	if !claimDailyEventSlot(id.UserID("@events-a:example"), day) {
		t.Fatal("player A should claim")
	}
	if !claimDailyEventSlot(id.UserID("@events-b:example"), day) {
		t.Error("player B should claim independently of A")
	}
}
