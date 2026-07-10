package plugin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// ── Types ────────────────────────────────────────────────────────────────────

type advActiveEvent struct {
	ID          int64
	UserID      id.UserID
	EventKey    string
	TriggeredAt time.Time
	ExpiresAt   time.Time
}

// ── Event anchors ────────────────────────────────────────────────────────────
// N1/A6 — events used to roll at 0.5%/player/day from a deferred ticker slot,
// which put one sighting in front of a daily player roughly every 200 days.
// They now roll at moments the player is demonstrably present and reading a
// DM: the end-of-day digest, a sale at Thom's, and an arena cashout. Each
// player still sees at most one event per UTC day.
//
// The per-anchor chances below put a player who hits all three at ~1 event
// per week; see TestAnchoredEventWeeklyRate.
const (
	advEventChanceDigest = 0.08
	advEventChanceSell   = 0.05
	advEventChanceArena  = 0.05
)

var (
	advEventFiredMu  sync.Mutex
	advEventFired    map[string]bool // userID -> an event already fired today
	advEventFiredDay string          // "2006-01-02" the map belongs to
)

// claimDailyEventSlot reserves today's single event slot for `userID`.
// Returns false when the player already had one fire today.
func claimDailyEventSlot(userID id.UserID, day string) bool {
	advEventFiredMu.Lock()
	defer advEventFiredMu.Unlock()
	if advEventFiredDay != day {
		advEventFired = make(map[string]bool)
		advEventFiredDay = day
	}
	if advEventFired[string(userID)] {
		return false
	}
	advEventFired[string(userID)] = true
	return true
}

// releaseDailyEventSlot hands the slot back when the trigger bailed before
// anything reached the player, so a later anchor the same day can still fire.
func releaseDailyEventSlot(userID id.UserID) {
	advEventFiredMu.Lock()
	defer advEventFiredMu.Unlock()
	delete(advEventFired, string(userID))
}

// maybeFireAnchoredEvent rolls `chance` at one of the A6 anchor moments and,
// on a hit, triggers the player's one mid-day event for today.
func (p *AdventurePlugin) maybeFireAnchoredEvent(userID id.UserID, chance float64) {
	if rand.Float64() >= chance {
		return
	}
	if !claimDailyEventSlot(userID, time.Now().UTC().Format("2006-01-02")) {
		return
	}
	if !p.tryTriggerEvent(userID) {
		releaseDailyEventSlot(userID)
	}
}

// ── Event Ticker ─────────────────────────────────────────────────────────────

func (p *AdventurePlugin) eventTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			// Expire stale pending events every tick
			expireAdvPendingEvents()

			// Auto-play any combat sessions past their 1h timeout.
			p.reapExpiredCombatSessions()
		}
	}
}

// tryTriggerEvent fires the player's mid-day event. Reports whether an event
// actually reached them — a false return means the caller's daily slot was
// never spent. The trigger roll itself lives in maybeFireAnchoredEvent.
func (p *AdventurePlugin) tryTriggerEvent(userID id.UserID) bool {
	// Load character — must be alive.
	char, err := loadAdvCharacter(userID)
	if err != nil || char == nil || !char.Alive {
		return false
	}

	// Already has an active event?
	active, _ := loadAdvActiveEvent(userID)
	if active != nil {
		return false
	}

	// Mid-fight: a turn-based session locks the run. Don't drop a random
	// overworld event into a live fight — the player can't act on it without
	// finishing the fight first, and the trigger DM talks over the combat feed.
	if hasActiveCombatSession(userID) {
		return false
	}

	// Determine today's activity for filtering
	activityType := advPlayerTodayActivity(userID)

	// Pick an event
	event := advPickRandomEvent(userID, activityType)
	if event == nil {
		return false
	}

	// Insert into DB
	now := time.Now().UTC()
	expiresAt := now.Add(2 * time.Hour)
	eventID, err := insertAdvEvent(userID, event.Key, expiresAt)
	if err != nil {
		slog.Error("adventure: events: failed to insert event", "user", userID, "err", err)
		return false
	}

	slog.Info("adventure: mid-day event triggered", "user", userID, "event", event.Key, "id", eventID)

	displayName, _ := loadDisplayName(userID)

	// DM the player
	triggerDM := advSubstituteFlavor(event.TriggerDM, map[string]string{
		"{name}": displayName,
	})
	if err := p.SendDM(userID, triggerDM); err != nil {
		slog.Error("adventure: events: failed to send trigger DM", "user", userID, "err", err)
	}

	// Post to game room
	gr := gamesRoom()
	if gr != "" {
		roomLine := advSubstituteFlavor(advEventRoomTriggerWrapper, map[string]string{
			"{trigger_room_line}": advSubstituteFlavor(event.TriggerRoomLine, map[string]string{"{name}": displayName}),
		})
		_ = p.SendMessage(id.RoomID(gr), roomLine)
	}
	return true
}

// handleEventRespond processes `!adventure respond`.
func (p *AdventurePlugin) handleEventRespond(ctx MessageContext) error {
	mu := p.advUserLock(ctx.Sender)
	mu.Lock()
	defer mu.Unlock()

	active, err := loadAdvActiveEvent(ctx.Sender)
	if err != nil {
		slog.Error("adventure: events: failed to load active event", "user", ctx.Sender, "err", err)
		return p.SendDM(ctx.Sender, "Something went wrong checking for events. Try again in a moment.")
	}
	if active == nil {
		return p.SendDM(ctx.Sender, "You don't have an active event to respond to.")
	}

	// Look up event definition
	event := advFindEventByKey(active.EventKey)
	if event == nil {
		slog.Error("adventure: events: unknown event key in DB", "key", active.EventKey)
		return p.SendDM(ctx.Sender, "Something went wrong — the event couldn't be found. This shouldn't happen.")
	}

	// Roll gold reward
	gold := event.GoldMin
	if event.GoldMax > event.GoldMin {
		gold += rand.Int64N(event.GoldMax - event.GoldMin + 1)
	}

	// Credit gold (5% community tax)
	net, _ := communityTax(ctx.Sender, float64(gold), 0.05)
	p.euro.Credit(ctx.Sender, net, "adventure_event_"+event.Key)

	// Apply XP if applicable
	xpSkill := event.XPSkill
	if xpSkill == "" && event.XP > 0 {
		// For "any" events, apply XP to whatever they did today
		activityType := advPlayerTodayActivity(ctx.Sender)
		xpSkill = advXPSkill(AdvActivityType(activityType))
	}

	if event.XP > 0 && xpSkill != "" {
		char, err := loadAdvCharacter(ctx.Sender)
		if err == nil && char != nil {
			switch xpSkill {
			case "combat":
				char.CombatXP += event.XP
			case "mining":
				char.MiningXP += event.XP
			case "foraging":
				char.ForagingXP += event.XP
			}
			checkAdvLevelUp(char, xpSkill)
			_ = saveAdvCharacter(char)
			_ = upsertPlayerMetaSkillState(ctx.Sender, skillStateFromAdvChar(char))
		}
	}

	// Mark responded in DB
	markAdvEventResponded(active.ID, gold, event.XP)

	// Load display name for substitutions
	displayName := p.DisplayName(ctx.Sender)

	// Send outcome DM
	goldStr := fmt.Sprintf("%d", gold)
	xpStr := fmt.Sprintf("%d", event.XP)
	outcomeDM := advSubstituteFlavor(event.OutcomeDM, map[string]string{
		"{gold}": goldStr,
		"{xp}":   xpStr,
		"{name}": displayName,
	})
	_ = p.SendDM(ctx.Sender, outcomeDM)

	// Post outcome to game room
	gr := gamesRoom()
	if gr != "" {
		xpSuffix := ""
		if event.XP > 0 {
			xpSuffix = fmt.Sprintf(" · +%d XP", event.XP)
		}
		roomLine := advSubstituteFlavor(advEventRoomOutcomeWrapper, map[string]string{
			"{outcome_room_line}": advSubstituteFlavor(event.OutcomeRoomLine, map[string]string{
				"{name}": displayName,
				"{gold}": goldStr,
			}),
			"{gold}":      goldStr,
			"{xp_suffix}": xpSuffix,
		})
		_ = p.SendMessage(id.RoomID(gr), roomLine)
	}

	return nil
}

// ── Event Selection ──────────────────────────────────────────────────────────

func advPickRandomEvent(userID id.UserID, activityType string) *AdvRandomEvent {
	// Load recent event keys for dedup
	recent, _ := loadAdvRecentEventKeys(userID, 10)
	recentSet := make(map[string]bool, len(recent))
	for _, k := range recent {
		recentSet[k] = true
	}

	// Filter eligible events
	var candidates []int
	for i, e := range advRandomEvents {
		if e.Activity != "any" && e.Activity != activityType {
			continue
		}
		if recentSet[e.Key] {
			continue
		}
		candidates = append(candidates, i)
	}

	if len(candidates) == 0 {
		return nil
	}

	return &advRandomEvents[candidates[rand.IntN(len(candidates))]]
}

func advFindEventByKey(key string) *AdvRandomEvent {
	for i := range advRandomEvents {
		if advRandomEvents[i].Key == key {
			return &advRandomEvents[i]
		}
	}
	return nil
}

// advPlayerTodayActivity returns the activity type string for what the player did today.
func advPlayerTodayActivity(userID id.UserID) string {
	d := db.Get()
	today := time.Now().UTC().Format("2006-01-02")
	var actType string
	err := d.QueryRow(`SELECT activity_type FROM adventure_activity_log
		WHERE user_id = ? AND logged_at >= ? AND logged_at < DATE(?, '+1 day') LIMIT 1`, string(userID), today, today).Scan(&actType)
	if err != nil {
		return ""
	}
	return actType
}

// ── DB Operations ────────────────────────────────────────────────────────────

func insertAdvEvent(userID id.UserID, eventKey string, expiresAt time.Time) (int64, error) {
	d := db.Get()
	res, err := d.Exec(`INSERT INTO adventure_events_log (user_id, event_key, expires_at)
		VALUES (?, ?, ?)`, string(userID), eventKey, expiresAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func loadAdvActiveEvent(userID id.UserID) (*advActiveEvent, error) {
	d := db.Get()
	var e advActiveEvent
	err := d.QueryRow(`SELECT id, user_id, event_key, triggered_at, expires_at
		FROM adventure_events_log
		WHERE user_id = ? AND outcome = 'pending' AND expires_at > CURRENT_TIMESTAMP
		ORDER BY triggered_at DESC LIMIT 1`, string(userID)).Scan(
		&e.ID, &e.UserID, &e.EventKey, &e.TriggeredAt, &e.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func markAdvEventResponded(eventID int64, goldAwarded int64, xpAwarded int) {
	db.Exec("adventure: mark event responded",
		`UPDATE adventure_events_log
		 SET outcome = 'responded', responded_at = CURRENT_TIMESTAMP,
		     gold_awarded = ?, xp_awarded = ?
		 WHERE id = ?`, goldAwarded, xpAwarded, eventID)
}

func expireAdvPendingEvents() {
	db.Exec("adventure: expire pending events",
		`UPDATE adventure_events_log SET outcome = 'expired'
		 WHERE outcome = 'pending' AND expires_at < CURRENT_TIMESTAMP`)
}

func loadAdvRecentEventKeys(userID id.UserID, limit int) ([]string, error) {
	d := db.Get()
	rows, err := d.Query(`SELECT event_key FROM adventure_events_log
		WHERE user_id = ? ORDER BY triggered_at DESC LIMIT ?`, string(userID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
