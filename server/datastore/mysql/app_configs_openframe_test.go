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

func openframeSharedTestContext(ctx context.Context, teamID uint) context.Context {
	return context.WithValue(fleet.NewOpenframeTeamContext(ctx, teamID), openframeSharedModeCtxKey{}, true)
}

func TestOpenframeInstanceMDMFlagsSharedAcrossTenants(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	instance := &fleet.AppConfig{OrgInfo: fleet.OrgInfo{OrgName: "instance-org"}}
	instance.MDM.EnableDiskEncryption = optjson.SetBool(true)
	require.NoError(t, ds.SaveAppConfig(ctx, instance))

	ctxA := openframeSharedTestContext(ctx, 5)
	ctxB := openframeSharedTestContext(ctx, 6)
	require.NoError(t, ds.SaveAppConfig(ctxA, &fleet.AppConfig{OrgInfo: fleet.OrgInfo{OrgName: "tenant-a"}}))
	require.NoError(t, ds.SaveAppConfig(ctxB, &fleet.AppConfig{OrgInfo: fleet.OrgInfo{OrgName: "tenant-b"}}))

	read := func(c context.Context) *fleet.AppConfig {
		ac, err := ds.AppConfig(c)
		require.NoError(t, err)
		return ac
	}

	staleB := read(ctxB)
	require.False(t, staleB.MDM.EnabledAndConfigured)

	uploadA := read(ctxA)
	uploadA.MDM.EnabledAndConfigured = true
	require.NoError(t, ds.SaveAppConfig(fleet.NewOpenframeInstanceMDMFlagContext(ctxA, fleet.OpenframeInstanceMDMFlagApple), uploadA))

	tenantB := read(ctxB)
	require.True(t, tenantB.MDM.EnabledAndConfigured, "an APNs upload in one tenant enables Apple MDM for every tenant")
	require.Equal(t, "tenant-b", tenantB.OrgInfo.OrgName)

	unpinned := read(ctx)
	require.True(t, unpinned.MDM.EnabledAndConfigured, "the unpinned enrollment routes read the instance row")
	require.Equal(t, "instance-org", unpinned.OrgInfo.OrgName, "only the MDM flag changes on the instance row")
	require.True(t, unpinned.MDM.EnableDiskEncryption.Value, "only the MDM flag changes on the instance row")

	staleB.OrgInfo.OrgName = "tenant-b-renamed"
	require.NoError(t, ds.SaveAppConfig(ctxB, staleB))
	require.Equal(t, "tenant-b-renamed", read(ctxB).OrgInfo.OrgName)
	require.True(t, read(ctxA).MDM.EnabledAndConfigured, "a generic save with a stale copy does not write the instance flags")
	require.True(t, read(ctx).MDM.EnabledAndConfigured, "a generic save with a stale copy does not write the instance flags")

	require.NoError(t, ds.SetAndroidEnabledAndConfigured(ctxB, true))
	require.True(t, read(ctxA).MDM.AndroidEnabledAndConfigured, "the Android signup enables Android MDM for every tenant")
	require.True(t, read(ctx).MDM.AndroidEnabledAndConfigured)
	require.True(t, read(ctxA).MDM.EnabledAndConfigured, "the Android write leaves the Apple flag alone")

	staleB.MDM.EnabledAndConfigured = false
	require.NoError(t, ds.SaveAppConfig(fleet.NewOpenframeInstanceMDMFlagContext(ctxB, fleet.OpenframeInstanceMDMFlagApple), staleB))
	require.False(t, read(ctxA).MDM.EnabledAndConfigured, "removing the APNs cert clears the flag for every tenant")
	require.True(t, read(ctxA).MDM.AndroidEnabledAndConfigured, "the Apple write leaves the Android flag alone")

	ctxProcessPinned := fleet.NewOpenframeTeamContext(ctx, 7)
	pinnedOnly := read(ctxProcessPinned)
	pinnedOnly.MDM.EnabledAndConfigured = true
	require.NoError(t, ds.SaveAppConfig(fleet.NewOpenframeInstanceMDMFlagContext(ctxProcessPinned, fleet.OpenframeInstanceMDMFlagApple), pinnedOnly))
	require.True(t, read(ctxProcessPinned).MDM.EnabledAndConfigured, "outside shared mode the tenant row keeps its own flag")
	require.False(t, read(ctx).MDM.EnabledAndConfigured, "outside shared mode the instance row is untouched")
}

func TestOpenframeInstanceMDMFlagsAppleBMStaysOnTenantRow(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	ctxA := openframeSharedTestContext(ctx, 5)
	ctxB := openframeSharedTestContext(ctx, 6)
	tenantA := &fleet.AppConfig{}
	tenantA.MDM.AppleBMEnabledAndConfigured = true
	require.NoError(t, ds.SaveAppConfig(ctxA, tenantA))
	require.NoError(t, ds.SaveAppConfig(ctxB, &fleet.AppConfig{}))

	a, err := ds.AppConfig(ctxA)
	require.NoError(t, err)
	require.True(t, a.MDM.AppleBMEnabledAndConfigured)
	b, err := ds.AppConfig(ctxB)
	require.NoError(t, err)
	require.False(t, b.MDM.AppleBMEnabledAndConfigured)
}

func TestOpenframeInstanceMDMFlagUnknown(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := openframeSharedTestContext(context.Background(), 5)

	err := ds.SaveAppConfig(fleet.NewOpenframeInstanceMDMFlagContext(ctx, "windows_enabled_and_configured"), &fleet.AppConfig{})
	require.ErrorContains(t, err, "unknown instance mdm flag")
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
