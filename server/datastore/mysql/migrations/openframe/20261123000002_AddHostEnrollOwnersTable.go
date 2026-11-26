package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20261123000002, Down_20261123000002)
}

func Up_20261123000002(tx *sql.Tx) error {
	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS openframe_host_enroll_owners (
  host_id INT UNSIGNED NOT NULL,
  instance_id VARCHAR(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  updated_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (host_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return fmt.Errorf("creating openframe_host_enroll_owners: %w", err)
	}
	return nil
}

func Down_20261123000002(tx *sql.Tx) error {
	return nil
}
