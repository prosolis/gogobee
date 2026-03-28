package plugin

import (
	"strings"
	"testing"
	"time"
)

func TestEasterDate(t *testing.T) {
	tests := []struct {
		year  int
		month time.Month
		day   int
	}{
		{2024, time.March, 31},
		{2025, time.April, 20},
		{2026, time.April, 5},
		{2027, time.March, 28},
		{2028, time.April, 16},
	}

	for _, tc := range tests {
		got := easterDate(tc.year)
		want := time.Date(tc.year, tc.month, tc.day, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("Easter %d: got %s, want %s", tc.year, got.Format("2006-01-02"), want.Format("2006-01-02"))
		}
	}
}

func TestNthMonday(t *testing.T) {
	tests := []struct {
		desc  string
		year  int
		month time.Month
		n     int
		want  string
	}{
		{"Family Day 2026", 2026, time.February, 3, "2026-02-16"},
		{"Labour Day 2026", 2026, time.September, 1, "2026-09-07"},
		{"Thanksgiving CA 2026", 2026, time.October, 2, "2026-10-12"},
		{"Family Day 2027", 2027, time.February, 3, "2027-02-15"},
	}

	for _, tc := range tests {
		got := nthMonday(tc.year, tc.month, tc.n)
		if got.Format("2006-01-02") != tc.want {
			t.Errorf("%s: got %s, want %s", tc.desc, got.Format("2006-01-02"), tc.want)
		}
	}
}

func TestMondayBeforeMay25(t *testing.T) {
	tests := []struct {
		year int
		want string
	}{
		{2026, "2026-05-18"},
		{2027, "2027-05-24"},
		{2028, "2028-05-22"},
	}

	for _, tc := range tests {
		got := mondayBeforeMay25(tc.year)
		if got.Format("2006-01-02") != tc.want {
			t.Errorf("Victoria Day %d: got %s, want %s", tc.year, got.Format("2006-01-02"), tc.want)
		}
	}
}

// ── Hebrew Calendar Tests ───────────────────────────────────────────────────

func TestHebrewRoshHashanah(t *testing.T) {
	// Verified against hebcal.com
	tests := []struct {
		year int
		want string
	}{
		{2023, "2023-09-16"}, // 5784
		{2024, "2024-10-03"}, // 5785
		{2025, "2025-09-23"}, // 5786
		{2026, "2026-09-12"}, // 5787
		{2027, "2027-10-02"}, // 5788
		{2028, "2028-09-21"}, // 5789
		{2030, "2030-09-28"}, // 5791
	}

	for _, tc := range tests {
		got := hebrewRoshHashanah(tc.year)
		if got.Format("2006-01-02") != tc.want {
			t.Errorf("Rosh Hashanah %d: got %s, want %s", tc.year, got.Format("2006-01-02"), tc.want)
		}
	}
}

func TestHebrewYomKippur(t *testing.T) {
	tests := []struct {
		year int
		want string
	}{
		{2026, "2026-09-21"},
		{2027, "2027-10-11"},
	}

	for _, tc := range tests {
		got := hebrewYomKippur(tc.year)
		if got.Format("2006-01-02") != tc.want {
			t.Errorf("Yom Kippur %d: got %s, want %s", tc.year, got.Format("2006-01-02"), tc.want)
		}
	}
}

func TestHebrewPassover(t *testing.T) {
	// 15 Nisan (first full day of Pesach)
	tests := []struct {
		year int
		want string
	}{
		{2024, "2024-04-23"},
		{2025, "2025-04-13"},
		{2026, "2026-04-02"},
		{2027, "2027-04-22"},
	}

	for _, tc := range tests {
		got := hebrewPassover(tc.year)
		if got.Format("2006-01-02") != tc.want {
			t.Errorf("Passover %d: got %s, want %s", tc.year, got.Format("2006-01-02"), tc.want)
		}
	}
}

func TestHebrewHanukkah(t *testing.T) {
	// 25 Kislev (first full day)
	tests := []struct {
		year int
		want string
	}{
		{2024, "2024-12-26"},
		{2025, "2025-12-15"},
		{2026, "2026-12-05"},
		{2027, "2027-12-25"},
	}

	for _, tc := range tests {
		got := hebrewHanukkah(tc.year)
		if got.Format("2006-01-02") != tc.want {
			t.Errorf("Hanukkah %d: got %s, want %s", tc.year, got.Format("2006-01-02"), tc.want)
		}
	}
}

// ── Tabular Islamic Calendar Tests ──────────────────────────────────────────

func TestIslamicEidAlFitr(t *testing.T) {
	// Tabular Islamic calendar — ±1 day from observed
	tests := []struct {
		year int
		want string
	}{
		{2026, "2026-03-20"},
		{2027, "2027-03-10"},
	}

	for _, tc := range tests {
		dates := tabularIslamicDates(tc.year, 10, 1)
		if len(dates) == 0 {
			t.Errorf("Eid al-Fitr %d: no dates returned", tc.year)
			continue
		}
		got := dates[0].Format("2006-01-02")
		if got != tc.want {
			t.Errorf("Eid al-Fitr %d: got %s, want %s", tc.year, got, tc.want)
		}
	}
}

func TestIslamicEidAlAdha(t *testing.T) {
	tests := []struct {
		year int
		want string
	}{
		{2026, "2026-05-27"},
		{2027, "2027-05-17"}, // Tabular Islamic calendar ±1 day from observed
	}

	for _, tc := range tests {
		dates := tabularIslamicDates(tc.year, 12, 10)
		if len(dates) == 0 {
			t.Errorf("Eid al-Adha %d: no dates returned", tc.year)
			continue
		}
		got := dates[0].Format("2006-01-02")
		if got != tc.want {
			t.Errorf("Eid al-Adha %d: got %s, want %s", tc.year, got, tc.want)
		}
	}
}

// ── Integration Tests ───────────────────────────────────────────────────────

func TestFloatingHolidays_AllComputed(t *testing.T) {
	for year := 2024; year <= 2035; year++ {
		holidays := floatingHolidays(year)
		if len(holidays) < 14 { // at least: 4 Easter + Nowruz + 4 Canadian + 4 Hebrew + 2 Islamic
			t.Errorf("year %d: only %d floating holidays, expected at least 14", year, len(holidays))
		}

		// Verify no zero-time dates (would indicate a computation failure)
		for _, h := range holidays {
			if h.Date.IsZero() {
				t.Errorf("year %d: holiday %q has zero date", year, h.Name)
			}
			if h.Date.Year() != year {
				t.Errorf("year %d: holiday %q falls in %d", year, h.Name, h.Date.Year())
			}
		}
	}
}

func TestFloatingHolidays_DateCollisions(t *testing.T) {
	holidays := floatingHolidays(2026)
	seen := make(map[string][]string)
	for _, h := range holidays {
		key := h.Date.Format("2006-01-02")
		seen[key] = append(seen[key], h.Name)
	}
	for date, names := range seen {
		if len(names) > 1 {
			t.Logf("NOTE: %d holidays on %s: %s", len(names), date, strings.Join(names, ", "))
		}
	}
}

func TestFixedHolidays_Christmas(t *testing.T) {
	found := false
	for _, h := range majorHolidays {
		if h.Month == 12 && h.Day == 25 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Christmas not in majorHolidays")
	}
}

func TestFixedHolidays_NoDuplicateMonthDay(t *testing.T) {
	seen := make(map[[2]int]string)
	for _, h := range majorHolidays {
		key := [2]int{h.Month, h.Day}
		if prev, ok := seen[key]; ok {
			t.Errorf("duplicate fixed holiday on %d/%d: %s and %s", h.Month, h.Day, prev, h.Name)
		}
		seen[key] = h.Name
	}
}

func TestHolidaySecondPromptRender(t *testing.T) {
	char := &AdventureCharacter{
		DisplayName:   "TestPlayer",
		CombatLevel:   5,
		MiningSkill:   3,
		ForagingSkill: 2,
	}
	equip := map[EquipmentSlot]*AdvEquipment{
		SlotWeapon: {Tier: 1, Condition: 100, Name: "Iron Sword"},
	}
	bonuses := &AdvBonusSummary{}

	text := renderAdvHolidaySecondPrompt(char, equip, bonuses)

	if !strings.Contains(text, "Action 1 complete") {
		t.Error("second prompt should mention action 1 complete")
	}
	if !strings.Contains(text, "second action") {
		t.Error("second prompt should mention second action")
	}
	if !strings.Contains(text, "Dungeon") {
		t.Error("second prompt should list Dungeon option")
	}
}

func TestDailySummary_HolidayBlock(t *testing.T) {
	players := []AdvPlayerDaySummary{
		{DisplayName: "Alice", Activity: "dungeon", Location: "Cellar", Outcome: "success", LootValue: 500, HolidayActions: 2},
		{DisplayName: "Bob", Activity: "mine", Location: "Quarry", Outcome: "success", LootValue: 300, HolidayActions: 1},
		{DisplayName: "Carol", IsDead: true, DeadUntil: "14:00 UTC", Activity: "dungeon", Location: "Cave", Outcome: "death", HolidayActions: 1},
	}

	text := renderAdvDailySummary("2026-12-25", nil, TwinBeeRewardSummary{}, players, "Christmas")

	if !strings.Contains(text, "Christmas") {
		t.Error("summary should contain holiday name")
	}
	if !strings.Contains(text, "two actions today") {
		t.Error("summary should mention two actions on holidays")
	}
	if !strings.Contains(text, "before their second action") {
		t.Error("summary should note Carol died before second action")
	}

	// Without holiday
	textNoHol := renderAdvDailySummary("2026-12-26", nil, TwinBeeRewardSummary{}, players, "")
	if strings.Contains(textNoHol, "two actions") {
		t.Error("non-holiday summary should NOT mention two actions")
	}
}

// ── JDN Roundtrip Test ──────────────────────────────────────────────────────

func TestGregorianFromJDN_Roundtrip(t *testing.T) {
	// Verify known JDN ↔ Gregorian conversions
	tests := []struct {
		jdn  int
		date string
	}{
		{2451545, "2000-01-01"}, // J2000.0
		{2440588, "1970-01-01"}, // Unix epoch
		{2299161, "1582-10-15"}, // Gregorian calendar adoption
	}

	for _, tc := range tests {
		got := gregorianFromJDN(tc.jdn)
		if got.Format("2006-01-02") != tc.date {
			t.Errorf("JDN %d: got %s, want %s", tc.jdn, got.Format("2006-01-02"), tc.date)
		}
	}
}
