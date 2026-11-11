package mysql

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestOpenframeAndroidReenrollAfterHostDeleted(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "tenant-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "tenant-b"})
	require.NoError(t, err)

	const esID = "es_id_openframe_reenroll"
	first := createAndroidHost(esID)
	first.TeamID = &teamA.ID
	enrolledA, err := ds.NewAndroidHost(ctx, first, false)
	require.NoError(t, err)

	require.NoError(t, ds.DeleteHost(ctx, enrolledA.Host.ID))

	second := createAndroidHost(esID)
	second.TeamID = &teamB.ID
	enrolledB, err := ds.NewAndroidHost(ctx, second, false)
	require.NoError(t, err)
	require.NotEqual(t, enrolledA.Host.ID, enrolledB.Host.ID)

	lite, err := ds.AndroidHostLite(ctx, esID)
	require.NoError(t, err)
	require.Equal(t, enrolledB.Host.ID, lite.Host.ID)
	require.Equal(t, &teamB.ID, lite.Host.TeamID)

	var deviceRows int
	require.NoError(t, sqlx.GetContext(ctx, ds.reader(ctx), &deviceRows,
		`SELECT COUNT(*) FROM android_devices WHERE enterprise_specific_id = ?`, esID))
	require.Equal(t, 1, deviceRows)

	lastTeam, found, err := ds.GetAndroidDeviceLastTeamID(ctx, esID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, &teamB.ID, lastTeam)

	_, err = ds.Host(ctx, enrolledA.Host.ID)
	require.True(t, fleet.IsNotFound(err), "the previous tenant's host row is gone")
}
