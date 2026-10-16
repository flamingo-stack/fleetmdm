package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20260218175704, Down_20260218175704)
}

func Up_20260218175704(tx *sql.Tx) error {
	// Idempotent migration.
	if indexExistsTx(tx, "software_installers", "idx_software_installers_team_id_title_id") {
		if _, err := tx.Exec(`ALTER TABLE software_installers DROP INDEX idx_software_installers_team_id_title_id`); err != nil {
			return fmt.Errorf("altering software_installers: %w", err)
		}
	}

	if indexExistsTx(tx, "software_installers", "idx_software_installers_platform_title_id") {
		if _, err := tx.Exec(`ALTER TABLE software_installers DROP INDEX idx_software_installers_platform_title_id`); err != nil {
			return fmt.Errorf("altering software_installers: %w", err)
		}
	}

	if !indexExistsTx(tx, "software_installers", "idx_software_installers_team_title_version") {
		if _, err := tx.Exec(`ALTER TABLE software_installers ADD UNIQUE INDEX idx_software_installers_team_title_version (global_or_team_id,title_id,version)`); err != nil {
			return fmt.Errorf("altering software_installers: %w (this can fail if duplicate (global_or_team_id, title_id, version) rows already exist in software_installers; such duplicates must be de-duplicated before this migration can succeed)", err)
		}
	}

	if !columnExists(tx, "software_installers", "is_active") {
		if _, err := tx.Exec(`ALTER TABLE software_installers ADD COLUMN is_active TINYINT(1) NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("altering software_installers: %w", err)
		}
	}

	// At migration time, the 1-installer-per-title rule is still enforced,
	// so every existing installer is the active one for its title. This
	// depends on the unique index above having succeeded (i.e., no
	// duplicate (global_or_team_id, title_id, version) rows exist); if that
	// index creation failed, we would not reach this point.
	_, err := tx.Exec(`UPDATE software_installers SET is_active = 1`)
	if err != nil {
		return fmt.Errorf("setting is_active for existing installers: %w", err)
	}

	return nil
}

func Down_20260218175704(tx *sql.Tx) error {
	return nil
}
