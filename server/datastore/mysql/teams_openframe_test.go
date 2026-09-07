package mysql

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/require"
)

// TestOpenframeEnsureTeamID verifies the UUID→team_id bridge: EnsureOpenframeTeamID creates a team
// for a new tenant UUID and is idempotent (same UUID → same id; different UUID → different id).
// Runs only under MYSQL_TEST=1.
func TestOpenframeEnsureTeamID(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	// The bridge column lives in the openframe migration pipeline (not schema.sql).
	require.NoError(t, ds.MigrateOpenframe(ctx))

	const uuidA = "3f1a9b2c-0000-4d5e-8f00-00000000000a"
	const uuidB = "3f1a9b2c-0000-4d5e-8f00-00000000000b"

	// First call creates the team.
	idA, err := ds.EnsureOpenframeTeamID(ctx, uuidA)
	require.NoError(t, err)
	require.NotZero(t, idA)

	// Idempotent: same UUID resolves to the same id, no duplicate team.
	idA2, err := ds.EnsureOpenframeTeamID(ctx, uuidA)
	require.NoError(t, err)
	require.Equal(t, idA, idA2)

	// A different tenant UUID gets its own team.
	idB, err := ds.EnsureOpenframeTeamID(ctx, uuidB)
	require.NoError(t, err)
	require.NotZero(t, idB)
	require.NotEqual(t, idA, idB)

	// The resolved id is a real team, and the bridge column is populated.
	team, err := ds.TeamLite(ctx, idA)
	require.NoError(t, err)
	require.Equal(t, idA, team.ID)

	var storedUUID string
	require.NoError(t, ds.writer(ctx).GetContext(ctx, &storedUUID,
		"SELECT openframe_tenant_uuid FROM teams WHERE id = ?", idA))
	require.Equal(t, uuidA, storedUUID)

	// The created team must carry a config with the mdm key: GetOrbitConfig reads
	// `config->'$.mdm'` (TeamMDMConfig) and dereferences the result unconditionally, so a
	// config-less team would panic the orbit config endpoint for every host on it.
	mdmConfig, err := ds.TeamMDMConfig(ctx, idA)
	require.NoError(t, err)
	require.NotNil(t, mdmConfig)

	// A newly created team is seeded with exactly one team-scoped enroll secret, so a fresh
	// tenant can enroll agents without any operator step.
	secretsA, err := ds.GetEnrollSecrets(ctx, &idA)
	require.NoError(t, err)
	require.Len(t, secretsA, 1)
	require.NotEmpty(t, secretsA[0].Secret)
	require.NotNil(t, secretsA[0].TeamID)
	require.Equal(t, idA, *secretsA[0].TeamID)

	// Each tenant gets its own distinct secret.
	secretsB, err := ds.GetEnrollSecrets(ctx, &idB)
	require.NoError(t, err)
	require.Len(t, secretsB, 1)
	require.NotEqual(t, secretsA[0].Secret, secretsB[0].Secret)

	// The seeded secret enrolls into the right team: VerifyEnrollSecret accepts it under the
	// owning team's pin and rejects it under another tenant's pin (the enrollment fence).
	ctxA := fleet.NewOpenframeTeamContext(ctx, idA)
	verified, err := ds.VerifyEnrollSecret(ctxA, secretsA[0].Secret)
	require.NoError(t, err)
	require.NotNil(t, verified.TeamID)
	require.Equal(t, idA, *verified.TeamID)

	ctxB := fleet.NewOpenframeTeamContext(ctx, idB)
	_, err = ds.VerifyEnrollSecret(ctxB, secretsA[0].Secret)
	require.Error(t, err, "tenant A's seeded secret must not verify under tenant B's pin")

	// Resolving an existing team again must not add or replace secrets.
	_, err = ds.EnsureOpenframeTeamID(ctx, uuidA)
	require.NoError(t, err)
	secretsAAgain, err := ds.GetEnrollSecrets(ctx, &idA)
	require.NoError(t, err)
	require.Len(t, secretsAAgain, 1)
	require.Equal(t, secretsA[0].Secret, secretsAAgain[0].Secret)

	// A newly created team is seeded with a persisted per-tenant app config row carrying
	// new-install defaults, so the pinned config read serves software inventory enabled instead
	// of falling back to ApplyDefaults (which leaves it — and vulnerabilities — off).
	var configRows int
	require.NoError(t, ds.writer(ctx).GetContext(ctx, &configRows,
		"SELECT COUNT(*) FROM app_config_json WHERE id = ?", idA))
	require.Equal(t, 1, configRows)

	appConfigA, err := ds.AppConfig(ctxA)
	require.NoError(t, err)
	require.True(t, appConfigA.Features.EnableSoftwareInventory)
	require.True(t, appConfigA.Features.EnableHostUsers)

	// The behavioral agent options are seeded (distributed_interval 10 et al.), already
	// trimmed of endpoint/plugin keys at the source - see OpenframeDefaultAppConfig.
	require.NotNil(t, appConfigA.AgentOptions)
	var agentOptionsA map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(*appConfigA.AgentOptions, &agentOptionsA))
	var optionsCfgA map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(agentOptionsA["config"], &optionsCfgA))
	var optsA map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(optionsCfgA["options"], &optsA))
	require.JSONEq(t, `10`, string(optsA["distributed_interval"]))
	require.NotContains(t, optsA, "logger_tls_endpoint")
	require.NotContains(t, optsA, "logger_plugin")
	require.NotContains(t, optsA, "distributed_plugin")

	// Re-resolving must not replace the tenant's config either: a manual change survives.
	appConfigA.Features.EnableSoftwareInventory = false
	require.NoError(t, ds.SaveAppConfig(ctxA, appConfigA))
	_, err = ds.EnsureOpenframeTeamID(ctx, uuidA)
	require.NoError(t, err)
	appConfigAAgain, err := ds.AppConfig(ctxA)
	require.NoError(t, err)
	require.False(t, appConfigAAgain.Features.EnableSoftwareInventory)

	// A pinned tenant whose config row is missing (pre-seeding team, manual deletion) still gets
	// new-install defaults from the read fallback — never the upgrade-semantics defaults that
	// would leave software inventory off.
	_, err = ds.writer(ctx).ExecContext(ctx, "DELETE FROM app_config_json WHERE id = ?", idB)
	require.NoError(t, err)
	appConfigB, err := ds.AppConfig(ctxB)
	require.NoError(t, err)
	require.True(t, appConfigB.Features.EnableSoftwareInventory)
	require.NotNil(t, appConfigB.AgentOptions)

	// A team with operator-applied (or backfilled) secrets keeps them: replace A's secret set,
	// re-resolve, and confirm the applied set is untouched.
	applied := []*fleet.EnrollSecret{{Secret: "openframe-test-applied-secret-A", TeamID: &idA}}
	require.NoError(t, ds.ApplyEnrollSecrets(ctx, &idA, applied))
	_, err = ds.EnsureOpenframeTeamID(ctx, uuidA)
	require.NoError(t, err)
	secretsAApplied, err := ds.GetEnrollSecrets(ctx, &idA)
	require.NoError(t, err)
	require.Len(t, secretsAApplied, 1)
	require.Equal(t, "openframe-test-applied-secret-A", secretsAApplied[0].Secret)
}
