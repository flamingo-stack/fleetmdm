package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20251209221850, Down_20251209221850)
}

// >>> OPENFRAME(host-certificate-templates-status-default): modifies shared host_certificate_templates table columns (fleet_challenge, status) — openframe/docs/host-certificate-templates.md
func Up_20251209221850(tx *sql.Tx) error {
	// Idempotent migration.
	// Make fleet_challenge nullable (it will be NULL for pending records,
	// and populated when transitioning to delivering).
	if columnExists(tx, "host_certificate_templates", "fleet_challenge") {
		if _, err := tx.Exec(`
		ALTER TABLE host_certificate_templates
		MODIFY COLUMN fleet_challenge char(32) COLLATE utf8mb4_unicode_ci NULL
	`); err != nil {
			return fmt.Errorf("make fleet_challenge nullable: %w", err)
		}
	}

	// Add default 'pending' to status column.
	if columnExists(tx, "host_certificate_templates", "status") {
		if _, err := tx.Exec(`
		ALTER TABLE host_certificate_templates
		MODIFY COLUMN status varchar(20) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending'
	`); err != nil {
			return fmt.Errorf("add default to status column: %w", err)
		}
	}

	return nil
}

// <<< OPENFRAME(host-certificate-templates-status-default)

func Down_20251209221850(tx *sql.Tx) error {
	return nil
}
