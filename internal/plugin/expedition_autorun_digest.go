package plugin

import (
	"fmt"
	"log/slog"
	"strings"
)

// Long-expedition D4-a — end-of-day digest renderer.
//
// When the autopilot pitches a Night=true camp the dungeon has effectively
// closed the day. Instead of dripping per-tick auto-walk DMs through the
// player's channel, D4-a suppresses those mid-day surfaces and rolls them
// into a single rollup posted alongside the night-camp block.
//
// Data source: dnd_expedition_log. The autorun ticker now writes a `walk`
// entry per successful background walk; combined with the existing
// harvest / interrupt / threat / milestone / narrative entries this gives
// the digest enough structure to summarize the day in counts + a small
// number of headline lines, without re-rendering the full per-room
// narration that the per-tick stream used to carry.
//
// Tone: terse, TwinBee first-person (feedback_twinbee_voice), no trailing
// recap paragraph (feedback_skip_recaps). The night-camp block follows
// immediately after.

// renderEndOfDayDigest pulls every log entry for (expID, prevDay) and
// returns a markdown rollup. Returns "" when prevDay has no entries —
// the caller should fall back to the bare camp block in that case.
func renderEndOfDayDigest(expID string, prevDay int) string {
	entries, err := dayExpeditionLog(expID, prevDay)
	if err != nil {
		slog.Warn("expedition: digest log fetch", "expedition", expID, "day", prevDay, "err", err)
		return ""
	}
	if len(entries) == 0 {
		return ""
	}
	var (
		walks         int
		harvests      int
		interrupts    int
		threatLines   []string
		milestoneLine []string
		narrativeBits []string
	)
	for _, e := range entries {
		switch e.Type {
		case "walk":
			walks++
		case "harvest":
			// Only count successful gathers — failed rolls / errors are noise.
			if strings.Contains(e.Summary, "success") {
				harvests++
			}
		case "interrupt":
			interrupts++
		case "threat":
			if e.Flavor != "" {
				threatLines = append(threatLines, e.Flavor)
			} else if e.Summary != "" {
				threatLines = append(threatLines, e.Summary)
			}
		case "milestone":
			milestoneLine = append(milestoneLine, e.Summary)
		case "narrative":
			// Surface real narrative beats (camp broken / extraction prompts),
			// skip housekeeping ones.
			if strings.Contains(e.Summary, "autopilot broke camp") {
				continue
			}
			narrativeBits = append(narrativeBits, e.Summary)
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("📜 *Day %d wraps up.*\n\n", prevDay))
	bulleted := false
	if walks > 0 {
		b.WriteString(fmt.Sprintf("• Walked **%d** auto-tick%s through the dark.\n", walks, pluralS(walks)))
		bulleted = true
	}
	if interrupts > 0 {
		b.WriteString(fmt.Sprintf("• Handled **%d** interrupt%s along the way.\n", interrupts, pluralS(interrupts)))
		bulleted = true
	}
	if harvests > 0 {
		b.WriteString(fmt.Sprintf("• Came back with **%d** harvest%s.\n", harvests, pluralS(harvests)))
		bulleted = true
	}
	for _, t := range threatLines {
		b.WriteString("• ")
		b.WriteString(t)
		b.WriteString("\n")
		bulleted = true
	}
	for _, m := range milestoneLine {
		b.WriteString("• ")
		b.WriteString(m)
		b.WriteString("\n")
		bulleted = true
	}
	for _, n := range narrativeBits {
		b.WriteString("• ")
		b.WriteString(n)
		b.WriteString("\n")
		bulleted = true
	}
	if !bulleted {
		// All entries were filtered out — fall back to the bare camp block.
		return ""
	}
	return b.String()
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
