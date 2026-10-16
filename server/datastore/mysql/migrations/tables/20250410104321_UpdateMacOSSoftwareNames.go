package tables

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/jmoiron/sqlx/reflectx"
	"github.com/pkg/errors"
)

func init() {
	MigrationClient.AddMigration(Up_20250410104321, Down_20250410104321)
}

func Up_20250410104321(tx *sql.Tx) error {
	// Idempotent migration.
	titleStmt := `UPDATE software_titles SET name = TRIM( TRAILING '.app' FROM name ) WHERE source = 'apps' AND bundle_identifier IS NOT NULL`
	_, err := tx.Exec(titleStmt)
	if err != nil {
		return fmt.Errorf("updating software_titles.name: %w", err)
	}

	dupeIDsStmt := `SELECT GROUP_CONCAT(id) AS ids, MD5(
			-- simulate new hash (note: does not include name, matching the checksum
			-- recomputation below; verify this matches the live ingestion checksum formula)
			CONCAT_WS(CHAR(0),
				version,
				source,
				bundle_identifier,
				` + "`release`" + `,
				arch,
				vendor,
				browser,
				extension_id
			)
		) AS new_checksum
				FROM software
				WHERE source = 'apps' AND bundle_identifier IS NOT NULL AND bundle_identifier != ''
				GROUP BY
					version, source, bundle_identifier,` + "`release`" + `, arch, vendor, browser, extension_id
				HAVING COUNT(*) > 1`

	txx := sqlx.Tx{Tx: tx, Mapper: reflectx.NewMapperFunc("db", sqlx.NameMapper)}
	selectedIDs := make(map[string]uint64)
	idsToMergeByNewChecksum := make(map[string][]uint64)
	var softwareGroups []struct {
		IDs         string `db:"ids"`
		NewChecksum string `db:"new_checksum"`
	}
	if err := txx.Select(&softwareGroups, dupeIDsStmt); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("selecting duplicate software rows: %w", err)
		}
	}

	for _, s := range softwareGroups {
		for _, idStr := range strings.Split(s.IDs, ",") {
			id, err := strconv.ParseUint(idStr, 10, 64)
			if err != nil {
				return fmt.Errorf("building duplicate IDs list %q: %w", idStr, err)
			}

			if _, ok := selectedIDs[s.NewChecksum]; !ok {
				selectedIDs[s.NewChecksum] = id
				continue
			}

			idsToMergeByNewChecksum[s.NewChecksum] = append(idsToMergeByNewChecksum[s.NewChecksum], id)
		}
	}

	getRecordToUpdateStmt := `
SELECT DISTINCT
	hs1.host_id
FROM
	host_software hs1
WHERE
	hs1.software_id IN (?)
	AND NOT EXISTS (
		SELECT
			*
		FROM
			host_software hs2
		WHERE
			hs2.software_id = ?
			AND hs2.host_id = hs1.host_id)`
	updateHostSoftwareInstalledPathsStmt := `UPDATE host_software_installed_paths SET software_id = ? WHERE software_id IN (?)`

	var allExcludedIDs []uint64
	addedTempIndex := false
	if !indexExistsTx(tx, "host_software_installed_paths", "software_id") {
		if _, err = tx.Exec(`ALTER TABLE host_software_installed_paths ADD INDEX software_id (software_id)`); err != nil {
			return fmt.Errorf("adding temporary index to host_software_installed_paths: %w", err)
		}
		addedTempIndex = true
	}

	// dropTempIndex ensures the temporary index added above (if any) is removed
	// before returning from this function, even on error paths, since DDL
	// statements are not part of the surrounding transaction on MySQL.
	dropTempIndex := func() error {
		if addedTempIndex && indexExistsTx(tx, "host_software_installed_paths", "software_id") {
			if _, err := tx.Exec(`ALTER TABLE host_software_installed_paths DROP INDEX software_id`); err != nil {
				return fmt.Errorf("removing temporary index from host_software_installed_paths: %w", err)
			}
		}
		return nil
	}

	for newChecksum, idsToMerge := range idsToMergeByNewChecksum {
		allExcludedIDs = append(allExcludedIDs, idsToMerge...)
		var hostIDRecordList []struct {
			HostID uint `db:"host_id"`
		}
		selectedID, ok := selectedIDs[newChecksum]
		if !ok {
			if dropErr := dropTempIndex(); dropErr != nil {
				return dropErr
			}
			return fmt.Errorf("%v excluded IDs but no selected ID", idsToMerge)
		}

		stmt, args, err := sqlx.In(getRecordToUpdateStmt, idsToMerge, selectedID)
		if err != nil {
			if dropErr := dropTempIndex(); dropErr != nil {
				return dropErr
			}
			return fmt.Errorf("sqlx.In for getting host software records to update for old software IDs %v: %w", idsToMerge, err)
		}

		if err := txx.Select(&hostIDRecordList, stmt, args...); err != nil {
			// Note: sqlx's Select into a slice does not return sql.ErrNoRows when
			// there are no matching rows (it returns nil with an empty slice), so
			// this branch is not expected to be reached in that case; it is kept
			// only as a defensive no-op for that specific error.
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if dropErr := dropTempIndex(); dropErr != nil {
				return dropErr
			}
			return fmt.Errorf("getting host software record to update for old software IDs %v: %w", idsToMerge, err)
		}

		if len(hostIDRecordList) > 0 { // batch host software inserts for query efficiency
			hostSoftwareInsertQuery := `INSERT IGNORE INTO host_software (host_id, software_id) VALUES `
			var hostSoftwareInsertParams []any

			for _, h := range hostIDRecordList {
				hostSoftwareInsertParams = append(hostSoftwareInsertParams, h.HostID, selectedID)
				hostSoftwareInsertQuery += "(?,?),"

				if len(hostSoftwareInsertParams) >= 20_000 { // update up to 10k hosts at a time
					_, err = tx.Exec(strings.TrimSuffix(hostSoftwareInsertQuery, ","), hostSoftwareInsertParams...)
					if err != nil {
						if dropErr := dropTempIndex(); dropErr != nil {
							return dropErr
						}
						return fmt.Errorf("updating host_software.software_id for old software IDs %v: %w", idsToMerge, err)
					}
					hostSoftwareInsertQuery = `INSERT IGNORE INTO host_software (host_id, software_id) VALUES `
					hostSoftwareInsertParams = []any{}
				}
			}

			if len(hostSoftwareInsertParams) > 0 { // flush last batch
				_, err = tx.Exec(strings.TrimSuffix(hostSoftwareInsertQuery, ","), hostSoftwareInsertParams...)
				if err != nil {
					if dropErr := dropTempIndex(); dropErr != nil {
						return dropErr
					}
					return fmt.Errorf("updating host_software.software_id for old software IDs %v: %w", idsToMerge, err)
				}
			}
		}

		// repoint host software installed paths to the software ID we're keeping
		stmt, args, err = sqlx.In(updateHostSoftwareInstalledPathsStmt, selectedID, idsToMerge)
		if err != nil {
			if dropErr := dropTempIndex(); dropErr != nil {
				return dropErr
			}
			return fmt.Errorf("sqlx.In for updating host software installed paths records for old software IDs %v: %w", idsToMerge, err)
		}

		if _, err := tx.Exec(stmt, args...); err != nil {
			if dropErr := dropTempIndex(); dropErr != nil {
				return dropErr
			}
			return fmt.Errorf("updating host software installed paths records for old software IDs %v: %w", idsToMerge, err)
		}
	}

	if err := dropTempIndex(); err != nil {
		return err
	}

	// at this point, every host that needs one has a pointer to the selected ID, so we can delete
	// all the records with the excluded IDs.
	if len(allExcludedIDs) > 0 {
		// First delete from software_cve
		deleteSoftwareCVEStmt := `DELETE FROM software_cve WHERE software_id IN (?)`
		deleteSoftwareCVEStmt, args, err := sqlx.In(deleteSoftwareCVEStmt, allExcludedIDs)
		if err != nil {
			return fmt.Errorf("sqlx.In for deleting excluded ids from software_cve: %w", err)
		}
		if _, err := tx.Exec(deleteSoftwareCVEStmt, args...); err != nil {
			return fmt.Errorf("deleting excluded ids from software_cve: %w", err)
		}

		// Now delete from host_software
		deleteHostSoftwareStmt := "DELETE FROM host_software WHERE software_id IN (?)"
		deleteHostSoftwareStmt, args, err = sqlx.In(deleteHostSoftwareStmt, allExcludedIDs)
		if err != nil {
			return fmt.Errorf("sqlx.In for deleting excluded ids from host_software: %w", err)
		}

		if _, err := tx.Exec(deleteHostSoftwareStmt, args...); err != nil {
			return fmt.Errorf("deleting excluded ids from host_software: %w", err)
		}

		deleteSoftwareDupesStmt := `DELETE FROM software WHERE id IN (?) AND bundle_identifier IS NOT NULL AND bundle_identifier != ''`
		deleteSoftwareDupesStmt, args, err = sqlx.In(deleteSoftwareDupesStmt, allExcludedIDs)
		if err != nil {
			return fmt.Errorf("sqlx.In for deleting duplicates from software: %w", err)
		}

		if _, err := tx.Exec(deleteSoftwareDupesStmt, args...); err != nil {
			return fmt.Errorf("deleting duplicates from software: %w", err)
		}
	}

	// Now we can update the software entries to use the new name
	softwareStmt := `
	UPDATE software SET 
		name = TRIM( TRAILING '.app' FROM name ),
		checksum = UNHEX(
		MD5(
			-- concatenate with separator \x00
			CONCAT_WS(CHAR(0),
				version,
				source,
				bundle_identifier,
				` + "`release`" + `,
				arch,
				vendor,
				browser,
				extension_id
			)
		)
	)
		WHERE source = 'apps'
		AND bundle_identifier IS NOT NULL
		AND bundle_identifier != ''
	`
	_, err = tx.Exec(softwareStmt)
	if err != nil {
		return fmt.Errorf("updating software name and checksum: %w", err)
	}

	if !columnExists(tx, "software", "name_source") {
		newColStmt := `ALTER TABLE software ADD COLUMN name_source enum('basic', 'bundle_4.67') DEFAULT 'basic' NOT NULL`
		if _, err = tx.Exec(newColStmt); err != nil {
			return fmt.Errorf("adding name_source column to software: %w", err)
		}
	}

	return nil
}

func Down_20250410104321(tx *sql.Tx) error {
	return nil
}
