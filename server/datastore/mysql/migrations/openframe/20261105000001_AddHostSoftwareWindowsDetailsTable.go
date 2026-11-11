package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20261105000001, Down_20261105000001)
}

// Up_20261105000001 adds `host_software_windows_details`: what the OpenFrame osquery `programs` table reports about a
// program installed on a host beyond what Fleet keeps (ARP key, package family name, SystemComponent, installer type,
// scope, architecture). Kept per host like `host_software_installed_paths`, because the same `software` row is shared
// by every host and tenant while an install's key and installer differ per host. See
// openframe/docs/software-windows-details.md.
//
// Idempotent.
func Up_20261105000001(tx *sql.Tx) error {
	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS host_software_windows_details (
  host_id INT UNSIGNED NOT NULL,
  software_id BIGINT UNSIGNED NOT NULL,
  arp_key VARCHAR(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  package_family_name VARCHAR(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  system_component TINYINT(1) DEFAULT NULL,
  installer_type VARCHAR(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  install_scope VARCHAR(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  install_arch VARCHAR(8) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  updated_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (host_id, software_id),
  KEY idx_host_software_windows_details_software_id (software_id),
  CONSTRAINT fk_host_software_windows_details_software_id FOREIGN KEY (software_id) REFERENCES software (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return fmt.Errorf("creating host_software_windows_details: %w", err)
	}
	return nil
}

func Down_20261105000001(tx *sql.Tx) error {
	return nil
}
