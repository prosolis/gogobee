package plugin

import (
	"testing"

	"maunium.net/go/mautrix/id"
)

// Phase 11 D1c — !zone command surface tests. Side-effect coverage only;
// SendDM is a no-op without a Matrix client (see plugin.SendDM), so these
// tests check the persisted state changes the command path produced
// rather than the rendered text.

func zoneCmdTestCharacter(t *testing.T, uid id.UserID, level int) {
	t.Helper()
	if err := createAdvCharacter(uid, "zonecmd"); err != nil {
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

func TestZoneCmd_ListNoCharIsBlocked(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-nochar:example")
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	// No character row → handler returns the setup nudge, no error.
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, ""); err != nil {
		t.Fatalf("handleDnDZoneCmd: %v", err)
	}
	if active, _ := getActiveZoneRun(uid); active != nil {
		t.Error("no run should exist")
	}
}

func TestZoneCmd_ListWithCharSucceeds(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-list:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, ""); err != nil {
		t.Fatalf("list: %v", err)
	}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "list"); err != nil {
		t.Fatalf("list explicit: %v", err)
	}
}

func TestZoneCmd_EnterStartsRun(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-enter:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "enter goblin_warrens"); err != nil {
		t.Fatalf("enter: %v", err)
	}
	run, err := getActiveZoneRun(uid)
	if err != nil {
		t.Fatal(err)
	}
	if run == nil {
		t.Fatal("expected active run after enter")
	}
	if run.ZoneID != ZoneGoblinWarrens {
		t.Errorf("zone = %s", run.ZoneID)
	}
}

func TestZoneCmd_EnterByIndex(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-enter-idx:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	available := zonesForLevel(1)
	if len(available) == 0 {
		t.Skip("no zones registered for L1")
	}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "enter 1"); err != nil {
		t.Fatalf("enter 1: %v", err)
	}
	run, _ := getActiveZoneRun(uid)
	if run == nil {
		t.Fatal("expected active run after enter 1")
	}
	if run.ZoneID != available[0].ID {
		t.Errorf("zone = %s, want %s", run.ZoneID, available[0].ID)
	}
}

func TestZoneCmd_EnterUnknownDoesNotStart(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-unk:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "enter not_a_real_zone"); err != nil {
		t.Fatalf("enter unknown: %v", err)
	}
	if run, _ := getActiveZoneRun(uid); run != nil {
		t.Error("unknown zone should not start a run")
	}
}

func TestZoneCmd_EnterRejectedWhileRunActive(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-dup:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "enter goblin_warrens"); err != nil {
		t.Fatal(err)
	}
	first, _ := getActiveZoneRun(uid)
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "enter crypt_valdris"); err != nil {
		t.Fatal(err)
	}
	now, _ := getActiveZoneRun(uid)
	if now == nil || now.RunID != first.RunID {
		t.Error("second enter should not replace the active run")
	}
}

func TestZoneCmd_StatusAndMapNoActiveRun(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-noactive:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "status"); err != nil {
		t.Fatal(err)
	}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "map"); err != nil {
		t.Fatal(err)
	}
}

func TestZoneCmd_AdvanceMovesRoom(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-adv:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "enter goblin_warrens"); err != nil {
		t.Fatal(err)
	}
	before, _ := getActiveZoneRun(uid)
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "advance"); err != nil {
		t.Fatal(err)
	}
	after, _ := getActiveZoneRun(uid)
	if after == nil {
		t.Fatal("run vanished after advance")
	}
	if after.CurrentRoom != before.CurrentRoom+1 {
		t.Errorf("current room: before=%d after=%d", before.CurrentRoom, after.CurrentRoom)
	}
	if len(after.RoomsCleared) != 1 {
		t.Errorf("rooms cleared = %d, want 1", len(after.RoomsCleared))
	}
}

func TestZoneCmd_AbandonClearsActive(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-aban:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "enter goblin_warrens"); err != nil {
		t.Fatal(err)
	}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "abandon"); err != nil {
		t.Fatal(err)
	}
	if run, _ := getActiveZoneRun(uid); run != nil {
		t.Error("active run after abandon")
	}
	// Second abandon should be a no-op (handler swallows ErrNoActiveRun
	// and DM-replies "no run to abandon" — no error returned).
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "abandon"); err != nil {
		t.Fatalf("second abandon: %v", err)
	}
}

func TestResolveZoneInput_MatchesIDIndexAndDisplay(t *testing.T) {
	avail := zonesForLevel(1)
	if len(avail) < 2 {
		t.Skip("need at least 2 zones registered")
	}
	cases := []struct {
		in   string
		want ZoneID
	}{
		{"goblin_warrens", ZoneGoblinWarrens},
		{"GOBLIN_WARRENS", ZoneGoblinWarrens},
		{"Goblin Warrens", ZoneGoblinWarrens},
		{"crypt of valdris", ZoneCryptValdris},
		{"1", avail[0].ID},
		{"2", avail[1].ID},
	}
	for _, c := range cases {
		got, ok := resolveZoneInput(c.in, avail)
		if !ok {
			t.Errorf("resolve(%q): not found", c.in)
			continue
		}
		if got != c.want {
			t.Errorf("resolve(%q) = %s, want %s", c.in, got, c.want)
		}
	}
	if _, ok := resolveZoneInput("nope", avail); ok {
		t.Error("expected resolveZoneInput(nope) to fail")
	}
	if _, ok := resolveZoneInput("999", avail); ok {
		t.Error("expected out-of-range index to fail")
	}
}

func TestGMMoodLabel_Bands(t *testing.T) {
	cases := map[int]string{
		100: "effusive",
		80:  "effusive",
		79:  "friendly",
		60:  "friendly",
		59:  "neutral",
		40:  "neutral",
		39:  "grumpy",
		20:  "grumpy",
		19:  "hostile",
		0:   "hostile",
	}
	for mood, want := range cases {
		if got := gmMoodLabel(mood); got != want {
			t.Errorf("gmMoodLabel(%d) = %s, want %s", mood, got, want)
		}
	}
}
