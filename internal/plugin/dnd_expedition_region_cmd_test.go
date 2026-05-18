package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestResolveTransitWanderingCheck_NoCampMod(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@transit-noncamp:example.org")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneUnderdark, "", ExpeditionSupplies{Max: 10, Current: 10, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}

	// Force d20 = 14 with no threat mod and no class mod → outcome
	// should be Minor (total=14 → 11..14 bucket).
	nc := resolveTransitWanderingCheck(exp, "", func() int { return 14 })
	if nc.CampMod != 0 {
		t.Errorf("transit campMod = %d, want 0", nc.CampMod)
	}
	if nc.Outcome != NightOutcomeMinor {
		t.Errorf("outcome = %v, want Minor", nc.Outcome)
	}
	// Ranger in wilderness zone gets -2.
	nc = resolveTransitWanderingCheck(exp, ClassRanger, func() int { return 14 })
	if nc.ClassMod != -2 {
		t.Errorf("ranger classMod = %d, want -2", nc.ClassMod)
	}
}

func TestRegionTravel_AdvancesDayBurnsSuppliesAndUpdatesRegion(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@region-travel:example.org")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneUnderdark, "",
		ExpeditionSupplies{Max: 20, Current: 20, DailyBurn: 2, HarshMod: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	startDay := exp.CurrentDay
	startSupplies := exp.Supplies.Current

	// Drive the travel manually (avoid command surface for unit isolation).
	if err := setCurrentRegion(exp, "underdark_surface_tunnels"); err != nil {
		t.Fatal(err)
	}
	cur, _ := CurrentRegion(exp)
	next, _ := nextRegion(exp.ZoneID, cur.ID)
	if next.ID != "underdark_drow_outpost" {
		t.Fatalf("next = %s", next.ID)
	}

	// Mimic regionCmdTravel core: burn one day, advance, mark visited.
	newSupplies, burned := applyDailyBurn(exp.Supplies, false, false)
	// Phase 5-B: applyDailyBurn × phase5B 50%; 2 base × 0.5 = 1.
	if burned != 1 {
		t.Errorf("burned = %v, want 1", burned)
	}
	exp.Supplies = newSupplies
	if err := updateSupplies(exp.ID, exp.Supplies); err != nil {
		t.Fatal(err)
	}
	if err := advanceExpeditionDay(exp.ID); err != nil {
		t.Fatal(err)
	}
	if err := setCurrentRegion(exp, next.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := MarkRegionVisited(exp, next.ID); err != nil {
		t.Fatal(err)
	}

	loaded, _ := getActiveExpedition(uid)
	if loaded.CurrentDay != startDay+1 {
		t.Errorf("day did not advance: %d → %d", startDay, loaded.CurrentDay)
	}
	if loaded.Supplies.Current != startSupplies-1 {
		t.Errorf("supplies after burn = %v, want %v", loaded.Supplies.Current, startSupplies-1)
	}
	if loaded.CurrentRegion != "underdark_drow_outpost" {
		t.Errorf("CurrentRegion = %q", loaded.CurrentRegion)
	}
	if !IsRegionVisited(loaded, "underdark_drow_outpost") {
		t.Error("next region not marked visited")
	}
}

func TestRenderRegionList_MarksCurrentAndCleared(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@region-render:example.org")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneAbyssPortal, "", ExpeditionSupplies{Max: 10, Current: 10, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MarkRegionBossDefeated(exp, "abyss_outer_rift"); err != nil {
		t.Fatal(err)
	}
	if err := setCurrentRegion(exp, "abyss_demon_assembly"); err != nil {
		t.Fatal(err)
	}
	if _, err := MarkRegionVisited(exp, "abyss_demon_assembly"); err != nil {
		t.Fatal(err)
	}

	out := renderRegionList(exp)
	if !strings.Contains(out, "✓ ") {
		t.Errorf("expected ✓ marker for cleared region; got:\n%s", out)
	}
	if !strings.Contains(out, "▶ ") {
		t.Errorf("expected ▶ marker for current region; got:\n%s", out)
	}
	if !strings.Contains(out, "Demon Assembly") || !strings.Contains(out, "Outer Rift") {
		t.Errorf("region names missing in render:\n%s", out)
	}
	if !strings.Contains(out, "★") {
		t.Errorf("expected ★ marker on zone-boss region:\n%s", out)
	}
}
