package plugin

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strings"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// The Shadow (N6/D3) — a per-player simulated rival adventurer. It "runs" the
// same zone progression the player does, advanced once per UTC day by the
// midnight ticker at ~1.3x the player's own long-run clear pace, so it stays
// just ahead: a rival you can always see and always nearly catch.
//
// It is pure theatre. There is no Shadow combat, no punishment, no state the
// player can lose. The only mechanics are race pressure (a morning-briefing
// line telling you where it is relative to you) and two payoffs at each zone
// clear: beat the Shadow to a zone and TwinBee crows a little bonus XP; let the
// Shadow clear it first and it leaves a journal page waiting in the boss room
// (the D1 campaign tie-in). All Shadow narration TwinBee speaks is first-person,
// he/him, implicit-subject — the campaign voice rules (feedback_twinbee_voice,
// feedback_twinbee_is_male). The Shadow itself is referred to as "the Shadow"
// (or by its name); "he" in TwinBee's lines is TwinBee, never a third-person
// "TwinBee".

const (
	// shadowPaceMultiplier keeps the Shadow just ahead: it clears at ~1.3x the
	// player's own long-run pace.
	shadowPaceMultiplier = 1.3
	// shadowMinDailyStep is the floor on a day's advance, so a brand-new or idle
	// player still watches the rival creep forward rather than sit frozen.
	shadowMinDailyStep = 0.2
	// shadowMaxDailyStep caps a day's advance at just under one zone, so even a
	// prolific player never sees the Shadow clear two zones overnight.
	shadowMaxDailyStep = 1.0
	// shadowMaxLead caps how far ahead the Shadow may run — it is a race, not a
	// runaway. It will never sit more than ~2.5 zones past the player's own count.
	shadowMaxLead = 2.5
	// shadowCrowXPPerTier scales the "you beat me here" bonus by zone tier, so a
	// T5 crow is worth more than a T1 one. Small on purpose (lift, not a spike).
	shadowCrowXPPerTier = 12
)

// shadowState mirrors the adventure_shadow row.
type shadowState struct {
	UserID       id.UserID
	Name         string
	Progress     float64
	ZonesCleared int
	PendingMask  int64
	CrowedMask   int64
	DayCounter   int
	LastAdvanced string
}

// The Shadow walks zoneOrder — the design-doc order the zone registry is built
// in — start to finish, tier 1 → 5. Bit i of a Shadow's masks is zoneOrder[i].

// shadowZoneIndex returns the progression index of a zone, or -1 if it is not
// on the path (the arena, an unknown id).
func shadowZoneIndex(z ZoneID) int {
	for i, id := range zoneOrder {
		if id == z {
			return i
		}
	}
	return -1
}

func shadowSetBit(mask int64, idx int) int64 {
	if idx < 0 || idx > 62 {
		return mask
	}
	return mask | (int64(1) << idx)
}

func shadowClearBit(mask int64, idx int) int64 {
	if idx < 0 || idx > 62 {
		return mask
	}
	return mask &^ (int64(1) << idx)
}

func shadowBitSet(mask int64, idx int) bool {
	if idx < 0 || idx > 62 {
		return false
	}
	return mask&(int64(1)<<idx) != 0
}

// loadShadow reads a player's Shadow, or (nil, nil) if it hasn't been born yet.
func loadShadow(userID id.UserID) (*shadowState, error) {
	s := &shadowState{UserID: userID}
	err := db.Get().QueryRow(
		`SELECT name, progress, zones_cleared, pending_mask, crowed_mask, day_counter, last_advanced
		   FROM adventure_shadow WHERE user_id = ?`,
		string(userID),
	).Scan(&s.Name, &s.Progress, &s.ZonesCleared, &s.PendingMask, &s.CrowedMask, &s.DayCounter, &s.LastAdvanced)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// upsertShadow persists the whole Shadow row. The ticker is the only writer, so
// a plain last-write-wins upsert is safe — there is no concurrent mutator to
// race (unlike the player_meta columns a character save also touches).
func upsertShadow(s *shadowState) error {
	_, err := db.Get().Exec(
		`INSERT INTO adventure_shadow
		   (user_id, name, progress, zones_cleared, pending_mask, crowed_mask, day_counter, last_advanced)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   name = excluded.name,
		   progress = excluded.progress,
		   zones_cleared = excluded.zones_cleared,
		   pending_mask = excluded.pending_mask,
		   crowed_mask = excluded.crowed_mask,
		   day_counter = excluded.day_counter,
		   last_advanced = excluded.last_advanced`,
		string(s.UserID), s.Name, s.Progress, s.ZonesCleared, s.PendingMask,
		s.CrowedMask, s.DayCounter, s.LastAdvanced,
	)
	return err
}

// shadowNames is the pool the rival's proper name is drawn from — cold,
// half-familiar names for a thing that shadows you. Picked deterministically
// from the player's own name so a given player always faces the same Shadow.
var shadowNames = []string{
	"Vael", "Corriss", "Mordane", "Ashen", "Locke", "Grimma", "Sable",
	"Thane", "Wren", "Vesper", "Cael", "Orrin", "Dain", "Wraithe",
	"Nyx", "Solenne",
}

// shadowNameFor seeds the Shadow's name from the player's display name so it is
// stable per player and "seeded from the player's" (D3 spec). Falls back to the
// user id when the display name is empty.
func shadowNameFor(displayName string, userID id.UserID) string {
	seed := strings.ToLower(strings.TrimSpace(displayName))
	if seed == "" {
		seed = string(userID)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	return shadowNames[int(h.Sum32())%len(shadowNames)]
}

// newShadow mints a fresh Shadow at the start line.
func newShadow(userID id.UserID, displayName string) *shadowState {
	return &shadowState{
		UserID: userID,
		Name:   shadowNameFor(displayName, userID),
	}
}

// advanceShadow moves a player's Shadow forward one UTC day. Idempotent per day
// via last_advanced, so the once-a-day midnight guard plus this makes a double
// tick a no-op. Mints the Shadow on first call. Errors are logged and swallowed
// — a stalled Shadow is cosmetic, and must never break the midnight reset loop.
func (p *AdventurePlugin) advanceShadow(char *AdventureCharacter) {
	today := time.Now().UTC().Format("2006-01-02")
	s, err := loadShadow(char.UserID)
	if err != nil {
		slog.Warn("shadow: load", "user", char.UserID, "err", err)
		return
	}
	if s == nil {
		s = newShadow(char.UserID, char.DisplayName)
	}
	if s.LastAdvanced == today {
		return // already advanced this UTC day
	}

	cleared := clearedZoneIDs(db.Get(), char.UserID)
	playerCount := len(cleared)

	// The player's long-run pace, in zones per day, drives the Shadow's speed.
	// Floor it so a new/idle player still gets a creeping rival; the 1.3x keeps
	// the Shadow just ahead; cap the daily step so it never leaps two zones.
	step := (float64(playerCount) / float64(shadowPlayerAgeDays(char))) * shadowPaceMultiplier
	if step < shadowMinDailyStep {
		step = shadowMinDailyStep
	}
	if step > shadowMaxDailyStep {
		step = shadowMaxDailyStep
	}

	newProg := s.Progress + step
	// Keep it a race: never more than shadowMaxLead zones past the player's count.
	if leadCap := float64(playerCount) + shadowMaxLead; newProg > leadCap {
		newProg = leadCap
	}
	if maxProg := float64(len(zoneOrder)); newProg > maxProg {
		newProg = maxProg
	}
	if newProg < s.Progress {
		newProg = s.Progress // never regress (a lead cap that dropped below current)
	}

	// Any progression zone the Shadow newly finishes, and the player hasn't,
	// leaves a page waiting. Compare per-zone (not by count): the Shadow walks
	// in order while the player may clear out of order, so "did the Shadow clear
	// zone i first" is a per-zone question.
	newCleared := int(newProg)
	if newCleared > len(zoneOrder) {
		newCleared = len(zoneOrder)
	}
	for idx := s.ZonesCleared; idx < newCleared; idx++ {
		if !cleared[zoneOrder[idx]] {
			s.PendingMask = shadowSetBit(s.PendingMask, idx)
		}
	}

	s.Progress = newProg
	s.ZonesCleared = newCleared
	s.DayCounter++
	s.LastAdvanced = today
	if err := upsertShadow(s); err != nil {
		slog.Warn("shadow: advance upsert", "user", char.UserID, "err", err)
	}
}

// shadowPlayerAgeDays estimates how many days the player has been adventuring,
// clamped to at least 1 so a same-day character never divides by zero. Used as
// the denominator of the player's pace.
func shadowPlayerAgeDays(char *AdventureCharacter) float64 {
	if char.CreatedAt.IsZero() {
		return 1
	}
	days := time.Since(char.CreatedAt).Hours() / 24
	if days < 1 {
		return 1
	}
	return days
}

// shadowOnPlayerZoneClear is the payoff, fired when the player clears a zone's
// boss. Three outcomes: the Shadow left a page here (pending bit) → grant the
// page and clear the bit; the player beat the Shadow to this zone → a crow plus
// a little bonus XP; or the Shadow already passed through with nothing owed →
// nothing. Returns the narration line to append to the clear message ("" for
// the silent case, or when the Shadow hasn't been born yet).
func (p *AdventurePlugin) shadowOnPlayerZoneClear(userID id.UserID, zoneID ZoneID) string {
	s, err := loadShadow(userID)
	if err != nil || s == nil {
		return ""
	}
	idx := shadowZoneIndex(zoneID)
	if idx < 0 {
		return ""
	}

	if shadowBitSet(s.PendingMask, idx) {
		// The Shadow got here first and left a page. Grant it BEFORE retiring the
		// debt: grantJournalPage returns "" both when the campaign is already
		// complete and on a transient DB error, so clear the pending bit only once
		// the page has actually landed (or the ledger is genuinely full). A
		// transient failure then leaves the bit set and the next clear retries,
		// rather than swallowing a page the player earned.
		pageLine := p.grantJournalPage(userID, nil)
		mask, _ := loadJournalPages(userID)
		if pageLine == "" && !journalComplete(mask) {
			slog.Warn("shadow: waiting page grant failed; leaving debt for retry",
				"user", userID, "zone", zoneID)
			return ""
		}
		s.PendingMask = shadowClearBit(s.PendingMask, idx)
		if err := upsertShadow(s); err != nil {
			// The page is already granted; a failed bit-clear can at worst re-grant
			// a different unfound page on a later re-clear, which is benign (pages
			// are collectible and the campaign self-limits). Better than losing it.
			slog.Warn("shadow: clear pending bit", "user", userID, "zone", zoneID, "err", err)
		}
		if pageLine == "" {
			// Ledger already complete — nothing to leave, but the Shadow was first.
			return shadowBeatYouHereLine(s)
		}
		return shadowLeftPageLine(s) + "\n" + pageLine
	}

	// No page owed. If the Shadow hasn't reached this zone yet, the player beat
	// it here — crow and hand over a small bonus, but only once per zone: the
	// crowed bit makes re-running a not-yet-reached zone unable to farm the XP.
	if int(s.Progress) <= idx && !shadowBitSet(s.CrowedMask, idx) {
		xp := shadowCrowXP(zoneID)
		if _, err := p.grantDnDXP(userID, xp); err != nil {
			// Don't promise XP we didn't grant, and don't burn the crow — leave the
			// bit unset so the next clear can crow for real.
			slog.Warn("shadow: crow xp", "user", userID, "err", err)
			return ""
		}
		s.CrowedMask = shadowSetBit(s.CrowedMask, idx)
		if err := upsertShadow(s); err != nil {
			slog.Warn("shadow: set crowed bit", "user", userID, "zone", zoneID, "err", err)
		}
		return shadowCrowLine(s, xp)
	}
	return ""
}

// shadowCrowXP is the bonus for beating the Shadow to a zone, scaled by tier.
func shadowCrowXP(zoneID ZoneID) int {
	tier := 1
	if z, ok := getZone(zoneID); ok {
		tier = int(z.Tier)
	}
	if tier < 1 {
		tier = 1
	}
	return tier * shadowCrowXPPerTier
}

// shadowBriefingLine is the morning-briefing race-pressure beat: where the
// Shadow stands relative to the player. Deterministic by day so a re-rendered
// briefing reads the same, and silent until the Shadow has been born and has
// something to say. TwinBee's voice.
func (p *AdventurePlugin) shadowBriefingLine(e *Expedition) string {
	userID := id.UserID(e.UserID)
	s, err := loadShadow(userID)
	if err != nil || s == nil {
		return ""
	}
	playerCount := len(clearedZoneIDs(db.Get(), userID))
	lead := s.ZonesCleared - playerCount
	return shadowRacePressureLine(s, e.CurrentDay, lead)
}

// ── Flavour ────────────────────────────────────────────────────────────────
//
// TwinBee's voice: first-person, implicit subject, he/him, one line. "The
// Shadow"/"{name}" is the rival; "I"/"we" is TwinBee and the player. Never the
// third-person "TwinBee" (guarded by test, same as the journal reactions).

// shadowAheadLines — TwinBee, the Shadow is ahead of us. Race pressure.
var shadowAheadLines = []string{
	"Passed the Shadow's camp at dawn — cold ashes, a day old. %s is ahead of us again.",
	"Found %s's mark cut into a doorframe up ahead. Still warm. We're not gaining.",
	"%s came through here already. I can smell the pitch of a torch not long snuffed.",
	"The Shadow's been and gone. %s leaves the doors open behind, like he wants us to know.",
}

// shadowBehindLines — TwinBee, we're ahead of the Shadow. Earned swagger.
var shadowBehindLines = []string{
	"No sign of %s ahead — I think we're out in front for once. Let's keep it that way.",
	"We've outpaced the Shadow. %s is somewhere behind us, eating our dust. Good.",
	"Quiet up ahead. %s hasn't reached this deep yet. First ones through.",
}

// shadowNeckLines — TwinBee, we're level with the Shadow. Tension.
var shadowNeckLines = []string{
	"%s is right on our heels — or we're on his. Hard to say who's chasing who now.",
	"Neck and neck with the Shadow. I keep catching %s at the edge of the torchlight.",
	"%s is close. Same rooms, hours apart. Whoever clears the next zone first wins the day.",
}

// shadowRacePressureLine picks a race-pressure line by the day and the Shadow's
// lead, deterministically. lead > 0: Shadow ahead; lead < 0: player ahead; 0:
// level.
func shadowRacePressureLine(s *shadowState, day, lead int) string {
	var pool []string
	switch {
	case lead > 0:
		pool = shadowAheadLines
	case lead < 0:
		pool = shadowBehindLines
	default:
		pool = shadowNeckLines
	}
	if len(pool) == 0 {
		return ""
	}
	idx := (day + s.DayCounter) % len(pool)
	if idx < 0 {
		idx = -idx
	}
	return "👤 " + fmt.Sprintf(pool[idx], s.Name)
}

// shadowLeftPageLine prefaces the waiting journal page: the Shadow cleared this
// zone first and left a page behind for us. TwinBee's voice.
func shadowLeftPageLine(s *shadowState) string {
	return fmt.Sprintf("👤 %s beat us to this one — but left a torn page pinned in the boss's lair, like a note for whoever came next.", s.Name)
}

// shadowBeatYouHereLine is the page-less acknowledgement (campaign already
// complete): the Shadow was first, but there's nothing left to find.
func shadowBeatYouHereLine(s *shadowState) string {
	return fmt.Sprintf("👤 %s was here before us. Boot-prints in the dust, and nothing left to take. We know how this ends now anyway.", s.Name)
}

// shadowCrowLine is TwinBee crowing that we beat the Shadow to a zone, with the
// bonus XP.
func shadowCrowLine(s *shadowState, xp int) string {
	return fmt.Sprintf("👤 First ones through — %s is still somewhere behind us. I'd crow if it wouldn't give us away. (**+%d XP**, for beating the Shadow here.)", s.Name, xp)
}

// handleShadowCmd renders the player's standing against the Shadow. A quiet
// status view — the feature lives mostly in the briefing and at zone clears.
func (p *AdventurePlugin) handleShadowCmd(ctx MessageContext) error {
	s, err := loadShadow(ctx.Sender)
	if err != nil {
		return p.SendReply(ctx.RoomID, ctx.EventID, "Couldn't read the trail just now. Try again in a moment.")
	}
	if s == nil {
		return p.SendDM(ctx.Sender, "👤 No one's shadowing you yet. Clear a zone or two and someone will take an interest.")
	}
	playerCount := len(clearedZoneIDs(db.Get(), ctx.Sender))
	total := len(zoneOrder)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("👤 **The Shadow — %s**\n\n", s.Name))
	b.WriteString(fmt.Sprintf("🏴 **%s's trail:** %d / %d zones cleared\n", s.Name, s.ZonesCleared, total))
	b.WriteString(fmt.Sprintf("🚩 **Your trail:** %d / %d zones cleared\n", playerCount, total))
	switch lead := s.ZonesCleared - playerCount; {
	case lead > 0:
		b.WriteString(fmt.Sprintf("\n_%s is **%d** ahead. Pages wait where he cleared first — take them back by clearing those zones yourself._", s.Name, lead))
	case lead < 0:
		b.WriteString(fmt.Sprintf("\n_You're **%d** ahead. Keep it that way._", -lead))
	default:
		b.WriteString("\n_Dead level. The next zone decides who's chasing whom._")
	}
	return p.SendDM(ctx.Sender, b.String())
}
