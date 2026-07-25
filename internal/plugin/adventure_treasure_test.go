package plugin

import (
	"testing"

	"gogobee/internal/db"
)

// N1/A1 — treasure drops were orphaned by the Phase R transition and now
// hang off zone combat. The weight is what makes that safe: expedition
// combat rolls far more often than the one-a-day legacy activity the base
// rates were tuned for, so only boss/elite moments get a multiplier.

func TestAdvTreasureDropRate_ScalesWithWeight(t *testing.T) {
	// A roll under the (weighted) rate reaches the duplicate check, which
	// reads the treasure table.
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	const user = "@weight:example.org"
	base := advTreasureDropRates[1]

	cases := []struct {
		name   string
		weight float64
		want   float64
	}{
		{"standard kill", advTreasureWeightStandard, base},
		{"elite doubles", advTreasureWeightElite, base * 2},
		{"boss quadruples", advTreasureWeightBoss, base * 4},
		{"zone clear is one bonus roll", advTreasureWeightZoneClear, base},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, rate := rollAdvTreasureDropDetailed(1, user, 0, c.weight)
			if rate != c.want {
				t.Fatalf("rate for weight %v = %v, want %v", c.weight, rate, c.want)
			}
		})
	}
}

func TestAdvTreasureDropRate_ZeroWeightNeverDrops(t *testing.T) {
	for i := 0; i < 200; i++ {
		drop, _, rate := rollAdvTreasureDropDetailed(1, "@zero:example.org", 0, 0)
		if drop != nil || rate != 0 {
			t.Fatalf("weight 0 produced drop=%v rate=%v", drop, rate)
		}
	}
}

// A forced roll must actually yield a treasure — this is the "the seam is
// live again" assertion. A weight above 1/rate drives the effective rate
// past 1.0, so every roll lands on the drop path.
func TestAdvTreasureDropDetailed_ForcedRollGrantsTreasure(t *testing.T) {
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}

	drop, _, _ := rollAdvTreasureDropDetailed(1, "@forced:example.org", 0, 1000)
	if drop == nil || drop.Def == nil {
		t.Fatal("forced roll produced no treasure")
	}
	if drop.Def.Tier != 1 {
		t.Fatalf("tier-1 roll produced a tier-%d treasure", drop.Def.Tier)
	}
}

// TestAdvTreasureTier6_PoolStillMissing pins the known tier 6 gap: Mythic zones
// carry a drop rate but no treasure pool, so a forced roll yields nothing and
// the player is told nothing either (a "just missed" DM for an unwinnable
// treasure would be a lie).
//
// This test is the reminder. Writing advAllTreasures[6] — see TODO(t6-treasures)
// in adventure_treasure.go — turns it red, and the fix is to delete it and
// assert the tier 6 drop instead, the way ForcedRollGrantsTreasure does for
// tier 1.
func TestAdvTreasureTier6_PoolStillMissing(t *testing.T) {
	if err := db.Init(t.TempDir()); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	if _, ok := advAllTreasures[6]; ok {
		t.Fatal("a tier 6 treasure pool now exists — drop this test and assert the drop instead")
	}
	// The rate is configured and waiting, so the gap is the pool alone.
	if advTreasureDropRates[6] == 0 {
		t.Error("tier 6 drop rate went missing; a Mythic pool would be unreachable")
	}
	drop, roll, rate := rollAdvTreasureDropDetailed(6, "@mythic:example.org", 0, 1000)
	if drop != nil {
		t.Fatalf("tier 6 produced a drop from an empty pool: %+v", drop.Def)
	}
	// Zeroed, not merely no-drop: this is what keeps the near-miss DM quiet.
	if roll != 0 || rate != 0 {
		t.Errorf("empty pool reported roll=%v rate=%v, want 0/0 so no near-miss fires", roll, rate)
	}
}

// The treasure and masterwork systems predate zones and speak AdvLocation.
func TestAdvLocForZone(t *testing.T) {
	loc := advLocForZone(ZoneGoblinWarrens)
	if loc.Activity != AdvActivityDungeon {
		t.Errorf("activity = %q, want dungeon", loc.Activity)
	}
	if want := zoneTierFromID(ZoneGoblinWarrens); loc.Tier != want {
		t.Errorf("tier = %d, want %d", loc.Tier, want)
	}
	if loc.Name == "" || loc.Name == string(ZoneGoblinWarrens) {
		t.Errorf("name = %q, want the zone's display name", loc.Name)
	}
}

// TestMaxTreasuresForTier pins the T3 "trophy room" 4th slot: the base cap is
// 3, and only a tier-3+ home lifts it to 4.
func TestMaxTreasuresForTier(t *testing.T) {
	for tier, want := range map[int]int{0: 3, 1: 3, 2: 3, 3: 4, 4: 4} {
		if got := maxTreasuresForTier(tier); got != want {
			t.Errorf("maxTreasuresForTier(%d) = %d, want %d", tier, got, want)
		}
	}
}
