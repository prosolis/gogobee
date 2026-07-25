package plugin

import (
	"strings"
	"testing"
	"time"

	"gogobee/internal/db"
	"gogobee/internal/peteclient"
	"maunium.net/go/mautrix/id"
)

// W5: the web action queue's game-side half. These pin the two things that would
// actually hurt in prod — a replayed order re-running a non-idempotent action,
// and a refusal being reported as a transient fault (which parks the order and
// leaves the player staring at "asked for…" forever).

// TestAdvOrderLedgerShortCircuitsAReoffer is the regression for the whole class
// of bug this ledger exists for. A verdict-ack lost on the wire means Pete
// re-offers an order we have already applied; if that re-offer reached
// applyAdvOrder it would extract a run the player had resumed, or spend a bout
// they were saving.
func TestAdvOrderLedgerShortCircuitsAReoffer(t *testing.T) {
	newMischiefTestDB(t)

	if _, _, ok := advOrderAlreadyApplied("guid-never-seen"); ok {
		t.Fatal("an unknown guid reported as already applied")
	}
	if err := recordAdvApplied("guid-1", "applied", "Out of the Goblin Warrens on day 3."); err != nil {
		t.Fatalf("recordAdvApplied: %v", err)
	}
	status, detail, ok := advOrderAlreadyApplied("guid-1")
	if !ok || status != "applied" || !strings.Contains(detail, "Goblin Warrens") {
		t.Fatalf("ledger read = %q/%q ok=%v, want the stored verdict back", status, detail, ok)
	}
	// A second stamp on the same guid must not error or overwrite — the re-file
	// path can reach it.
	if err := recordAdvApplied("guid-1", "rejected_not_running", "nonsense"); err != nil {
		t.Fatalf("re-record: %v", err)
	}
	if status, _, _ := advOrderAlreadyApplied("guid-1"); status != "applied" {
		t.Fatalf("verdict changed under a re-record: %q", status)
	}
}

// TestExtractOrderRefusalsAreTerminal: neither refusal may come back as retry.
// A retried refusal never reaches a verdict, so the order sits pending forever
// and Pete's strip never stops saying "asked for…".
func TestExtractOrderRefusalsAreTerminal(t *testing.T) {
	// W9: was setupZoneRunTestDB, which copies data/gogobee.db and t.Skip()s when
	// it is missing — and that file is deleted after every local run, so this and
	// the extraction test below have been green-by-skipping since W5a. Neither
	// needs a prod row; startExpedition builds everything they touch.
	setupEmptyTestDB(t)
	uid := id.UserID("@web-extract-none:example.org")
	defer cleanupExpeditions(uid)
	p := &AdventurePlugin{}

	status, detail, retry := p.applyAdvOrder(uid, peteclient.AdvOrder{
		GUID: "g", Action: peteclient.AdvOrderExtract,
	})
	if retry {
		t.Fatal("no-expedition extract asked for a retry; it must be terminal")
	}
	if status != "rejected_not_running" || detail == "" {
		t.Fatalf("status = %q detail = %q, want rejected_not_running with prose", status, detail)
	}
}

// TestExtractOrderIsTheSameExtraction: the web verb must run the game's own
// extraction, not a lookalike. The proof is the state the row lands in —
// 'extracting' (a resumable limbo) rather than 'abandoned' — plus the day burn
// and the log line the DM path writes.
func TestExtractOrderIsTheSameExtraction(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@web-extract-live:example.org")
	defer cleanupExpeditions(uid)
	p := &AdventurePlugin{}

	if _, err := startExpedition(uid, ZoneGoblinWarrens, "", ExpeditionSupplies{
		Current: 10, Max: 10, DailyBurn: 1, HarshMod: 1, PacksStandard: 1,
	}); err != nil {
		t.Fatalf("startExpedition: %v", err)
	}

	status, detail, retry := p.applyAdvOrder(uid, peteclient.AdvOrder{
		GUID: "g-extract", Action: peteclient.AdvOrderExtract,
	})
	if retry || status != "applied" {
		t.Fatalf("extract = %q retry=%v, want applied", status, retry)
	}
	if !strings.Contains(detail, "resume") {
		t.Fatalf("verdict %q never mentions the resume window, which is the whole point of an extraction", detail)
	}

	var dbStatus string
	var day int
	if err := db.Get().QueryRow(
		`SELECT status, current_day FROM dnd_expedition WHERE user_id = ?`, string(uid),
	).Scan(&dbStatus, &day); err != nil {
		t.Fatalf("read expedition: %v", err)
	}
	if dbStatus != ExpeditionStatusExtracting {
		t.Fatalf("expedition status = %q, want %q — a web extract must be resumable like the command's",
			dbStatus, ExpeditionStatusExtracting)
	}
	if day != 2 {
		t.Fatalf("current_day = %d, want 2 — extraction burns the day", day)
	}
}

// TestSiegeJoinRefusalsAreTerminal covers the two refusals a bout can hit without
// running any combat: nothing camped, and a bout already spent today. Both must
// be terminal for the same reason as the extract refusals, and "already fought"
// especially — it is the one a double-click produces.
func TestSiegeJoinRefusalsAreTerminal(t *testing.T) {
	newMischiefTestDB(t)
	uid := id.UserID("@web-siege:example.org")
	p := &AdventurePlugin{}

	status, _, retry := p.applyAdvOrder(uid, peteclient.AdvOrder{
		GUID: "g1", Action: peteclient.AdvOrderSiegeJoin,
	})
	if retry || status != "rejected_no_siege" {
		t.Fatalf("no-boss bout = %q retry=%v, want rejected_no_siege", status, retry)
	}

	// Camp a boss and spend the day's bout, then ask again.
	now := time.Now().UTC()
	bossID, err := insertWorldBoss("Grelloth", 3, 18000, now.Add(-time.Hour), now.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("insertWorldBoss: %v", err)
	}
	if err := createAdvCharacter(uid, "Rurina"); err != nil {
		t.Fatalf("createAdvCharacter: %v", err)
	}
	today := now.Format("2006-01-02")
	if err := upsertWorldBossContrib(bossID, uid, 250, today); err != nil {
		t.Fatalf("upsertWorldBossContrib: %v", err)
	}

	status, detail, retry := p.applyAdvOrder(uid, peteclient.AdvOrder{
		GUID: "g2", Action: peteclient.AdvOrderSiegeJoin,
	})
	if retry || status != "rejected_already_fought" {
		t.Fatalf("second bout = %q retry=%v, want rejected_already_fought", status, retry)
	}
	if detail == "" {
		t.Fatal("a refusal with no prose leaves the strip saying nothing useful")
	}
}

// TestUnknownActionIsRejectedNotRetried: Pete validates the action before it ever
// queues one, so an unknown verb is a contract breach. Spinning on it would poll
// the same dead order every 15 seconds forever.
func TestUnknownActionIsRejectedNotRetried(t *testing.T) {
	newMischiefTestDB(t)
	p := &AdventurePlugin{}
	status, _, retry := p.applyAdvOrder("@x:example.org", peteclient.AdvOrder{
		GUID: "g", Action: "sell_house",
	})
	if retry || !strings.HasPrefix(status, "rejected_") {
		t.Fatalf("unknown action = %q retry=%v, want a terminal rejection", status, retry)
	}
}

// TestAdvOrderPlainText: the Siege verdict is the Matrix footer reused, and Pete
// renders a verdict as text — so the markdown has to come off or the player reads
// literal asterisks.
func TestAdvOrderPlainText(t *testing.T) {
	got := advOrderPlainText("💥 You deal **412** damage. **Grelloth** has **5,800 / 18,000 HP** left.\nYou stagger out at 1 HP.")
	if strings.Contains(got, "*") || strings.Contains(got, "\n") {
		t.Fatalf("plain text still carries markup or a newline: %q", got)
	}
	if !strings.Contains(got, "412") || !strings.Contains(got, "Grelloth") {
		t.Fatalf("plain text lost the facts: %q", got)
	}
}

// ── W5b: the three verbs that take arguments and spend coins ───────────────────

// webOrderTestChar builds a character solvent enough to outfit an expedition.
func webOrderTestChar(t *testing.T, uid id.UserID, level int, coins float64) *AdventurePlugin {
	t.Helper()
	if err := createAdvCharacter(uid, "weborder"); err != nil {
		t.Fatal(err)
	}
	c := &DnDCharacter{
		UserID: uid, Race: RaceHuman, Class: ClassFighter, Level: level,
		STR: 14, DEX: 12, CON: 14, INT: 10, WIS: 10, CHA: 10,
		HPMax: 30, HPCurrent: 30, ArmorClass: 14,
	}
	if err := SaveDnDCharacter(c); err != nil {
		t.Fatal(err)
	}
	euro := &EuroPlugin{}
	euro.ensureBalance(uid)
	if coins > 0 {
		euro.Credit(uid, coins, "test bankroll")
	}
	return &AdventurePlugin{euro: euro}
}

// A forged zone must buy nothing. Pete only ever offers what gogobee quoted it,
// but a quote is a stale snapshot and not a permission — so the order path
// re-resolves against availableZonesFor and refuses anything that isn't there.
func TestWebExpeditionStartReResolvesTheZone(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@web-start-forged:example.org")
	t.Cleanup(func() { cleanupExpeditions(uid); cleanupZoneRuns(uid) })
	p := webOrderTestChar(t, uid, 2, 100000)

	before := p.euro.GetBalance(uid)
	status, _, retry := p.applyAdvOrder(uid, peteclient.AdvOrder{
		GUID: "forged-zone", Action: peteclient.AdvOrderExpedition,
		Params: &peteclient.AdvOrderParams{Zone: "dragons_lair", Loadout: "lean"},
	})
	if retry {
		t.Fatal("a locked zone asked for a retry; it must be terminal")
	}
	if status != "rejected_zone_locked" {
		t.Fatalf("status = %q, want rejected_zone_locked", status)
	}
	if after := p.euro.GetBalance(uid); after != before {
		t.Fatalf("a refused departure moved money: %.0f -> %.0f", before, after)
	}
	if exp, _ := getActiveExpedition(uid); exp != nil {
		t.Fatal("a refused departure started an expedition anyway")
	}
}

// An unknown loadout is refused, never defaulted. Defaulting would spend coins on
// a pack size the player never picked.
func TestWebExpeditionStartRefusesAnUnknownLoadout(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@web-start-loadout:example.org")
	t.Cleanup(func() { cleanupExpeditions(uid); cleanupZoneRuns(uid) })
	p := webOrderTestChar(t, uid, 2, 100000)

	before := p.euro.GetBalance(uid)
	status, _, _ := p.applyAdvOrder(uid, peteclient.AdvOrder{
		GUID: "bad-loadout", Action: peteclient.AdvOrderExpedition,
		Params: &peteclient.AdvOrderParams{Zone: string(ZoneGoblinWarrens), Loadout: "enormous"},
	})
	if status != "rejected_unavailable" {
		t.Fatalf("status = %q, want rejected_unavailable", status)
	}
	if after := p.euro.GetBalance(uid); after != before {
		t.Fatalf("a refused loadout moved money: %.0f -> %.0f", before, after)
	}
}

// The money test that matters: a re-offered order (verdict-ack lost before the
// ledger stamped it) must not charge twice, and must not answer "you're already
// on an expedition" for the expedition it just started.
func TestWebExpeditionStartChargesOnceOnAReoffer(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@web-start-idem:example.org")
	t.Cleanup(func() { cleanupExpeditions(uid); cleanupZoneRuns(uid) })
	p := webOrderTestChar(t, uid, 2, 100000)

	order := peteclient.AdvOrder{
		GUID: "start-once", Action: peteclient.AdvOrderExpedition,
		Params: &peteclient.AdvOrderParams{Zone: string(ZoneGoblinWarrens), Loadout: "lean"},
	}
	before := p.euro.GetBalance(uid)
	status, _, retry := p.applyAdvOrder(uid, order)
	if retry || status != "applied" {
		t.Fatalf("first apply = %q retry=%v, want applied", status, retry)
	}
	afterFirst := p.euro.GetBalance(uid)
	if afterFirst >= before {
		t.Fatalf("outfitting cost nothing: %.0f -> %.0f", before, afterFirst)
	}

	// The re-offer. applyAdvOrder is reached directly here on purpose: the
	// adv_applied_orders ledger would normally short-circuit it, and this asserts
	// the layer *underneath* that guard is safe too.
	status, _, retry = p.applyAdvOrder(uid, order)
	if retry {
		t.Fatal("the re-offer asked for a retry")
	}
	if status != "applied" {
		t.Fatalf("re-offer = %q, want applied — the settled debit is what tells a "+
			"replay apart from a player who really is already out", status)
	}
	if after := p.euro.GetBalance(uid); after != afterFirst {
		t.Fatalf("the re-offer charged again: %.0f -> %.0f", afterFirst, after)
	}
}

// Babysit: same replay contract, plus the two durations are the only two sold.
func TestWebBabysitChargesOnceAndSellsTwoDurations(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@web-sitter:example.org")
	p := webOrderTestChar(t, uid, 2, 100000)

	status, _, _ := p.applyAdvOrder(uid, peteclient.AdvOrder{
		GUID: "sitter-odd", Action: peteclient.AdvOrderBabysit,
		Params: &peteclient.AdvOrderParams{Days: 3},
	})
	if status != "rejected_unavailable" {
		t.Fatalf("3-day sitter = %q, want rejected_unavailable", status)
	}

	order := peteclient.AdvOrder{
		GUID: "sitter-once", Action: peteclient.AdvOrderBabysit,
		Params: &peteclient.AdvOrderParams{Days: 7},
	}
	before := p.euro.GetBalance(uid)
	if status, _, _ := p.applyAdvOrder(uid, order); status != "applied" {
		t.Fatalf("hire = %q, want applied", status)
	}
	afterFirst := p.euro.GetBalance(uid)
	if afterFirst >= before {
		t.Fatalf("the sitter worked for free: %.0f -> %.0f", before, afterFirst)
	}
	if status, _, _ := p.applyAdvOrder(uid, order); status != "applied" {
		t.Fatalf("re-offer = %q, want applied", status)
	}
	if after := p.euro.GetBalance(uid); after != afterFirst {
		t.Fatalf("the re-offer charged again: %.0f -> %.0f", afterFirst, after)
	}
}

// A refusal must never come back as retry: a retried refusal never reaches a
// verdict, so the order parks and the strip says "asked for…" forever.
func TestWebMoneyVerbRefusalsAreTerminal(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@web-broke:example.org")
	t.Cleanup(func() { cleanupExpeditions(uid); cleanupZoneRuns(uid) })
	p := webOrderTestChar(t, uid, 2, 0)

	for _, tc := range []struct {
		name  string
		order peteclient.AdvOrder
		want  string
	}{
		{"broke departure", peteclient.AdvOrder{
			GUID: "broke-1", Action: peteclient.AdvOrderExpedition,
			Params: &peteclient.AdvOrderParams{Zone: string(ZoneGoblinWarrens), Loadout: "heavy"},
		}, "rejected_insufficient_funds"},
		{"nothing to resume", peteclient.AdvOrder{
			GUID: "resume-1", Action: peteclient.AdvOrderResume,
			Params: &peteclient.AdvOrderParams{Loadout: "lean"},
		}, "rejected_nothing_to_resume"},
		{"broke sitter", peteclient.AdvOrder{
			GUID: "broke-2", Action: peteclient.AdvOrderBabysit,
			Params: &peteclient.AdvOrderParams{Days: 30},
		}, "rejected_insufficient_funds"},
	} {
		status, detail, retry := p.applyAdvOrder(uid, tc.order)
		if retry {
			t.Fatalf("%s asked for a retry; it must be terminal", tc.name)
		}
		if status != tc.want {
			t.Fatalf("%s = %q, want %q (detail %q)", tc.name, status, tc.want, detail)
		}
		if strings.ContainsAny(detail, "*`") {
			t.Fatalf("%s verdict still carries Matrix markdown: %q", tc.name, detail)
		}
	}
}

// An order with no params at all is a contract breach, not a user mistake, and
// must be refused rather than defaulted into spending money.
func TestWebMoneyVerbsRefuseMissingParams(t *testing.T) {
	setupEmptyTestDB(t)
	uid := id.UserID("@web-noparams:example.org")
	t.Cleanup(func() { cleanupExpeditions(uid); cleanupZoneRuns(uid) })
	p := webOrderTestChar(t, uid, 2, 100000)

	before := p.euro.GetBalance(uid)
	for _, action := range []string{
		peteclient.AdvOrderExpedition, peteclient.AdvOrderResume, peteclient.AdvOrderBabysit,
	} {
		status, _, retry := p.applyAdvOrder(uid, peteclient.AdvOrder{GUID: "np-" + action, Action: action})
		if retry || status != "rejected_unavailable" {
			t.Fatalf("%s with no params = %q retry=%v, want rejected_unavailable", action, status, retry)
		}
	}
	if after := p.euro.GetBalance(uid); after != before {
		t.Fatalf("a paramless order moved money: %.0f -> %.0f", before, after)
	}
}
