package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260409153716, Down_20260409153716)
}

func Up_20260409153716(tx *sql.Tx) error {
	hasAwaitingConfiguration := columnExists(tx, "mdm_windows_enrollments", "awaiting_configuration")
	hasAwaitingConfigurationAt := columnExists(tx, "mdm_windows_enrollments", "awaiting_configuration_at")

	if hasAwaitingConfiguration && hasAwaitingConfigurationAt {
		return nil
	}

	if !hasAwaitingConfiguration {
		if _, err := tx.Exec(`
			ALTER TABLE mdm_windows_enrollments
			ADD COLUMN awaiting_configuration TINYINT(1) NOT NULL DEFAULT 0
		`); err != nil {
			return fmt.Errorf("failed to add awaiting_configuration column to mdm_windows_enrollments: %w", err)
		}
	}

	if !hasAwaitingConfigurationAt {
		if _, err := tx.Exec(`
			ALTER TABLE mdm_windows_enrollments
			ADD COLUMN awaiting_configuration_at DATETIME(6) DEFAULT NULL
		`); err != nil {
			return fmt.Errorf("failed to add awaiting_configuration_at column to mdm_windows_enrollments: %w", err)
		}
	}

	return nil
}

func Down_20260409153716(tx *sql.Tx) error {
	return nil
}

