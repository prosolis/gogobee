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

// ── In-memory schedule ───────────────────────────────────────────────────────
// Each day, every eligible player is assigned a random minute between 10:00
// and 16:00 UTC at which the 0.5% trigger roll happens.

var (
	advEventScheduleMu sync.Mutex
	advEventSchedule   map[string]int // userID string -> minute-of-day (600..959)
	advEventScheduleDay string        // "2006-01-02" the schedule was built for
)

func advBuildEventSchedule(chars []AdventureCharacter) {
	advEventSchedule = make(map[string]int, len(chars))
	for _, c := range chars {
		if !c.Alive {
			continue
		}
		// Random minute between 600 (10:00) and 959 (15:59)
		advEventSchedule[string(c.UserID)] = 600 + rand.IntN(360)
	}
	advEventScheduleDay = time.Now().UTC().Format("2006-01-02")
}

// ── Event Ticker ─────────────────────────────────────────────────────────────

func (p *AdventurePlugin) eventTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UTC()
		dateKey := now.Format("2006-01-02")

		// Expire stale pending events every tick
		expireAdvPendingEvents()

		// Outside the trigger window — nothing to do
		if now.Hour() < 10 || now.Hour() >= 16 {
			continue
		}

		// Rebuild schedule if it's a new day or uninitialised
		advEventScheduleMu.Lock()
		if advEventScheduleDay != dateKey {
			chars, err := loadAllAdvCharacters()
			if err != nil {
				slog.Error("adventure: events: failed to load chars for schedule", "err", err)
				advEventScheduleMu.Unlock()
				continue
			}
			advBuildEventSchedule(chars)
			slog.Info("adventure: event schedule built", "players", len(advEventSchedule))
		}

		// Find players whose roll minute is now
		currentMinute := now.Hour()*60 + now.Minute()
		var toRoll []id.UserID
		for uid, minute := range advEventSchedule {
			if minute == currentMinute {
				toRoll = append(toRoll, id.UserID(uid))
			}
		}
		advEventScheduleMu.Unlock()

		for _, uid := range toRoll {
			p.tryTriggerEvent(uid)
		}
	}
}

func (p *AdventurePlugin) tryTriggerEvent(userID id.UserID) {
	// Load character — must be alive and have acted today
	char, err := loadAdvCharacter(userID)
	if err != nil || char == nil || !char.Alive || !char.ActionTakenToday {
		return
	}

	// Already has an active event?
	active, _ := loadAdvActiveEvent(userID)
	if active != nil {
		return
	}

	// 0.5% chance
	if rand.Float64() >= 0.005 {
		return
	}

	// Determine today's activity for filtering
	activityType := advPlayerTodayActivity(userID)

	// Pick an event
	event := advPickRandomEvent(userID, activityType)
	if event == nil {
		return
	}

	// Insert into DB
	now := time.Now().UTC()
	expiresAt := now.Add(2 * time.Hour)
	eventID, err := insertAdvEvent(userID, event.Key, expiresAt)
	if err != nil {
		slog.Error("adventure: events: failed to insert event", "user", userID, "err", err)
		return
	}

	slog.Info("adventure: mid-day event triggered", "user", userID, "event", event.Key, "id", eventID)

	// DM the player
	triggerDM := advSubstituteFlavor(event.TriggerDM, map[string]string{
		"{name}": char.DisplayName,
	})
	if err := p.SendDM(userID, triggerDM); err != nil {
		slog.Error("adventure: events: failed to send trigger DM", "user", userID, "err", err)
	}

	// Post to game room
	gr := gamesRoom()
	if gr != "" {
		roomLine := advSubstituteFlavor(advEventRoomTriggerWrapper, map[string]string{
			"{trigger_room_line}": advSubstituteFlavor(event.TriggerRoomLine, map[string]string{"{name}": char.DisplayName}),
		})
		_ = p.SendMessage(id.RoomID(gr), roomLine)
	}
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

	// Credit gold
	p.euro.Credit(ctx.Sender, float64(gold), "adventure_event_"+event.Key)

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
