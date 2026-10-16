package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20250219100000, Down_20250219100000)
}

func Up_20250219100000(tx *sql.Tx) error {
	// Idempotent migration.
	if !columnsExists(tx, "vpp_apps_teams", "created_at", "updated_at") {
		_, err := tx.Exec(`
		ALTER TABLE vpp_apps_teams
		ADD COLUMN created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		ADD COLUMN updated_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)`)
		if err != nil {
			return fmt.Errorf("adding timestamps to vpp_apps_teams: %w", err)
		}
	}

	// Guard against data drift: vpp_apps is expected to have at most one row per
	// (platform, adam_id). If that assumption is violated, the backfill UPDATE below
	// could nondeterministically apply timestamps from an arbitrary matching row, so
	// we assert uniqueness before running it.
	var dupCount int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT platform, adam_id
			FROM vpp_apps
			GROUP BY platform, adam_id
			HAVING COUNT(*) > 1
		) dups`).Scan(&dupCount); err != nil {
		return fmt.Errorf("checking vpp_apps for duplicate platform/adam_id rows: %w", err)
	}
	if dupCount > 0 {
		return fmt.Errorf("found %d duplicate (platform, adam_id) combinations in vpp_apps; refusing to backfill vpp_apps_teams timestamps to avoid nondeterministic results", dupCount)
	}

	// make a quick guess at created/updated timestamps; getting more exact timestamps requires looking at the activity
	// feed, which may have been purged, so that query will be available for admins to run manually
	_, err := tx.Exec(`UPDATE vpp_apps_teams vt
    	JOIN vpp_apps v ON v.platform = vt.platform AND v.adam_id = vt.adam_id
    	SET vt.created_at = v.created_at, vt.updated_at = v.updated_at`)
	if err != nil {
		return fmt.Errorf("backfilling timestamps on vpp_apps_teams: %w", err)
	}

	return nil
}

func Down_20250219100000(tx *sql.Tx) error {
	return nil
}
