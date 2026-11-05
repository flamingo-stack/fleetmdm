package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20261105000001, Down_20261105000001)
}

// Up_20261105000001 adds `software_arp_keys`: the Uninstall registry subkey name (osquery `programs.arp_key`) of a
// Windows `software` row. It is kept beside `software` rather than in it so the software checksum, and with it every
// existing row, stays as it is. See openframe/docs/software-arp-keys.md.
//
// Idempotent.
func Up_20261105000001(tx *sql.Tx) error {
	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS software_arp_keys (
  software_id BIGINT UNSIGNED NOT NULL,
  arp_key VARCHAR(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  updated_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (software_id),
  CONSTRAINT fk_software_arp_keys_software_id FOREIGN KEY (software_id) REFERENCES software (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return fmt.Errorf("creating software_arp_keys: %w", err)
	}
	return nil
}

func Down_20261105000001(tx *sql.Tx) error {
	return nil
}
