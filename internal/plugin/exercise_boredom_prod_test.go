//go:build prodexercise

package plugin

// Headless end-to-end exercise of the bored-adventurer path against a COPY of
// the prod DB. This covers the one seam the unit tests can't reach: the
// tryBoredomStart → beginExpedition → autopilot chain, which needs a live
// EuroPlugin and a real character with real gear.
//
//	GOGOBEE_PROD_DB_DIR=/path/to/dbcopy \
//	    go test -tags prodexercise -run TestExerciseBoredomProd -v ./internal/plugin/
//
// As with TestExerciseNewFeaturesProd, the DB copy is re-copied into t.TempDir()
// before opening, so even the copy is left pristine and nothing can reach prod.
//
// What it has to prove:
//  1. The idle clock picks out exactly the idle players — a player who acted an
//     hour ago must NOT be swept up. (playerIsIdle fails open, so a regression
//     here marches the whole server into a dungeon at once.)
//  2. A bored run actually starts against a real character: real zone, lean
//     supplies, coins debited.
//  3. It buys and equips nothing. That's the whole mechanic.
//  4. The expedition is flagged boredom=1, so the idle reaper won't let it
//     shield an absent player's streak.
//  5. It re-entrantly refuses: a second tick inside the cooldown does nothing.
//  6. The autopilot will actually walk it.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

func TestExerciseBoredomProd(t *testing.T) {
	srcDir := os.Getenv("GOGOBEE_PROD_DB_DIR")
	if srcDir == "" {
		t.Skip("set GOGOBEE_PROD_DB_DIR to a dir holding a COPY of gogobee.db")
	}
	if _, err := os.Stat(filepath.Join(srcDir, "gogobee.db")); err != nil {
		t.Skipf("no gogobee.db in %s", srcDir)
	}

	tmp := t.TempDir()
	for _, f := range []string{"gogobee.db", "gogobee.db-wal", "gogobee.db-shm"} {
		copyIfPresent(t, filepath.Join(srcDir, f), filepath.Join(tmp, f))
	}
	db.Close()
	if err := db.Init(tmp); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(db.Close)

	simOmenDisabled = true

	euro := NewEuroPlugin(nil)
	xp := NewXPPlugin(nil)
	p := NewAdventurePlugin(nil, euro, xp)
	sink := &captureSink{}
	p.Sink = sink
	mark := 0
	drain := func() {
		for _, m := range sink.msgs[mark:] {
			who := string(m.ToUser)
			if who == "" {
				who = "room:" + string(m.ToRoom)
			}
			t.Logf("      → [%s]\n%s", who, indent(m.Text))
		}
		mark = len(sink.msgs)
	}

	chars := runnableChars(t)
	if len(chars) < 2 {
		t.Fatalf("need 2+ runnable characters in the DB copy, got %d", len(chars))
	}

	// A pinned clock, 14:00 UTC today: outside the ambient quiet windows (06:00
	// briefing / 21:00 recap) that fireExpeditionBoredom deliberately refuses to
	// talk over, so the run is deterministic regardless of when we invoke it.
	now := time.Now().UTC().Truncate(24 * time.Hour).Add(14 * time.Hour)
	if inAmbientQuietWindow(now) {
		t.Fatal("pinned clock landed in the quiet window — pick another hour")
	}

	// Clear the decks: nobody in the copy should be mid-expedition when we start,
	// or the structural guards will (correctly) refuse and prove nothing.
	for _, c := range chars {
		clearExpeditionState(t, c.uid)
		healToFull(t, c.uid)
		euro.Credit(c.uid, 2000, "boredom exercise bankroll")
	}

	// The control arm. Without one, "everybody went" and "the clock works" look
	// identical — and the failure mode we're hunting (playerIsIdle failing open)
	// produces exactly the former.
	active := chars[0]
	idle := chars[1:]
	setLastAction(t, active.uid, now.Add(-1*time.Hour))
	for _, c := range idle {
		setLastAction(t, c.uid, now.Add(-72*time.Hour))
	}
	// Everyone else in the DB is left alone but must not be dragged along either;
	// snapshot who has an expedition now so we can diff at the end.
	before := activeExpeditionOwners(t)

	t.Logf("──────── clock ────────")
	t.Logf("  control (acted 1h ago): %s L%d %s", active.uid, active.level, active.class)
	for _, c := range idle {
		t.Logf("  idle    (72h silent):   %s L%d %s", c.uid, c.level, c.class)
	}

	// ── 1. The candidate query ──────────────────────────────────────────────
	cands, err := loadBoredomCandidates(now)
	if err != nil {
		t.Fatalf("loadBoredomCandidates: %v", err)
	}
	candSet := map[id.UserID]bool{}
	for _, u := range cands {
		candSet[u] = true
	}
	t.Logf("  loadBoredomCandidates → %d players", len(cands))
	if candSet[active.uid] {
		t.Errorf("control %s acted an hour ago but is a boredom candidate — the idle clock is not being read", active.uid)
	}
	for _, c := range idle {
		if !candSet[c.uid] {
			t.Errorf("idle %s has been silent 72h but is not a candidate", c.uid)
		}
		if playerIsIdle(active.uid, now) {
			t.Fatalf("playerIsIdle says the control player is idle — it is failing open, ABORT (this is the bug that marches the whole server into a dungeon)")
		}
		if !playerIsIdle(c.uid, now) {
			t.Errorf("playerIsIdle(%s) = false after 72h of silence", c.uid)
		}
	}

	// ── 2/3/4. The run itself ───────────────────────────────────────────────
	type snap struct {
		balance float64
		gear    string
	}
	pre := map[id.UserID]snap{}
	for _, c := range chars {
		pre[c.uid] = snap{balance: euro.GetBalance(c.uid), gear: gearFingerprint(t, c.uid)}
	}

	t.Logf("──────── fireExpeditionBoredom ────────")
	p.fireExpeditionBoredom(now)
	drain()

	for _, c := range idle {
		exp, err := getActiveExpedition(c.uid)
		if err != nil || exp == nil {
			t.Errorf("%s (L%d): no expedition started (err=%v) — the boredom start path is broken", c.uid, c.level, err)
			continue
		}
		zone, ok := getZone(exp.ZoneID)
		if !ok {
			t.Errorf("%s: started an unknown zone %q", c.uid, exp.ZoneID)
			continue
		}
		spent := pre[c.uid].balance - euro.GetBalance(c.uid)
		lean := loadoutPurchase(zone.Tier, LoadoutLean)
		t.Logf("  %s L%d → %s (T%d) — spent %.0f coins (lean = %d)",
			c.uid, c.level, zone.Display, int(zone.Tier), spent, lean.Cost())

		// In band.
		if c.level < zone.LevelMin || c.level > zone.LevelMax {
			t.Errorf("%s L%d sent to %s, band %d–%d — out of band",
				c.uid, c.level, zone.ID, zone.LevelMin, zone.LevelMax)
		}
		// Lean supplies, exactly. Anything else means it went shopping.
		if int(spent) != lean.Cost() {
			t.Errorf("%s spent %.0f coins, lean pack for T%d is %d — it bought something it shouldn't have",
				c.uid, spent, int(zone.Tier), lean.Cost())
		}
		// Gear untouched. This is the mechanic, not a nicety.
		if got := gearFingerprint(t, c.uid); got != pre[c.uid].gear {
			t.Errorf("%s: equipment changed across a bored run.\n  was: %s\n  now: %s", c.uid, pre[c.uid].gear, got)
		}
		// Flagged, so the idle reaper won't let it hold their streak.
		if !expeditionIsBoredom(t, exp.ID) {
			t.Errorf("%s: expedition %s not flagged boredom=1 — it will shield an absent player's streak", c.uid, exp.ID)
		}
		if !isBoredomDriven(c.uid, now) {
			t.Errorf("isBoredomDriven(%s) = false right after a bored start", c.uid)
		}
		// Raid only where there was no alternative.
		if w := raidContentWarning(zone.ID); w != "" && c.level < 13 {
			t.Errorf("%s L%d was sent into raid zone %s while non-raid zones were in band", c.uid, c.level, zone.ID)
		}
	}

	// The control player must be exactly where we left them.
	if exp, _ := getActiveExpedition(active.uid); exp != nil {
		t.Errorf("control %s acted an hour ago and was still sent on an expedition (%s)", active.uid, exp.ZoneID)
	}
	if b := euro.GetBalance(active.uid); b != pre[active.uid].balance {
		t.Errorf("control %s was charged %.0f coins for an expedition they didn't take",
			active.uid, pre[active.uid].balance-b)
	}
	// The sweep reaches every idle player in the DB, not just the two we staged —
	// that's the feature working, not a leak. The invariant that actually matters
	// is that *everyone who left was idle*. (runnableChars only lists characters
	// with the legacy adventure_characters row, so the DB holds idle players it
	// doesn't return; they're still real characters and they're right to go.)
	departed := diffOwners(activeExpeditionOwners(t), before)
	for _, uid := range departed {
		if !playerIsIdle(uid, now) {
			t.Errorf("%s was sent on an expedition but is NOT idle — the clock is not gating the sweep", uid)
		}
	}
	t.Logf("  departed: %d of %d candidates", len(departed), len(cands))
	// The gap between candidates and departures is the structural guards doing
	// their job (mid-run, resting, unfinished setup, broke). Name them, so a
	// silent refusal can't hide here.
	left := map[id.UserID]bool{}
	for _, uid := range departed {
		left[uid] = true
	}
	for _, uid := range cands {
		if !left[uid] {
			t.Logf("    stayed home: %-28s %s", uid, whyNoBoredomRun(uid))
		}
	}

	// ── 5. The cooldown ─────────────────────────────────────────────────────
	t.Logf("──────── second tick (must be a no-op) ────────")
	balBefore := map[id.UserID]float64{}
	for _, c := range idle {
		balBefore[c.uid] = euro.GetBalance(c.uid)
	}
	p.fireExpeditionBoredom(now.Add(30 * time.Minute))
	drain()
	for _, c := range idle {
		if b := euro.GetBalance(c.uid); b != balBefore[c.uid] {
			t.Errorf("%s was charged %.0f more coins on a second tick inside the cooldown — the CAS claim isn't holding",
				c.uid, balBefore[c.uid]-b)
		}
	}

	// ── 6. Does it actually walk? ───────────────────────────────────────────
	t.Logf("──────── autopilot walks the bored run ────────")
	for _, c := range idle {
		exp, _ := getActiveExpedition(c.uid)
		if exp == nil {
			continue
		}
		ctx := MessageContext{Sender: c.uid, RoomID: id.RoomID("!exercise:sim"), EventID: "$ex", Body: ""}
		// Several segments, as the ambient ticker would over a day. One call can
		// legitimately stop early (stopHarvestCombat, a fork, a boss doorway); the
		// question is whether the run keeps making progress when it's called again.
		total := 0
		for seg := 0; seg < 4; seg++ {
			r := p.runAutopilotWalkDriven(ctx, 6)
			if r.initErr != "" {
				t.Errorf("%s: autopilot refused to walk the bored expedition: %s", c.uid, r.initErr)
				break
			}
			total += r.rooms
			t.Logf("  %s seg%d: +%d rooms (total %d), stopped: %s", c.uid, seg, r.rooms, total, stopName(r.reason))
			if r.reason == stopComplete || r.reason == stopEnded {
				break
			}
		}
		if total == 0 {
			t.Errorf("%s: the bored expedition never took a single step", c.uid)
		}
	}
	drain()
}

// ---- helpers ---------------------------------------------------------------

// clearExpeditionState puts the character back on the doorstep: no expedition,
// no zone run, no combat session. The prod copy is a live snapshot, so some
// characters are mid-run — and the structural guards would (correctly) refuse
// to start a bored run on top of that, which would test nothing.
func clearExpeditionState(t *testing.T, uid id.UserID) {
	t.Helper()
	for _, q := range []string{
		`UPDATE dnd_expedition SET status = 'complete' WHERE user_id = ? AND status IN ('active','extracted')`,
		`DELETE FROM dnd_zone_run WHERE user_id = ?`,
		`DELETE FROM combat_session WHERE user_id = ?`,
		`UPDATE player_meta SET last_boredom_at = NULL WHERE user_id = ?`,
	} {
		if _, err := db.Get().Exec(q, string(uid)); err != nil {
			t.Logf("  clearExpeditionState(%s): %v (continuing)", uid, err)
		}
	}
}

func setLastAction(t *testing.T, uid id.UserID, at time.Time) {
	t.Helper()
	if _, err := db.Get().Exec(
		`UPDATE player_meta SET last_player_action_at = ? WHERE user_id = ?`,
		at, string(uid)); err != nil {
		t.Fatalf("setLastAction(%s): %v", uid, err)
	}
}

// gearFingerprint is what the bored run must not change.
func gearFingerprint(t *testing.T, uid id.UserID) string {
	t.Helper()
	eq, err := loadAdvEquipment(uid)
	if err != nil {
		return "ERR:" + err.Error()
	}
	var parts []string
	for slot, e := range eq {
		if e == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s/T%d", slot, e.Name, e.Tier))
	}
	sort.Strings(parts)
	return fmt.Sprint(parts)
}

func expeditionIsBoredom(t *testing.T, expID string) bool {
	t.Helper()
	var b bool
	if err := db.Get().QueryRow(
		`SELECT boredom FROM dnd_expedition WHERE expedition_id = ?`, expID).Scan(&b); err != nil {
		t.Logf("  expeditionIsBoredom(%s): %v", expID, err)
		return false
	}
	return b
}

func activeExpeditionOwners(t *testing.T) map[id.UserID]bool {
	t.Helper()
	out := map[id.UserID]bool{}
	rows, err := db.Get().Query(`SELECT user_id FROM dnd_expedition WHERE status = 'active'`)
	if err != nil {
		t.Fatalf("activeExpeditionOwners: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out[id.UserID(s)] = true
	}
	return out
}

// whyNoBoredomRun re-runs tryBoredomStart's guards for a candidate who didn't
// leave, so the candidates-vs-departures gap is explained rather than assumed.
func whyNoBoredomRun(uid id.UserID) string {
	c, err := LoadDnDCharacter(uid)
	switch {
	case err != nil:
		return "character won't load: " + err.Error()
	case c == nil:
		return "no character"
	case c.PendingSetup:
		return "character creation never finished"
	}
	if restingLockoutRemaining(c) > 0 {
		return "resting lockout"
	}
	if hasActiveCombatSession(uid) {
		return "in combat"
	}
	if seated, _ := seatedExpeditionFor(uid); seated != nil {
		return "seated on someone else's party"
	}
	if active, _ := getActiveExpedition(uid); active != nil {
		return "already on an expedition"
	}
	if pending, _ := getResumableExpedition(uid); pending != nil {
		return "extracted, waiting on !resume"
	}
	if run, _ := getActiveZoneRun(uid); run != nil {
		return "mid zone run"
	}
	zone, ok := pickBoredomZone(uid, c.Level)
	if !ok {
		return fmt.Sprintf("no zone in band for L%d", c.Level)
	}
	return fmt.Sprintf("(no guard tripped — would have gone to %s; check cooldown/coins)", zone.ID)
}

func stopName(r stopReason) string {
	switch r {
	case stopOK:
		return "ok"
	case stopFork:
		return "fork"
	case stopElite:
		return "elite doorway"
	case stopBoss:
		return "boss doorway"
	case stopEnded:
		return "died"
	case stopComplete:
		return "zone cleared"
	case stopBlocked:
		return "blocked by combat session"
	case stopHarvestCombat:
		return "harvest pulled a fight"
	case stopPreflight:
		return "preflight (low HP/SU)"
	case stopBossSafety:
		return "boss-safety gate"
	}
	return fmt.Sprintf("stopReason(%d)", int(r))
}

func diffOwners(after, before map[id.UserID]bool) []id.UserID {
	var out []id.UserID
	for uid := range after {
		if !before[uid] {
			out = append(out, uid)
		}
	}
	return out
}
