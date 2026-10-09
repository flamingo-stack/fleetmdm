package openframe

import (
	"database/sql"

	"github.com/fleetdm/fleet/v4/server/goose"
)

// MigrationClient tracks openframe-specific schema changes independently
// from the upstream Fleet migration pipeline (migration_status_tables).
// This avoids version conflicts when rebasing onto newer upstream releases.
var MigrationClient = goose.New("migration_status_openframe", goose.MySqlDialect{})

// indexExists reports whether the named index exists on table in the current database.
// OpenFrame migrations use it to make index DDL idempotent, since MySQL has no
// DROP/CREATE INDEX IF [NOT] EXISTS.
func indexExists(tx *sql.Tx, table, index string) (bool, error) {
	var count int
	if err := tx.QueryRow(
		`SELECT COUNT(1) FROM information_schema.STATISTICS
		WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`,
		table, index,
	).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// columnExists reports whether the named column exists on table in the current database.
// Used to make ADD COLUMN DDL idempotent (MySQL has no ADD COLUMN IF NOT EXISTS).
func columnExists(tx *sql.Tx, table, column string) (bool, error) {
	var count int
	if err := tx.QueryRow(
		`SELECT COUNT(1) FROM information_schema.COLUMNS
		WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
		table, column,
	).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
