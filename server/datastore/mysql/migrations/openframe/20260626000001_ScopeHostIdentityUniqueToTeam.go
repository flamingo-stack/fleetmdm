package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260626000001, Down_20260626000001)
}

// Up_20260626000001 changes host osquery-identity uniqueness from a global
// UNIQUE(osquery_host_id) to a per-team unique so that, under shared-database
// multitenancy, the SAME physical device (same hardware UUID /
// osquery identifier) may exist in more than one tenant team.
//
// Why this is needed: the enrollment matcher fence (matchHostDuringEnrollment) scopes host matching
// to the request's tenant team. So when a device already enrolled in tenant A
// enrolls in tenant B, the matcher correctly finds no team-B match and falls to
// a plain INSERT — which, with a GLOBAL UNIQUE(osquery_host_id), fails with a
// duplicate-key error and blocks the legitimate cross-tenant enrollment (and can
// also abort the data merge). Scoping the unique per team makes "at most one
// host per (team, osquery_host_id)" the invariant — exactly what the fence
// enforces on the read side.
//
// The unique is NOT a plain (team_id, osquery_host_id): MySQL treats NULL as
// distinct in unique keys, so that composite would silently stop enforcing
// osquery-identity uniqueness among team_id = NULL ("No team") rows — the only
// rows single-tenant / flag-off Fleet has. Instead, the unique is built over a
// VIRTUAL generated column openframe_team_key = IFNULL(team_id, 0), which
// collapses all NULL-team rows onto the sentinel 0 (team ids are auto-increment
// starting at 1, so 0 can never collide with a real team):
//   - flag off / pre-backfill: every row has team_id = NULL ⇒ key 0 ⇒
//     UNIQUE(osquery_host_id) exactly as upstream, a bit for bit;
//   - per tenant: at most one host per (team, osquery_host_id).
//
// Column order (osquery_host_id, openframe_team_key) keeps the index usable by
// the existing `WHERE osquery_host_id = ?` prefix lookups (matchHostDuringEnrollment).
// osquery_host_id itself stays nullable; rows with a NULL osquery_host_id remain
// exempt from the unique (any NULL key part ⇒ no constraint), as upstream.
// Upstream precedent for UNIQUE over a generated column: tables/20240905200001_AddPoliciesToNoTeam.go.
// (hosts has no `SELECT *` sqlx scans, so no struct-mapping change is needed.)
//
// node_key / orbit_node_key are deliberately LEFT global-unique: they are random
// auth secrets (bearer tokens) resolved by `WHERE node_key = ?` with no team
// context, so global uniqueness is correct there.
//
// Idempotent.
//
// SEMANTIC-CONFLICT WATCHLIST (openframe/docs/upstream-sync-conflict-resolution.md):
// this ALTERs the upstream `hosts` table from the OpenFrame pipeline. If a future
// upstream release changes the hosts table's uniqueness (osquery_host_id) or
// rebuilds the table, re-verify this migration after the sync.
func Up_20260626000001(tx *sql.Tx) error {
	const (
		table     = "hosts"
		oldIndex  = "idx_osquery_host_id"
		newIndex  = "idx_hosts_team_osquery_host_id"
		genColumn = "openframe_team_key"
	)

	hasCol, err := columnExists(tx, table, genColumn)
	if err != nil {
		return fmt.Errorf("checking %s column: %w", genColumn, err)
	}
	if !hasCol {
		if _, err := tx.Exec(
			"ALTER TABLE hosts ADD COLUMN openframe_team_key INT UNSIGNED GENERATED ALWAYS AS (IFNULL(team_id, 0)) VIRTUAL",
		); err != nil {
			return fmt.Errorf("adding %s generated column: %w", genColumn, err)
		}
	}

	hasNew, err := indexExists(tx, table, newIndex)
	if err != nil {
		return fmt.Errorf("checking %s index: %w", newIndex, err)
	}
	if !hasNew {
		// Safe to add: the existing global UNIQUE(osquery_host_id) guarantees the
		// superset (osquery_host_id, IFNULL(team_id,0)) is already unique, so this cannot fail.
		if _, err := tx.Exec("ALTER TABLE hosts ADD UNIQUE KEY idx_hosts_team_osquery_host_id (osquery_host_id, openframe_team_key)"); err != nil {
			return fmt.Errorf("adding %s unique index: %w", newIndex, err)
		}
	}

	hasOld, err := indexExists(tx, table, oldIndex)
	if err != nil {
		return fmt.Errorf("checking %s index: %w", oldIndex, err)
	}
	if hasOld {
		if _, err := tx.Exec("ALTER TABLE hosts DROP INDEX idx_osquery_host_id"); err != nil {
			return fmt.Errorf("dropping %s index: %w", oldIndex, err)
		}
	}
	return nil
}

func Down_20260626000001(tx *sql.Tx) error {
	return nil
}
