package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20261105000001, Down_20261105000001)
}

// Up_20261105000001 adds `software_windows_details`: what the OpenFrame osquery `programs` table reports about a
// Windows `software` row beyond what Fleet keeps (ARP key, package family name, SystemComponent, installer type,
// scope, architecture). It is kept beside `software` rather than in it so the software checksum, and with it every
// existing row, stays as it is. See openframe/docs/software-windows-details.md.
//
// Idempotent.
func Up_20261105000001(tx *sql.Tx) error {
	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS software_windows_details (
  software_id BIGINT UNSIGNED NOT NULL,
  arp_key VARCHAR(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  package_family_name VARCHAR(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  system_component TINYINT(1) DEFAULT NULL,
  installer_type VARCHAR(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  install_scope VARCHAR(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  install_arch VARCHAR(8) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  updated_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (software_id),
  CONSTRAINT fk_software_windows_details_software_id FOREIGN KEY (software_id) REFERENCES software (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return fmt.Errorf("creating software_windows_details: %w", err)
	}
	return nil
}

func Down_20261105000001(tx *sql.Tx) error {
	return nil
}
