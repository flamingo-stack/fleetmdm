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
	activitiesExists := tableExists(tx, "activities")
	hostActivitiesExists := tableExists(tx, "host_activities")
	if !activitiesExists && !hostActivitiesExists {
		return nil
	}
	if activitiesExists && hostActivitiesExists {
		_, err := tx.Exec(`RENAME TABLE activities TO activity_past, host_activities TO activity_host_past`)
		if err != nil {
			return fmt.Errorf("rename activities tables: %w", err)
		}
		return nil
	}
	if activitiesExists {
		_, err := tx.Exec(`RENAME TABLE activities TO activity_past`)
		if err != nil {
			return fmt.Errorf("rename activities table: %w", err)
		}
		return nil
	}
	_, err := tx.Exec(`RENAME TABLE host_activities TO activity_host_past`)
	if err != nil {
		return fmt.Errorf("rename host_activities table: %w", err)
	}
	return nil
}

func Down_20260316120008(tx *sql.Tx) error {
	return nil
}
