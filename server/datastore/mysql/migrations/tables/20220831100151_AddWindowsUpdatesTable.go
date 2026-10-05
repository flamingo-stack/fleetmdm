package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20220831100151, Down_20220831100151)
}

func Up_20220831100151(tx *sql.Tx) error {
	// Idempotent migration.
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS windows_updates (
	id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
	host_id INT UNSIGNED NOT NULL,
	date_epoch INT UNSIGNED NOT NULL,
	kb_id INT UNSIGNED NOT NULL,
	UNIQUE KEY idx_unique_windows_updates (host_id, kb_id),
	KEY idx_update_date (host_id, date_epoch)
)`)
	if err != nil {
		return fmt.Errorf("create windows_updates table: %w", err)
	}
	return nil
}

func Down_20220831100151(tx *sql.Tx) error {
	return nil
}
