//go:build prodexercise

package plugin

// Headless smoke-exercise of the N-series features (parties aside — those have
// their own sim path) against a COPY of the prod DB. Nothing here touches the
// live prod file: point GOGOBEE_PROD_DB_DIR at a directory holding a *copy* of
// gogobee.db (+ optional -wal/-shm) and the test re-copies that into t.TempDir()
// before opening it, so even the copy is left pristine.
//
//	GOGOBEE_PROD_DB_DIR=/path/to/dbcopy \
//	    go test -tags prodexercise -run TestExerciseNewFeaturesProd -v ./internal/plugin/
//
// Every outbound DM / room message a handler would have sent to Matrix is
// captured via the existing MessageSink seam and dumped with t.Log, so the run
// shows exactly what a player would see. Each feature is wrapped so a panic or
// error in one is reported and the battery continues.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gogobee/internal/db"

	"maunium.net/go/mautrix/id"
)

func TestExerciseNewFeaturesProd(t *testing.T) {
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

	// Reproducibility: the weekly Omen keys off wall-clock time.Now(); pin it off
	// so the same DB copy exercises identically regardless of which week we run.
	simOmenDisabled = true
	simAutoArmEnabled = true

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
	exercise := func(name string, fn func()) {
		defer func() {
			if r := recover(); r != nil {
				t.Logf("  !! %s PANICKED: %v", name, r)
				mark = len(sink.msgs)
			}
		}()
		t.Logf("──────── %s ────────", name)
		fn()
		drain()
	}

	chars := runnableChars(t)
	if len(chars) == 0 {
		t.Fatal("no runnable characters (dnd_character with class + adventure_characters row) in the DB copy")
	}
	t.Logf("runnable characters: %d", len(chars))
	for _, c := range chars {
		healToFull(t, c.uid)
		euro.Credit(c.uid, 5000, "exercise bankroll")
		t.Logf("  • %-28s %s L%d", c.uid, c.class, c.level)
	}
	ctxFor := func(uid id.UserID, body string) MessageContext {
		return MessageContext{Sender: uid, RoomID: id.RoomID("!exercise:sim"), EventID: "$ex", Body: body}
	}

	// ── N7/B2 Renown & prestige ─────────────────────────────────────────────
	exercise("N7/B2 Renown — standing + prestige announce", func() {
		for _, c := range chars {
			xp, _ := loadRenownXP(c.uid)
			lvl := renownLevelForUser(c.uid)
			t.Logf("    %s: renownXP=%d renownLevel=%d rank=%q", c.uid, xp, lvl, renownRankFor(lvl))
		}
		// Push the first character up a renown level to fire the announce path.
		c0 := chars[0]
		before, after, err := addRenownXP(c0.uid, 500)
		if err != nil {
			t.Logf("    addRenownXP: %v", err)
			return
		}
		t.Logf("    %s renownXP %d→%d", c0.uid, before, after)
		p.announceRenown(c0.uid, renownLevelFor(before), renownLevelFor(after))
	})

	// ── N6/C3 World Boss / Siege raid ───────────────────────────────────────
	exercise("N6/C3 World Boss — spawn, status, each fighter's bout", func() {
		boss, err := p.spawnWorldBoss("exercise-siege")
		if err != nil {
			t.Logf("    spawnWorldBoss: %v", err)
		}
		if boss != nil {
			t.Logf("    spawned %q tier=%d hp=%d", boss.Name, boss.Tier, boss.HPMax)
		}
		p.handleWorldBossCmd(ctxFor(chars[0].uid, ""), "status")
		for _, c := range chars {
			t.Logf("    -- %s takes a bout --", c.uid)
			p.handleWorldBossCmd(ctxFor(c.uid, ""), "fight")
		}
		t.Logf("    -- pool after the raid --")
		p.handleWorldBossCmd(ctxFor(chars[0].uid, ""), "status")
	})

	// ── N6/C2 Duel (needs two eligible players) ─────────────────────────────
	exercise("N6/C2 Duel — challenge, accept, resolve", func() {
		if len(chars) < 2 {
			t.Log("    need 2 characters for a duel; skipping")
			return
		}
		a, b := chars[0], chars[1]
		p.handleDuelCmd(ctxFor(a.uid, ""), fmt.Sprintf("%s 100", b.uid))
		p.handleDuelCmd(ctxFor(b.uid, ""), "accept")
	})

	// ── N6/D3 The Shadow ────────────────────────────────────────────────────
	exercise("N6/D3 Shadow — standing vs the rival", func() {
		for _, c := range chars {
			p.handleShadowCmd(ctxFor(c.uid, ""))
		}
	})

	// ── N5/D1a Campaign journal ─────────────────────────────────────────────
	exercise("N5/D1a Journal — collected campaign pages", func() {
		for _, c := range chars {
			p.handleJournalCmd(ctxFor(c.uid, ""))
		}
	})

	// ── N4/E3 Town registries ───────────────────────────────────────────────
	exercise("N4/E3 Town — !town / !graveyard / !rivals", func() {
		p.handleTownCmd(ctxFor(chars[0].uid, ""))
		p.handleGraveyardCmd(ctxFor(chars[0].uid, ""))
		p.handleRivalsCmd(ctxFor(chars[0].uid, ""))
		p.handleRivalsTopCmd(ctxFor(chars[0].uid, ""), "")
	})

	// ── N4/E1 Estate vault ──────────────────────────────────────────────────
	exercise("N4/E1 Vault — list (gated on T4 estate)", func() {
		for _, c := range chars {
			p.handleVaultCmd(ctxFor(c.uid, ""), "")
		}
	})

	// ── N4/E2 Item gifting ──────────────────────────────────────────────────
	exercise("N4/E2 Gifting — !give <item> @user", func() {
		if len(chars) < 2 {
			t.Log("    need 2 characters to gift; skipping")
			return
		}
		giver, receiver := chars[0], chars[1]
		item := firstInventoryItem(giver.uid)
		if item == "" {
			t.Logf("    %s has no inventory items to gift", giver.uid)
			return
		}
		body := fmt.Sprintf("!give %s %s", item, receiver.uid)
		t.Logf("    %s gifts %q to %s", giver.uid, item, receiver.uid)
		p.handleGiveCmd(ctxFor(giver.uid, body))
	})

	// ── N7/B4 Achievements wing (best-effort; needs a registry) ─────────────
	exercise("N7/B4 Achievements — !achievements", func() {
		ach := NewAchievementsPlugin(nil, nil)
		ach.Sink = sink
		p.SetAchievements(ach)
		for _, c := range chars {
			ach.handleAchievements(ctxFor(c.uid, ""))
		}
	})
}

// ---- helpers ---------------------------------------------------------------

type exChar struct {
	uid   id.UserID
	class string
	level int
}

// runnableChars lists characters that have both a completed dnd_character (class
// set) and the legacy adventure_characters row the expedition/feature paths load.
func runnableChars(t *testing.T) []exChar {
	t.Helper()
	d := db.Get()
	rows, err := d.Query(`
		SELECT d.user_id, d.class, d.dnd_level
		FROM dnd_character d
		JOIN adventure_characters a ON a.user_id = d.user_id
		WHERE d.class != ''`)
	if err != nil {
		t.Fatalf("query chars: %v", err)
	}
	defer rows.Close()
	var out []exChar
	for rows.Next() {
		var c exChar
		var uid string
		if err := rows.Scan(&uid, &c.class, &c.level); err != nil {
			t.Fatal(err)
		}
		c.uid = id.UserID(uid)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].level > out[j].level })
	return out
}

func healToFull(t *testing.T, uid id.UserID) {
	t.Helper()
	c, err := LoadDnDCharacter(uid)
	if err != nil || c == nil {
		t.Logf("    healToFull: load %s: %v", uid, err)
		return
	}
	c.HPCurrent = c.HPMax
	c.TempHP = 0
	c.Exhaustion = 0
	c.ShortRestCharges = c.Level
	if err := SaveDnDCharacter(c); err != nil {
		t.Logf("    healToFull: save %s: %v", uid, err)
	}
}

func firstInventoryItem(uid id.UserID) string {
	d := db.Get()
	var name string
	// adventure_inventory holds the player's consumables/misc, one row per item.
	_ = d.QueryRow(
		`SELECT name FROM adventure_inventory WHERE user_id=? LIMIT 1`,
		string(uid),
	).Scan(&name)
	return name
}

func copyIfPresent(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		return // wal/shm may be absent — fine
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "        " + l
	}
	return strings.Join(lines, "\n")
}
