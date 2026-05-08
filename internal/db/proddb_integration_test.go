package db

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestProdDBMigrations runs the full migration suite against a copy of the
// real prod database to verify Phase 1 + Phase 2 schema changes apply cleanly.
//
// Gated on the prod-db file actually existing — skips silently in CI.
// Mutations write only to the tempdir copy; the real prod file is untouched.
func TestProdDBMigrations(t *testing.T) {
	src := "/home/reala-misaki/git/gogobee/data/gogobee.db"
	if _, err := os.Stat(src); err != nil {
		t.Skip("prod db not present at " + src)
	}

	dir := t.TempDir()
	dst := filepath.Join(dir, "gogobee.db")
	copyFile(t, src, dst)

	// Snapshot row counts BEFORE migration via a raw connection.
	preCounts := tableCounts(t, dst, []string{
		"adventure_characters",
		"euro_balances",
		"user_archetypes",
		"adventure_equipment",
		"adventure_inventory",
		"adventure_treasures",
	})
	t.Logf("pre-migration counts: %+v", preCounts)

	// Reset global state (other tests may have run Init already).
	Close()

	// Run migrations via the production code path.
	if err := Init(dir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	t.Cleanup(Close)

	d := Get()

	// 1. New D&D tables must exist.
	for _, table := range []string{
		"dnd_character", "dnd_abilities", "dnd_resources",
		"dnd_combat_state", "adventure_characters_pre_dnd",
	} {
		var n int
		if err := d.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
			table,
		).Scan(&n); err != nil {
			t.Fatalf("query %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %s missing after migrations", table)
		}
	}

	// 2. Existing-data row counts unchanged.
	postCounts := tableCounts(t, dst, []string{
		"adventure_characters",
		"euro_balances",
		"user_archetypes",
		"adventure_equipment",
		"adventure_inventory",
		"adventure_treasures",
	})
	for k, pre := range preCounts {
		if post := postCounts[k]; post != pre {
			t.Errorf("row count drift in %s: pre=%d post=%d", k, pre, post)
		}
	}
	t.Logf("post-migration counts: %+v", postCounts)

	// 3. Pre-DnD snapshot populated for every adventure character.
	var advCount, snapCount int
	d.QueryRow(`SELECT COUNT(*) FROM adventure_characters`).Scan(&advCount)
	d.QueryRow(`SELECT COUNT(*) FROM adventure_characters_pre_dnd`).Scan(&snapCount)
	if snapCount != advCount {
		t.Errorf("snapshot rows %d != adventure_characters %d", snapCount, advCount)
	}
	t.Logf("snapshot populated: %d rows", snapCount)

	// 4. Snapshot JSON parses and contains key fields. SQLite's single-conn
	// pool means we must drain the rows before issuing follow-up queries.
	rows, err := d.Query(`SELECT user_id, snapshot_json FROM adventure_characters_pre_dnd`)
	if err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	type snapPair struct{ uid, j string }
	var snaps []snapPair
	for rows.Next() {
		var uid, j string
		if err := rows.Scan(&uid, &j); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		snaps = append(snaps, snapPair{uid, j})
	}
	rows.Close()

	for _, s := range snaps {
		var checkCL int
		if err := d.QueryRow(`SELECT json_extract(?, '$.combat_level')`, s.j).Scan(&checkCL); err != nil {
			t.Errorf("snapshot for %s: bad json: %v", s.uid, err)
			continue
		}
		var realCL int
		d.QueryRow(`SELECT combat_level FROM adventure_characters WHERE user_id=?`, s.uid).Scan(&realCL)
		if checkCL != realCL {
			t.Errorf("snapshot %s: combat_level mismatch snap=%d real=%d", s.uid, checkCL, realCL)
		}
	}

	// 5. New columns on adventure_equipment / adventure_inventory exist and default empty.
	rows2, err := d.Query(`SELECT dnd_rarity, dnd_stat_bonus_json, dnd_attuned FROM adventure_equipment LIMIT 5`)
	if err != nil {
		t.Errorf("new equipment columns query failed: %v", err)
	} else {
		rows2.Close()
	}
	rows3, err := d.Query(`SELECT dnd_rarity, dnd_stat_bonus_json FROM adventure_inventory LIMIT 5`)
	if err != nil {
		t.Errorf("new inventory columns query failed: %v", err)
	} else {
		rows3.Close()
	}

	// 6. auto_migrated column on dnd_character exists.
	if _, err := d.Exec(`SELECT auto_migrated FROM dnd_character LIMIT 0`); err != nil {
		t.Errorf("auto_migrated column missing: %v", err)
	}

	// 7. Idempotent: running Init again must not error or change row counts.
	Close()
	if err := Init(dir); err != nil {
		t.Fatalf("second Init failed: %v", err)
	}
	postCounts2 := tableCounts(t, dst, []string{"adventure_characters_pre_dnd"})
	if postCounts2["adventure_characters_pre_dnd"] != snapCount {
		t.Errorf("snapshot row count drifted on second Init: %d → %d",
			snapCount, postCounts2["adventure_characters_pre_dnd"])
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
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

func tableCounts(t *testing.T, dbPath string, tables []string) map[string]int {
	t.Helper()
	d, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	out := map[string]int{}
	for _, tbl := range tables {
		var n int
		err := d.QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&n)
		if err != nil {
			out[tbl] = -1
			continue
		}
		out[tbl] = n
	}
	return out
}
