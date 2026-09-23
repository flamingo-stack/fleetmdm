package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20251208215800, Down_20251208215800)
}

// >>> OPENFRAME(host-cert-templates-operation-type): Add operation_type column and FK to mdm_operation_types for host_certificate_templates, tracking install/remove state consistent with other MDM profile tables — openframe/docs/mdm-certificate-templates.md
func Up_20251208215800(tx *sql.Tx) error {
	// Idempotent migration.
	// Add operation_type column to host_certificate_templates table.
	// This column tracks whether the certificate is being installed or removed,
	// consistent with other MDM profile tables.
	// Note: VARCHAR(20) with FK constraint is not efficient, but consistent with the other similar tables.
	// A more efficient approach would be: ENUM('install', 'remove')
	if !columnExists(tx, "host_certificate_templates", "operation_type") {
		addColumnStmt := `
ALTER TABLE host_certificate_templates
	ADD COLUMN operation_type VARCHAR(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'install'
`
		if _, err := tx.Exec(addColumnStmt); err != nil {
			return fmt.Errorf("add operation_type column to host_certificate_templates: %w", err)
		}
	}

	if !constraintExists(tx, "host_certificate_templates", "fk_host_certificate_templates_operation_type") {
		if _, err := tx.Exec(`
ALTER TABLE host_certificate_templates
	ADD CONSTRAINT fk_host_certificate_templates_operation_type
		FOREIGN KEY (operation_type) REFERENCES mdm_operation_types (operation_type) ON UPDATE CASCADE
`); err != nil {
			return fmt.Errorf("add FK fk_host_certificate_templates_operation_type: %w", err)
		}
	}

	return nil
}

func Down_20251208215800(tx *sql.Tx) error {
	return nil
}

// <<< OPENFRAME(host-cert-templates-operation-type)
