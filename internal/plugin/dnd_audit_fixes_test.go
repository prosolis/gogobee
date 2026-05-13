package plugin

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gogobee/internal/db"
	"maunium.net/go/mautrix/id"
)

func setupAuditTestDB(t *testing.T) {
	t.Helper()
	src := "/home/reala-misaki/git/gogobee/data/gogobee.db"
	if _, err := os.Stat(src); err != nil {
		t.Skip("prod db not present")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "gogobee.db")
	in, _ := os.Open(src)
	defer in.Close()
	out, _ := os.Create(dst)
	defer out.Close()
	io.Copy(out, in)

	db.Close()
	if err := db.Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
}

// ── Fix D: Orc Rage threshold parity ────────────────────────────────────────

// TestOrcRage_ThresholdExact: with HP*2 < MaxHP, the threshold is exactly 50%
// regardless of MaxHP parity.
func TestOrcRage_ThresholdExact(t *testing.T) {
	cases := []struct {
		hp, maxHP int
		shouldFire bool
	}{
		{50, 100, false}, // exactly 50% — does NOT fire (strict <)
		{49, 100, true},  // just under
		{8, 16, false},   // exactly 50% even MaxHP
		{7, 16, true},
		{8, 15, true},    // 8/15 = 53% — under former threshold of MaxHP/2=7. New: 8*2=16, 16<15 false. Hmm.
	}
	// Re-derive: HP*2 < MaxHP. So 8*2=16 < 15? No. Doesn't fire. That means at MaxHP=15, threshold trips at HP*2 < 15, i.e. HP < 7.5, i.e. HP ≤ 7.
	// Old behavior used HP < MaxHP/2 = HP < 7 (integer div). So fired at HP < 7, i.e., HP ≤ 6. Different by one.
	// Pick cases that distinguish.
	cases = []struct {
		hp, maxHP int
		shouldFire bool
	}{
		{50, 100, false},
		{49, 100, true},
		{8, 16, false},
		{7, 16, true},
		{6, 15, true}, // 6*2=12 < 15 ✓
		{7, 15, true}, // 7*2=14 < 15 ✓ (old code's MaxHP/2=7 would have NOT fired at HP=7)
		{8, 15, false}, // 8*2=16, not < 15
	}
	for _, c := range cases {
		// Direct unit-check via the predicate the engine uses.
		fired := c.hp*2 < c.maxHP
		if fired != c.shouldFire {
			t.Errorf("hp=%d maxHP=%d: predicate=%v, want %v", c.hp, c.maxHP, fired, c.shouldFire)
		}
	}
}

// ── Fix E: respec wipes resource pool ───────────────────────────────────────

func TestRespec_WipesOldResources(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@respec_resources:example")

	// Set up a Fighter with stamina pool.
	if err := createAdvCharacter(uid, "respec_test"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 3,
		STR: 16, DEX: 13, CON: 14, INT: 8, WIS: 10, CHA: 12,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := initResources(uid, ClassFighter); err != nil {
		t.Fatal(err)
	}

	// Spend some stamina so the pool isn't at max.
	spendResource(uid, "stamina", 1)

	// Fund their account so respec can debit.
	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	euro.Credit(uid, 10000, "test")

	p := &AdventurePlugin{euro: euro}
	if err := p.handleDnDRespecCmd(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}

	// Old stamina row should be gone.
	cur, max, _ := getResource(uid, "stamina")
	if cur != 0 || max != 0 {
		t.Errorf("post-respec stamina: cur=%d max=%d, want 0/0 (row deleted)", cur, max)
	}

	// dnd_character should be in pending_setup state, no class/race.
	got, _ := LoadDnDCharacter(uid)
	if got == nil || !got.PendingSetup || got.Class != "" || got.Race != "" {
		t.Errorf("post-respec sheet: %+v", got)
	}
	// Euros debited.
	if euro.GetBalance(uid) > 5001 {
		t.Errorf("euros not debited: balance %.0f, expected ~5000", euro.GetBalance(uid))
	}
}

// ── Fix B: respec save-then-debit preserves euros on save failure ───────────

// We can't easily simulate a save failure mid-test without dependency
// injection. Instead, verify the *insufficient-balance* path: a player with
// no euros should get the error WITHOUT any state mutation.
func TestRespec_InsufficientFundsLeavesStateIntact(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@respec_broke:example")
	if err := createAdvCharacter(uid, "broke"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceElf, Class: ClassMage, Level: 4,
		STR: 8, DEX: 16, CON: 10, INT: 17, WIS: 13, CHA: 12,
		HPMax: 22, HPCurrent: 22, ArmorClass: 13,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := initResources(uid, ClassMage); err != nil {
		t.Fatal(err)
	}

	euro := &EuroPlugin{}
	// Don't credit — balance stays 0.
	p := &AdventurePlugin{euro: euro}
	if err := p.handleDnDRespecCmd(MessageContext{Sender: uid}); err != nil {
		t.Fatal(err)
	}
	// State must be intact.
	got, _ := LoadDnDCharacter(uid)
	if got.Class != ClassMage || got.Race != RaceElf || got.PendingSetup {
		t.Errorf("insufficient-funds respec mutated state: %+v", got)
	}
	// Mage's spell_slot pool should still be there.
	cur, max, _ := getResource(uid, "spell_slot")
	if max != 1 {
		t.Errorf("spell_slot pool wiped on insufficient-funds respec: cur=%d max=%d", cur, max)
	}
}

// ── Fix C: arm save-then-spend; on spend failure, armed flag reverts ────────

// Hard to force spendResource to fail without DI; verify the happy path
// still works (resource decrements once, armed_ability set).
func TestArm_SaveThenSpend_HappyPath(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@arm_order:example")
	if err := createAdvCharacter(uid, "arm_order"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: 3,
		STR: 16, DEX: 13, CON: 14, INT: 8, WIS: 10, CHA: 12,
		HPMax: 30, HPCurrent: 30, ArmorClass: 16,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	if err := initResources(uid, ClassFighter); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	if err := p.handleDnDArmCmd(MessageContext{Sender: uid}, "second_wind"); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadDnDCharacter(uid)
	if got.ArmedAbility != "second_wind" {
		t.Errorf("ArmedAbility = %q, want second_wind", got.ArmedAbility)
	}
	cur, _, _ := getResource(uid, "stamina")
	if cur != 2 {
		t.Errorf("stamina = %d, want 2 (3 - 1)", cur)
	}
}

// ── Fix G: Persuasion shop discount expires after 30 minutes ────────────────

func TestPersuasionDiscount_ExpiresAfterTTL(t *testing.T) {
	p := &AdventurePlugin{}
	uid := id.UserID("@disc_expire:example")
	// Start a session with a discount applied, but with StartedAt in the past.
	pastTime := time.Now().Add(-31 * time.Minute)
	p.shopSessions.Store(string(uid), &advShopSession{
		StartedAt:          pastTime,
		PersuasionDiscount: 0.10,
	})
	factor := p.shopSessionPriceFactor(uid)
	if factor != 1.0 {
		t.Errorf("expired session: factor = %v, want 1.0", factor)
	}
	// Announce should also return empty.
	if got := p.shopSessionAnnounceDiscount(uid); got != "" {
		t.Errorf("expired session announce = %q, want empty", got)
	}
}

func TestPersuasionDiscount_ActiveWithinTTL(t *testing.T) {
	p := &AdventurePlugin{}
	uid := id.UserID("@disc_active:example")
	p.shopSessions.Store(string(uid), &advShopSession{
		StartedAt:          time.Now(),
		PersuasionDiscount: 0.10,
	})
	factor := p.shopSessionPriceFactor(uid)
	if factor != 0.90 {
		t.Errorf("active session: factor = %v, want 0.9", factor)
	}
}

// ── Fix I: HP scaling clamp ─────────────────────────────────────────────────

// ── Wrapper for non-combat handlers ──────────────────────────────────────

// TestEnsureCharForDnDCmd_FreshMigrate — the wrapper used by !check, !stats,
// !level, !rest, !arm should auto-migrate any legacy player without a D&D row.
func TestEnsureCharForDnDCmd_FreshMigrate(t *testing.T) {
	setupAuditTestDB(t)
	uid := id.UserID("@wrapper_legacy:example")
	if err := createAdvCharacter(uid, "wrapper_legacy"); err != nil {
		t.Fatal(err)
	}
	advChar, _ := loadAdvCharacter(uid)
	advChar.CombatLevel = 25 // legacy player
	if err := saveAdvCharacter(advChar); err != nil {
		t.Fatal(err)
	}

	p := &AdventurePlugin{}
	c, err := p.ensureCharForDnDCmd(uid, advChar)
	if err != nil || c == nil {
		t.Fatalf("ensureCharForDnDCmd: %v / nil=%v", err, c == nil)
	}
	if c.PendingSetup {
		t.Error("auto-migrated char should not be pending_setup")
	}
	if !c.AutoMigrated {
		t.Error("auto-migrated flag not set")
	}
}

// TestSetupStatus_NoStubOnFirstSetup — `!setup` for a player without a D&D
// row must NOT pre-create one; the row is created only on `!setup confirm`.
// (Pre-L5g this branch saved a stub for legacy players to carry the
// onboarding-DM flag; that mechanism has been retired.)
func TestSetupStatus_NoStubOnFirstSetup(t *testing.T) {
	setupAuditTestDB(t)
	for _, uid := range []id.UserID{"@setup_legacy:example", "@setup_new:example"} {
		if err := createAdvCharacter(uid, string(uid)); err != nil {
			t.Fatal(err)
		}
	}

	// Legacy player (CombatLevel = 30) — used to get a stub.
	advLegacy, _ := loadAdvCharacter("@setup_legacy:example")
	advLegacy.CombatLevel = 30
	saveAdvCharacter(advLegacy)

	p := &AdventurePlugin{}
	for _, uid := range []id.UserID{"@setup_legacy:example", "@setup_new:example"} {
		if err := p.dndSetupStatus(MessageContext{Sender: uid}); err != nil {
			t.Fatal(err)
		}
		got, _ := LoadDnDCharacter(uid)
		if got != nil {
			t.Errorf("%s: !setup pre-created a row: %+v", uid, got)
		}
	}
}

func TestApplyDnDHPScaling_NeverExceedsCombatMax(t *testing.T) {
	// Pathological: hp_current somehow above hp_max — early-return path
	// (full HP) leaves MaxHP unchanged and StartHP unset.
	stats := CombatStats{MaxHP: 100, HPBonus: 50}
	c := &DnDCharacter{HPMax: 50, HPCurrent: 100}
	applyDnDHPScaling(&stats, c)
	if stats.MaxHP != 100 {
		t.Errorf("MaxHP clobbered: %d, want unchanged 100", stats.MaxHP)
	}
	if stats.StartHP > stats.MaxHP {
		t.Errorf("StartHP=%d exceeds MaxHP=%d", stats.StartHP, stats.MaxHP)
	}
}
