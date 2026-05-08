package plugin

import (
	"strings"
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

// Phase 12 E3a — Sunken Temple tidal event.

// setExpDay rewrites the current_day column for tests.
func setExpDay(t *testing.T, expID string, day int) {
	t.Helper()
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET current_day = ? WHERE expedition_id = ?`,
		day, expID); err != nil {
		t.Fatalf("set current_day: %v", err)
	}
}

func TestSunkenTemple_TidalWarningDay4(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-tidal-d4:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneSunkenTemple, "",
		ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1.5, HarshMod: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	setExpDay(t, exp.ID, 3) // → briefing rolls to Day 4
	exp, _ = getExpedition(exp.ID)

	p := &AdventurePlugin{}
	startSU := exp.Supplies.Current
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	got, _ := getExpedition(exp.ID)
	if got.CurrentDay != 4 {
		t.Fatalf("CurrentDay = %d, want 4", got.CurrentDay)
	}
	// Warning day: NO doubled burn — single 1.5 SU burn.
	if got.Supplies.Current != startSU-1.5 {
		t.Errorf("supplies = %v, want %v (single burn)", got.Supplies.Current, startSU-1.5)
	}
	entries, _ := recentExpeditionLog(exp.ID, 10)
	foundWarn := false
	for _, e := range entries {
		if e.Type == "temporal" && strings.Contains(e.Summary, "tidal warning") {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Error("expected a 'temporal' tidal-warning log entry on Day 4")
	}
}

func TestSunkenTemple_TidalEventDay6_ForcesDoubleBurn(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-tidal-d6:example")
	defer cleanupExpeditions(uid)

	// Tier-2 supplies: HarshMod 1.5 normally. Tidal must force 2×.
	exp, err := startExpedition(uid, ZoneSunkenTemple, "",
		ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1.5, HarshMod: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	setExpDay(t, exp.ID, 5) // → rolls to Day 6
	exp, _ = getExpedition(exp.ID)

	p := &AdventurePlugin{}
	startSU := exp.Supplies.Current
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	got, _ := getExpedition(exp.ID)
	if got.CurrentDay != 6 {
		t.Fatalf("CurrentDay = %d, want 6", got.CurrentDay)
	}
	// Forced 2× burn = 1.5 * 2 = 3.0 SU
	wantBurn := float32(3.0)
	if startSU-got.Supplies.Current != wantBurn {
		t.Errorf("burn = %v, want %v (forced 2× tidal)", startSU-got.Supplies.Current, wantBurn)
	}
	entries, _ := recentExpeditionLog(exp.ID, 10)
	foundEvent := false
	for _, e := range entries {
		if e.Type == "temporal" && strings.Contains(e.Summary, "tidal peak") {
			foundEvent = true
		}
	}
	if !foundEvent {
		t.Error("expected a 'temporal' tidal-peak log entry on Day 6")
	}
}

func TestSunkenTemple_BossDefeatedSilencesTidal(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-tidal-boss:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneSunkenTemple, "",
		ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 1.5, HarshMod: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET boss_defeated = 1, current_day = 5 WHERE expedition_id = ?`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	exp, _ = getExpedition(exp.ID)

	p := &AdventurePlugin{}
	startSU := exp.Supplies.Current
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	got, _ := getExpedition(exp.ID)
	// Single normal burn, no tidal multiplier.
	if got.Supplies.Current != startSU-1.5 {
		t.Errorf("supplies = %v, want %v (no tidal mult)", got.Supplies.Current, startSU-1.5)
	}
	entries, _ := recentExpeditionLog(exp.ID, 10)
	for _, e := range entries {
		if e.Type == "temporal" {
			t.Errorf("did not expect temporal log entry post-boss; got %+v", e)
		}
	}
}

func TestManor_NightlyReset_FiresEveryThreeDays(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-manor-reset:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneManorBlackspire, "",
		ExpeditionSupplies{Current: 60, Max: 60, DailyBurn: 2, HarshMod: 2})
	if err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}

	cases := []struct {
		fromDay   int
		wantReset bool
	}{
		{1, false}, // → Day 2
		{2, true},  // → Day 3
		{3, false}, // → Day 4
		{4, false}, // → Day 5
		{5, true},  // → Day 6
		{8, true},  // → Day 9
	}
	for _, c := range cases {
		setExpDay(t, exp.ID, c.fromDay)
		// Reset last_briefing_at so the next briefing actually fires.
		if _, err := db.Get().Exec(
			`UPDATE dnd_expedition SET last_briefing_at = NULL WHERE expedition_id = ?`, exp.ID); err != nil {
			t.Fatal(err)
		}
		fresh, _ := getExpedition(exp.ID)
		// Drain log entries from prior iterations so we count only today's.
		preCount := manorResetLogCount(t, exp.ID)
		if err := p.deliverBriefing(fresh, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		got := manorResetLogCount(t, exp.ID)
		gained := got - preCount
		if c.wantReset && gained == 0 {
			t.Errorf("from Day %d → expected a manor-reset entry; got none", c.fromDay)
		}
		if !c.wantReset && gained != 0 {
			t.Errorf("from Day %d → expected no reset; got %d new entries", c.fromDay, gained)
		}
	}
}

func manorResetLogCount(t *testing.T, expID string) int {
	t.Helper()
	rows, err := db.Get().Query(`
		SELECT COUNT(*) FROM dnd_expedition_log
		 WHERE expedition_id = ? AND entry_type = 'temporal'
		   AND summary LIKE 'manor reset%'`, expID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		return 0
	}
	var n int
	_ = rows.Scan(&n)
	return n
}

func TestManor_BossDefeatedSilencesReset(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-manor-boss:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneManorBlackspire, "",
		ExpeditionSupplies{Current: 60, Max: 60, DailyBurn: 2, HarshMod: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET boss_defeated = 1, current_day = 2 WHERE expedition_id = ?`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	exp, _ = getExpedition(exp.ID)

	p := &AdventurePlugin{}
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if got := manorResetLogCount(t, exp.ID); got != 0 {
		t.Errorf("boss-defeated manor should not reset; got %d entries", got)
	}
}

func TestManorReset_PredicateOnly(t *testing.T) {
	e := &Expedition{ZoneID: ZoneManorBlackspire}
	for _, d := range []int{0, 1, 2, 4, 5, 7, 8} {
		if ManorReset(e, d) {
			t.Errorf("ManorReset(day %d) = true; want false", d)
		}
	}
	for _, d := range []int{3, 6, 9, 12, 30} {
		if !ManorReset(e, d) {
			t.Errorf("ManorReset(day %d) = false; want true", d)
		}
	}
	// Wrong zone is always false.
	other := &Expedition{ZoneID: ZoneGoblinWarrens}
	if ManorReset(other, 3) {
		t.Error("ManorReset on non-Manor zone should be false")
	}
}

func TestUnderforge_HeatStackIncrementsAndBands(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-uf-heat:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneUnderforge, "",
		ExpeditionSupplies{Current: 60, Max: 60, DailyBurn: 2, HarshMod: 2})
	if err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}

	// Drive 11 mornings — heat should top out at 10.
	for i := 0; i < 11; i++ {
		fresh, _ := getExpedition(exp.ID)
		if _, err := db.Get().Exec(
			`UPDATE dnd_expedition SET last_briefing_at = NULL WHERE expedition_id = ?`,
			exp.ID); err != nil {
			t.Fatal(err)
		}
		// Top up supplies so we don't hit starvation; not relevant here.
		if _, err := db.Get().Exec(
			`UPDATE dnd_expedition SET supplies_json = ? WHERE expedition_id = ?`,
			`{"current":99,"max":99,"daily_burn":2,"harsh_mod":2,"foraged_today":false,"packs_standard":3,"packs_deluxe":0}`,
			exp.ID); err != nil {
			t.Fatal(err)
		}
		fresh, _ = getExpedition(exp.ID)
		if err := p.deliverBriefing(fresh, time.Now().UTC()); err != nil {
			t.Fatalf("briefing %d: %v", i, err)
		}
	}
	got, _ := getExpedition(exp.ID)
	if got.TemporalStack != 10 {
		t.Errorf("heat stack = %d, want 10 (capped)", got.TemporalStack)
	}

	// Verify warning + critical narrations both fired.
	rows, _ := db.Get().Query(
		`SELECT summary FROM dnd_expedition_log WHERE expedition_id = ? AND entry_type = 'temporal' ORDER BY entry_id`,
		exp.ID)
	defer rows.Close()
	var summaries []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		summaries = append(summaries, s)
	}
	want := []string{"warning band", "+50%", "exhaustion"}
	for _, w := range want {
		found := false
		for _, s := range summaries {
			if strings.Contains(s, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing temporal entry containing %q in %v", w, summaries)
		}
	}
}

func TestUnderforge_HarshAtHeat7(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-uf-harsh:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneUnderforge, "",
		ExpeditionSupplies{Current: 60, Max: 60, DailyBurn: 2, HarshMod: 2})
	if err != nil {
		t.Fatal(err)
	}
	// Heat 6 — should NOT be harsh.
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET temporal_stack = 6, current_day = 7 WHERE expedition_id = ?`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	exp, _ = getExpedition(exp.ID)
	if zoneTemporalHarsh(exp) {
		t.Error("heat=6 should not trigger harsh")
	}
	// Heat 7 — should be harsh.
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET temporal_stack = 7 WHERE expedition_id = ?`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	exp, _ = getExpedition(exp.ID)
	if !zoneTemporalHarsh(exp) {
		t.Error("heat=7 should trigger harsh")
	}
}

func TestUnderforgeHeatBandFor(t *testing.T) {
	cases := []struct {
		stacks int
		want   string
	}{
		{0, "none"},
		{1, "ambient"}, {3, "ambient"},
		{4, "warning"}, {6, "warning"},
		{7, "supply"}, {9, "supply"},
		{10, "critical"},
	}
	for _, c := range cases {
		if got := UnderforgeHeatBandFor(c.stacks); got != c.want {
			t.Errorf("UnderforgeHeatBandFor(%d) = %q, want %q", c.stacks, got, c.want)
		}
	}
}

func TestUnderforge_FortifiedRestReducesHeat(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-uf-rest:example")
	defer cleanupExpeditions(uid)

	exp := &Expedition{
		ID:          "fake-uf",
		ZoneID:      ZoneUnderforge,
		TemporalStack: 5,
	}
	// Insert minimal row so updateTemporalStack works.
	if _, err := db.Get().Exec(`
		INSERT INTO dnd_expedition (expedition_id, user_id, zone_id, run_id, status, start_date, current_day, supplies_json, threat_events, region_state, last_activity, temporal_stack)
		VALUES (?, ?, ?, NULL, 'active', CURRENT_TIMESTAMP, 1, '{}', '[]', '{}', CURRENT_TIMESTAMP, 5)`,
		exp.ID, string(uid), string(ZoneUnderforge)); err != nil {
		t.Fatal(err)
	}
	defer db.Get().Exec(`DELETE FROM dnd_expedition WHERE expedition_id = ?`, exp.ID)

	got := reduceUnderforgeHeat(exp)
	if got != 3 {
		t.Errorf("reduceUnderforgeHeat(5) = %d, want 3", got)
	}
	exp.TemporalStack = 1
	got = reduceUnderforgeHeat(exp)
	if got != 0 {
		t.Errorf("reduceUnderforgeHeat(1) = %d, want 0 (no underflow)", got)
	}
	// Wrong zone — no-op.
	other := &Expedition{ZoneID: ZoneGoblinWarrens, TemporalStack: 5}
	if reduceUnderforgeHeat(other) != 5 {
		t.Error("reduceUnderforgeHeat on non-Underforge should be no-op")
	}
}

func TestFeywild_ResolveDistortion_Buckets(t *testing.T) {
	cases := []struct {
		d6   int
		want FeywildDistortion
	}{
		{1, FeywildDistortionNormal},
		{2, FeywildDistortionNormal},
		{3, FeywildDistortionHalf},
		{4, FeywildDistortionHalf},
		{5, FeywildDistortionDouble},
		{6, FeywildDistortionLoop},
	}
	for _, c := range cases {
		if got := feywildResolveDistortion(c.d6); got != c.want {
			t.Errorf("d6=%d → %q, want %q", c.d6, got, c.want)
		}
	}
}

func TestFeywild_HalfDay_HalvesBurn(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-fey-half:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneFeywildCrossing, "",
		ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 3, HarshMod: 2.5})
	if err != nil {
		t.Fatal(err)
	}
	prevRoll := feywildRollFn
	defer func() { feywildRollFn = prevRoll }()
	feywildRollFn = func() int { return 3 } // half

	startSU := exp.Supplies.Current
	if err := (&AdventurePlugin{}).deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	wantBurn := float32(1.5) // 3 * 0.5
	if startSU-got.Supplies.Current != wantBurn {
		t.Errorf("burn = %v, want %v (half-day)", startSU-got.Supplies.Current, wantBurn)
	}
	if today, _ := got.RegionState["feywild_today"].(string); today != "half" {
		t.Errorf("RegionState feywild_today = %q, want %q", today, "half")
	}
}

func TestFeywild_DoubleDay_DoublesBurn(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-fey-double:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneFeywildCrossing, "",
		ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 3, HarshMod: 2.5})
	if err != nil {
		t.Fatal(err)
	}
	prevRoll := feywildRollFn
	defer func() { feywildRollFn = prevRoll }()
	feywildRollFn = func() int { return 5 }

	startSU := exp.Supplies.Current
	if err := (&AdventurePlugin{}).deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	wantBurn := float32(6.0)
	if startSU-got.Supplies.Current != wantBurn {
		t.Errorf("burn = %v, want %v (double-day)", startSU-got.Supplies.Current, wantBurn)
	}
}

func TestFeywild_TimeLoop_LogsAndDoesNotChangeBurn(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-fey-loop:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneFeywildCrossing, "",
		ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 3, HarshMod: 2.5})
	if err != nil {
		t.Fatal(err)
	}
	prevRoll := feywildRollFn
	defer func() { feywildRollFn = prevRoll }()
	feywildRollFn = func() int { return 6 }

	startSU := exp.Supplies.Current
	if err := (&AdventurePlugin{}).deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	// Loop has no burn override → default pipeline (no harsh, no siege) = 3 SU.
	if startSU-got.Supplies.Current != 3 {
		t.Errorf("burn = %v, want 3 (loop default)", startSU-got.Supplies.Current)
	}
	entries, _ := recentExpeditionLog(exp.ID, 10)
	found := false
	for _, e := range entries {
		if e.Type == "temporal" && strings.Contains(e.Summary, "time loop") {
			found = true
		}
	}
	if !found {
		t.Error("expected a 'time loop' temporal log entry")
	}
}

func TestFeywild_NormalDay_NoTemporalLog(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-fey-norm:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneFeywildCrossing, "",
		ExpeditionSupplies{Current: 30, Max: 30, DailyBurn: 3, HarshMod: 2.5})
	if err != nil {
		t.Fatal(err)
	}
	prevRoll := feywildRollFn
	defer func() { feywildRollFn = prevRoll }()
	feywildRollFn = func() int { return 1 }

	if err := (&AdventurePlugin{}).deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	entries, _ := recentExpeditionLog(exp.ID, 10)
	for _, e := range entries {
		if e.Type == "temporal" {
			t.Errorf("normal-day distortion should not log temporal; got %+v", e)
		}
	}
}

func TestDragonsLair_AwarenessPulseFiresEveryThreeDays(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-dragon-pulse:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneDragonsLair, "",
		ExpeditionSupplies{Current: 60, Max: 60, DailyBurn: 4, HarshMod: 3})
	if err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}

	// Roll forward to Day 3 — first awareness pulse.
	setExpDay(t, exp.ID, 2)
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET last_briefing_at = NULL, supplies_json = ? WHERE expedition_id = ?`,
		`{"current":99,"max":99,"daily_burn":4,"harsh_mod":3,"foraged_today":false,"packs_standard":3,"packs_deluxe":0}`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	fresh, _ := getExpedition(exp.ID)
	prevThreat := fresh.ThreatLevel
	if err := p.deliverBriefing(fresh, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	// +10 awareness pulse + ~3 daily threat drift = 13 (mod GMMood 50 = neutral).
	if got.ThreatLevel < prevThreat+10 {
		t.Errorf("threat = %d, want ≥ %d (awareness pulse +10)", got.ThreatLevel, prevThreat+10)
	}

	// Verify pulse log entry.
	rows, _ := db.Get().Query(
		`SELECT summary FROM dnd_expedition_log WHERE expedition_id = ? AND entry_type = 'temporal' ORDER BY entry_id`,
		exp.ID)
	defer rows.Close()
	foundPulse := false
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		if strings.Contains(s, "awareness pulse") {
			foundPulse = true
		}
	}
	if !foundPulse {
		t.Error("expected an 'awareness pulse' temporal log entry")
	}
}

func TestDragonsLair_NonPulseDay_NoThreatJump(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-dragon-nonpulse:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneDragonsLair, "",
		ExpeditionSupplies{Current: 60, Max: 60, DailyBurn: 4, HarshMod: 3})
	if err != nil {
		t.Fatal(err)
	}
	setExpDay(t, exp.ID, 1) // → Day 2; not a pulse day.
	fresh, _ := getExpedition(exp.ID)
	prev := fresh.ThreatLevel
	if err := (&AdventurePlugin{}).deliverBriefing(fresh, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	// Only the +3 daily drift should apply.
	if got.ThreatLevel-prev > 5 {
		t.Errorf("non-pulse day threat jump = %d, want ≤ 5", got.ThreatLevel-prev)
	}
}

func TestDragonsLair_Day14_Awakens(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-dragon-wake:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneDragonsLair, "",
		ExpeditionSupplies{Current: 60, Max: 60, DailyBurn: 4, HarshMod: 3})
	if err != nil {
		t.Fatal(err)
	}
	setExpDay(t, exp.ID, 13) // → Day 14
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET supplies_json = ? WHERE expedition_id = ?`,
		`{"current":99,"max":99,"daily_burn":4,"harsh_mod":3,"foraged_today":false,"packs_standard":3,"packs_deluxe":0}`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	fresh, _ := getExpedition(exp.ID)
	if err := (&AdventurePlugin{}).deliverBriefing(fresh, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if awake, _ := got.RegionState["infernax_awake"].(bool); !awake {
		t.Error("infernax_awake should be true on Day 14")
	}

	// Verify both Awaken (and possibly pulse) log entries fired.
	rows, _ := db.Get().Query(
		`SELECT summary FROM dnd_expedition_log WHERE expedition_id = ? AND entry_type = 'temporal'`,
		exp.ID)
	defer rows.Close()
	foundAwaken := false
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		if strings.Contains(s, "infernax awakens") {
			foundAwaken = true
		}
	}
	if !foundAwaken {
		t.Error("expected 'infernax awakens' temporal entry")
	}
}

func TestDragonsLair_BossDefeatedSilencesPulses(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-dragon-boss:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneDragonsLair, "",
		ExpeditionSupplies{Current: 60, Max: 60, DailyBurn: 4, HarshMod: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET boss_defeated = 1, current_day = 5 WHERE expedition_id = ?`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	fresh, _ := getExpedition(exp.ID)
	prev := fresh.ThreatLevel
	if err := (&AdventurePlugin{}).deliverBriefing(fresh, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.ThreatLevel != prev {
		t.Errorf("post-boss threat changed: %d → %d", prev, got.ThreatLevel)
	}
}

func TestAbyssInstabilityBandFor(t *testing.T) {
	cases := []struct {
		stack int
		want  string
	}{
		{0, "normal"}, {20, "normal"},
		{21, "mild"}, {40, "mild"},
		{41, "warp"}, {60, "warp"},
		{61, "surges"}, {80, "surges"},
		{81, "unravel"}, {99, "unravel"},
		{100, "collapse"},
	}
	for _, c := range cases {
		if got := AbyssInstabilityBandFor(c.stack); got != c.want {
			t.Errorf("Abyss band(%d) = %q, want %q", c.stack, got, c.want)
		}
	}
}

func TestAbyss_DailyInstabilityIncrements(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-abyss-tick:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneAbyssPortal, "",
		ExpeditionSupplies{Current: 100, Max: 100, DailyBurn: 4, HarshMod: 3})
	if err != nil {
		t.Fatal(err)
	}
	p := &AdventurePlugin{}
	// Three briefings → instability should hit 15.
	for i := 0; i < 3; i++ {
		fresh, _ := getExpedition(exp.ID)
		if _, err := db.Get().Exec(
			`UPDATE dnd_expedition SET last_briefing_at = NULL, supplies_json = ? WHERE expedition_id = ?`,
			`{"current":99,"max":99,"daily_burn":4,"harsh_mod":3,"foraged_today":false,"packs_standard":3,"packs_deluxe":0}`,
			exp.ID); err != nil {
			t.Fatal(err)
		}
		fresh, _ = getExpedition(exp.ID)
		if err := p.deliverBriefing(fresh, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := getExpedition(exp.ID)
	if got.TemporalStack != 15 {
		t.Errorf("instability = %d, want 15", got.TemporalStack)
	}
}

func TestAbyss_UnravelingDoublesBurn(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-abyss-unravel:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneAbyssPortal, "",
		ExpeditionSupplies{Current: 100, Max: 100, DailyBurn: 4, HarshMod: 3})
	if err != nil {
		t.Fatal(err)
	}
	// Set instability to 85 (unravel band).
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET temporal_stack = 85 WHERE expedition_id = ?`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	fresh, _ := getExpedition(exp.ID)
	startSU := fresh.Supplies.Current
	if err := (&AdventurePlugin{}).deliverBriefing(fresh, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	wantBurn := float32(8.0) // 4 * 2 (unraveling override)
	if startSU-got.Supplies.Current != wantBurn {
		t.Errorf("burn = %v, want %v (unraveling 2×)", startSU-got.Supplies.Current, wantBurn)
	}
}

func TestAbyss_CollapseFailsExpedition(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-abyss-collapse:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneAbyssPortal, "",
		ExpeditionSupplies{Current: 100, Max: 100, DailyBurn: 4, HarshMod: 3})
	if err != nil {
		t.Fatal(err)
	}
	// Set instability to 95 — next daily +5 lands on 100 (collapse).
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET temporal_stack = 95 WHERE expedition_id = ?`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	fresh, _ := getExpedition(exp.ID)
	if err := (&AdventurePlugin{}).deliverBriefing(fresh, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.TemporalStack != 100 {
		t.Errorf("instability = %d, want 100", got.TemporalStack)
	}
	// E5b: collapse is a §10.2 forced extraction → 'abandoned' (not 'failed').
	if got.Status != ExpeditionStatusAbandoned {
		t.Errorf("status = %q, want %q after collapse", got.Status, ExpeditionStatusAbandoned)
	}
}

func TestReduceAbyssInstability(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-abyss-reduce:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneAbyssPortal, "",
		ExpeditionSupplies{Current: 100, Max: 100, DailyBurn: 4, HarshMod: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get().Exec(
		`UPDATE dnd_expedition SET temporal_stack = 25 WHERE expedition_id = ?`,
		exp.ID); err != nil {
		t.Fatal(err)
	}
	fresh, _ := getExpedition(exp.ID)
	if got := ReduceAbyssInstability(fresh, 10); got != 15 {
		t.Errorf("ReduceAbyssInstability(25, 10) = %d, want 15", got)
	}
	// Wrong zone — no-op.
	other := &Expedition{ZoneID: ZoneGoblinWarrens, TemporalStack: 50}
	if ReduceAbyssInstability(other, 10) != 50 {
		t.Error("ReduceAbyssInstability on non-Abyss zone should be no-op")
	}
}

func TestSunkenTemple_NoTidalOnNonZone(t *testing.T) {
	setupZoneRunTestDB(t)
	uid := id.UserID("@exp-tidal-other:example")
	defer cleanupExpeditions(uid)

	exp, err := startExpedition(uid, ZoneGoblinWarrens, "",
		ExpeditionSupplies{Current: 20, Max: 20, DailyBurn: 1, HarshMod: 1})
	if err != nil {
		t.Fatal(err)
	}
	setExpDay(t, exp.ID, 5)
	exp, _ = getExpedition(exp.ID)

	p := &AdventurePlugin{}
	startSU := exp.Supplies.Current
	if err := p.deliverBriefing(exp, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	got, _ := getExpedition(exp.ID)
	if got.Supplies.Current != startSU-1 {
		t.Errorf("non-tidal zone burned %v, want 1", startSU-got.Supplies.Current)
	}
}
