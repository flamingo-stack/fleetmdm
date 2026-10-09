// OPENFRAME(host-assignments): verifies the separate OpenFrame migration pipeline —
// openframe/docs/migrations.md
//
// IMPORTANT: the datastore test harness loads server/datastore/mysql/schema.sql,
// which is dumped from the UPSTREAM tables/ migrations only — it does NOT contain
// policy_hosts / query_hosts / migration_status_openframe. So any test that needs
// the OpenFrame tables must call ds.MigrateOpenframe(ctx) first (as this test does).
// This also guards the sync hazard documented in
// openframe/docs/upstream-sync-conflict-resolution.md: `make dump-test-schema`
// regenerates schema.sql without the OpenFrame tables.
package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/stretchr/testify/require"
)

func TestMigrateOpenframeIdempotent(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	// schema.sql does not include the OpenFrame tables; the first call creates
	// policy_hosts / query_hosts via the separate goose client.
	require.NoError(t, ds.MigrateOpenframe(ctx))

	// Running again must be a no-op: the openframe migrations are idempotent
	// (CREATE TABLE IF NOT EXISTS) and tracked in migration_status_openframe.
	require.NoError(t, ds.MigrateOpenframe(ctx))

	// The join tables must now exist.
	for _, table := range []string{"policy_hosts", "query_hosts"} {
		var n int
		require.NoError(t,
			ds.writer(ctx).GetContext(ctx, &n,
				"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?",
				table),
			"checking table %s exists", table)
		require.Equal(t, 1, n, "expected table %s to exist after MigrateOpenframe", table)
	}
}

// TestMigrateOpenframeLabelsUniqueByTeam verifies 20260620000001: labels uniqueness moves
// from global UNIQUE(name) to a per-team unique over the generated column
// openframe_team_key = IFNULL(team_id, 0), so tenants can reuse label names while
// NULL-team rows (single-tenant / flag-off mode) keep the original UNIQUE(name) guarantee.
func TestMigrateOpenframeLabelsUniqueByTeam(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	indexCount := func(index string) int {
		var n int
		require.NoError(t, ds.writer(ctx).GetContext(ctx, &n,
			"SELECT COUNT(*) FROM information_schema.STATISTICS WHERE table_schema = DATABASE() AND table_name = 'labels' AND index_name = ?",
			index))
		return n
	}
	colCount := func() int {
		var n int
		require.NoError(t, ds.writer(ctx).GetContext(ctx, &n,
			"SELECT COUNT(*) FROM information_schema.COLUMNS WHERE table_schema = DATABASE() AND table_name = 'labels' AND column_name = 'openframe_team_key'"))
		return n
	}

	// Baseline from schema.sql: global UNIQUE(name) present, team-scoped one absent.
	require.Positive(t, indexCount("idx_label_unique_name"))
	require.Equal(t, 0, indexCount("idx_label_team_name"))
	require.Equal(t, 0, colCount())

	require.NoError(t, ds.MigrateOpenframe(ctx))

	// After: generated column + team-scoped unique present, the global one dropped.
	require.Equal(t, 0, indexCount("idx_label_unique_name"))
	require.Positive(t, indexCount("idx_label_team_name"))
	require.Equal(t, 1, colCount())

	// Idempotent re-run.
	require.NoError(t, ds.MigrateOpenframe(ctx))
	require.Equal(t, 0, indexCount("idx_label_unique_name"))
	require.Positive(t, indexCount("idx_label_team_name"))
	require.Equal(t, 1, colCount())

	// Behavior — NULL-team rows keep the original global uniqueness (IFNULL collapses
	// them onto sentinel 0): a duplicate name with no team must still be rejected.
	_, err := ds.NewLabel(ctx, &fleet.Label{Name: "openframe-null-dup", Query: "SELECT 1"})
	require.NoError(t, err)
	_, err = ds.NewLabel(ctx, &fleet.Label{Name: "openframe-null-dup", Query: "SELECT 1"})
	require.Error(t, err, "duplicate NULL-team label name must be rejected")

	// Behavior — different teams may reuse the same label name (the multitenancy point),
	// and a team-scoped name does not collide with the NULL-team copy.
	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-labels-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-labels-b"})
	require.NoError(t, err)
	_, err = ds.NewLabel(ctx, &fleet.Label{Name: "openframe-per-team", Query: "SELECT 1"})
	require.NoError(t, err)
	_, err = ds.NewLabel(ctx, &fleet.Label{Name: "openframe-per-team", Query: "SELECT 1", TeamID: &teamA.ID})
	require.NoError(t, err)
	_, err = ds.NewLabel(ctx, &fleet.Label{Name: "openframe-per-team", Query: "SELECT 1", TeamID: &teamB.ID})
	require.NoError(t, err)
	_, err = ds.NewLabel(ctx, &fleet.Label{Name: "openframe-per-team", Query: "SELECT 1", TeamID: &teamA.ID})
	require.Error(t, err, "duplicate name within the same team must be rejected")
}

// TestMigrateOpenframeApplyLabelSpecsUpsert verifies that ApplyLabelSpecs'
// INSERT ... ON DUPLICATE KEY UPDATE still upserts (not duplicates) NULL-team labels after
// 20260620000001 — the regression the generated-column unique exists to prevent: a plain
// (team_id, name) composite stops matching NULL-team rows and every spec apply would insert
// a new duplicate row instead of updating.
func TestMigrateOpenframeApplyLabelSpecsUpsert(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	require.NoError(t, ds.MigrateOpenframe(ctx))

	spec := &fleet.LabelSpec{Name: "openframe-upsert", Query: "SELECT 1", Description: "v1"}
	require.NoError(t, ds.ApplyLabelSpecs(ctx, []*fleet.LabelSpec{spec}))
	spec.Description = "v2"
	require.NoError(t, ds.ApplyLabelSpecs(ctx, []*fleet.LabelSpec{spec}))

	var rows []struct {
		Description string `db:"description"`
	}
	require.NoError(t, ds.writer(ctx).SelectContext(ctx, &rows,
		"SELECT description FROM labels WHERE name = 'openframe-upsert'"))
	require.Len(t, rows, 1, "re-applying the spec must update the existing row, not insert a duplicate")
	require.Equal(t, "v2", rows[0].Description)
}

// TestMigrateOpenframeHostIdentityUniqueByTeam verifies 20260626000001: host osquery-identity
// uniqueness moves from a global UNIQUE(osquery_host_id) to a per-team unique over the generated
// column openframe_team_key = IFNULL(team_id, 0), so the same device can enroll into more than
// one tenant team while NULL-team rows (single-tenant / flag-off mode) keep the original global
// uniqueness. node_key / orbit_node_key stay global-unique (auth secrets) and must be untouched.
func TestMigrateOpenframeHostIdentityUniqueByTeam(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	indexCount := func(index string) int {
		var n int
		require.NoError(t, ds.writer(ctx).GetContext(ctx, &n,
			"SELECT COUNT(*) FROM information_schema.STATISTICS WHERE table_schema = DATABASE() AND table_name = 'hosts' AND index_name = ?",
			index))
		return n
	}
	colCount := func() int {
		var n int
		require.NoError(t, ds.writer(ctx).GetContext(ctx, &n,
			"SELECT COUNT(*) FROM information_schema.COLUMNS WHERE table_schema = DATABASE() AND table_name = 'hosts' AND column_name = 'openframe_team_key'"))
		return n
	}

	// Baseline from schema.sql: global UNIQUE(osquery_host_id) present, team-scoped one absent.
	require.Positive(t, indexCount("idx_osquery_host_id"))
	require.Equal(t, 0, indexCount("idx_hosts_team_osquery_host_id"))
	require.Equal(t, 0, colCount())

	require.NoError(t, ds.MigrateOpenframe(ctx))

	// After: generated column + team-scoped unique present, the global one dropped.
	require.Equal(t, 0, indexCount("idx_osquery_host_id"))
	require.Positive(t, indexCount("idx_hosts_team_osquery_host_id"))
	require.Equal(t, 1, colCount())

	// The auth-secret uniques are left global-unique (not scoped to team).
	require.Positive(t, indexCount("idx_host_unique_nodekey"))
	require.Positive(t, indexCount("idx_host_unique_orbitnodekey"))

	// Idempotent re-run.
	require.NoError(t, ds.MigrateOpenframe(ctx))
	require.Equal(t, 0, indexCount("idx_osquery_host_id"))
	require.Positive(t, indexCount("idx_hosts_team_osquery_host_id"))

	// Behavior — NULL-team rows keep the original global uniqueness (IFNULL collapses them
	// onto sentinel 0), and different teams may hold the same osquery identity.
	newHost := func(osqueryID, nodeKey string, teamID *uint) error {
		_, err := ds.NewHost(ctx, &fleet.Host{
			DetailUpdatedAt: time.Now(),
			LabelUpdatedAt:  time.Now(),
			PolicyUpdatedAt: time.Now(),
			SeenTime:        time.Now(),
			OsqueryHostID:   &osqueryID,
			NodeKey:         &nodeKey,
			UUID:            "uuid-" + nodeKey,
			Hostname:        "host-" + nodeKey,
			TeamID:          teamID,
		})
		return err
	}
	require.NoError(t, newHost("of-mig-dup", "nk-mig-1", nil))
	require.Error(t, newHost("of-mig-dup", "nk-mig-2", nil),
		"duplicate NULL-team osquery_host_id must be rejected")

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-hosts-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-hosts-b"})
	require.NoError(t, err)
	require.NoError(t, newHost("of-mig-shared", "nk-mig-3", &teamA.ID))
	require.NoError(t, newHost("of-mig-shared", "nk-mig-4", &teamB.ID),
		"the same device identity must be allowed in a different team")
	require.Error(t, newHost("of-mig-shared", "nk-mig-5", &teamA.ID),
		"duplicate osquery_host_id within the same team must be rejected")
}

// TestMigrateOpenframeTeamsTenantUUID verifies 20260629000001: teams gains the
// openframe_tenant_uuid bridge column + unique index, idempotently.
func TestMigrateOpenframeTeamsTenantUUID(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	colCount := func() int {
		var n int
		require.NoError(t, ds.writer(ctx).GetContext(ctx, &n,
			"SELECT COUNT(*) FROM information_schema.COLUMNS WHERE table_schema = DATABASE() AND table_name = 'teams' AND column_name = 'openframe_tenant_uuid'"))
		return n
	}
	idxCount := func() int {
		var n int
		require.NoError(t, ds.writer(ctx).GetContext(ctx, &n,
			"SELECT COUNT(*) FROM information_schema.STATISTICS WHERE table_schema = DATABASE() AND table_name = 'teams' AND index_name = 'idx_teams_openframe_tenant_uuid'"))
		return n
	}

	// schema.sql is upstream-only: the column/index are absent before the openframe migrations.
	require.Equal(t, 0, colCount())
	require.Equal(t, 0, idxCount())

	require.NoError(t, ds.MigrateOpenframe(ctx))
	require.Equal(t, 1, colCount())
	require.Positive(t, idxCount())

	// Idempotent re-run.
	require.NoError(t, ds.MigrateOpenframe(ctx))
	require.Equal(t, 1, colCount())
	require.Positive(t, idxCount())
}

// TestOpenframeMigrationLock verifies AcquireOpenframeMigrationLock: a second contender times
// out while the lock is held (GET_LOCK is exclusive across sessions) and succeeds after release.
func TestOpenframeMigrationLock(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	release, err := ds.AcquireOpenframeMigrationLock(ctx, time.Second)
	require.NoError(t, err)

	// A concurrent run must NOT get the lock while it is held.
	_, err = ds.AcquireOpenframeMigrationLock(ctx, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "openframe_fleet_migrations")

	release()

	// After release the lock is free again.
	release2, err := ds.AcquireOpenframeMigrationLock(ctx, time.Second)
	require.NoError(t, err)
	release2()
}

// TestMigrateOpenframeCdcTeamIdStamping verifies 20260722000001 (team_id on the Debezium
// CDC-captured tables) plus the write-path stamping:
//   - unpinned context (flag off / fork-main behavior) leaves team_id NULL;
//   - a team-pinned context stamps query_results rows;
//   - the async policy-membership collector (no pinned context) stamps each row from its
//     host's own team via the flag-gated scalar subselect.
func TestMigrateOpenframeCdcTeamIdStamping(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	require.NoError(t, ds.MigrateOpenframe(ctx))
	for _, table := range []string{"activity_past", "activity_host_past", "query_results", "policy_membership"} {
		var n int
		require.NoError(t, ds.writer(ctx).GetContext(ctx, &n,
			"SELECT COUNT(*) FROM information_schema.COLUMNS WHERE table_schema = DATABASE() AND table_name = ? AND column_name = 'team_id'",
			table))
		require.Equal(t, 1, n, "expected %s.team_id after MigrateOpenframe", table)
	}

	user := test.NewUser(t, ds, "CDC User", "cdc@example.com", true)
	query := test.NewQuery(t, ds, nil, "CDC Query", "SELECT 1", user.ID, true)
	host := test.NewHost(t, ds, "cdc-host", "192.168.1.10", "cdc-key", "cdc-uuid", time.Now())

	rows := []*fleet.ScheduledQueryResultRow{
		{QueryID: query.ID, HostID: host.ID, LastFetched: time.Now().UTC().Truncate(time.Second)},
	}

	// Unpinned: team_id stays NULL — byte-identical to fork-main.
	_, err := ds.OverwriteQueryResultRows(ctx, rows, fleet.DefaultMaxQueryReportRows)
	require.NoError(t, err)
	var teamIDs []sql.NullInt64
	require.NoError(t, ds.writer(ctx).SelectContext(ctx, &teamIDs,
		"SELECT team_id FROM query_results WHERE host_id = ?", host.ID))
	require.Len(t, teamIDs, 1)
	require.False(t, teamIDs[0].Valid)

	// Pinned: stamped with the pin (overwrite deletes + reinserts the host's rows).
	pinned := fleet.NewOpenframeTeamContext(ctx, 42)
	_, err = ds.OverwriteQueryResultRows(pinned, rows, fleet.DefaultMaxQueryReportRows)
	require.NoError(t, err)
	require.NoError(t, ds.writer(ctx).SelectContext(ctx, &teamIDs,
		"SELECT team_id FROM query_results WHERE host_id = ?", host.ID))
	require.Len(t, teamIDs, 1)
	require.True(t, teamIDs[0].Valid)
	require.EqualValues(t, 42, teamIDs[0].Int64)

	// Async policy membership: stamping comes from the host row via the flag-gated subselect.
	// The gate is (multitenancy env || ctx pin); the pinned context triggers it here without
	// t.Setenv (forbidden — CreateMySQLDS forces t.Parallel), and the SQL ignores the pin value
	// and reads hosts.team_id, exactly as the unpinned production cron path does.
	team, err := ds.NewTeam(ctx, &fleet.Team{Name: "cdc-team"})
	require.NoError(t, err)
	require.NoError(t, ds.AddHostsToTeam(ctx, fleet.NewAddHostsToTeamParams(&team.ID, []uint{host.ID})))
	pol, err := ds.NewGlobalPolicy(ctx, &user.ID, fleet.PolicyPayload{Name: "cdc-policy", Query: "SELECT 1"})
	require.NoError(t, err)

	passes := true
	require.NoError(t, ds.AsyncBatchInsertPolicyMembership(pinned, []fleet.PolicyMembershipResult{
		{PolicyID: pol.ID, HostID: host.ID, Passes: &passes},
	}))
	var memberTeam sql.NullInt64
	require.NoError(t, ds.writer(ctx).GetContext(ctx, &memberTeam,
		"SELECT team_id FROM policy_membership WHERE policy_id = ? AND host_id = ?", pol.ID, host.ID))
	require.True(t, memberTeam.Valid)
	require.EqualValues(t, team.ID, memberTeam.Int64)
}

// TestMigrateOpenframeSeedGlobalAppConfig verifies 20260831000001: instance row (id = 1)
// seeded when absent, repaired when degenerate (schema.sql ships exactly that fixture),
// tenant rows and team id 1 protected.
func TestMigrateOpenframeSeedGlobalAppConfig(t *testing.T) {
	ctx := context.Background()

	readFlags := func(t *testing.T, ds *Datastore, id uint) (swInv, hostUsers, histVulns bool, orgName string) {
		var raw []byte
		require.NoError(t, ds.writer(ctx).GetContext(ctx, &raw,
			`SELECT json_value FROM app_config_json WHERE id = ?`, id))
		var ac fleet.AppConfig
		require.NoError(t, json.Unmarshal(raw, &ac))
		return ac.Features.EnableSoftwareInventory, ac.Features.EnableHostUsers,
			ac.Features.HistoricalData.Vulnerabilities, ac.OrgInfo.OrgName
	}

	t.Run("repairs the degenerate row without clobbering siblings", func(t *testing.T) {
		ds := CreateMySQLDS(t)
		// Marker outside features proves JSON_MERGE_PATCH leaves sibling keys alone.
		_, err := ds.writer(ctx).ExecContext(ctx,
			`UPDATE app_config_json SET json_value = JSON_MERGE_PATCH(json_value, '{"org_info":{"org_name":"keep-me"}}') WHERE id = 1`)
		require.NoError(t, err)

		require.NoError(t, ds.MigrateOpenframe(ctx))

		swInv, hostUsers, histVulns, orgName := readFlags(t, ds, 1)
		require.True(t, swInv)
		require.True(t, hostUsers)
		require.True(t, histVulns)
		require.Equal(t, "keep-me", orgName)
	})

	t.Run("seeds the row when absent", func(t *testing.T) {
		ds := CreateMySQLDS(t)
		_, err := ds.writer(ctx).ExecContext(ctx, `DELETE FROM app_config_json`)
		require.NoError(t, err)

		require.NoError(t, ds.MigrateOpenframe(ctx))

		swInv, hostUsers, histVulns, _ := readFlags(t, ds, 1)
		require.True(t, swInv)
		require.True(t, hostUsers)
		require.True(t, histVulns)
	})

	t.Run("tenant rows untouched, team id 1 reserved", func(t *testing.T) {
		ds := CreateMySQLDS(t)
		_, err := ds.writer(ctx).ExecContext(ctx,
			`INSERT INTO app_config_json (id, json_value) VALUES (5, '{"features":{"enable_software_inventory":false}}')`)
		require.NoError(t, err)

		require.NoError(t, ds.MigrateOpenframe(ctx))
		// Idempotent: a second run must not change anything.
		require.NoError(t, ds.MigrateOpenframe(ctx))

		swInv, _, _, _ := readFlags(t, ds, 5)
		require.False(t, swInv, "tenant rows (id > 1) must not be patched")

		team, err := ds.NewTeam(ctx, &fleet.Team{Name: "first-after-reserve"})
		require.NoError(t, err)
		require.Greater(t, team.ID, uint(1), "team id 1 is reserved for the instance config row")
	})
}
