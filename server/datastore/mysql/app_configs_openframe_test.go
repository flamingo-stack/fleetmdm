package mysql

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/pkg/optjson"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// TestOpenframeAppConfigTeamIsolation verifies the OPENFRAME(mysql-multitenancy) change to the
// app_config read/write: when the request is scoped to a tenant team (via context, or
// FLEET_OPENFRAME_TEAM_ID in production), its config is stored/read under id = team id, so
// different tenants do not share the single app_config row. Runs only under MYSQL_TEST=1.
func TestOpenframeAppConfigTeamIsolation(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	ctx5 := fleet.NewOpenframeTeamContext(ctx, 5)
	ctx6 := fleet.NewOpenframeTeamContext(ctx, 6)

	save := func(c context.Context, orgName string) {
		require.NoError(t, ds.SaveAppConfig(c, &fleet.AppConfig{OrgInfo: fleet.OrgInfo{OrgName: orgName}}))
	}
	readOrg := func(c context.Context) string {
		ac, err := ds.AppConfig(c)
		require.NoError(t, err)
		return ac.OrgInfo.OrgName
	}

	// Each team starts with no config of its own.
	require.Empty(t, readOrg(ctx5))
	require.Empty(t, readOrg(ctx6))

	save(ctx5, "team5-org")
	save(ctx6, "team6-org")

	// Each team reads back its own config, not the other's.
	require.Equal(t, "team5-org", readOrg(ctx5))
	require.Equal(t, "team6-org", readOrg(ctx6))

	// Overwriting team 6 does not affect team 5.
	save(ctx6, "team6-org-v2")
	require.Equal(t, "team5-org", readOrg(ctx5))
	require.Equal(t, "team6-org-v2", readOrg(ctx6))
}

// TestOpenframeAppConfigDefaultsForConfiglessTenant verifies that a pinned tenant with no
// app_config_json row yet (team created, config never saved) reads defaults — not zero-value
// config that would silently disable host users. The team id must be > 1 to avoid the legacy
// singleton row (id = 1).
func TestOpenframeAppConfigDefaultsForConfiglessTenant(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	_, err := ds.NewTeam(ctx, &fleet.Team{Name: "configless-filler"}) // consumes team id 1
	require.NoError(t, err)
	team, err := ds.NewTeam(ctx, &fleet.Team{Name: "configless"})
	require.NoError(t, err)
	require.Greater(t, team.ID, uint(1))
	ctxT := fleet.NewOpenframeTeamContext(ctx, team.ID)

	ac, err := ds.AppConfig(ctxT)
	require.NoError(t, err)
	require.True(t, ac.Features.EnableHostUsers, "defaults must be applied for a config-less tenant")
	require.Equal(t, 24*time.Hour, ac.WebhookSettings.Interval.Duration)
}

// TestOpenframeAppConfigSelectDecision covers the row-selection rule: pinned → own tenant row,
// unpinned multitenant → instance row (id = 1), non-multitenant → upstream statement.
func TestOpenframeAppConfigSelectDecision(t *testing.T) {
	t.Run("pinned tenant reads its own row", func(t *testing.T) {
		stmt, args := openframeAppConfigSelectDecision(32, true, true)
		require.Equal(t, openframeAppConfigSelectByID, stmt)
		require.Equal(t, []any{uint(32)}, args)
	})

	t.Run("unpinned multitenant reads the instance row", func(t *testing.T) {
		stmt, args := openframeAppConfigSelectDecision(0, false, true)
		require.Equal(t, openframeAppConfigSelectByID, stmt)
		require.Equal(t, []any{openframeGlobalAppConfigID}, args,
			"crons must not read whichever row the storage engine happens to return first")
	})

	t.Run("non-multitenant keeps the upstream statement", func(t *testing.T) {
		stmt, args := openframeAppConfigSelectDecision(0, false, false)
		require.Equal(t, openframeAppConfigSelectAny, stmt)
		require.Nil(t, args)
	})

	t.Run("a pin wins over the multitenancy flag being off", func(t *testing.T) {
		stmt, args := openframeAppConfigSelectDecision(7, true, false)
		require.Equal(t, openframeAppConfigSelectByID, stmt)
		require.Equal(t, []any{uint(7)}, args)
	})
}

// TestOpenframeAppConfigUnpinnedReadsInstanceRow verifies against MySQL that the statement the
// unpinned-multitenant decision picks returns the instance row (id = 1), not a tenant's. The
// env-to-statement wiring itself is covered by TestOpenframeAppConfigSelectDecision (the flag
// cannot be set here: CreateMySQLDS forces t.Parallel, which forbids t.Setenv, and mutating
// process env would race parallel tests). Runs only under MYSQL_TEST=1.
func TestOpenframeAppConfigUnpinnedReadsInstanceRow(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	// The instance row (id = 1, via the upstream unpinned save) and a tenant row that must
	// never be served to an unpinned read.
	require.NoError(t, ds.SaveAppConfig(ctx, &fleet.AppConfig{OrgInfo: fleet.OrgInfo{OrgName: "instance-org"}}))
	ctxTenant := fleet.NewOpenframeTeamContext(ctx, 32)
	require.NoError(t, ds.SaveAppConfig(ctxTenant, &fleet.AppConfig{OrgInfo: fleet.OrgInfo{OrgName: "tenant32-org"}}))

	readOrgWith := func(stmt string, args []any) string {
		var raw []byte
		require.NoError(t, sqlx.GetContext(ctx, ds.reader(ctx), &raw, stmt, args...))
		var ac fleet.AppConfig
		require.NoError(t, json.Unmarshal(raw, &ac))
		return ac.OrgInfo.OrgName
	}

	stmt, args := openframeAppConfigSelectDecision(0, false, true)
	require.Equal(t, "instance-org", readOrgWith(stmt, args))

	stmt, args = openframeAppConfigSelectDecision(32, true, true)
	require.Equal(t, "tenant32-org", readOrgWith(stmt, args))
}

// TestOpenframeAppConfigDefaultsOnMissing verifies the missing-row fallback rule: any
// multitenant read falls back to the seeded defaults, never a zero-value config.
func TestOpenframeAppConfigDefaultsOnMissing(t *testing.T) {
	ctx := context.Background()

	require.True(t, openframeAppConfigDefaultsOnMissing(fleet.NewOpenframeTeamContext(ctx, 32)),
		"a pinned tenant with no row yet must get defaults")

	t.Run("unpinned", func(t *testing.T) {
		got := openframeAppConfigDefaultsOnMissing(ctx)
		require.Equal(t, fleet.IsOpenframeMultitenancy(), got,
			"unpinned falls back to defaults exactly when multitenancy is on")
	})
}

func TestOpenframeInstanceMDMFlagsDecision(t *testing.T) {
	cases := []struct {
		name           string
		pinned, shared bool
		want           bool
	}{
		{"pinned request in shared mode uses the instance row", true, true, true},
		{"unpinned reader already reads the instance row", false, true, false},
		{"process-pinned mode keeps the tenant row", true, false, false},
		{"multitenancy off keeps upstream", false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, openframeInstanceMDMFlagsDecision(c.pinned, c.shared))
		})
	}
}

func TestOpenframeInstanceMDMFlagsSharedAcrossTenants(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	instance := &fleet.AppConfig{OrgInfo: fleet.OrgInfo{OrgName: "instance-org"}}
	instance.MDM.EnableDiskEncryption = optjson.SetBool(true)
	require.NoError(t, ds.SaveAppConfig(ctx, instance))

	ctxA := fleet.NewOpenframeTeamContext(ctx, 5)
	ctxB := fleet.NewOpenframeTeamContext(ctx, 6)
	require.NoError(t, ds.SaveAppConfig(ctxA, &fleet.AppConfig{OrgInfo: fleet.OrgInfo{OrgName: "tenant-a"}}))
	require.NoError(t, ds.SaveAppConfig(ctxB, &fleet.AppConfig{OrgInfo: fleet.OrgInfo{OrgName: "tenant-b"}}))

	tenantA, err := ds.AppConfig(ctxA)
	require.NoError(t, err)
	tenantA.MDM.EnabledAndConfigured = true
	tenantA.MDM.AndroidEnabledAndConfigured = true
	require.NoError(t, openframeWriteInstanceMDMFlags(ctx, ds.writer(ctx), tenantA))

	flags, err := openframeReadInstanceMDMFlags(ctx, ds.reader(ctx))
	require.NoError(t, err)
	require.Equal(t, openframeInstanceMDMFlags{EnabledAndConfigured: true, AndroidEnabledAndConfigured: true}, flags)

	unpinned, err := ds.AppConfig(ctx)
	require.NoError(t, err)
	require.True(t, unpinned.MDM.EnabledAndConfigured, "the unpinned enrollment routes read the instance row")
	require.Equal(t, "instance-org", unpinned.OrgInfo.OrgName, "only the MDM flags change on the instance row")
	require.True(t, unpinned.MDM.EnableDiskEncryption.Value, "only the MDM flags change on the instance row")

	tenantB, err := ds.AppConfig(ctxB)
	require.NoError(t, err)
	require.False(t, tenantB.MDM.EnabledAndConfigured, "the stored tenant row is untouched")
	openframeApplyInstanceMDMFlags(tenantB, flags)
	require.True(t, tenantB.MDM.EnabledAndConfigured)
	require.True(t, tenantB.MDM.AndroidEnabledAndConfigured)
	require.False(t, tenantB.MDM.AppleBMEnabledAndConfigured)
	require.Equal(t, "tenant-b", tenantB.OrgInfo.OrgName)

	tenantA.MDM.EnabledAndConfigured = false
	require.NoError(t, openframeWriteInstanceMDMFlags(ctx, ds.writer(ctx), tenantA))
	flags, err = openframeReadInstanceMDMFlags(ctx, ds.reader(ctx))
	require.NoError(t, err)
	require.False(t, flags.EnabledAndConfigured, "removing the APNs cert clears the flag for every tenant")
	require.True(t, flags.AndroidEnabledAndConfigured)
}

func TestOpenframeInstanceMDMFlagsMissingInstanceRow(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	_, err := ds.writer(ctx).ExecContext(ctx, `DELETE FROM app_config_json WHERE id = ?`, openframeGlobalAppConfigID)
	require.NoError(t, err)

	flags, err := openframeReadInstanceMDMFlags(ctx, ds.reader(ctx))
	require.NoError(t, err)
	require.Equal(t, openframeInstanceMDMFlags{}, flags)
}
