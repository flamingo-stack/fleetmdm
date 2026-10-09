package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260818000002, Down_20260818000002)
}

// Up_20260818000002 adds `queries.openframe_managed` — the queries twin of
// `policies.openframe_managed` (20260818000001). A managed query is omitted from the query list and
// its counts while it keeps running on hosts and keeps reporting results.
// See openframe/docs/managed-queries.md.
//
// Idempotent.
//
// SEMANTIC-CONFLICT WATCHLIST (openframe/docs/upstream-sync-conflict-resolution.md):
// this ALTERs the upstream `queries` table from the OpenFrame pipeline. If a future upstream
// release rebuilds that table, re-verify this migration after the sync.
func Up_20260818000002(tx *sql.Tx) error {
	const (
		table  = "queries"
		column = "openframe_managed"
	)

	hasCol, err := columnExists(tx, table, column)
	if err != nil {
		return fmt.Errorf("checking %s.%s column: %w", table, column, err)
	}
	if hasCol {
		return nil
	}

	if _, err := tx.Exec("ALTER TABLE queries ADD COLUMN openframe_managed TINYINT(1) NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("adding %s.%s column: %w", table, column, err)
	}
	return nil
}

func Down_20260818000002(tx *sql.Tx) error {
	return nil
}
