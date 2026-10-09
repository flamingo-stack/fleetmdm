package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/stretchr/testify/require"
)

// TestOpenframeTeamReadFence verifies the OPENFRAME(mysql-multitenancy) team scoping: a pinned
// tenant can read/list only its own team; a foreign team id is NotFound and never appears in the
// list. This also hardens the ListTeamPolicies path (its TeamLite guard). Runs under MYSQL_TEST=1.
func TestOpenframeTeamReadFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "tr-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "tr-b"})
	require.NoError(t, err)
	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)

	// TeamLite: own resolves, foreign is NotFound.
	got, err := ds.TeamLite(ctxA, teamA.ID)
	require.NoError(t, err)
	require.Equal(t, teamA.ID, got.ID)

	_, err = ds.TeamLite(ctxA, teamB.ID)
	require.True(t, fleet.IsNotFound(err), "foreign team must be NotFound, got %v", err)

	// ListTeams: even a global-admin caller (OpenFrame's token) sees only its pinned team.
	teams, err := ds.ListTeams(ctxA, fleet.TeamFilter{User: test.UserAdmin, IncludeObserver: true}, fleet.ListOptions{})
	require.NoError(t, err)
	require.Len(t, teams, 1)
	require.Equal(t, teamA.ID, teams[0].ID)
}

// TestOpenframeAddHostsToTeamFence verifies that a shared-DB "transfer" (AddHostsToTeam) cannot move
// hosts across tenants: the target team must be the pinned team, and hosts the tenant does not own
// are dropped. Runs under MYSQL_TEST=1.
func TestOpenframeAddHostsToTeamFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "xfer-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "xfer-b"})
	require.NoError(t, err)

	newHost := func(name string, teamID *uint) *fleet.Host {
		h, err := ds.NewHost(ctx, &fleet.Host{
			DetailUpdatedAt: time.Now(),
			LabelUpdatedAt:  time.Now(),
			PolicyUpdatedAt: time.Now(),
			SeenTime:        time.Now(),
			OsqueryHostID:   &name,
			NodeKey:         &name,
			UUID:            "uuid-" + name,
			Hostname:        name,
			TeamID:          teamID,
		})
		require.NoError(t, err)
		return h
	}
	hostA := newHost("xfer-hostA", &teamA.ID)
	hostB := newHost("xfer-hostB", &teamB.ID)

	teamOf := func(id uint) *uint {
		var tid *uint
		require.NoError(t, ds.writer(ctx).GetContext(ctx, &tid, "SELECT team_id FROM hosts WHERE id = ?", id))
		return tid
	}

	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)

	t.Run("cannot move own host into another tenant's team", func(t *testing.T) {
		err := ds.AddHostsToTeam(ctxA, fleet.NewAddHostsToTeamParams(&teamB.ID, []uint{hostA.ID}))
		require.True(t, fleet.IsNotFound(err), "foreign target team must be rejected, got %v", err)
		require.Equal(t, teamA.ID, *teamOf(hostA.ID), "host A must stay in team A")
	})

	t.Run("cannot move another tenant's host (foreign source dropped)", func(t *testing.T) {
		// Target is A's own team (valid), but the host belongs to B → dropped, B's host untouched.
		require.NoError(t, ds.AddHostsToTeam(ctxA, fleet.NewAddHostsToTeamParams(&teamA.ID, []uint{hostB.ID})))
		require.Equal(t, teamB.ID, *teamOf(hostB.ID), "host B must stay in team B")
	})

	t.Run("cannot move to No team (nil target)", func(t *testing.T) {
		err := ds.AddHostsToTeam(ctxA, fleet.NewAddHostsToTeamParams(nil, []uint{hostA.ID}))
		require.True(t, fleet.IsNotFound(err), "nil (No team) target must be rejected, got %v", err)
		require.Equal(t, teamA.ID, *teamOf(hostA.ID))
	})

	t.Run("moving own host to own team is allowed (no-op)", func(t *testing.T) {
		require.NoError(t, ds.AddHostsToTeam(ctxA, fleet.NewAddHostsToTeamParams(&teamA.ID, []uint{hostA.ID})))
		require.Equal(t, teamA.ID, *teamOf(hostA.ID))
	})
}
