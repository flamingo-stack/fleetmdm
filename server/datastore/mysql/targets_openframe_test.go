package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/ptr"
	"github.com/stretchr/testify/require"
)

// TestOpenframeLiveQueryTargetTeamFence verifies the OPENFRAME(mysql-multitenancy) fence in
// HostIDsInTargets / CountHostsInTargets: a team-scoped process resolving live-query targets that
// include another tenant's host ids only sees/counts its own team's hosts — even though the caller
// is a global-admin (whose TeamFilter matches all teams) and /queries/run has no team_id param.
// Runs only under MYSQL_TEST=1.
func TestOpenframeLiveQueryTargetTeamFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	// Global-admin filter: matches all teams, so only the OpenFrame fence restricts the result.
	filter := fleet.TeamFilter{User: &fleet.User{GlobalRole: ptr.String(fleet.RoleAdmin)}}

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "lq-tenant-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "lq-tenant-b"})
	require.NoError(t, err)

	mk := func(team *fleet.Team, key string) *fleet.Host {
		h, err := ds.NewHost(ctx, &fleet.Host{
			DetailUpdatedAt: time.Now(),
			LabelUpdatedAt:  time.Now(),
			PolicyUpdatedAt: time.Now(),
			SeenTime:        time.Now(),
			OsqueryHostID:   ptr.String(key),
			NodeKey:         ptr.String("nk-" + key),
			UUID:            key,
			Hostname:        "host-" + key,
			Platform:        "darwin",
			TeamID:          &team.ID,
		})
		require.NoError(t, err)
		return h
	}

	hostA := mk(teamA, "lq-A")
	hostB := mk(teamB, "lq-B")

	targets := fleet.HostTargets{HostIDs: []uint{hostA.ID, hostB.ID}}

	// Baseline (no team scope): both hosts resolve — the cross-tenant exposure the fence closes.
	ids, err := ds.HostIDsInTargets(ctx, filter, targets)
	require.NoError(t, err)
	require.ElementsMatch(t, []uint{hostA.ID, hostB.ID}, ids)

	// Team-A-scoped: only team A's host resolves; team B's is fenced out.
	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)
	ids, err = ds.HostIDsInTargets(ctxA, filter, targets)
	require.NoError(t, err)
	require.Equal(t, []uint{hostA.ID}, ids)

	// And the count reflects only team A's host.
	metrics, err := ds.CountHostsInTargets(ctxA, filter, targets, time.Now())
	require.NoError(t, err)
	require.Equal(t, uint(1), metrics.TotalHosts)
}
