package mysql

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/stretchr/testify/require"
)

// TestOpenframePolicyQueryByIDTeamFence verifies the OPENFRAME(mysql-multitenancy) by-id read fences
// on policies (Policy / PolicyLite) and queries (Query): a team-scoped process can read its own
// policy/query by id but gets NotFound for another tenant's, even though those endpoints have no
// team param. No-op when unpinned. Runs only under MYSQL_TEST=1.
func TestOpenframePolicyQueryByIDTeamFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "pq-tenant-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "pq-tenant-b"})
	require.NoError(t, err)

	polA, err := ds.NewTeamPolicy(ctx, teamA.ID, nil, fleet.PolicyPayload{Name: "polA", Query: "SELECT 1"})
	require.NoError(t, err)
	polB, err := ds.NewTeamPolicy(ctx, teamB.ID, nil, fleet.PolicyPayload{Name: "polB", Query: "SELECT 1"})
	require.NoError(t, err)

	mkQuery := func(team *fleet.Team, name string) *fleet.Query {
		q, err := ds.NewQuery(ctx, &fleet.Query{
			Name:    name,
			Query:   "SELECT 1",
			Saved:   true,
			TeamID:  &team.ID,
			Logging: fleet.LoggingSnapshot,
		})
		require.NoError(t, err)
		return q
	}
	qA := mkQuery(teamA, "qA")
	qB := mkQuery(teamB, "qB")

	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)

	t.Run("Policy: own ok, foreign NotFound", func(t *testing.T) {
		got, err := ds.Policy(ctxA, polA.ID)
		require.NoError(t, err)
		require.Equal(t, polA.ID, got.ID)

		_, err = ds.Policy(ctxA, polB.ID)
		require.True(t, fleet.IsNotFound(err), "foreign policy by id must be NotFound, got %v", err)
	})

	t.Run("PolicyLite: own ok, foreign NotFound", func(t *testing.T) {
		got, err := ds.PolicyLite(ctxA, polA.ID)
		require.NoError(t, err)
		require.Equal(t, polA.ID, got.ID)

		_, err = ds.PolicyLite(ctxA, polB.ID)
		require.True(t, fleet.IsNotFound(err), "foreign policy-lite by id must be NotFound, got %v", err)
	})

	t.Run("Query: own ok, foreign NotFound", func(t *testing.T) {
		got, err := ds.Query(ctxA, qA.ID)
		require.NoError(t, err)
		require.Equal(t, qA.ID, got.ID)

		_, err = ds.Query(ctxA, qB.ID)
		require.True(t, fleet.IsNotFound(err), "foreign query by id must be NotFound, got %v", err)
	})

	t.Run("PoliciesByID: own ok, foreign NotFound", func(t *testing.T) {
		got, err := ds.PoliciesByID(ctxA, []uint{polA.ID})
		require.NoError(t, err)
		require.Equal(t, polA.ID, got[polA.ID].ID)

		_, err = ds.PoliciesByID(ctxA, []uint{polB.ID})
		require.True(t, fleet.IsNotFound(err), "foreign policy in batch by-id must be NotFound, got %v", err)

		// A mixed batch fails too — the foreign id is indistinguishable from a nonexistent one.
		_, err = ds.PoliciesByID(ctxA, []uint{polA.ID, polB.ID})
		require.True(t, fleet.IsNotFound(err), "mixed batch with foreign id must be NotFound, got %v", err)
	})

	t.Run("unpinned baseline: foreign reads still succeed", func(t *testing.T) {
		_, err := ds.Policy(ctx, polB.ID)
		require.NoError(t, err)
		_, err = ds.Query(ctx, qB.ID)
		require.NoError(t, err)
		_, err = ds.PoliciesByID(ctx, []uint{polA.ID, polB.ID})
		require.NoError(t, err)
	})
}

// TestOpenframePolicyQueryCRUDTeamFence verifies the OPENFRAME(mysql-multitenancy) global→pinned
// redirect on the policy/query list, create, update, and delete paths: a pinned process creates
// objects in its own team, lists only its own, and cannot update/delete another tenant's.
// Runs only under MYSQL_TEST=1.
func TestOpenframePolicyQueryCRUDTeamFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "crud-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "crud-b"})
	require.NoError(t, err)
	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)

	t.Run("policies create/list/update/delete", func(t *testing.T) {
		// Create via the "global" entry point while pinned → lands in team A (redirect).
		polA, err := ds.NewGlobalPolicy(ctxA, nil, fleet.PolicyPayload{Name: "cp-A", Query: "SELECT 1"})
		require.NoError(t, err)
		require.NotNil(t, polA.TeamID)
		require.Equal(t, teamA.ID, *polA.TeamID, "global create must redirect to the pinned team")

		// Another tenant's policy.
		polB, err := ds.NewTeamPolicy(ctx, teamB.ID, nil, fleet.PolicyPayload{Name: "cp-B", Query: "SELECT 1"})
		require.NoError(t, err)

		// List via "global" while pinned → only team A's.
		list, err := ds.ListGlobalPolicies(ctxA, fleet.ListOptions{})
		require.NoError(t, err)
		ids := map[uint]bool{}
		for _, p := range list {
			ids[p.ID] = true
		}
		require.True(t, ids[polA.ID], "own policy must be listed")
		require.False(t, ids[polB.ID], "another tenant's policy must not be listed")

		// Update another tenant's policy while pinned → blocked.
		polB.Description = "hijacked"
		err = ds.SavePolicy(ctxA, polB, false, false)
		require.True(t, fleet.IsNotFound(err), "saving a foreign policy must be blocked, got %v", err)

		// Delete a mix → only own removed.
		deleted, err := ds.DeleteGlobalPolicies(ctxA, []uint{polA.ID, polB.ID})
		require.NoError(t, err)
		require.Equal(t, []uint{polA.ID}, deleted)
		_, err = ds.Policy(ctx, polB.ID) // unpinned read: still exists
		require.NoError(t, err)
	})

	t.Run("queries create/list/update/delete", func(t *testing.T) {
		// Create with no team while pinned → lands in team A.
		qA, err := ds.NewQuery(ctxA, &fleet.Query{Name: "cq-A", Query: "SELECT 1", Saved: true, Logging: fleet.LoggingSnapshot})
		require.NoError(t, err)
		require.NotNil(t, qA.TeamID)
		require.Equal(t, teamA.ID, *qA.TeamID, "create must pin to the process team")

		qB, err := ds.NewQuery(ctx, &fleet.Query{Name: "cq-B", Query: "SELECT 1", Saved: true, TeamID: &teamB.ID, Logging: fleet.LoggingSnapshot})
		require.NoError(t, err)

		// List while pinned → only team A's.
		list, _, _, _, err := ds.ListQueries(ctxA, fleet.ListQueryOptions{})
		require.NoError(t, err)
		ids := map[uint]bool{}
		for _, q := range list {
			ids[q.ID] = true
		}
		require.True(t, ids[qA.ID], "own query must be listed")
		require.False(t, ids[qB.ID], "another tenant's query must not be listed")

		// Update another tenant's query while pinned → blocked.
		qB.Description = "hijacked"
		err = ds.SaveQuery(ctxA, qB, false, false)
		require.True(t, fleet.IsNotFound(err), "saving a foreign query must be blocked, got %v", err)

		// Delete a mix → only own removed.
		deleted, err := ds.DeleteQueries(ctxA, []uint{qA.ID, qB.ID})
		require.NoError(t, err)
		require.Equal(t, uint(1), deleted)
		_, err = ds.Query(ctx, qB.ID) // unpinned read: still exists
		require.NoError(t, err)
	})
}

// TestOpenframeExplicitTeamAndGitOpsFence verifies the explicit-team (URL fleet_id) rejections and
// the GitOps batch redirect: a pinned process cannot read/delete another tenant's team objects, and
// ApplyPolicySpecs/ApplyQueries re-home all specs/queries to the pinned team. Runs only under
// MYSQL_TEST=1.
func TestOpenframeExplicitTeamAndGitOpsFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()
	user := test.NewUser(t, ds, "Author", "author@example.com", true)

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "et-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "et-b"})
	require.NoError(t, err)
	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)

	polB, err := ds.NewTeamPolicy(ctx, teamB.ID, &user.ID, fleet.PolicyPayload{Name: "etPolB", Query: "SELECT 1"})
	require.NoError(t, err)
	qB, err := ds.NewQuery(ctx, &fleet.Query{Name: "etQB", Query: "SELECT 1", Saved: true, TeamID: &teamB.ID, Logging: fleet.LoggingSnapshot})
	require.NoError(t, err)

	t.Run("explicit-team paths reject another tenant", func(t *testing.T) {
		// Read a foreign team's policy via the team route → NotFound.
		_, err := ds.TeamPolicy(ctxA, teamB.ID, polB.ID)
		require.True(t, fleet.IsNotFound(err), "TeamPolicy foreign must be NotFound, got %v", err)

		// List a foreign team's policies → empty (no leak) on both the plain and merge_inherited
		// paths (they serve the same endpoint via the merge_inherited query param).
		tp, _, err := ds.ListTeamPolicies(ctxA, teamB.ID, fleet.ListOptions{}, fleet.ListOptions{}, "")
		require.NoError(t, err)
		require.Empty(t, tp)

		merged, err := ds.ListMergedTeamPolicies(ctxA, teamB.ID, fleet.ListOptions{}, "")
		require.NoError(t, err)
		require.Empty(t, merged, "merge_inherited must not leak a foreign tenant's policies")

		// Counts of a foreign team → 0 on both count paths.
		cnt, err := ds.CountMergedTeamPolicies(ctxA, teamB.ID, "", "")
		require.NoError(t, err)
		require.Zero(t, cnt)
		cnt, err = ds.CountPolicies(ctxA, &teamB.ID, "", "")
		require.NoError(t, err)
		require.Zero(t, cnt, "explicit foreign-team count must be 0")

		// Delete against a foreign team → nothing deleted; polB survives.
		deleted, err := ds.DeleteTeamPolicies(ctxA, teamB.ID, []uint{polB.ID})
		require.NoError(t, err)
		require.Empty(t, deleted)
		_, err = ds.Policy(ctx, polB.ID)
		require.NoError(t, err)

		// QueryByName against a foreign team → NotFound.
		_, err = ds.QueryByName(ctxA, &teamB.ID, "etQB")
		require.True(t, fleet.IsNotFound(err), "QueryByName foreign must be NotFound, got %v", err)
		_ = qB
	})

	t.Run("GitOps ApplyPolicySpecs re-homes to pinned team", func(t *testing.T) {
		// Multiple "No team" (global) specs applied while pinned → all land in team A.
		require.NoError(t, ds.ApplyPolicySpecs(ctxA, user.ID, []*fleet.PolicySpec{
			{Name: "gitopsPol", Query: "SELECT 1", Team: "No team"},
			{Name: "gitopsPol2", Query: "SELECT 2", Team: "No team"},
		}))
		var teamIDs []*uint
		require.NoError(t, ds.writer(ctx).SelectContext(ctx, &teamIDs,
			"SELECT team_id FROM policies WHERE name IN ('gitopsPol', 'gitopsPol2')"))
		require.Len(t, teamIDs, 2)
		for _, tid := range teamIDs {
			require.NotNil(t, tid)
			require.Equal(t, teamA.ID, *tid)
		}
	})

	t.Run("GitOps ApplyQueries pins to pinned team", func(t *testing.T) {
		require.NoError(t, ds.ApplyQueries(ctxA, user.ID, []*fleet.Query{
			{Name: "gitopsQ", Query: "SELECT 1", Saved: true, Logging: fleet.LoggingSnapshot},
		}, nil))
		var teamIDs []*uint
		require.NoError(t, ds.writer(ctx).SelectContext(ctx, &teamIDs,
			"SELECT team_id FROM queries WHERE name = 'gitopsQ'"))
		require.Len(t, teamIDs, 1)
		require.NotNil(t, teamIDs[0])
		require.Equal(t, teamA.ID, *teamIDs[0])
	})
}
