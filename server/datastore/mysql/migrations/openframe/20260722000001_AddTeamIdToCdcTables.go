package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260722000001, Down_20260722000001)
}

// Up_20260722000001 adds a nullable `team_id` column to the four tables captured by the
// OpenFrame Debezium CDC pipeline: activity_past, activity_host_past, query_results and
// policy_membership. Under shared-database multitenancy one MySQL serves every tenant, so a
// CDC record must carry its own tenant discriminator — the connector's SMTs are stateless and
// cannot join host_id → hosts.team_id, and activity_past rows have no host reference at all.
// The column is stamped at write time from the request's team pin (fleet.OpenframeTeamID) or,
// for writes without a pinned context (async policy membership, host activities), from the
// host's own team via a scalar subselect.
//
// Rows written with the multitenancy flag off (or by unpinned background jobs) keep team_id
// NULL — byte-identical to pre-migration behavior. No index: nothing queries these tables by
// team; the column exists solely so the change-stream consumer can resolve the tenant.
//
// Intentionally NO foreign key to teams(id): a team deletion must not rewrite or cascade over
// millions of historical CDC rows.
//
// Idempotent.
func Up_20260722000001(tx *sql.Tx) error {
	for _, table := range []string{"activity_past", "activity_host_past", "query_results", "policy_membership"} {
		hasCol, err := columnExists(tx, table, "team_id")
		if err != nil {
			return fmt.Errorf("checking %s.team_id column: %w", table, err)
		}
		if hasCol {
			continue
		}
		if _, err := tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN team_id INT UNSIGNED NULL", table)); err != nil {
			return fmt.Errorf("adding team_id column to %s: %w", table, err)
		}
	}
	return nil
}

func Down_20260722000001(tx *sql.Tx) error {
	return nil
}
