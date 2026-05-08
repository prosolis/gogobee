package plugin

import (
	"strings"
	"testing"
)

func TestRenderRegionChain_SingleRegionZone(t *testing.T) {
	e := &Expedition{ZoneID: ZoneSunkenTemple}
	if got := renderRegionChain(e); got != "" {
		t.Errorf("expected empty chain for single-region zone, got %q", got)
	}
}

func TestRenderRegionChain_MultiRegionMarkers(t *testing.T) {
	e := &Expedition{
		ZoneID:        ZoneUnderdark,
		CurrentRegion: "underdark_drow_outpost",
		RegionState: map[string]any{
			regionStateClearedKey:  []string{"underdark_surface_tunnels"},
			regionStateBaseCampKey: []string{"underdark_drow_outpost"},
		},
	}
	out := renderRegionChain(e)
	if out == "" {
		t.Fatalf("expected non-empty chain for multi-region zone")
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d (%q)", len(lines), out)
	}
	bot := lines[1]
	// 4 regions: cleared / camp(at-current) / pending / pending
	// Camp marker overrides the ▶ (here) per renderRegionChain.
	if !strings.Contains(bot, "✓") {
		t.Errorf("expected ✓ for cleared region: %q", bot)
	}
	if !strings.Contains(bot, "⛺") {
		t.Errorf("expected ⛺ at base-camp region: %q", bot)
	}
	if !strings.Contains(bot, "·") {
		t.Errorf("expected · for pending region: %q", bot)
	}
}

func TestRenderRegionChain_CurrentMarker(t *testing.T) {
	e := &Expedition{
		ZoneID:        ZoneUnderdark,
		CurrentRegion: "underdark_drow_outpost",
	}
	out := renderRegionChain(e)
	if !strings.Contains(out, "▶") {
		t.Errorf("expected ▶ at current region (no camps yet): %q", out)
	}
}

func TestRegionGlyph(t *testing.T) {
	cases := map[string]string{
		"Surface Tunnels":  "ST",
		"The Deep Throne":  "TDT",
		"Drow Outpost":     "DO",
		"Illithid Warren":  "IW",
		"Kobold Warrens":   "KW",
	}
	for name, want := range cases {
		if got := regionGlyph(name); got != want {
			t.Errorf("regionGlyph(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestRenderRegionLegend_HasAllRegions(t *testing.T) {
	e := &Expedition{ZoneID: ZoneUnderdark}
	got := renderRegionLegend(e)
	for _, want := range []string{"Surface Tunnels", "Drow Outpost", "Illithid Warren", "The Deep Throne"} {
		if !strings.Contains(got, want) {
			t.Errorf("legend missing %q: %s", want, got)
		}
	}
}
