package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20251028140000, Down_20251028140000)
}

func Up_20251028140000(tx *sql.Tx) error {
	// Idempotent migration.
	// Create the pre-aggregated OS version vulnerabilities table
	// This table contains ONLY Linux kernel vulnerabilities
	// team_id semantics:
	//   NULL  = "all teams" (pre-aggregated across all teams)
	//   0     = "no team" (hosts without team assignment)
	//   >0    = specific team ID
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS operating_system_version_vulnerabilities (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			os_version_id INT UNSIGNED NOT NULL,
			cve VARCHAR(255) COLLATE utf8mb4_unicode_ci NOT NULL,
			team_id INT UNSIGNED DEFAULT NULL,
			source SMALLINT DEFAULT 0,
			resolved_in_version VARCHAR(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
			created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			updated_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
			PRIMARY KEY (id),
		    -- for inserts
			UNIQUE KEY idx_os_version_vulnerabilities_unq_os_version_team_cve ((IFNULL(CAST(team_id AS SIGNED), -1)), os_version_id, cve),
		    -- for reads
		    KEY idx_os_version_vulnerabilities_os_version_team_cve (team_id, os_version_id, cve),
		    -- for cleanup
			KEY idx_os_version_vulnerabilities_updated_at (updated_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`)
	if err != nil {
		return fmt.Errorf("creating operating_system_version_vulnerabilities table: %w", err)
	}

	// NOTE: The historical backfill of this table (previously performed here via
	// large INSERT ... SELECT ... GROUP BY statements joining kernel_host_counts
	// and software_cve) has been intentionally removed from this schema migration.
	// Running such a backfill synchronously inside the migration transaction can
	// hold locks on kernel_host_counts and software_cve for a long time on large
	// deployments, risking migration timeouts and blocking concurrent writes.
	// The backfill is instead performed by a background job/worker after the
	// schema migration completes.

	return nil
}

func Down_20251028140000(_ *sql.Tx) error {
	return nil
}
