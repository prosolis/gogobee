package plugin

import (
	"fmt"
	"strings"
)

// Phase 12 E6 — Expedition map renderer.
// Spec: gogobee_expedition_system.md §14 (`!map` — "Display expedition
// progress as ASCII region/room map").
//
// For multi-region zones (Underdark / Dragon's Lair / Abyss Portal) the
// output is a two-row region chain on top of the existing per-run room
// map. Single-region zones render just the room map. The room map is
// rendered by renderZoneMap (dnd_zone_cmd.go) — this file only adds the
// region chain layer and the !map / !expedition map command wiring.

// renderRegionChain renders the multi-region progression as
// Region1──Region2──Region3──Region4 with a status row underneath
// (✓ cleared, ▶ here, · pending). Returns "" for single-region zones.
func renderRegionChain(e *Expedition) string {
	regions := regionsForZone(e.ZoneID)
	if len(regions) == 0 {
		return ""
	}
	cur, _ := CurrentRegion(e)
	cleared := regionListFromState(e, regionStateClearedKey)
	camps := regionListFromState(e, regionStateBaseCampKey)

	var top, bot strings.Builder
	for i, r := range regions {
		if i > 0 {
			top.WriteString("──")
			bot.WriteString("  ")
		}
		// Top row: short region label (first letters of each word, max 3).
		top.WriteString(regionGlyph(r.Name))
		// Bottom row: status marker, with ⛺ overlay when a base camp
		// has been pitched there.
		var marker string
		switch {
		case regionListContains(cleared, r.ID):
			marker = "✓"
		case r.ID == cur.ID:
			marker = "▶"
		default:
			marker = "·"
		}
		if regionListContains(camps, r.ID) {
			marker = "⛺"
		}
		bot.WriteString(marker)
	}
	return top.String() + "\n" + bot.String()
}

// regionGlyph builds a 2-3 char abbreviation for a region name. Initials
// of each word, capped at 3 chars. "Surface Tunnels" → "ST",
// "The Deep Throne" → "TDT", "Drow Outpost" → "DO".
func regionGlyph(name string) string {
	parts := strings.Fields(name)
	var b strings.Builder
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		b.WriteByte(p[0])
		if b.Len() >= 3 {
			break
		}
	}
	return strings.ToUpper(b.String())
}

// renderRegionLegend lists the abbreviations used in renderRegionChain so
// the player can decode the glyphs. Empty for single-region zones.
func renderRegionLegend(e *Expedition) string {
	regions := regionsForZone(e.ZoneID)
	if len(regions) == 0 {
		return ""
	}
	parts := make([]string, 0, len(regions))
	for _, r := range regions {
		parts = append(parts, fmt.Sprintf("%s=%s", regionGlyph(r.Name), r.Name))
	}
	return strings.Join(parts, "  ")
}

// ── !map command ────────────────────────────────────────────────────────────

func (p *AdventurePlugin) handleExpeditionMapCmd(ctx MessageContext, _ string) error {
	exp, err := getActiveExpedition(ctx.Sender)
	if err != nil {
		return p.SendDM(ctx.Sender, "Couldn't read expedition state: "+err.Error())
	}
	if exp == nil {
		return p.SendDM(ctx.Sender, "No active expedition. Use `!expedition start <zone>`.")
	}
	zone, _ := getZone(exp.ZoneID)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("🗺 **%s — Expedition Map (Day %d)**\n", zone.Display, exp.CurrentDay))

	chain := renderRegionChain(exp)
	if chain != "" {
		cur, _ := CurrentRegion(exp)
		b.WriteString("```\n")
		b.WriteString(chain)
		b.WriteString("\n```\n")
		b.WriteString(fmt.Sprintf("_Current region: **%s**_\n\n", cur.Name))
	}

	// Per-run room map. The expedition's zone run is the per-region
	// dungeon instance; for single-region zones it's the only run.
	run, _ := getActiveZoneRun(ctx.Sender)
	if run != nil {
		if g, ok := loadZoneGraph(run.ZoneID); ok {
			b.WriteString("```\n")
			b.WriteString(renderZoneGraphMap(g, run))
			b.WriteString("\n```\n")
			b.WriteString("_E=Entry  ?=Exploration  T=Trap  ★=Elite  ☠=Boss  ⚿=Secret · ✓ cleared  ▶ here  · pending  ╳ locked_")
			if path := renderVisitedPath(g, run); path != "" {
				b.WriteString("\n**Path:** " + path)
			}
			if chain != "" {
				b.WriteString("\n_Regions: ")
				b.WriteString(renderRegionLegend(exp))
				b.WriteString(" · ⛺ base camp pitched_")
			}
		} else {
			b.WriteString("_(no room layout — between rooms or pre-entry)_")
			if chain != "" {
				b.WriteString("\n_Regions: ")
				b.WriteString(renderRegionLegend(exp))
				b.WriteString(" · ⛺ base camp pitched_")
			}
		}
	} else {
		b.WriteString("_(no room layout — between rooms or pre-entry)_")
		if chain != "" {
			b.WriteString("\n_Regions: ")
			b.WriteString(renderRegionLegend(exp))
			b.WriteString(" · ⛺ base camp pitched_")
		}
	}
	return p.SendDM(ctx.Sender, b.String())
}
