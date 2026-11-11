package openframe

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20261111000002, Down_20261111000002)
}

// Up_20261111000002 adds `openframe_apple_enrollment_intents`: in shared mode, the team an Apple device asked to enroll
// into through another tenant's OTA link while its host row still belongs to a different team. The MDM Authenticate
// check-in applies it. See openframe/docs/mysql-multitenancy-feature.md.
//
// Idempotent.
func Up_20261111000002(tx *sql.Tx) error {
	if _, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS openframe_apple_enrollment_intents (
  hardware_serial VARCHAR(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  uuid VARCHAR(255) COLLATE utf8mb4_unicode_ci NOT NULL,
  team_id INT UNSIGNED DEFAULT NULL,
  idp_uuid VARCHAR(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (hardware_serial)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return fmt.Errorf("creating openframe_apple_enrollment_intents: %w", err)
	}
	return nil
}

func Down_20261111000002(tx *sql.Tx) error {
	return nil
}
