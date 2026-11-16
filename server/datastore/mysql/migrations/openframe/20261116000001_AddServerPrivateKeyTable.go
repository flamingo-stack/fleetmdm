package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20261116000001, Down_20261116000001)
}

func Up_20261116000001(tx *sql.Tx) error {
	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS openframe_server_private_key (
  id TINYINT UNSIGNED NOT NULL,
  kms_key_name VARCHAR(512) COLLATE utf8mb4_unicode_ci NOT NULL,
  wrapped_key VARBINARY(1024) NOT NULL,
  created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (id),
  CONSTRAINT ck_openframe_server_private_key_single_row CHECK (id = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return fmt.Errorf("creating openframe_server_private_key: %w", err)
	}
	return nil
}

func Down_20261116000001(tx *sql.Tx) error {
	return nil
}
