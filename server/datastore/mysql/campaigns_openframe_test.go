package mysql

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/stretchr/testify/require"
)

// TestOpenframeCampaignTeamFence verifies the campaign read fence on a shared DB: a campaign's
// tenant is its query's team, so a pinned context must not load (and then stream) another
// tenant's campaign by id. Unpinned contexts keep upstream behavior. Runs only under MYSQL_TEST=1.
func TestOpenframeCampaignTeamFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	user := test.NewUser(t, ds, "Fence", "fence@openframe.local", true)
	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-campaign-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-campaign-b"})
	require.NoError(t, err)

	queryA := test.NewQuery(t, ds, &teamA.ID, "campaign-fence-a", "SELECT 1", user.ID, true)
	queryB := test.NewQuery(t, ds, &teamB.ID, "campaign-fence-b", "SELECT 1", user.ID, true)
	campA := test.NewCampaign(t, ds, queryA.ID, fleet.QueryRunning, time.Now())
	campB := test.NewCampaign(t, ds, queryB.ID, fleet.QueryRunning, time.Now())

	_, err = ds.NewDistributedQueryCampaignTarget(ctx, &fleet.DistributedQueryCampaignTarget{
		Type: fleet.TargetTeam, DistributedQueryCampaignID: campA.ID, TargetID: teamA.ID,
	})
	require.NoError(t, err)
	_, err = ds.NewDistributedQueryCampaignTarget(ctx, &fleet.DistributedQueryCampaignTarget{
		Type: fleet.TargetTeam, DistributedQueryCampaignID: campB.ID, TargetID: teamB.ID,
	})
	require.NoError(t, err)

	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)

	// Pinned to A: own campaign loads, B's campaign is invisible.
	got, err := ds.DistributedQueryCampaign(ctxA, campA.ID)
	require.NoError(t, err)
	require.Equal(t, campA.ID, got.ID)

	_, err = ds.DistributedQueryCampaign(ctxA, campB.ID)
	require.Error(t, err, "a foreign campaign must not load under another tenant's pin")
	require.True(t, errors.Is(err, sql.ErrNoRows))

	// Same fence for target ids: own targets load, foreign campaign's targets come back empty.
	targetsA, err := ds.DistributedQueryCampaignTargetIDs(ctxA, campA.ID)
	require.NoError(t, err)
	require.Equal(t, []uint{teamA.ID}, targetsA.TeamIDs)

	targetsB, err := ds.DistributedQueryCampaignTargetIDs(ctxA, campB.ID)
	require.NoError(t, err)
	require.Empty(t, targetsB.TeamIDs)
	require.Empty(t, targetsB.HostIDs)
	require.Empty(t, targetsB.LabelIDs)

	// Unpinned: upstream behavior — both campaigns load.
	for _, id := range []uint{campA.ID, campB.ID} {
		got, err := ds.DistributedQueryCampaign(ctx, id)
		require.NoError(t, err)
		require.Equal(t, id, got.ID)
	}
	targetsB, err = ds.DistributedQueryCampaignTargetIDs(ctx, campB.ID)
	require.NoError(t, err)
	require.Equal(t, []uint{teamB.ID}, targetsB.TeamIDs)
}
