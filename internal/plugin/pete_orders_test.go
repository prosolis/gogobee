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
	setupZoneRunTestDB(t)
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
	setupZoneRunTestDB(t)
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
