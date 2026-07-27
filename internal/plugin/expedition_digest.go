package plugin

// Once-a-day cadence (2026-07-26).
//
// Adventure's web feed is now the place to watch a run move minute to minute,
// so the bot no longer narrates every beat into Matrix. The three per-player
// DM sources that fired on a clock — the 06:00 briefing, the 21:00 recap, and
// the 6-hourly ambient event — collapse into a single morning message.
//
// The rule that keeps this honest: only the *messaging* goes quiet. Every
// mechanical effect still fires on exactly the schedule it always did. The
// ambient ticker still applies its ±SU nudges, the recap still runs the night
// wandering check and its threat bump, the briefing still burns supply and
// rolls the day. What changes is that ambient and recap now write their
// outcome to the expedition log and stop there; the next morning's briefing
// reads that log back and reports it.
//
// Interrupt-driven DMs are deliberately untouched. A fork needs a human, a
// death and a run-completion are terminal, and a mischief hit or a rival
// challenge is somebody else acting on you. Those still arrive when they
// happen; batching them to the next morning would either strand a decision
// behind an 8h auto-pick or report a finished story.

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"maunium.net/go/mautrix/id"
)

const (
	// defaultPeteSiteURL — Pete's public site. Distinct from PETE_INGEST_URL,
	// which is the Headscale-only ingest endpoint and is not reachable by a
	// player clicking a link in a DM.
	defaultPeteSiteURL = "https://news.parodia.dev"

	// digestMaxLines — how many prior-day log lines the morning digest
	// carries before it defers to the site. The cap is the whole point of
	// the change: the digest is a teaser for the feed, not a transcript.
	digestMaxLines = 8
)

// peteSiteURL returns the public base URL for Pete's site, without a
// trailing slash. Overridable so a dev instance can point its links at
// a local Pete instead of prod.
func peteSiteURL() string {
	if v := strings.TrimRight(os.Getenv("PETE_PUBLIC_URL"), "/"); v != "" {
		return v
	}
	return defaultPeteSiteURL
}

// adventureFeedURL is the general Adventure feed: everyone's activity.
func adventureFeedURL() string {
	return peteSiteURL() + "/adventure"
}

// adventureWhoURL is the reader's own adventurer page. Keyed by the same
// salted, one-way roster token the board already publishes (see
// pete_roster.go), so a DM link and a board link resolve to one page and
// neither one leaks a Matrix handle.
func adventureWhoURL(uid id.UserID) string {
	if uid == "" {
		return adventureFeedURL()
	}
	return peteSiteURL() + "/adventure/who/" + eventToken(uid, "roster")
}

// digestSiteFooter appends the reader's own site link. Per-reader rather
// than per-expedition: a party shares a briefing body but each member's
// link goes to their own sheet.
func digestSiteFooter(uid id.UserID, body string) string {
	return body + "\n\n🔗 _Watch it live: " + adventureWhoURL(uid) + "_"
}

// briefingPerReader is the per-reader decorator for the one daily message:
// the reader's own pet event on the front, the reader's own site link on the
// back. Both are per-member, so a party's shared briefing body still reaches
// each player personalised at both ends.
func (p *AdventurePlugin) briefingPerReader(uid id.UserID, body string) string {
	return digestSiteFooter(uid, p.briefingPetPrefix(uid, body))
}

// fireDigestEventAnchor rolls N1/A6's digest-anchored mid-day event for each
// member. It used to hang off the autopilot's night-camp digest DM; that DM is
// gone, so it moved here — the briefing is now the message the player is
// demonstrably reading, which is the whole premise of an anchored roll.
//
// Still a per-player roll against a per-player daily slot: a party does not
// share one event.
func (p *AdventurePlugin) fireDigestEventAnchor(e *Expedition) {
	for _, member := range expeditionAudience(e) {
		p.maybeFireAnchoredEvent(member, advEventChanceDigest)
	}
}

// appendOvernightDigest folds the day that just ended into a briefing body.
// A log read failure is non-fatal: the briefing is the player's only daily
// message now, so a missing digest block must never cost them the whole DM.
func appendOvernightDigest(body, expID string, priorDay int) string {
	entries, err := dayLogEntries(expID, priorDay)
	if err != nil {
		slog.Warn("expedition: digest entries", "expedition", expID, "err", err)
		return body
	}
	digest := renderOvernightDigest(entries)
	if digest == "" {
		return body
	}
	return body + "\n" + digest
}

// digestSkipTypes — log entry types the morning digest never echoes.
// `briefing` and `recap` are the frame itself, and the free-narration
// types are the per-room prose the site renders in full.
var digestSkipTypes = map[string]bool{
	"briefing":  true,
	"recap":     true,
	"narrative": true,
	"transit":   true,
	"action":    true,
	"journal":   true,
}

// renderOvernightDigest condenses one expedition-day of log entries into the
// "here is what you missed" block that opens the morning briefing. Walks
// collapse to a count; everything notable keeps its own summary line, capped
// at digestMaxLines with an explicit overflow note so a truncated digest
// never reads as a complete one.
//
// Returns "" when there is nothing worth reporting — a day with only walks
// and narration gets no block at all rather than an empty header.
func renderOvernightDigest(entries []ExpeditionEntry) string {
	var walks int
	var lines []string
	for _, en := range entries {
		if en.Type == "walk" {
			walks++
			continue
		}
		if digestSkipTypes[en.Type] {
			continue
		}
		s := strings.TrimSpace(en.Summary)
		if s == "" {
			continue
		}
		lines = append(lines, s)
	}
	if walks == 0 && len(lines) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("📜 **Since yesterday**\n")
	if walks > 0 {
		b.WriteString(fmt.Sprintf("• walked %s\n", pluralRooms(walks)))
	}
	shown := lines
	overflow := 0
	if len(shown) > digestMaxLines {
		overflow = len(shown) - digestMaxLines
		shown = shown[:digestMaxLines]
	}
	for _, l := range shown {
		b.WriteString("• " + l + "\n")
	}
	if overflow > 0 {
		b.WriteString(fmt.Sprintf("• _...and %d more, on the site._\n", overflow))
	}
	return b.String()
}

// pluralRooms renders a room count with the right noun.
func pluralRooms(n int) string {
	if n == 1 {
		return "1 room"
	}
	return fmt.Sprintf("%d rooms", n)
}
