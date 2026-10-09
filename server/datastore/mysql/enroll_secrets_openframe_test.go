package mysql

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/require"
)

// TestOpenframeEnrollSecretTeamFence verifies the OPENFRAME(mysql-multitenancy) enroll-secret
// fences: an agent (VerifyEnrollSecret) may only enroll with this process's tenant secret, and the
// read/write paths (GetEnrollSecrets/ApplyEnrollSecrets) are scoped to the pinned team. No-op when
// unpinned. Runs only under MYSQL_TEST=1.
func TestOpenframeEnrollSecretTeamFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "es-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "es-b"})
	require.NoError(t, err)

	// Seed each team's secret while unpinned (explicit team).
	require.NoError(t, ds.ApplyEnrollSecrets(ctx, &teamA.ID, []*fleet.EnrollSecret{{Secret: "secret-A"}}))
	require.NoError(t, ds.ApplyEnrollSecrets(ctx, &teamB.ID, []*fleet.EnrollSecret{{Secret: "secret-B"}}))

	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)

	t.Run("VerifyEnrollSecret: own ok, foreign rejected", func(t *testing.T) {
		s, err := ds.VerifyEnrollSecret(ctxA, "secret-A")
		require.NoError(t, err)
		require.NotNil(t, s.TeamID)
		require.Equal(t, teamA.ID, *s.TeamID)

		_, err = ds.VerifyEnrollSecret(ctxA, "secret-B")
		require.True(t, fleet.IsNotFound(err), "a foreign tenant's secret must be rejected, got %v", err)
	})

	t.Run("GetEnrollSecrets: pinned returns only this team's", func(t *testing.T) {
		secs, err := ds.GetEnrollSecrets(ctxA, nil) // ask "global" → scoped to pinned
		require.NoError(t, err)
		got := map[string]bool{}
		for _, s := range secs {
			got[s.Secret] = true
		}
		require.True(t, got["secret-A"])
		require.False(t, got["secret-B"], "another tenant's secret must not be returned")
	})

	t.Run("ApplyEnrollSecrets: pinned writes to this team only", func(t *testing.T) {
		// Apply with nil team while pinned → forced to team A.
		require.NoError(t, ds.ApplyEnrollSecrets(ctxA, nil, []*fleet.EnrollSecret{{Secret: "secret-A"}, {Secret: "secret-A2"}}))

		secs, err := ds.GetEnrollSecrets(ctxA, nil)
		require.NoError(t, err)
		require.Len(t, secs, 2)

		// Team B is untouched.
		s, err := ds.VerifyEnrollSecret(ctx, "secret-B") // unpinned
		require.NoError(t, err)
		require.Equal(t, teamB.ID, *s.TeamID)
	})
}
