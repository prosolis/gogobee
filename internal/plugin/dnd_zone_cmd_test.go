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

// Phase 11 D7 — !zone taunt / !zone compliment.

func TestZoneCmd_TauntAppliesMoodPenalty(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-taunt:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "enter goblin_warrens"); err != nil {
		t.Fatalf("enter: %v", err)
	}
	before, err := getActiveZoneRun(uid)
	if err != nil || before == nil {
		t.Fatalf("active run after enter: %v", err)
	}
	startMood := before.GMMood
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "taunt"); err != nil {
		t.Fatalf("taunt: %v", err)
	}
	after, err := getActiveZoneRun(uid)
	if err != nil || after == nil {
		t.Fatalf("active run after taunt: %v", err)
	}
	want := clampMood(startMood + MoodEventDelta(MoodEventTaunt))
	if after.GMMood != want {
		t.Errorf("mood after taunt = %d, want %d (start %d, delta %d)",
			after.GMMood, want, startMood, MoodEventDelta(MoodEventTaunt))
	}
}

func TestZoneCmd_ComplimentAppliesMoodBonus(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-compliment:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "enter goblin_warrens"); err != nil {
		t.Fatalf("enter: %v", err)
	}
	before, err := getActiveZoneRun(uid)
	if err != nil || before == nil {
		t.Fatalf("active run after enter: %v", err)
	}
	startMood := before.GMMood
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "compliment"); err != nil {
		t.Fatalf("compliment: %v", err)
	}
	after, err := getActiveZoneRun(uid)
	if err != nil || after == nil {
		t.Fatalf("active run after compliment: %v", err)
	}
	want := clampMood(startMood + MoodEventDelta(MoodEventCompliment))
	if after.GMMood != want {
		t.Errorf("mood after compliment = %d, want %d (start %d, delta %d)",
			after.GMMood, want, startMood, MoodEventDelta(MoodEventCompliment))
	}
}

func TestZoneCmd_TauntWithoutRunNoCrash(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-taunt-norun:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	// No active run — should return cleanly with the no-run nudge.
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "taunt"); err != nil {
		t.Fatalf("taunt no-run: %v", err)
	}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "compliment"); err != nil {
		t.Fatalf("compliment no-run: %v", err)
	}
}

// Phase 11 D9 — !zone lore.

func TestZoneCmd_LoreWithoutRunNoCrash(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-lore-norun:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "lore"); err != nil {
		t.Fatalf("lore no-run: %v", err)
	}
}

func TestZoneCmd_LoreWithActiveRunNoSideEffects(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@zone-cmd-lore-run:example")
	zoneCmdTestCharacter(t, uid, 1)
	defer cleanupZoneRuns(uid)
	p := &AdventurePlugin{}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "enter goblin_warrens"); err != nil {
		t.Fatalf("enter: %v", err)
	}
	before, err := getActiveZoneRun(uid)
	if err != nil || before == nil {
		t.Fatalf("active run after enter: %v", err)
	}
	if err := p.handleDnDZoneCmd(MessageContext{Sender: uid}, "lore"); err != nil {
		t.Fatalf("lore: %v", err)
	}
	after, err := getActiveZoneRun(uid)
	if err != nil || after == nil {
		t.Fatalf("active run after lore: %v", err)
	}
	if after.GMMood != before.GMMood {
		t.Errorf("lore should not move mood: before %d, after %d", before.GMMood, after.GMMood)
	}
	if after.CurrentRoom != before.CurrentRoom {
		t.Errorf("lore should not advance room: before %d, after %d", before.CurrentRoom, after.CurrentRoom)
	}
}

func TestTauntComplimentResponseLines_Deterministic(t *testing.T) {
	const runID = "run-d7-test"
	t1 := tauntResponseLine(runID, 0)
	t2 := tauntResponseLine(runID, 0)
	if t1 == "" || t1 != t2 {
		t.Errorf("taunt lines not deterministic: %q vs %q", t1, t2)
	}
	c1 := complimentResponseLine(runID, 0)
	c2 := complimentResponseLine(runID, 0)
	if c1 == "" || c1 != c2 {
		t.Errorf("compliment lines not deterministic: %q vs %q", c1, c2)
	}
	if t1 == c1 {
		t.Errorf("taunt and compliment shouldn't collide on same key: %q", t1)
	}
}

func TestRenderZoneMap_LayoutAndMarkers(t *testing.T) {
	seq := []RoomType{RoomEntry, RoomExploration, RoomTrap, RoomElite, RoomBoss}
	cleared := map[int]bool{0: true, 1: true}
	got := renderZoneMap(seq, 2, cleared)
	want := "E──?──T──★──☠\n✓  ✓  ▶  ·  ·"
	if got != want {
		t.Errorf("renderZoneMap mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestRenderZoneMap_Empty(t *testing.T) {
	if got := renderZoneMap(nil, 0, nil); got != "(no rooms)" {
		t.Errorf("empty seq: got %q", got)
	}
}

// TestNarrationCoverage_AllZonesAllTypes — D6 polish guarantee. Locks in
// that every GMNarrationType resolves to a non-empty pool for every
// registered zone. Adding a new zone or new GMNarrationType without
// wiring its pool will fail this test.
func TestNarrationCoverage_AllZonesAllTypes(t *testing.T) {
	types := []GMNarrationType{
		GMRoomEntry, GMCombatStart, GMCombatEnd, GMNat20, GMNat1,
		GMBossEntry, GMBossDeath, GMPlayerDeath, GMZoneComplete,
		GMTrapDetected, GMTrapTripped, GMLore,
	}
	for _, z := range allZones() {
		for _, k := range types {
			pool := pickPool(z.ID, k)
			if len(pool) == 0 {
				t.Errorf("zone %s × %s: empty pool — every (zone, type) must route to prewritten flavor", z.ID, k)
			}
		}
	}
}
