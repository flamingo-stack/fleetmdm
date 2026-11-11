package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20261111000001, Down_20261111000001)
}

// Up_20261111000001 adds `host_software_windows_details.winget_id`: the winget package id of a software row the
// OpenFrame osquery `winget_packages` table reported on the host. Fleet's software row has no column for it. See
// openframe/docs/winget-packages.md.
//
// Idempotent.
func Up_20261111000001(tx *sql.Tx) error {
	const (
		table  = "host_software_windows_details"
		column = "winget_id"
	)

	hasCol, err := columnExists(tx, table, column)
	if err != nil {
		return fmt.Errorf("checking %s.%s column: %w", table, column, err)
	}
	if hasCol {
		return nil
	}

	if _, err := tx.Exec("ALTER TABLE host_software_windows_details ADD COLUMN winget_id VARCHAR(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL AFTER install_arch"); err != nil {
		return fmt.Errorf("adding %s.%s column: %w", table, column, err)
	}
	return nil
}

func Down_20261111000001(tx *sql.Tx) error {
	return nil
}
