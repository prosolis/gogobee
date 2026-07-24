package plugin

import (
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

// seedSiege writes a world_boss row directly and returns it. Real rows on
// purpose: the history query scans two declared DATETIME columns (resolved_at,
// ends_at) and the modernc affinity trap only fires against actual stored
// values, never against a hand-built struct.
func seedSiege(t *testing.T, name string, tier, hpMax, hpCurrent int, status string, starts, ends time.Time, resolved *time.Time) int64 {
	t.Helper()
	res, err := db.Get().Exec(
		`INSERT INTO world_boss (name, tier, hp_max, hp_current, status, starts_at, ends_at, resolved_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		name, tier, hpMax, hpCurrent, status, starts, ends, resolved)
	if err != nil {
		t.Fatalf("seed world_boss: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedContrib(t *testing.T, bossID int64, uid id.UserID, fights, damage int, lastDate string) {
	t.Helper()
	if _, err := db.Get().Exec(
		`INSERT INTO world_boss_contrib (boss_id, user_id, fights, damage, last_fight_date)
		 VALUES (?, ?, ?, ?, ?)`, bossID, string(uid), fights, damage, lastDate); err != nil {
		t.Fatalf("seed contrib: %v", err)
	}
}

// TestSiegeSnapshotMustersEveryoneAlive is the reason the payload carries
// zero-fight rows at all. The mechanic is one bout per person per day, so the
// interesting number is not "who fought" but "who still could" — and Pete can
// only draw that column if the people in it are on the wire. A snapshot of
// contributors alone would render a board that quietly congratulates itself.
func TestSiegeSnapshotMustersEveryoneAlive(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-40 * time.Hour)
	today := now.Format("2006-01-02")

	seedRosterPlayer(t, "@a:test", "Josie", &old, &old)
	seedRosterPlayer(t, "@b:test", "Quack", &old, &old)
	seedRosterPlayer(t, "@c:test", "Camcast", &old, &old)

	bossID := seedSiege(t, "Gorloth the Sunderer", 4, 1000, 400, "active",
		now.Add(-2*time.Hour), now.Add(70*time.Hour), nil)
	seedContrib(t, bossID, "@a:test", 3, 500, today)
	seedContrib(t, bossID, "@b:test", 1, 100, now.AddDate(0, 0, -1).Format("2006-01-02"))

	snap, err := buildSiegeSnapshot(now)
	if err != nil {
		t.Fatalf("buildSiegeSnapshot: %v", err)
	}
	if !snap.Active || snap.BossName != "Gorloth the Sunderer" {
		t.Fatalf("snapshot missed the live boss: %+v", snap)
	}
	if snap.HPCurrent != 400 || snap.HPMax != 1000 {
		t.Errorf("pool = %d/%d, want 400/1000", snap.HPCurrent, snap.HPMax)
	}
	if len(snap.Defenders) != 3 {
		t.Fatalf("muster has %d rows, want 3 — the un-fought must have a row to stand in", len(snap.Defenders))
	}
	if snap.BoutsToday != 1 {
		t.Errorf("bouts_today = %d, want 1 — only Josie has been out today", snap.BoutsToday)
	}

	// Ranked by damage: Josie, Quack, then the adventurer who hasn't started.
	if snap.Defenders[0].Name != "Josie" || !snap.Defenders[0].FoughtToday {
		t.Errorf("top of the muster = %+v, want Josie having fought today", snap.Defenders[0])
	}
	if snap.Defenders[1].Name != "Quack" || snap.Defenders[1].FoughtToday {
		t.Errorf("second = %+v, want Quack with a bout still spare (hers was yesterday)", snap.Defenders[1])
	}
	if snap.Defenders[2].Name != "Camcast" || snap.Defenders[2].Fights != 0 {
		t.Errorf("third = %+v, want Camcast at zero fights", snap.Defenders[2])
	}
	for _, d := range snap.Defenders {
		if d.Token == "" {
			t.Errorf("%s has no board token — the defender board can't link to their page", d.Name)
		}
	}
}

// TestSiegeOptOutAnonymisesContributorAndDropsBystander is the one place the
// Siege deliberately breaks the board's opt-out rule, so it is worth pinning
// both halves.
//
// The board omits an opted-out player outright: a row showing class + level +
// zone re-identifies them, so absence is the only honest option. Here a
// CONTRIBUTOR is anonymised instead — the damage they did is on the boss and is
// part of what the town accomplished, and deleting it would understate the
// shared effort and stop the totals adding up. A non-contributor is still
// dropped, because there is nothing to account for and naming their absence
// would be exposure for nothing.
func TestSiegeOptOutAnonymisesContributorAndDropsBystander(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-40 * time.Hour)
	today := now.Format("2006-01-02")

	seedRosterPlayer(t, "@shy:test", "Ghost", &old, &old)   // opted out, fought
	seedRosterPlayer(t, "@lurk:test", "Silent", &old, &old) // opted out, never fought
	seedRosterPlayer(t, "@open:test", "Josie", &old, &old)  // opted in, fought
	setNewsOptout("@shy:test", true)
	setNewsOptout("@lurk:test", true)

	bossID := seedSiege(t, "The Iron Colossus", 4, 1000, 200, "active",
		now.Add(-time.Hour), now.Add(71*time.Hour), nil)
	seedContrib(t, bossID, "@shy:test", 4, 600, today)
	seedContrib(t, bossID, "@open:test", 1, 200, today)

	snap, err := buildSiegeSnapshot(now)
	if err != nil {
		t.Fatalf("buildSiegeSnapshot: %v", err)
	}
	if len(snap.Defenders) != 2 {
		t.Fatalf("muster has %d rows, want 2 — the opted-out bystander should be gone and the opted-out contributor kept", len(snap.Defenders))
	}

	top := snap.Defenders[0]
	if top.Damage != 600 {
		t.Fatalf("top of the muster did %d damage, want 600 — the anonymous contributor lost their rank", top.Damage)
	}
	if top.Name != anonName {
		t.Errorf("opted-out contributor rendered as %q, want %q", top.Name, anonName)
	}
	if top.Token != "" {
		t.Error("opted-out contributor carries a board token — that is a link straight back to a page that names them")
	}
	if top.Level != 0 {
		t.Errorf("opted-out contributor leaked level %d — level + damage is most of a re-identification", top.Level)
	}

	for _, d := range snap.Defenders {
		if d.Name == "Silent" || d.Name == "Ghost" {
			t.Errorf("opted-out player %q reached the wire under their character name", d.Name)
		}
	}
}

// TestSiegeHistoryReadsResolvedClock is the scan-affinity guard for the history
// query, the exact trap buildRosterSnapshot documents: resolved_at and ends_at
// are declared DATETIME, and a COALESCE() in the SQL would erase that affinity
// so the Scan fails — which would silently publish an empty history rather than
// anything obviously broken.
func TestSiegeHistoryReadsResolvedClock(t *testing.T) {
	newBoredomTestDB(t)
	now := time.Now().UTC()
	old := now.Add(-40 * time.Hour)

	seedRosterPlayer(t, "@a:test", "Josie", &old, &old)
	seedRosterPlayer(t, "@b:test", "Quack", &old, &old)

	resolved := now.Add(-24 * time.Hour)
	won := seedSiege(t, "The Ashen Wyrm", 5, 1200, 0, "defeated",
		now.Add(-96*time.Hour), now.Add(-24*time.Hour), &resolved)
	seedContrib(t, won, "@a:test", 6, 700, "2026-07-01")
	seedContrib(t, won, "@b:test", 2, 500, "2026-07-01")

	// A legacy row with no resolved_at must still date itself — off the window's
	// close, which is when the Siege ended either way and is never null.
	lost := seedSiege(t, "Kravok, Maw of the Deep", 4, 800, 300, "survived",
		now.Add(-200*time.Hour), now.Add(-128*time.Hour), nil)
	seedContrib(t, lost, "@a:test", 1, 500, "2026-06-01")

	snap, err := buildSiegeSnapshot(now)
	if err != nil {
		t.Fatalf("buildSiegeSnapshot: %v", err)
	}
	if snap.Active {
		t.Error("no boss is camped but the snapshot claims one is")
	}
	if len(snap.History) != 2 {
		t.Fatalf("history has %d rows, want 2", len(snap.History))
	}

	// Newest first (id desc): the survived Kravok was inserted last.
	h := snap.History[0]
	if h.BossName != "Kravok, Maw of the Deep" || h.Outcome != "survived" {
		t.Fatalf("history[0] = %+v, want the survived Kravok first", h)
	}
	if h.HPRemaining != 300 || h.HPMax != 800 {
		t.Errorf("survived bar = %d/%d, want 300/800", h.HPRemaining, h.HPMax)
	}
	if h.EndedAt != now.Add(-128*time.Hour).Unix() {
		t.Errorf("legacy row dated %d, want the window close %d", h.EndedAt, now.Add(-128*time.Hour).Unix())
	}

	w := snap.History[1]
	if w.Outcome != "defeated" || w.HPRemaining != 0 {
		t.Errorf("defeated Siege = %+v, want a pool at zero", w)
	}
	if w.EndedAt != resolved.Unix() {
		t.Errorf("resolved row dated %d, want resolved_at %d", w.EndedAt, resolved.Unix())
	}
	if w.Defenders != 2 {
		t.Errorf("defenders = %d, want 2", w.Defenders)
	}
	// MVP is by fights, not damage — the same accessibility call the payout split
	// makes. Josie fought six times for 700; had it been by damage she'd still win,
	// so the ordering is pinned by loadWorldBossContribs' fights-desc ordering.
	if w.MVP != "Josie" || w.MVPFights != 6 {
		t.Errorf("MVP = %q with %d fights, want Josie with 6", w.MVP, w.MVPFights)
	}
}

// TestSiegeDispatchesFireOncePerSiege. The three Siege beats key their GUID on
// the boss row id, so a resolution path re-entered (a redeploy mid-window, the
// ticker's safety net firing after an inline kill) files the same dispatch guid
// and Pete dedupes it. Without that, a restart could announce the same Siege
// twice to the whole room.
func TestSiegeDispatchesFireOncePerSiege(t *testing.T) {
	if a, b := siegeGUID("siege_start", 7), siegeGUID("siege_start", 7); a != b {
		t.Errorf("guid not stable: %q vs %q", a, b)
	}
	if a, b := siegeGUID("siege_start", 7), siegeGUID("siege_start", 8); a == b {
		t.Errorf("two different Sieges share guid %q", a)
	}
	if a, b := siegeGUID("siege_win", 7), siegeGUID("siege_loss", 7); a == b {
		t.Errorf("win and loss share guid %q — one Siege cannot file both", a)
	}
}

// TestSiegeWindowPhrase: the siege_start template says "You've got %s", so the
// stakes field has to read as a duration in a sentence, not as a timestamp.
func TestSiegeWindowPhrase(t *testing.T) {
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		window time.Duration
		want   string
	}{
		{worldBossWindow, "3 days"},
		{24 * time.Hour, "a day"},
		{36 * time.Hour, "36 hours"},
		{0, "no time at all"},
	}
	for _, c := range cases {
		b := &worldBossState{StartsAt: base, EndsAt: base.Add(c.window)}
		if got := siegeWindowPhrase(b); got != c.want {
			t.Errorf("siegeWindowPhrase(%v) = %q, want %q", c.window, got, c.want)
		}
	}
}
