package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260620000001, Down_20260620000001)
}

// Up_20260620000001 changes label name uniqueness from a global UNIQUE(name) to a per-team
// unique so that, under shared-database multitenancy, different teams (tenants)
// may each define a label with the same name. Built-in / global labels keep team_id = NULL and
// remain shared.
//
// The unique is NOT a plain (team_id, name): MySQL treats NULL as distinct in unique keys, so
// that composite would silently stop enforcing name uniqueness among team_id = NULL rows — the
// only rows single-tenant / flag-off Fleet has — and would also break ApplyLabelSpecs'
// INSERT ... ON DUPLICATE KEY UPDATE upsert (duplicate built-in labels instead of updates).
// Instead, the unique is built over a VIRTUAL generated column openframe_team_key =
// IFNULL(team_id, 0), which collapses all NULL-team rows onto the sentinel 0 (team ids are
// auto-increment starting at 1, so 0 can never collide with a real team):
//   - flag off / pre-backfill: every row has team_id = NULL ⇒ key 0 ⇒ UNIQUE(name) exactly as
//     upstream, a bit for bit;
//   - per tenant: at most one label per (team, name), which is the multitenancy invariant.
//
// Column order (name, openframe_team_key) keeps the index usable by the existing
// `WHERE name = ?` prefix lookups (LabelByName, LabelsByName).
// Upstream precedent for UNIQUE over a generated column: tables/20240905200001_AddPoliciesToNoTeam.go.
// The column is mapped by an ignored field on fleet.Label so `SELECT l.*` sqlx scans keep working.
//
// Idempotent.
//
// SEMANTIC-CONFLICT WATCHLIST (openframe/docs/upstream-sync-conflict-resolution.md):
// this ALTERs the upstream `labels` table from the OpenFrame pipeline. If a future
// upstream release changes the labels table's uniqueness or rebuilds the table,
// re-verify this migration after the sync.
func Up_20260620000001(tx *sql.Tx) error {
	const (
		table     = "labels"
		oldIndex  = "idx_label_unique_name"
		newIndex  = "idx_label_team_name"
		genColumn = "openframe_team_key"
	)

	hasCol, err := columnExists(tx, table, genColumn)
	if err != nil {
		return fmt.Errorf("checking %s column: %w", genColumn, err)
	}
	if !hasCol {
		if _, err := tx.Exec(
			"ALTER TABLE labels ADD COLUMN openframe_team_key INT UNSIGNED GENERATED ALWAYS AS (IFNULL(team_id, 0)) VIRTUAL",
		); err != nil {
			return fmt.Errorf("adding %s generated column: %w", genColumn, err)
		}
	}

	hasNew, err := indexExists(tx, table, newIndex)
	if err != nil {
		return fmt.Errorf("checking %s index: %w", newIndex, err)
	}
	if !hasNew {
		// Safe to add: the existing UNIQUE(name) guarantees (name, IFNULL(team_id,0)) is
		// already unique.
		if _, err := tx.Exec("ALTER TABLE labels ADD UNIQUE KEY idx_label_team_name (name, openframe_team_key)"); err != nil {
			return fmt.Errorf("adding %s unique index: %w", newIndex, err)
		}
	}

	hasOld, err := indexExists(tx, table, oldIndex)
	if err != nil {
		return fmt.Errorf("checking %s index: %w", oldIndex, err)
	}
	if hasOld {
		if _, err := tx.Exec("ALTER TABLE labels DROP INDEX idx_label_unique_name"); err != nil {
			return fmt.Errorf("dropping %s index: %w", oldIndex, err)
		}
	}
	return nil
}

func Down_20260620000001(tx *sql.Tx) error {
	return nil
}
