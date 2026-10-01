package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20250807140441, Down_20250807140441)
}

func Up_20250807140441(tx *sql.Tx) error {
	// Idempotent migration.
	// Update batch activities table to add new columns and rename existing ones.
	// Each operation is guarded independently so that the migration can be
	// safely re-run if a previous attempt partially applied changes.
	if !columnExists(tx, "batch_activities", "started_at") {
		if _, err := tx.Exec(`
ALTER TABLE batch_activities
ADD COLUMN started_at datetime NULL DEFAULT NULL AFTER updated_at;
`); err != nil {
			return fmt.Errorf("failed to add started_at column to batch_activities: %w", err)
		}
	}

	if columnExists(tx, "batch_activities", "completed_at") && !columnExists(tx, "batch_activities", "finished_at") {
		if _, err := tx.Exec(`
ALTER TABLE batch_activities
RENAME COLUMN completed_at TO finished_at;
`); err != nil {
			return fmt.Errorf("failed to rename completed_at column on batch_activities: %w", err)
		}
	}

	if !columnExists(tx, "batch_activities", "canceled") {
		if _, err := tx.Exec(`
ALTER TABLE batch_activities
ADD COLUMN canceled bool DEFAULT false AFTER finished_at;
`); err != nil {
			return fmt.Errorf("failed to add canceled column to batch_activities: %w", err)
		}
	}

	if columnExists(tx, "batch_activities", "canceled_at") {
		if _, err := tx.Exec(`
ALTER TABLE batch_activities
DROP COLUMN canceled_at;
`); err != nil {
			return fmt.Errorf("failed to drop canceled_at column from batch_activities: %w", err)
		}
	}

	return nil
}

func Down_20250807140441(tx *sql.Tx) error {
	return nil
}
