package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func openMigrated(t *testing.T) *sql.DB {
	t.Helper()
	database, err := OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := Migrate(database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return database
}

func TestOpenSQLiteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	database, err := OpenSQLite(filepath.Join(dir, "sub", "test.db"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer database.Close()
	if err := database.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestOpenSQLitePragmas(t *testing.T) {
	database, err := OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer database.Close()

	checks := []struct {
		pragma string
		want   string
	}{
		{"PRAGMA foreign_keys;", "1"},
		{"PRAGMA journal_mode;", "wal"},
	}
	for _, c := range checks {
		var val string
		if err := database.QueryRow(c.pragma).Scan(&val); err != nil {
			t.Fatalf("%s scan: %v", c.pragma, err)
		}
		if val != c.want {
			t.Errorf("%s = %q, want %q", c.pragma, val, c.want)
		}
	}
}

func TestMigrateCreatesAllTables(t *testing.T) {
	database := openMigrated(t)

	tables := []string{
		"assets",
		"positions",
		"asset_metadata",
		"import_jobs",
		"sentiment_snapshots",
		"sentiment_sources",
		"sentiment_refresh_log",
	}
	for _, table := range tables {
		var name string
		err := database.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	database := openMigrated(t)
	if err := Migrate(database); err != nil {
		t.Fatalf("second Migrate call returned error: %v", err)
	}
}

func TestMigratePositionsHasHiddenColumn(t *testing.T) {
	database := openMigrated(t)

	_, err := database.Exec(
		`INSERT INTO assets(ticker, asset_type) VALUES('TEST','stock')`)
	if err != nil {
		t.Fatalf("insert asset: %v", err)
	}
	_, err = database.Exec(
		`INSERT INTO positions(user_id, asset_id, quantity, avg_price, hidden) VALUES(1,1,10,50,1)`)
	if err != nil {
		t.Fatalf("insert position with hidden column: %v", err)
	}

	var hidden int
	if err := database.QueryRow(`SELECT hidden FROM positions WHERE asset_id=1`).Scan(&hidden); err != nil {
		t.Fatalf("scan hidden: %v", err)
	}
	if hidden != 1 {
		t.Errorf("hidden: got %d, want 1", hidden)
	}
}

func TestMigrateForeignKeyEnforced(t *testing.T) {
	database := openMigrated(t)

	_, err := database.Exec(
		`INSERT INTO positions(user_id, asset_id, quantity, avg_price) VALUES(1, 9999, 10, 50)`)
	if err == nil {
		t.Error("expected foreign key violation, got nil")
	}
}
