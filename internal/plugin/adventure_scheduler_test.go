package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// TestMidnightReset_Branching exercises the three idle-reaper branches:
//   - engaged: LastActionDate stamped today/yesterday → streak bumps
//   - activity-without-tap: autopilot/background activity logged today but
//     no LastActionDate stamp → streak holds, no shame DM, no decay
//   - truly idle: nothing anywhere → streak halves, StreakDecayed=true
//
// Regression guard for the autopilot bug: a player who let the background
// auto-run walk an expedition all day used to get shamed at midnight because
// markActedToday is gated on !compact at dnd_zone_cmd.go.
func TestMidnightReset_Branching(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)

	today := time.Now().UTC().Format("2006-01-02")
	yesterday := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")

	mk := func(uid, lastAction string, streak int) id.UserID {
		u := id.UserID(uid)
		if err := createAdvCharacter(u, uid); err != nil {
			t.Fatalf("createAdvCharacter %s: %v", uid, err)
		}
		c, err := loadAdvCharacter(u)
		if err != nil {
			t.Fatalf("load %s: %v", uid, err)
		}
		c.LastActionDate = lastAction
		c.CurrentStreak = streak
		c.BestStreak = streak
		if err := saveAdvCharacter(c); err != nil {
			t.Fatalf("save %s: %v", uid, err)
		}
		return u
	}

	engaged := mk("@engaged:example", yesterday, 5)
	autopilot := mk("@autopilot:example", "2020-01-01", 7)
	idle := mk("@idle:example", "2020-01-01", 8)

	// Simulate autopilot activity: a legacy activity-log row dated today.
	// loadAdvDailyActivity unions this in, so the reaper sees the player as
	// "had activity" without their LastActionDate being current.
	if _, err := db.Get().Exec(
		`INSERT INTO adventure_activity_log
		 (user_id, activity_type, location, outcome, loot_value, xp_gained, flavor_key, logged_at)
		 VALUES (?, 'zone', 'Test Zone', 'in_progress', 0, 0, '', CURRENT_TIMESTAMP)`,
		string(autopilot),
	); err != nil {
		t.Fatalf("insert activity row: %v", err)
	}

	p := &AdventurePlugin{}
	if err := p.midnightReset(); err != nil {
		t.Fatalf("midnightReset: %v", err)
	}

	// Engaged → streak bumped, LastActionDate restamped to today, no decay.
	if c, _ := loadAdvCharacter(engaged); c != nil {
		if c.CurrentStreak != 6 {
			t.Errorf("engaged: streak = %d, want 6", c.CurrentStreak)
		}
		if c.LastActionDate != today {
			t.Errorf("engaged: LastActionDate = %q, want %q", c.LastActionDate, today)
		}
		if c.StreakDecayed {
			t.Error("engaged: StreakDecayed = true, want false")
		}
	}

	// Autopilot → hold. Streak unchanged, LastActionDate stays stale, no decay.
	if c, _ := loadAdvCharacter(autopilot); c != nil {
		if c.CurrentStreak != 7 {
			t.Errorf("autopilot: streak = %d, want 7 (held)", c.CurrentStreak)
		}
		if c.StreakDecayed {
			t.Error("autopilot: StreakDecayed = true, want false (shamed by mistake)")
		}
		if c.LastActionDate == today {
			t.Errorf("autopilot: LastActionDate restamped to today — autopilot must not credit streak via LastActionDate")
		}
	}

	// Idle → shame DM (no-op without Matrix client) + streak halved + decay flagged.
	if c, _ := loadAdvCharacter(idle); c != nil {
		if c.CurrentStreak != 4 {
			t.Errorf("idle: streak = %d, want 4 (halved from 8)", c.CurrentStreak)
		}
		if !c.StreakDecayed {
			t.Error("idle: StreakDecayed = false, want true")
		}
	}
}

// TestMidnightReset_DeathWindowSkips: a player who died today/yesterday is
// left untouched — no shame, no streak change, no decay flag.
func TestMidnightReset_DeathWindowSkips(t *testing.T) {
	dir := t.TempDir()
	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)

	today := time.Now().UTC().Format("2006-01-02")

	u := id.UserID("@dead:example")
	if err := createAdvCharacter(u, "dead"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	c, _ := loadAdvCharacter(u)
	c.LastActionDate = "2020-01-01"
	c.CurrentStreak = 10
	c.BestStreak = 10
	c.LastDeathDate = today
	if err := saveAdvCharacter(c); err != nil {
		t.Fatalf("save: %v", err)
	}

	p := &AdventurePlugin{}
	if err := p.midnightReset(); err != nil {
		t.Fatalf("midnightReset: %v", err)
	}

	got, _ := loadAdvCharacter(u)
	if got.CurrentStreak != 10 {
		t.Errorf("dead: streak = %d, want 10 (untouched)", got.CurrentStreak)
	}
	if got.StreakDecayed {
		t.Error("dead: StreakDecayed = true, want false")
	}
}
