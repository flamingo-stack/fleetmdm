package openframe

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/fleetdm/fleet/v4/server/fleet"
)

func init() {
	MigrationClient.AddMigration(Up_20260831000001, Down_20260831000001)
}

// Up_20260831000001 makes the instance app config row (app_config_json id = 1) valid under
// shared-DB multitenancy — see openframe/docs/mysql-multitenancy-feature.md. Nothing else
// maintains it there (setup is tenant-pinned), yet every unpinned reader — the vulnerability
// and chart-collection crons included — reads it, so a degenerate row disables those jobs
// instance-wide.
//
// Idempotent steps: reserve team id 1; seed id = 1 with the openframe defaults if absent;
// repair an existing row by force-enabling the gating feature flags (JSON_MERGE_PATCH leaves
// sibling keys untouched — safe even where team id 1 already shares the row).
//
// SEMANTIC-CONFLICT WATCHLIST (openframe/docs/upstream-sync-conflict-resolution.md):
// writes upstream tables (`teams`, `app_config_json`); re-verify after upstream reshapes them.
func Up_20260831000001(tx *sql.Tx) error {
	if _, err := tx.Exec("ALTER TABLE teams AUTO_INCREMENT = 2"); err != nil {
		return fmt.Errorf("reserving team id 1: %w", err)
	}

	configBytes, err := json.Marshal(fleet.OpenframeDefaultAppConfig())
	if err != nil {
		return fmt.Errorf("marshaling default app config: %w", err)
	}
	if _, err := tx.Exec(
		"INSERT INTO app_config_json (id, json_value) VALUES (1, ?) ON DUPLICATE KEY UPDATE id = id",
		configBytes,
	); err != nil {
		return fmt.Errorf("seeding instance app config row: %w", err)
	}

	if _, err := tx.Exec(`
		UPDATE app_config_json SET json_value = JSON_MERGE_PATCH(json_value,
			'{"features":{"enable_software_inventory":true,"enable_host_users":true,"historical_data":{"uptime":true,"vulnerabilities":true}}}')
		WHERE id = 1
	`); err != nil {
		return fmt.Errorf("repairing instance app config row: %w", err)
	}
	return nil
}

func Down_20260831000001(tx *sql.Tx) error {
	return nil
}
