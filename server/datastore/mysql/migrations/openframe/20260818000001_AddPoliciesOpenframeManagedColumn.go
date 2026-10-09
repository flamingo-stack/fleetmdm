package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260818000001, Down_20260818000001)
}

// Up_20260818000001 adds `policies.openframe_managed` — the flag that keeps a policy out of the policy list
// endpoints (the set the main UI renders) while it keeps running on hosts and keeps reporting
// results. See openframe/docs/managed-policies.md.
//
// It is also the escape hatch for host-assignment scoping: an OpenFrame-managed policy runs on every in-scope
// host without any policy_hosts rows, so a platform-owned check needs no per-host assignment
// backfill for hosts that enroll later.
//
// Idempotent.
//
// SEMANTIC-CONFLICT WATCHLIST (openframe/docs/upstream-sync-conflict-resolution.md):
// this ALTERs the upstream `policies` table from the OpenFrame pipeline. If a future upstream
// release rebuilds that table, re-verify this migration after the sync.
func Up_20260818000001(tx *sql.Tx) error {
	const (
		table  = "policies"
		column = "openframe_managed"
	)

	hasCol, err := columnExists(tx, table, column)
	if err != nil {
		return fmt.Errorf("checking %s.%s column: %w", table, column, err)
	}
	if hasCol {
		return nil
	}

	if _, err := tx.Exec("ALTER TABLE policies ADD COLUMN openframe_managed TINYINT(1) NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("adding %s.%s column: %w", table, column, err)
	}
	return nil
}

func Down_20260818000001(tx *sql.Tx) error {
	return nil
}
