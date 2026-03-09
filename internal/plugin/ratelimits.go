package plugin

import (
	"log/slog"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// RateLimitsPlugin provides rate-limiting utilities for other plugins.
type RateLimitsPlugin struct {
	Base
}

// NewRateLimitsPlugin creates a new rate limits plugin.
func NewRateLimitsPlugin(client *mautrix.Client) *RateLimitsPlugin {
	return &RateLimitsPlugin{
		Base: NewBase(client),
	}
}

func (p *RateLimitsPlugin) Name() string { return "ratelimits" }

func (p *RateLimitsPlugin) Commands() []CommandDef { return nil }

func (p *RateLimitsPlugin) Init() error { return nil }

func (p *RateLimitsPlugin) OnMessage(_ MessageContext) error { return nil }

func (p *RateLimitsPlugin) OnReaction(_ ReactionContext) error { return nil }

// CheckLimit returns true if the user is under the rate limit for the given action.
// Admin users always bypass rate limits.
func (p *RateLimitsPlugin) CheckLimit(userID id.UserID, action string, maxPerDay int) bool {
	if p.IsAdmin(userID) {
		return true
	}

	d := db.Get()
	today := time.Now().UTC().Format("2006-01-02")

	// Atomic check-and-increment: only increment if under limit
	// This avoids the TOCTOU race between SELECT and UPDATE
	_, err := d.Exec(
		`INSERT INTO rate_limits (user_id, action, date, count) VALUES (?, ?, ?, 1)
		 ON CONFLICT(user_id, action, date) DO UPDATE SET count = count + 1
		 WHERE count < ?`,
		string(userID), action, today, maxPerDay,
	)
	if err != nil {
		slog.Error("ratelimits: increment", "err", err)
		return true // Fail open on error
	}

	// Check current count to determine if we're over limit
	var count int
	err = d.QueryRow(
		`SELECT count FROM rate_limits WHERE user_id = ? AND action = ? AND date = ?`,
		string(userID), action, today,
	).Scan(&count)
	if err != nil {
		return true // Fail open
	}

	return count <= maxPerDay
}

// Remaining returns how many uses remain for the given action today.
// Admin users get maxPerDay (effectively unlimited).
func (p *RateLimitsPlugin) Remaining(userID id.UserID, action string, maxPerDay int) int {
	if p.IsAdmin(userID) {
		return maxPerDay
	}

	d := db.Get()
	today := time.Now().UTC().Format("2006-01-02")

	var count int
	err := d.QueryRow(
		`SELECT count FROM rate_limits WHERE user_id = ? AND action = ? AND date = ?`,
		string(userID), action, today,
	).Scan(&count)
	if err != nil {
		count = 0
	}

	remaining := maxPerDay - count
	if remaining < 0 {
		return 0
	}
	return remaining
}
