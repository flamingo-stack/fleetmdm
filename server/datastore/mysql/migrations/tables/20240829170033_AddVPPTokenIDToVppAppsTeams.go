package tables

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/pkg/errors"
)

func init() {
	MigrationClient.AddMigration(Up_20240829170033, Down_20240829170033)
}

func Up_20240829170033(tx *sql.Tx) error {
	// Idempotent migration.
	if !columnExists(tx, "vpp_apps_teams", "vpp_token_id") {
		stmtAddColumn := `
ALTER TABLE vpp_apps_teams
	ADD COLUMN vpp_token_id int(10) UNSIGNED NOT NULL`

		stmtFindToken := `SELECT id FROM vpp_tokens LIMIT 1` //nolint:gosec

		stmtCountAssociations := `SELECT COUNT(*) FROM vpp_apps_teams`

		stmtCleanAssociations := `DELETE FROM vpp_apps_teams`

		stmtAssociate := `UPDATE vpp_apps_teams SET vpp_token_id = ?`

		if _, err := tx.Exec(stmtAddColumn); err != nil {
			return fmt.Errorf("failed to add vpp_token_id column to table: %w", err)
		}

		// Associate apps with the first token available. If we're
		// migrating from single-token VPP this should be correct.
		var vppTokenID uint
		err := tx.QueryRow(stmtFindToken).Scan(&vppTokenID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("get existing VPP token ID: %w", err)
			}
		}

		if vppTokenID > 0 {
			if _, err := tx.Exec(stmtAssociate, vppTokenID); err != nil {
				return fmt.Errorf("failed to associate vpp apps with first token: %w", err)
			}
		} else {
			var associationCount int
			if err := tx.QueryRow(stmtCountAssociations).Scan(&associationCount); err != nil {
				return fmt.Errorf("failed to count existing VPP team associations: %w", err)
			}
			if associationCount > 0 {
				log.Printf("WARNING: no VPP token found; deleting %d orphaned VPP team association(s) from vpp_apps_teams", associationCount)
			}
			if _, err := tx.Exec(stmtCleanAssociations); err != nil {
				return fmt.Errorf("failed clean orphaned VPP team associations: %w", err)
			}
		}
	}

	if !fkExists(tx, "vpp_apps_teams", "fk_vpp_apps_teams_vpp_token_id") {
		stmtAddConstraint := `
ALTER TABLE vpp_apps_teams
	ADD CONSTRAINT fk_vpp_apps_teams_vpp_token_id
		FOREIGN KEY (vpp_token_id) REFERENCES vpp_tokens(id) ON DELETE CASCADE`

		if _, err := tx.Exec(stmtAddConstraint); err != nil {
			return fmt.Errorf("failed to add vpp token id constraint: %w", err)
		}
	}

	return nil
}

func Down_20240829170033(tx *sql.Tx) error {
	return nil
}
