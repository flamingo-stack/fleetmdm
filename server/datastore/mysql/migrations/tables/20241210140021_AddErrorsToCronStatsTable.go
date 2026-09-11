package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20241210140021, Down_20241210140021)
}

func Up_20241210140021(tx *sql.Tx) error {
	// Idempotent migration.
	// Add columns
	if !columnExists(tx, "cron_stats", "errors") {
		_, err := tx.Exec(`ALTER TABLE cron_stats ADD COLUMN errors JSON`)
		if err != nil {
			return fmt.Errorf("failed to add errors to cron_stats: %w", err)
		}
	}
	return nil
}

func Down_20241210140021(tx *sql.Tx) error {
	return nil
}
