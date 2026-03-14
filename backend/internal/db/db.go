package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func OpenSQLite(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA synchronous = NORMAL;",
	}
	for _, stmt := range pragmas {
		if _, err := database.Exec(stmt); err != nil {
			_ = database.Close()
			return nil, fmt.Errorf("sqlite pragma failed: %w", err)
		}
	}
	return database, nil
}

func Migrate(database *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS assets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ticker TEXT NOT NULL UNIQUE,
			asset_type TEXT NOT NULL,
			currency TEXT NOT NULL DEFAULT 'BRL'
		);`,
		`CREATE TABLE IF NOT EXISTS positions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			asset_id INTEGER NOT NULL,
			quantity REAL NOT NULL,
			avg_price REAL NOT NULL,
			broker TEXT,
			source TEXT NOT NULL DEFAULT 'b3',
			last_updated TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, asset_id),
			FOREIGN KEY(asset_id) REFERENCES assets(id)
		);`,
		`CREATE TABLE IF NOT EXISTS asset_metadata (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			asset_id INTEGER NOT NULL UNIQUE,
			company_name TEXT,
			tax_id TEXT,
			last_updated TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(asset_id) REFERENCES assets(id)
		);`,
		`CREATE TABLE IF NOT EXISTS import_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL DEFAULT 'b3',
			status TEXT NOT NULL DEFAULT 'queued',
			detail TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS sentiment_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			asset_id INTEGER NOT NULL UNIQUE,
			status TEXT NOT NULL,
			score REAL,
			label TEXT,
			confidence REAL,
			trend TEXT,
			source_count INTEGER NOT NULL DEFAULT 0,
			window_start TEXT,
			window_end TEXT,
			last_refreshed_at TEXT,
			expires_at TEXT,
			last_error TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(asset_id) REFERENCES assets(id)
		);`,
		`CREATE TABLE IF NOT EXISTS sentiment_sources (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id INTEGER NOT NULL,
			source_type TEXT NOT NULL,
			provider TEXT NOT NULL,
			title TEXT NOT NULL,
			url TEXT NOT NULL,
			published_at TEXT,
			language TEXT,
			excerpt TEXT,
			ticker TEXT NOT NULL,
			company_name TEXT,
			sentiment_score REAL,
			weight REAL NOT NULL,
			signal_tags TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(snapshot_id) REFERENCES sentiment_snapshots(id)
		);`,
		`CREATE TABLE IF NOT EXISTS sentiment_refresh_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			asset_id INTEGER NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT,
			status TEXT NOT NULL,
			sources_found INTEGER NOT NULL DEFAULT 0,
			error TEXT,
			FOREIGN KEY(asset_id) REFERENCES assets(id)
		);`,
		`ALTER TABLE positions ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0;`,
	}
	for _, stmt := range stmts {
		if _, err := database.Exec(stmt); err != nil {
			// Ignore "duplicate column" errors from idempotent ALTER TABLE statements.
			if strings.Contains(stmt, "ALTER TABLE") && strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return err
		}
	}
	return nil
}
