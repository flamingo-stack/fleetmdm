package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260316120008, Down_20260316120008)
}

func Up_20260316120008(tx *sql.Tx) error {
	// Idempotent migration.
	// RENAME TABLE is not re-runnable; skip if the source tables are already renamed.
	if !tableExists(tx, "activities") {
		return nil
	}
	_, err := tx.Exec(`RENAME TABLE activities TO activity_past, host_activities TO activity_host_past`)
	if err != nil {
		return fmt.Errorf("rename activities tables: %w", err)
	}
	return nil
}

func Down_20260316120008(tx *sql.Tx) error {
	// Reverse the rename performed in Up_20260316120008, if it was applied.
	// This provides a rollback path for the destructive RENAME TABLE above.
	if !tableExists(tx, "activity_past") {
		return nil
	}
	_, err := tx.Exec(`RENAME TABLE activity_past TO activities, activity_host_past TO host_activities`)
	if err != nil {
		return fmt.Errorf("revert rename of activities tables: %w", err)
	}
	return nil
}

