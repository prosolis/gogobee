package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestRegionsForZone_OnlyMultiRegion(t *testing.T) {
	for _, z := range []ZoneID{ZoneUnderdark, ZoneDragonsLair, ZoneAbyssPortal} {
		rs := regionsForZone(z)
		if len(rs) != 4 {
			t.Errorf("zone %s: got %d regions, want 4", z, len(rs))
		}
		if !IsMultiRegionZone(z) {
			t.Errorf("IsMultiRegionZone(%s) = false", z)
		}
		// Verify last region marks IsZoneBoss; first does not.
		if rs[0].IsZoneBoss {
			t.Errorf("zone %s: first region marked IsZoneBoss", z)
		}
		if !rs[len(rs)-1].IsZoneBoss {
			t.Errorf("zone %s: last region not marked IsZoneBoss", z)
		}
		// Verify Order is 1..N.
		for i, r := range rs {
			if r.Order != i+1 {
				t.Errorf("zone %s: regions[%d].Order = %d", z, i, r.Order)
			}
			if r.ZoneID != z {
				t.Errorf("zone %s: region %s has ZoneID %s", z, r.ID, r.ZoneID)
			}
		}
	}
	// Non-multi-region zone returns nil.
	if rs := regionsForZone(ZoneSunkenTemple); rs != nil {
		t.Errorf("ZoneSunkenTemple regions = %v, want nil", rs)
	}
	if IsMultiRegionZone(ZoneSunkenTemple) {
		t.Error("ZoneSunkenTemple should not be multi-region")
	}
}

func TestRegionLookups(t *testing.T) {
	if r, ok := firstRegion(ZoneUnderdark); !ok || r.ID != "underdark_surface_tunnels" {
		t.Errorf("firstRegion underdark = %+v / %v", r, ok)
	}
	if r, ok := nextRegion(ZoneUnderdark, "underdark_surface_tunnels"); !ok || r.ID != "underdark_drow_outpost" {
		t.Errorf("nextRegion = %+v / %v", r, ok)
	}
	if _, ok := nextRegion(ZoneUnderdark, "underdark_deep_throne"); ok {
		t.Error("nextRegion past last should be false")
	}
	if _, ok := findRegion(ZoneUnderdark, "no_such_region"); ok {
		t.Error("findRegion of missing id returned ok")
	}
}

func TestCurrentRegion_DefaultsToFirst(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@region-cur:example.org")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneUnderdark, "", ExpeditionSupplies{Max: 10, Current: 10, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}
	r, ok := CurrentRegion(exp)
	if !ok {
		t.Fatal("CurrentRegion returned !ok")
	}
	if r.ID != "underdark_surface_tunnels" {
		t.Errorf("CurrentRegion default = %s, want underdark_surface_tunnels", r.ID)
	}

	// Single-region zone returns false.
	cleanupExpeditions(uid)
	exp2, err := startExpedition(uid, ZoneSunkenTemple, "", ExpeditionSupplies{Max: 10, Current: 10, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := CurrentRegion(exp2); ok {
		t.Error("CurrentRegion on single-region zone returned ok")
	}
}

func TestRegionStateLists_PersistAndRoundTrip(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@region-state:example.org")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneUnderdark, "", ExpeditionSupplies{Max: 10, Current: 10, DailyBurn: 1})
	if err != nil {
		t.Fatal(err)
	}

	if changed, err := addRegionListEntry(exp, regionStateClearedKey, "underdark_surface_tunnels"); err != nil || !changed {
		t.Fatalf("first add: changed=%v err=%v", changed, err)
	}
	if changed, err := addRegionListEntry(exp, regionStateClearedKey, "underdark_surface_tunnels"); err != nil || changed {
		t.Errorf("duplicate add: changed=%v err=%v (want false/nil)", changed, err)
	}
	if !IsRegionCleared(exp, "underdark_surface_tunnels") {
		t.Error("IsRegionCleared after add = false")
	}

	// Round-trip through DB.
	loaded, err := getActiveExpedition(uid)
	if err != nil || loaded == nil {
		t.Fatalf("reload: %v / %v", loaded, err)
	}
	if !IsRegionCleared(loaded, "underdark_surface_tunnels") {
		t.Error("IsRegionCleared after reload = false (state did not persist)")
	}
	if IsRegionCleared(loaded, "underdark_drow_outpost") {
		t.Error("IsRegionCleared for unwritten region = true")
	}
	if HasBaseCampAt(loaded, "underdark_drow_outpost") {
		t.Error("HasBaseCampAt unset = true")
	}
}
