package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/ptr"
	"github.com/stretchr/testify/require"
)

// TestOpenframeHostAssignmentTeamFence verifies the OPENFRAME(mysql-multitenancy) fences on the
// host-assignment CRUD (policy_hosts / query_hosts): a pinned process cannot assign/list against
// another tenant's policy/query (NotFound), and foreign host ids are dropped from assignments to
// its own. Runs only under MYSQL_TEST=1.
func TestOpenframeHostAssignmentTeamFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	// policy_hosts / query_hosts live in the openframe migration pipeline (not schema.sql).
	require.NoError(t, ds.MigrateOpenframe(ctx))

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "ha-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "ha-b"})
	require.NoError(t, err)

	mkHost := func(team *fleet.Team, key string) *fleet.Host {
		h, err := ds.NewHost(ctx, &fleet.Host{
			DetailUpdatedAt: time.Now(), LabelUpdatedAt: time.Now(), PolicyUpdatedAt: time.Now(), SeenTime: time.Now(),
			OsqueryHostID: ptr.String(key), NodeKey: ptr.String("nk-" + key), UUID: key, Hostname: "h-" + key,
			Platform: "darwin", TeamID: &team.ID,
		})
		require.NoError(t, err)
		return h
	}
	hostA := mkHost(teamA, "ha-A")
	hostB := mkHost(teamB, "ha-B")

	polA, err := ds.NewTeamPolicy(ctx, teamA.ID, nil, fleet.PolicyPayload{Name: "haPolA", Query: "SELECT 1"})
	require.NoError(t, err)
	polB, err := ds.NewTeamPolicy(ctx, teamB.ID, nil, fleet.PolicyPayload{Name: "haPolB", Query: "SELECT 1"})
	require.NoError(t, err)

	qA, err := ds.NewQuery(ctx, &fleet.Query{Name: "haQA", Query: "SELECT 1", Saved: true, TeamID: &teamA.ID, Logging: fleet.LoggingSnapshot})
	require.NoError(t, err)
	qB, err := ds.NewQuery(ctx, &fleet.Query{Name: "haQB", Query: "SELECT 1", Saved: true, TeamID: &teamB.ID, Logging: fleet.LoggingSnapshot})
	require.NoError(t, err)

	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)

	t.Run("policy assignment", func(t *testing.T) {
		// Assigning a mix to own policy → only own host added.
		n, err := ds.AddPolicyHosts(ctxA, polA.ID, []uint{hostA.ID, hostB.ID})
		require.NoError(t, err)
		require.Equal(t, uint(1), n)

		hosts, _, err := ds.ListPolicyHosts(ctxA, polA.ID, fleet.ListOptions{})
		require.NoError(t, err)
		require.Len(t, hosts, 1)
		require.Equal(t, hostA.ID, hosts[0].HostID)

		// Operating on another tenant's policy → NotFound.
		_, err = ds.AddPolicyHosts(ctxA, polB.ID, []uint{hostB.ID})
		require.True(t, fleet.IsNotFound(err), "add to foreign policy must be NotFound, got %v", err)
		_, _, err = ds.ListPolicyHosts(ctxA, polB.ID, fleet.ListOptions{})
		require.True(t, fleet.IsNotFound(err), "list foreign policy hosts must be NotFound, got %v", err)
	})

	t.Run("query assignment", func(t *testing.T) {
		n, err := ds.AddQueryHosts(ctxA, qA.ID, []uint{hostA.ID, hostB.ID})
		require.NoError(t, err)
		require.Equal(t, uint(1), n)

		hosts, _, err := ds.ListQueryHosts(ctxA, qA.ID, fleet.ListOptions{})
		require.NoError(t, err)
		require.Len(t, hosts, 1)
		require.Equal(t, hostA.ID, hosts[0].HostID)

		_, err = ds.AddQueryHosts(ctxA, qB.ID, []uint{hostB.ID})
		require.True(t, fleet.IsNotFound(err), "add to foreign query must be NotFound, got %v", err)
		_, _, err = ds.ListQueryHosts(ctxA, qB.ID, fleet.ListOptions{})
		require.True(t, fleet.IsNotFound(err), "list foreign query hosts must be NotFound, got %v", err)
	})
}
