package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260217141240, Down_20260217141240)
}

func Up_20260217141240(tx *sql.Tx) error {
	// Idempotent migration. Naturally re-runnable (UPDATE/MODIFY/JSON-config only).
	_, err := tx.Exec(`UPDATE labels SET platform = '' WHERE platform NOT IN ('', 'centos', 'darwin', 'windows', 'ubuntu')`)
	if err != nil {
		return fmt.Errorf("resetting invalid platform on labels: %w", err)
	}
	return nil
}

func Down_20260217141240(tx *sql.Tx) error {
	return nil
}
