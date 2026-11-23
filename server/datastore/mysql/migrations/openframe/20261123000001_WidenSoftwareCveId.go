package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20261123000001, Down_20261123000001)
}

// Up_20261123000001 widens `software_cve.id` from INT UNSIGNED to BIGINT UNSIGNED.
//
// The vulnerability cron re-saves every CVE match on each run with INSERT ... ON DUPLICATE KEY UPDATE, and InnoDB
// takes a new auto-increment value for every row it attempts, even when the row already exists. OpenFrame runs one
// vulnerability cron per tenant cluster against the shared database, so the INT range (~4.29 billion) runs out and
// every insert fails with error 1467 ("Failed to read auto-increment value from storage engine"). Nothing reads
// software_cve.id, so only the column type changes.
//
// Idempotent: a no-op once the column is BIGINT.
func Up_20261123000001(tx *sql.Tx) error {
	var dataType string
	if err := tx.QueryRow(
		`SELECT DATA_TYPE FROM information_schema.COLUMNS
		WHERE table_schema = DATABASE() AND table_name = 'software_cve' AND column_name = 'id'`,
	).Scan(&dataType); err != nil {
		return fmt.Errorf("reading software_cve.id type: %w", err)
	}
	if dataType == "bigint" {
		return nil
	}

	if _, err := tx.Exec("ALTER TABLE software_cve MODIFY id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT"); err != nil {
		return fmt.Errorf("widening software_cve.id: %w", err)
	}
	return nil
}

func Down_20261123000001(tx *sql.Tx) error {
	return nil
}
