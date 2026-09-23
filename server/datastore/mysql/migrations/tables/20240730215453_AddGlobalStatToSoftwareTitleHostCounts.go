package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20240730215453, Down_20240730215453)
}

func Up_20240730215453(tx *sql.Tx) error {
	// Idempotent migration.
	// Check if global_stats is already part of the primary key AND the column exists,
	// to avoid trusting a single partial signal for overall migration completion.
	var count int
	pkErr := tx.QueryRow(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'software_titles_host_counts' AND INDEX_NAME = 'PRIMARY' AND COLUMN_NAME = 'global_stats'`).Scan(&count)
	primaryKeyHasGlobalStats := pkErr == nil && count > 0
	columnAlreadyExists := columnExists(tx, "software_titles_host_counts", "global_stats")

	if primaryKeyHasGlobalStats && columnAlreadyExists {
		return nil // already migrated
	}

	if !columnAlreadyExists {
		stmt := `
		ALTER TABLE software_titles_host_counts
		ADD COLUMN global_stats tinyint unsigned NOT NULL DEFAULT '0',
		DROP PRIMARY KEY,
		ADD PRIMARY KEY (software_title_id, team_id, global_stats)
	`

		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("add global_stats column to software_titles_host_counts: %w", err)
		}
	} else if !primaryKeyHasGlobalStats {
		// Column exists from a prior partial run, but the primary key was not updated.
		stmt := `
		ALTER TABLE software_titles_host_counts
		DROP PRIMARY KEY,
		ADD PRIMARY KEY (software_title_id, team_id, global_stats)
	`
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("add global_stats to primary key of software_titles_host_counts: %w", err)
		}
	}

	// update team counts to have global_stats = 0
	stmt := `
		UPDATE software_titles_host_counts
		SET global_stats = 1
		WHERE team_id = 0
	`
	if _, err := tx.Exec(stmt); err != nil {
		return fmt.Errorf("update global_stats for team_id = 0: %w", err)
	}

	// Insert "no team" counts
	stmt = `
		INSERT IGNORE INTO software_titles_host_counts (software_title_id, hosts_count, team_id, global_stats)
		SELECT
			sthc1.software_title_id,
			GREATEST(sthc1.hosts_count - COALESCE(SUM(sthc2.hosts_count), 0),0) AS hosts_count,
			0 AS team_id,
			0 AS global_stats
		FROM
			software_titles_host_counts sthc1
		LEFT JOIN
			software_titles_host_counts sthc2 ON sthc1.software_title_id = sthc2.software_title_id AND sthc2.team_id != 0 AND sthc2.global_stats = 0
		WHERE
			sthc1.team_id = 0 AND sthc1.global_stats = 1
		GROUP BY
			sthc1.software_title_id, sthc1.hosts_count
	`
	if _, err := tx.Exec(stmt); err != nil {
		return fmt.Errorf("insert no team counts: %w", err)
	}

	return nil
}

func Down_20240730215453(tx *sql.Tx) error {
	return nil
}
