package tables

import (
	"database/sql"
)

func init() {
	MigrationClient.AddMigration(Up_20260610172952, Down_20260610172952)
}

func Up_20260610172952(tx *sql.Tx) error {
	// Idempotent migration.
	// has_acme_payload lets the RemoveProfile CertificateList trigger detect an
	// ACME profile without re-reading the by-then-deleted config profile.
	// Backfill from still-present config profiles; explicitly reassign
	// updated_at to its own current value so the backfill doesn't bump the
	// ON UPDATE CURRENT_TIMESTAMP timestamp. This relies on documented
	// MySQL/MariaDB behavior: a column is only considered "changed" (and thus
	// triggers ON UPDATE CURRENT_TIMESTAMP) if it's explicitly assigned a
	// value different from its current one. If the updated_at column
	// definition on host_mdm_apple_profiles is ever changed to remove
	// ON UPDATE CURRENT_TIMESTAMP, or this migration is run against a
	// different engine, this trick has no effect either way, so it remains
	// safe; but if a future migration changes updated_at's semantics such
	// that self-assignment does trigger an update, this comment and technique
	// should be revisited.
	steps := []migrationStep{}
	if !columnExists(tx, "host_mdm_apple_profiles", "has_acme_payload") {
		steps = append(steps, basicMigrationStep(
			`ALTER TABLE host_mdm_apple_profiles ADD COLUMN has_acme_payload TINYINT(1) NOT NULL DEFAULT 0`,
			"adding has_acme_payload to host_mdm_apple_profiles",
		))
	}
	// UPDATE ... WHERE is naturally idempotent; always run the backfill.
	steps = append(steps, basicMigrationStep(
		`UPDATE host_mdm_apple_profiles hmap
				JOIN mdm_apple_configuration_profiles mac ON mac.profile_uuid = hmap.profile_uuid
				SET hmap.has_acme_payload = 1, hmap.updated_at = hmap.updated_at
				WHERE LOCATE('com.apple.security.acme', mac.mobileconfig) > 0`,
		"backfilling has_acme_payload from config profiles",
	))
	return withSteps(steps, tx)
}

func Down_20260610172952(tx *sql.Tx) error {
	return nil
}
