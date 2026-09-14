package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260423161824, Down_20260423161824)
}

func Up_20260423161824(tx *sql.Tx) error {
	// Instead of an unconditional, unrecoverable DROP, rename the table to
	// preserve any existing data during a deprecation period. This allows a
	// rollback path (Down) to restore the original table name, and avoids
	// hard failures if another still-deployed service version expects
	// windows_updates to exist.
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'windows_updates'`).Scan(&exists); err != nil {
		return fmt.Errorf("check windows_updates table existence: %w", err)
	}
	if exists == 0 {
		return nil
	}

	if _, err := tx.Exec(`RENAME TABLE windows_updates TO windows_updates_deprecated`); err != nil {
		return fmt.Errorf("rename windows_updates table: %w", err)
	}
	return nil
}

func Down_20260423161824(tx *sql.Tx) error {
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'windows_updates_deprecated'`).Scan(&exists); err != nil {
		return fmt.Errorf("check windows_updates_deprecated table existence: %w", err)
	}
	if exists == 0 {
		return nil
	}

	if _, err := tx.Exec(`RENAME TABLE windows_updates_deprecated TO windows_updates`); err != nil {
		return fmt.Errorf("rename windows_updates_deprecated table: %w", err)
	}
	return nil
}
