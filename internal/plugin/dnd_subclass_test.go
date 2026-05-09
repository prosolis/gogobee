package plugin

import (
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix/id"
)

// Phase 10 SUB1 — subclass selection.

// ── Registry sanity ──────────────────────────────────────────────────────

func TestSubclassRegistry_FifteenEntriesThreePerClass(t *testing.T) {
	if got := len(dndSubclassRegistry); got != 15 {
		t.Fatalf("registry size: got %d, want 15", got)
	}
	perClass := map[DnDClass]int{}
	for _, s := range dndSubclassRegistry {
		perClass[s.Class]++
		if s.ID == "" || s.Display == "" || s.Tagline == "" || s.Flavor == "" {
			t.Errorf("incomplete entry: %+v", s)
		}
	}
	for _, cls := range []DnDClass{ClassFighter, ClassRogue, ClassMage, ClassCleric, ClassRanger} {
		if perClass[cls] != 3 {
			t.Errorf("%s: %d subclasses, want 3", cls, perClass[cls])
		}
	}
}

func TestSubclassRegistry_UniqueIDs(t *testing.T) {
	seen := map[DnDSubclass]bool{}
	for _, s := range dndSubclassRegistry {
		if seen[s.ID] {
			t.Errorf("duplicate subclass id: %s", s.ID)
		}
		seen[s.ID] = true
	}
}

// ── resolveSubclassInput ─────────────────────────────────────────────────

func TestResolveSubclassInput_ByNumber(t *testing.T) {
	got, ok := resolveSubclassInput(ClassFighter, "1")
	if !ok || got != SubclassChampion {
		t.Errorf("Fighter '1' = (%q, %v), want (champion, true)", got, ok)
	}
	got, ok = resolveSubclassInput(ClassFighter, "3")
	if !ok || got != SubclassBerserker {
		t.Errorf("Fighter '3' = (%q, %v), want (berserker, true)", got, ok)
	}
	if _, ok := resolveSubclassInput(ClassFighter, "4"); ok {
		t.Error("Fighter '4' should be invalid")
	}
}

func TestResolveSubclassInput_ByNameAndDisplay(t *testing.T) {
	cases := []struct {
		class DnDClass
		input string
		want  DnDSubclass
	}{
		{ClassFighter, "champion", SubclassChampion},
		{ClassFighter, "Champion", SubclassChampion},
		{ClassFighter, "Battle Master", SubclassBattleMaster},
		{ClassFighter, "battlemaster", SubclassBattleMaster},
		{ClassFighter, "battle_master", SubclassBattleMaster},
		{ClassRogue, "Arcane Trickster", SubclassArcaneTrickster},
		{ClassMage, "necromancy", SubclassNecromancy},
		{ClassCleric, "life domain", SubclassLifeDomain},
		{ClassRanger, "Gloom-Stalker", SubclassGloomStalker},
	}
	for _, tc := range cases {
		got, ok := resolveSubclassInput(tc.class, tc.input)
		if !ok || got != tc.want {
			t.Errorf("%s/%q: got (%q,%v), want (%q,true)", tc.class, tc.input, got, ok, tc.want)
		}
	}
}

func TestResolveSubclassInput_WrongClass(t *testing.T) {
	// "champion" belongs to Fighter — should not resolve for Rogue.
	if _, ok := resolveSubclassInput(ClassRogue, "champion"); ok {
		t.Error("champion should not resolve under Rogue")
	}
}

// ── renderSubclassPrompt ─────────────────────────────────────────────────

func TestRenderSubclassPrompt_ContainsAllThree(t *testing.T) {
	got := renderSubclassPrompt(ClassMage)
	for _, want := range []string{"Evocation", "Abjuration", "Necromancy", "Mage"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q:\n%s", want, got)
		}
	}
}

// ── !subclass command flow ───────────────────────────────────────────────

func subclassTestCharacter(t *testing.T, uid id.UserID, level int) *DnDCharacter {
	t.Helper()
	if err := createAdvCharacter(uid, "subclass"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: level,
		STR: 16, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSubclass_BlockedBelowLevel5(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@subclass_low:example")
	subclassTestCharacter(t, uid, 4)
	p := &AdventurePlugin{euro: &EuroPlugin{}}
	// Should not error, just send a "level" message; we rely on the row
	// staying empty.
	if err := p.handleDnDSubclassCmd(MessageContext{Sender: uid}, "champion"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.Subclass != "" {
		t.Errorf("subclass set despite L4: %q", got.Subclass)
	}
}

func TestSubclass_FirstSelectionFreeAndPersists(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@subclass_first:example")
	subclassTestCharacter(t, uid, 5)
	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	bal := euro.GetBalance(uid)
	p := &AdventurePlugin{euro: euro}

	if err := p.handleDnDSubclassCmd(MessageContext{Sender: uid}, "champion"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.Subclass != SubclassChampion {
		t.Errorf("subclass = %q, want champion", got.Subclass)
	}
	if got.LastSubclassRespecAt != nil {
		t.Errorf("first selection should not set cooldown timestamp")
	}
	if euro.GetBalance(uid) != bal {
		t.Errorf("first selection should be free; balance %.0f → %.0f", bal, euro.GetBalance(uid))
	}
}

func TestSubclass_SameChoiceIsNoop(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@subclass_same:example")
	c := subclassTestCharacter(t, uid, 5)
	c.Subclass = SubclassChampion
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	euro.Credit(uid, 10000, "test")
	bal := euro.GetBalance(uid)
	p := &AdventurePlugin{euro: euro}

	if err := p.handleDnDSubclassCmd(MessageContext{Sender: uid}, "champion"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.LastSubclassRespecAt != nil {
		t.Error("same-choice no-op should not set cooldown")
	}
	if euro.GetBalance(uid) != bal {
		t.Error("same-choice no-op should not debit")
	}
}

func TestSubclass_ChangeChargesAndSetsCooldown(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@subclass_change:example")
	c := subclassTestCharacter(t, uid, 6)
	c.Subclass = SubclassChampion
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	euro.Credit(uid, 10000, "test")
	startBal := euro.GetBalance(uid)
	p := &AdventurePlugin{euro: euro}

	if err := p.handleDnDSubclassCmd(MessageContext{Sender: uid}, "berserker"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.Subclass != SubclassBerserker {
		t.Errorf("subclass = %q, want berserker", got.Subclass)
	}
	if got.LastSubclassRespecAt == nil {
		t.Error("cooldown timestamp not set after charged change")
	}
	if delta := startBal - euro.GetBalance(uid); delta < float64(dndSubclassRespecCost) {
		t.Errorf("debited %.0f, want at least %d", delta, dndSubclassRespecCost)
	}
}

func TestSubclass_ChangeBlockedByCooldown(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@subclass_cd:example")
	c := subclassTestCharacter(t, uid, 6)
	c.Subclass = SubclassChampion
	now := time.Now().UTC().Add(-1 * time.Hour) // 1h ago — well inside 30d
	c.LastSubclassRespecAt = &now
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	euro.Credit(uid, 10000, "test")
	bal := euro.GetBalance(uid)
	p := &AdventurePlugin{euro: euro}

	if err := p.handleDnDSubclassCmd(MessageContext{Sender: uid}, "berserker"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.Subclass != SubclassChampion {
		t.Errorf("subclass changed despite cooldown: %q", got.Subclass)
	}
	if euro.GetBalance(uid) != bal {
		t.Error("cooldown-blocked change should not debit")
	}
}

func TestSubclass_ChangeBlockedByInsufficientFunds(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@subclass_broke:example")
	c := subclassTestCharacter(t, uid, 6)
	c.Subclass = SubclassChampion
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	// No credit — balance ~0.
	p := &AdventurePlugin{euro: euro}

	if err := p.handleDnDSubclassCmd(MessageContext{Sender: uid}, "berserker"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.Subclass != SubclassChampion {
		t.Errorf("subclass changed despite insufficient funds: %q", got.Subclass)
	}
}

// ── level-up DM cue ──────────────────────────────────────────────────────

func TestBuildLevelUpMessage_PromptsAtL5WhenUnchosen(t *testing.T) {
	c := &DnDCharacter{
		UserID: "@x:example", Race: RaceHuman, Class: ClassFighter,
		Level: 5, HPMax: 40,
	}
	msg := buildLevelUpMessage(c, []LevelUpEvent{{NewLevel: 5, HPGain: 7}}, "")
	if !strings.Contains(msg, "Choose your path") {
		t.Errorf("L5 level-up missing subclass prompt:\n%s", msg)
	}
	if !strings.Contains(msg, "Champion") {
		t.Errorf("L5 prompt should list Champion:\n%s", msg)
	}
}

func TestBuildLevelUpMessage_NoPromptIfAlreadyChosen(t *testing.T) {
	c := &DnDCharacter{
		UserID: "@x:example", Race: RaceHuman, Class: ClassFighter,
		Level: 6, HPMax: 47, Subclass: SubclassChampion,
	}
	msg := buildLevelUpMessage(c, []LevelUpEvent{{NewLevel: 6, HPGain: 7}}, "")
	if strings.Contains(msg, "Choose your path") {
		t.Errorf("L6 level-up should not re-prompt:\n%s", msg)
	}
}

func TestBuildLevelUpMessage_NoPromptBelowL5(t *testing.T) {
	c := &DnDCharacter{
		UserID: "@x:example", Race: RaceHuman, Class: ClassFighter,
		Level: 4, HPMax: 33,
	}
	msg := buildLevelUpMessage(c, []LevelUpEvent{{NewLevel: 4, HPGain: 7}}, "")
	if strings.Contains(msg, "Choose your path") {
		t.Errorf("L4 should not prompt:\n%s", msg)
	}
}

// ── sheet display ────────────────────────────────────────────────────────

func TestSheet_ShowsSubclassWhenChosen(t *testing.T) {
	c := &DnDCharacter{
		UserID: "@x:example", Race: RaceHuman, Class: ClassFighter,
		Level: 7, HPMax: 50, HPCurrent: 50, ArmorClass: 16,
		STR: 16, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 10,
		Subclass: SubclassBattleMaster,
	}
	out := renderDnDSheet(c, nil, nil, HouseState{}, nil, nil)
	if !strings.Contains(out, "Battle Master") {
		t.Errorf("sheet missing subclass name:\n%s", out)
	}
}

func TestSheet_PromptsWhenUnchosenAtL5(t *testing.T) {
	c := &DnDCharacter{
		UserID: "@x:example", Race: RaceHuman, Class: ClassFighter,
		Level: 5, HPMax: 40, HPCurrent: 40, ArmorClass: 16,
		STR: 16, DEX: 12, CON: 14, INT: 8, WIS: 10, CHA: 10,
	}
	out := renderDnDSheet(c, nil, nil, HouseState{}, nil, nil)
	if !strings.Contains(out, "!subclass") {
		t.Errorf("sheet should nudge `!subclass` when unchosen at L5:\n%s", out)
	}
}

// ── schema migration round-trip ──────────────────────────────────────────

func TestSubclass_SchemaRoundTrip(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@subclass_rt:example")
	if err := createAdvCharacter(uid, "rt"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	c := &DnDCharacter{
		UserID: uid, Race: RaceElf, Class: ClassMage, Level: 7,
		STR: 8, DEX: 14, CON: 12, INT: 17, WIS: 12, CHA: 10,
		HPMax: 30, HPCurrent: 30, ArmorClass: 12,
		Subclass: SubclassEvocation, LastSubclassRespecAt: &now,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDnDCharacter(uid)
	if err != nil || got == nil {
		t.Fatalf("load: %v %v", got, err)
	}
	if got.Subclass != SubclassEvocation {
		t.Errorf("subclass roundtrip: got %q, want evocation", got.Subclass)
	}
	if got.LastSubclassRespecAt == nil || !got.LastSubclassRespecAt.Equal(now) {
		t.Errorf("LastSubclassRespecAt roundtrip: got %v, want %v", got.LastSubclassRespecAt, now)
	}
}
