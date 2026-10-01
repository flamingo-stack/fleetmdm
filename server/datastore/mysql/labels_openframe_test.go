package mysql

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/stretchr/testify/require"
)

// TestOpenframeLabelTeamFence verifies the OPENFRAME(mysql-multitenancy) label scoping: a pinned
// tenant sees built-in (global) labels plus its own custom labels, never another tenant's custom
// labels — across list, by-id, by-name, and delete. New custom labels are re-homed to the pinned
// team. The datastore fences are the boundary; the caller's TeamFilter uses a global-admin user
// (what OpenFrame's api-only token maps to) to prove role is not the boundary. Runs under MYSQL_TEST=1.
func TestOpenframeLabelTeamFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	require.NoError(t, ds.MigrateOpenframe(ctx)) // needs the openframe_team_key generated column

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "lbl-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "lbl-b"})
	require.NoError(t, err)

	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)
	ctxB := fleet.NewOpenframeTeamContext(ctx, teamB.ID)
	admin := fleet.TeamFilter{User: test.UserAdmin, IncludeObserver: true}

	// A built-in (global) label — created unpinned, team_id stays NULL, visible to everyone.
	builtin, err := ds.NewLabel(ctx, &fleet.Label{Name: "All Hosts", Query: "SELECT 1", LabelType: fleet.LabelTypeBuiltIn})
	require.NoError(t, err)
	require.Nil(t, builtin.TeamID, "built-in labels stay global")

	// Each tenant creates a custom label while pinned → re-homed to its own team.
	labelA, err := ds.NewLabel(ctxA, &fleet.Label{Name: "custom-A", Query: "SELECT 1"})
	require.NoError(t, err)
	require.NotNil(t, labelA.TeamID)
	require.Equal(t, teamA.ID, *labelA.TeamID)

	labelB, err := ds.NewLabel(ctxB, &fleet.Label{Name: "custom-B", Query: "SELECT 1"})
	require.NoError(t, err)
	require.Equal(t, teamB.ID, *labelB.TeamID)

	names := func(ls []*fleet.Label) map[string]bool {
		m := map[string]bool{}
		for _, l := range ls {
			m[l.Name] = true
		}
		return m
	}

	t.Run("ListLabels shows built-ins + own custom, not the other tenant's", func(t *testing.T) {
		got, err := ds.ListLabels(ctxA, admin, fleet.ListOptions{}, false)
		require.NoError(t, err)
		n := names(got)
		require.True(t, n["All Hosts"], "built-in label must be visible")
		require.True(t, n["custom-A"], "own custom label must be visible")
		require.False(t, n["custom-B"], "another tenant's custom label must NOT be visible")
	})

	t.Run("by-id: foreign custom label is NotFound, own + built-in resolve", func(t *testing.T) {
		_, _, err := ds.Label(ctxA, labelB.ID, admin)
		require.True(t, fleet.IsNotFound(err), "foreign label by id must be NotFound, got %v", err)

		_, _, err = ds.Label(ctxA, labelA.ID, admin)
		require.NoError(t, err)
		_, _, err = ds.Label(ctxA, builtin.ID, admin)
		require.NoError(t, err, "built-in label must resolve for any tenant")
	})

	t.Run("by-name: same name in another tenant is not visible", func(t *testing.T) {
		// Both tenants define a label with the same name; each sees only its own.
		shA, err := ds.NewLabel(ctxA, &fleet.Label{Name: "shared-name", Query: "SELECT 1"})
		require.NoError(t, err)
		shB, err := ds.NewLabel(ctxB, &fleet.Label{Name: "shared-name", Query: "SELECT 1"})
		require.NoError(t, err)
		require.NotEqual(t, shA.ID, shB.ID)

		gotA, err := ds.LabelByName(ctxA, "shared-name", admin)
		require.NoError(t, err)
		require.Equal(t, shA.ID, gotA.ID, "must resolve to the caller tenant's label")
	})

	t.Run("delete: cannot delete another tenant's label", func(t *testing.T) {
		// Tenant A tries to delete tenant B's label by name → NotFound; B's label survives.
		err := ds.DeleteLabel(ctxA, "custom-B", admin)
		require.True(t, fleet.IsNotFound(err), "deleting a foreign label must be NotFound, got %v", err)
		_, _, err = ds.Label(ctxB, labelB.ID, admin)
		require.NoError(t, err, "tenant B's label must survive tenant A's delete attempt")

		// Tenant B can delete its own.
		require.NoError(t, ds.DeleteLabel(ctxB, "custom-B", admin))
	})
}
