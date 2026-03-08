package db

import (
	"database/sql"
	"os"
	"path/filepath"

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
	}
	for _, stmt := range stmts {
		if _, err := database.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
