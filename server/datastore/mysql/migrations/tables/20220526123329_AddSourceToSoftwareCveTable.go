package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20220526123329, Down_20220526123329)
}

func Up_20220526123329(tx *sql.Tx) error {
	// Idempotent migration.
	if !columnExists(tx, "software_cve", "source") {
		if _, err := tx.Exec(
			"ALTER TABLE `software_cve` ADD COLUMN `source` int DEFAULT '0'",
		); err != nil {
			return fmt.Errorf("add 'source' column to 'software_cve': %w", err)
		}
	}
	return nil
}

func Down_20220526123329(tx *sql.Tx) error {
	return nil
}

