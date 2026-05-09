package plugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gogobee/internal/db"
)

// TestProdDB_DailyReportPreview renders the Adv 2.0 daily summary against a
// copy of the prod DB and prints it. Manual smoke for the D&D-aware
// renderer.
func TestProdDB_DailyReportPreview(t *testing.T) {
	src := "/home/reala-misaki/git/gogobee/data/gogobee.db"
	if _, err := os.Stat(src); err != nil {
		t.Skip("prod db not present")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "gogobee.db")
	copyTestFile(t, src, dst)

	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)

	bootstrapPlayerMetaFromLegacy()

	chars, err := loadAllAdvCharacters()
	if err != nil {
		t.Fatalf("loadAllAdvCharacters: %v", err)
	}

	// Use the most recent activity date in the DB so we render against real
	// data instead of an empty "today".
	now := time.Now().UTC()
	dateKey := now.Format("2006-01-02")
	if acts, _ := loadAdvDailyActivity(dateKey); len(acts) == 0 {
		// No activity today; walk back up to 7 days for a date that has rows.
		for i := 1; i <= 7; i++ {
			cand := now.AddDate(0, 0, -i).Format("2006-01-02")
			if acts, _ := loadAdvDailyActivity(cand); len(acts) > 0 {
				dateKey = cand
				break
			}
		}
	}
	t.Logf("rendering report for date=%s", dateKey)

	todayActs, _ := loadAdvDailyActivity(dateKey)
	activityMap := make(map[string][]AdvDailyActivity)
	for uid, e := range todayActs {
		activityMap[string(uid)] = e
	}

	var players []AdvPlayerDaySummary
	for _, c := range chars {
		dispName, _ := loadDisplayName(c.UserID)
		ps := AdvPlayerDaySummary{
			DisplayName:   dispName,
			Level:         c.CombatLevel,
			MiningSkill:   c.MiningSkill,
			ForagingSkill: c.ForagingSkill,
			FishingSkill:  c.FishingSkill,
		}
		if dnd, _ := LoadDnDCharacter(c.UserID); dnd != nil {
			ps.HasDnDChar = true
			ps.DnDLevel = dnd.Level
			ps.DnDRace = string(dnd.Race)
			ps.DnDClass = string(dnd.Class)
			ps.HPCurrent = dnd.HPCurrent
			ps.HPMax = dnd.HPMax
		}

		acts := activityMap[string(c.UserID)]
		if !c.Alive {
			ps.IsDead = true
			ps.DeathSource = c.DeathSource
			ps.DeathLocation = c.DeathLocation
			if c.DeadUntil != nil {
				ps.DeadUntil = c.DeadUntil.Format("15:04") + " UTC"
			}
		} else if len(acts) == 0 {
			ps.IsResting = true
			ps.SummaryLine = "Stayed in town. The world's still here tomorrow."
		} else {
			last := acts[len(acts)-1]
			ps.Activity = last.Activity
			ps.Location = last.Location
			ps.Outcome = last.Outcome
			ps.SummaryLine = last.Summary
		}
		players = append(players, ps)
	}

	out := renderAdvDailySummary(dateKey, nil, TwinBeeRewardSummary{}, players, "")
	t.Logf("\n%s", out)
}
