package tables

import (
	"database/sql"
	"fmt"
)

func init() {
	MigrationClient.AddMigration(Up_20240829170023, Down_20240829170023)
}

func Up_20240829170023(tx *sql.Tx) error {
	// Idempotent migration.
	_, err := tx.Exec(`
CREATE TABLE IF NOT EXISTS vpp_token_teams (
	id int unsigned NOT NULL PRIMARY KEY AUTO_INCREMENT,
    vpp_token_id int unsigned NOT NULL,
	team_id int unsigned,
	null_team_type enum('none','allteams','noteam') COLLATE utf8mb4_unicode_ci DEFAULT 'none',
	UNIQUE KEY idx_vpp_token_teams_team_id (team_id),
	-- Note that this is only a partial constraint. There can be only
	-- one token per team, but the team "No team" and "all teams" have
	-- to be checked manually in go code
	CONSTRAINT fk_vpp_token_teams_team_id FOREIGN KEY (team_id) REFERENCES teams (id) ON DELETE CASCADE,
	CONSTRAINT fk_vpp_token_teams_vpp_token_id FOREIGN KEY (vpp_token_id) REFERENCES vpp_tokens (id) ON DELETE CASCADE
)`)
	if err != nil {
		return fmt.Errorf("creating vpp_token_teams table: %w", err)
	}

	if columnExists(tx, "vpp_tokens", "team_id") {
		_, err = tx.Exec(`
INSERT IGNORE INTO vpp_token_teams (
	vpp_token_id,
	team_id,
	null_team_type
) SELECT
	id,
	team_id,
	null_team_type
FROM vpp_tokens`)
		if err != nil {
			return fmt.Errorf("migrating vpp_tokens associations to join table: %w", err)
		}
	}

	if fkExists(tx, "vpp_tokens", "fk_vpp_tokens_team_id") {
		if _, err := tx.Exec(`ALTER TABLE vpp_tokens DROP FOREIGN KEY fk_vpp_tokens_team_id`); err != nil {
			return fmt.Errorf("dropping fk_vpp_tokens_team_id: %w", err)
		}
	}

	if indexExistsTx(tx, "vpp_tokens", "idx_vpp_tokens_team_id") {
		if _, err := tx.Exec(`ALTER TABLE vpp_tokens DROP INDEX idx_vpp_tokens_team_id`); err != nil {
			return fmt.Errorf("dropping idx_vpp_tokens_team_id: %w", err)
		}
	}

	if columnExists(tx, "vpp_tokens", "team_id") {
		if _, err := tx.Exec(`ALTER TABLE vpp_tokens DROP COLUMN team_id`); err != nil {
			return fmt.Errorf("dropping team_id from vpp_tokens: %w", err)
		}
	}

	if columnExists(tx, "vpp_tokens", "null_team_type") {
		if _, err := tx.Exec(`ALTER TABLE vpp_tokens DROP COLUMN null_team_type`); err != nil {
			return fmt.Errorf("dropping null_team_type from vpp_tokens: %w", err)
		}
	}

	return nil
}

func Down_20240829170023(tx *sql.Tx) error {
	return nil
}
