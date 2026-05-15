package plugin

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

// Phase 12 E1c — !expedition command tests. Mirrors the !zone test
// pattern: side-effect coverage (persisted state) only, since SendDM is
// a no-op without a Matrix client.

func expeditionCmdTestCharacter(t *testing.T, uid id.UserID, level int) {
	t.Helper()
	if err := createAdvCharacter(uid, "expcmd"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: level,
		STR: 14, DEX: 12, CON: 14, INT: 10, WIS: 10, CHA: 10,
		HPMax: 20, HPCurrent: 20, ArmorClass: 14,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
}

func TestParseSupplyArgs_Defaults(t *testing.T) {
	p, err := parseSupplyArgs("")
	if err != nil {
		t.Fatal(err)
	}
	if p.StandardPacks != 1 || p.DeluxePacks != 0 {
		t.Errorf("default = %+v, want 1 standard / 0 deluxe", p)
	}
}

func TestParseSupplyArgs_Combinations(t *testing.T) {
	cases := []struct {
		in       string
		std, dlx int
		wantErr  bool
	}{
		{"2s", 2, 0, false},
		{"2s 1d", 2, 1, false},
		{"3 standard 1 deluxe", 0, 0, true}, // unbound number tokens fail
		{"3", 3, 0, false},
		{"3std 1dlx", 3, 1, false},
		{"abc", 0, 0, true},
		{"2x", 0, 0, true},
		{"-1s", 0, 0, true},
	}
	for _, c := range cases {
		got, err := parseSupplyArgs(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("%q: err = %v, wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if c.wantErr {
			continue
		}
		if got.StandardPacks != c.std || got.DeluxePacks != c.dlx {
			t.Errorf("%q: got %+v, want std=%d dlx=%d", c.in, got, c.std, c.dlx)
		}
	}
}

func TestThreatThresholdLabel_Bands(t *testing.T) {
	cases := []struct {
		level int
		siege bool
		want  string
	}{
		{0, false, "Quiet"},
		{20, false, "Quiet"},
		{21, false, "Stirring"},
		{40, false, "Stirring"},
		{41, false, "Alert"},
		{60, false, "Alert"},
		{61, false, "Hostile"},
		{80, false, "Hostile"},
		{81, false, "Siege"},
		{100, false, "Siege"},
		{50, true, "Siege"}, // siege flag forces Siege regardless
	}
	for _, c := range cases {
		if got := threatThresholdLabel(c.level, c.siege); got != c.want {
			t.Errorf("level=%d siege=%v: got %q, want %q", c.level, c.siege, got, c.want)
		}
	}
}

func TestEstimateDays(t *testing.T) {
	cases := []struct {
		max, burn float32
		want      int
	}{
		{20, 1, 20},
		{20, 1.5, 13}, // floor(13.33)
		{10, 0, 0},    // div-by-zero guard
		{0, 1, 0},
	}
	for _, c := range cases {
		if got := estimateDays(c.max, c.burn); got != c.want {
			t.Errorf("max=%v burn=%v: got %d, want %d", c.max, c.burn, got, c.want)
		}
	}
}

func TestExpeditionCmd_StartAbandonRoundtrip(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@exp-cmd-start:example")
	expeditionCmdTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	euro.Credit(uid, 200, "test setup")
	p := &AdventurePlugin{euro: euro}

	// Default 1 standard pack = 50 coins. Should land an active expedition.
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "start goblin_warrens"); err != nil {
		t.Fatal(err)
	}
	exp, err := getActiveExpedition(uid)
	if err != nil {
		t.Fatal(err)
	}
	if exp == nil {
		t.Fatal("expected active expedition after start")
	}
	if exp.ZoneID != ZoneGoblinWarrens {
		t.Errorf("zone = %s", exp.ZoneID)
	}
	if exp.Supplies.Max != 10 {
		t.Errorf("max SU = %v, want 10", exp.Supplies.Max)
	}
	// Coin balance should have dropped by 50.
	if got := euro.GetBalance(uid); got > 200-50+0.001 || got < 200-50-0.001 {
		t.Errorf("balance = %v, want 150", got)
	}
	// Log should have at least the start entry.
	entries, _ := recentExpeditionLog(exp.ID, 5)
	if len(entries) == 0 {
		t.Error("expected start entry in log")
	}

	// Concurrent start should fail.
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "start crypt_valdris"); err != nil {
		t.Fatal(err)
	}
	// Status path:
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "status"); err != nil {
		t.Fatal(err)
	}
	// Log path:
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "log"); err != nil {
		t.Fatal(err)
	}
	// Abandon.
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "abandon"); err != nil {
		t.Fatal(err)
	}
	if got, _ := getActiveExpedition(uid); got != nil {
		t.Error("expected no active expedition after abandon")
	}
}

func TestExpeditionCmd_StartInsufficientCoins(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@exp-cmd-broke:example")
	expeditionCmdTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	// Don't credit — default seed is < 50.
	bal := euro.GetBalance(uid)
	p := &AdventurePlugin{euro: euro}

	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "start goblin_warrens 3s 1d"); err != nil {
		t.Fatal(err)
	}
	if got, _ := getActiveExpedition(uid); got != nil {
		t.Error("expedition should not start when underfunded")
	}
	if got := euro.GetBalance(uid); got != bal {
		t.Errorf("balance moved despite failed start: %v → %v", bal, got)
	}
}

// TestExpeditionCmd_RunNoExpedition: !expedition run with no active
// expedition is a friendly no-op, not an error.
func TestExpeditionCmd_RunNoExpedition(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@exp-cmd-run-noexp:example")
	expeditionCmdTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	p := &AdventurePlugin{}
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "run"); err != nil {
		t.Fatalf("run with no expedition: %v", err)
	}
}

// TestExpeditionCmd_RunWalksRooms: starting an expedition then calling
// `!expedition run` advances the underlying zone-run by at least one
// room (and stops cleanly at a natural interrupt — fork, elite, boss,
// complete, or the autopilot room cap).
func TestExpeditionCmd_RunWalksRooms(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@exp-cmd-run-walk:example")
	expeditionCmdTestCharacter(t, uid, 3)
	defer cleanupExpeditions(uid)

	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	euro.Credit(uid, 200, "test setup")
	p := &AdventurePlugin{euro: euro}

	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "start goblin_warrens"); err != nil {
		t.Fatal(err)
	}
	exp, _ := getActiveExpedition(uid)
	if exp == nil || exp.RunID == "" {
		t.Fatal("expected active expedition with run")
	}
	startRun, _ := getZoneRun(exp.RunID)
	if startRun == nil {
		t.Fatal("expected zone run for expedition")
	}
	startRoom := startRun.CurrentRoom

	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, "run"); err != nil {
		t.Fatalf("run: %v", err)
	}

	// After autopilot, the run should have advanced. Either the room
	// pointer moved, or the run completed (TotalRooms reached). For a
	// fresh L3 run at GoblinWarrens we expect at least one room walked
	// — the entry room is narration-only and never blocks.
	endRun, _ := getZoneRun(exp.RunID)
	if endRun == nil {
		// Run could legitimately complete and be cleared; that's also fine.
		return
	}
	if endRun.CurrentRoom <= startRoom {
		t.Errorf("autopilot did not advance: room %d → %d", startRoom, endRun.CurrentRoom)
	}
}

// TestAutopilotPreflight_LowHP: HP at or below the threshold trips the
// preflight stop with the low-HP footer.
func TestAutopilotPreflight_LowHP(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@exp-autopilot-lowhp:example")
	expeditionCmdTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	c, _ := LoadDnDCharacter(uid)
	c.HPCurrent = 5 // 5/20 = 25%, below 30% threshold
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	exp := &Expedition{
		Supplies: ExpeditionSupplies{Current: 10, DailyBurn: 1},
	}
	msg, stop := autopilotPreflight(uid, exp)
	if !stop {
		t.Fatal("expected low-HP stop")
	}
	if !strings.Contains(msg, "HP low") {
		t.Errorf("expected HP-low footer, got %q", msg)
	}
}

// TestAutopilotPreflight_LowSU: supplies under one day's burn trips.
func TestAutopilotPreflight_LowSU(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@exp-autopilot-lowsu:example")
	expeditionCmdTestCharacter(t, uid, 1)
	defer cleanupExpeditions(uid)

	exp := &Expedition{
		Supplies: ExpeditionSupplies{Current: 0.5, DailyBurn: 1.5},
	}
	msg, stop := autopilotPreflight(uid, exp)
	if !stop {
		t.Fatal("expected low-SU stop")
	}
	if !strings.Contains(msg, "supplies low") {
		t.Errorf("expected SU-low footer, got %q", msg)
	}
}

// TestAutopilotFooter_Reasons: each stop reason gets a distinct,
// non-empty footer (or empty for stops where the final block already
// ends the message — death, complete, blocked).
func TestAutopilotFooter_Reasons(t *testing.T) {
	cases := []struct {
		r       stopReason
		want    string // substring; empty means footer must be empty
		wantHas bool
	}{
		{stopFork, "fork", true},
		{stopElite, "elite", true},
		{stopBoss, "boss", true},
		{stopOK, "stretch", true}, // hit room cap
		{stopEnded, "", false},
		{stopComplete, "", false},
		{stopBlocked, "", false},
	}
	for _, c := range cases {
		got := autopilotFooter(c.r, 3)
		if c.wantHas {
			if got == "" || !strings.Contains(got, c.want) {
				t.Errorf("%v: got %q, want substring %q", c.r, got, c.want)
			}
		} else if got != "" {
			t.Errorf("%v: expected empty footer, got %q", c.r, got)
		}
	}
}

func TestExpeditionCmd_ListWithoutCharBlocked(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@exp-cmd-nochar:example")
	defer cleanupExpeditions(uid)
	p := &AdventurePlugin{}
	if err := p.handleDnDExpeditionCmd(MessageContext{Sender: uid}, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := getActiveExpedition(uid); got != nil {
		t.Error("no expedition should exist")
	}
}
