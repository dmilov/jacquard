package store

import "database/sql"

// Migrate creates the Jacquard schema if it doesn't already exist.
func Migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS launched_looms (
			id         TEXT     NOT NULL PRIMARY KEY,
			name       TEXT     NOT NULL DEFAULT '',
			command    TEXT     NOT NULL,
			work_dir   TEXT     NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL
		);
	`)
	return err
}
