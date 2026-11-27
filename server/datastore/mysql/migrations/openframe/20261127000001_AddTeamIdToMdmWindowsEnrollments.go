package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20261127000001, Down_20261127000001)
}

// Up_20261127000001 adds a nullable `team_id` column to mdm_windows_enrollments: the tenant team the
// device enrolled into, stamped at enrollment from the host of the orbit node key. In shared mode the
// device's other identifiers (hardware id, device id, host uuid) come from the device, so the
// re-enrollment cleanup, the host link and the management session take the tenant from this column.
//
// Rows enrolled before this migration, or outside shared mode, keep team_id NULL; no data changes.
//
// Idempotent.
func Up_20261127000001(tx *sql.Tx) error {
	hasCol, err := columnExists(tx, "mdm_windows_enrollments", "team_id")
	if err != nil {
		return fmt.Errorf("checking mdm_windows_enrollments.team_id column: %w", err)
	}
	if hasCol {
		return nil
	}
	if _, err := tx.Exec("ALTER TABLE mdm_windows_enrollments ADD COLUMN team_id INT UNSIGNED NULL"); err != nil {
		return fmt.Errorf("adding team_id column to mdm_windows_enrollments: %w", err)
	}
	return nil
}

func Down_20261127000001(tx *sql.Tx) error {
	return nil
}
