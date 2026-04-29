package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitAndClose(t *testing.T) {
	dir := t.TempDir()

	if err := Init(dir); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// DB should be usable
	d := Get()
	if d == nil {
		t.Fatal("Get() returned nil after Init")
	}

	// Verify DB file was created
	dbPath := filepath.Join(dir, "gogobee.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("DB file not created: %v", err)
	}

	// Close should not panic
	Close()

	// After Close, globalDB should be nil
	if globalDB != nil {
		t.Error("globalDB should be nil after Close")
	}

	// Double Close should not panic
	Close()
}

func TestInit_Idempotent(t *testing.T) {
	dir := t.TempDir()

	if err := Init(dir); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	// Second Init should be a no-op (globalDB already set)
	if err := Init(dir); err != nil {
		t.Fatalf("second Init: %v", err)
	}

	Close()
}

func TestInit_SchemaContainsIndexes(t *testing.T) {
	// Verify the schema string contains our audit-added indexes
	if !containsSubstring(schema, "idx_arena_runs_user") {
		t.Error("schema missing idx_arena_runs_user index")
	}
	if !containsSubstring(schema, "idx_euro_bal_user") {
		t.Error("schema missing idx_euro_bal_user index")
	}
}

func TestInit_SchemaRunsCleanly(t *testing.T) {
	dir := t.TempDir()

	if err := Init(dir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer Close()

	// Verify we can query a table that was created by the schema
	d := Get()
	var count int
	err := d.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		t.Fatalf("query users table: %v", err)
	}

	// Verify indexes exist
	rows, err := d.Query("SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'")
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()

	indexes := make(map[string]bool)
	for rows.Next() {
		var name string
		rows.Scan(&name)
		indexes[name] = true
	}

	for _, idx := range []string{"idx_arena_runs_user", "idx_euro_bal_user", "idx_euro_tx_user"} {
		if !indexes[idx] {
			t.Errorf("missing index: %s", idx)
		}
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && searchSubstring(s, sub)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
