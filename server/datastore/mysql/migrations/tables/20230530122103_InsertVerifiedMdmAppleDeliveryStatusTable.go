package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20230530122103, Down_20230530122103)
}

func Up_20230530122103(tx *sql.Tx) error {
	// Idempotent migration.
	_, err := tx.Exec(`INSERT IGNORE INTO mdm_apple_delivery_status (status) VALUES(?)`, "verified")
	if err != nil {
		return fmt.Errorf("insert verified mdm_apple_delivery_status: %w", err)
	}

	return nil
}

func Down_20230530122103(tx *sql.Tx) error {
	return nil
}
