package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260629000001, Down_20260629000001)
}

// Up_20260629000001 adds the bridge column `teams.openframe_tenant_uuid` (+ a unique index) that
// maps a Flamingo tenant UUID to its Fleet team. Under shared-database multitenancy
// each OpenFrame process is pinned to its tenant by the Flamingo tenant UUID
// (FLEET_OPENFRAME_TENANT_UUID), which is resolved at startup to Fleet's integer `team_id` via this
// column (EnsureOpenframeTeamID). Fleet's `team_id` stays an int (referenced by FKs everywhere); the
// UUID stays the platform identity — this column is the bridge, so neither id format changes.
//
// The column is NULL for non-OpenFrame teams; the unique index permits many NULLs (MySQL treats
// NULL as distinct) and enforces one team per tenant UUID.
//
// Idempotent.
//
// SEMANTIC-CONFLICT WATCHLIST (openframe/docs/upstream-sync-conflict-resolution.md):
// this ALTERs the upstream `teams` table from the OpenFrame pipeline. If a future upstream release
// rebuilds the teams table, re-verify this migration after the sync.
func Up_20260629000001(tx *sql.Tx) error {
	const (
		table  = "teams"
		column = "openframe_tenant_uuid"
		index  = "idx_teams_openframe_tenant_uuid"
	)

	hasCol, err := columnExists(tx, table, column)
	if err != nil {
		return fmt.Errorf("checking %s.%s column: %w", table, column, err)
	}
	if !hasCol {
		if _, err := tx.Exec("ALTER TABLE teams ADD COLUMN openframe_tenant_uuid CHAR(36) NULL"); err != nil {
			return fmt.Errorf("adding %s column: %w", column, err)
		}
	}

	hasIdx, err := indexExists(tx, table, index)
	if err != nil {
		return fmt.Errorf("checking %s index: %w", index, err)
	}
	if !hasIdx {
		if _, err := tx.Exec("ALTER TABLE teams ADD UNIQUE KEY idx_teams_openframe_tenant_uuid (openframe_tenant_uuid)"); err != nil {
			return fmt.Errorf("adding %s unique index: %w", index, err)
		}
	}
	return nil
}

func Down_20260629000001(tx *sql.Tx) error {
	return nil
}
