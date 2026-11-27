// OPENFRAME(mysql-multitenancy)
package mysql

import (
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestOpenframeCleanupWindowsProfilesScopedToPinnedTeam(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := t.Context()

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-win-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-win-b"})
	require.NoError(t, err)

	newWindowsHost := func(name string, teamID uint) *fleet.Host {
		h := test.NewHost(t, ds, name, "", name+"-key", name+"-uuid", time.Now())
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			_, err := q.ExecContext(ctx, `UPDATE hosts SET team_id = ?, platform = 'windows' WHERE id = ?`, teamID, h.ID)
			return err
		})
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			_, err := q.ExecContext(ctx, `
				INSERT INTO host_mdm_windows_profiles (host_uuid, profile_uuid, profile_name, command_uuid, status, operation_type)
				VALUES (?, 'w-profile-1', 'Profile 1', ?, 'verified', 'install')`, h.UUID, name+"-cmd")
			return err
		})
		return h
	}
	hostA := newWindowsHost("openframe-win-host-a", teamA.ID)
	hostB := newWindowsHost("openframe-win-host-b", teamB.ID)

	countRows := func(hostUUID string) int {
		var n int
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			return sqlx.GetContext(ctx, q, &n, `SELECT COUNT(*) FROM host_mdm_windows_profiles WHERE host_uuid = ?`, hostUUID)
		})
		return n
	}

	require.NoError(t, ds.CleanupAllHostMDMProfilesForPlatform(fleet.NewOpenframeTeamContext(ctx, teamA.ID), "windows"))
	require.Zero(t, countRows(hostA.UUID), "the pinned tenant's rows are cleared")
	require.Equal(t, 1, countRows(hostB.UUID), "another tenant's rows are untouched")

	require.NoError(t, ds.CleanupAllHostMDMProfilesForPlatform(ctx, "windows"))
	require.Zero(t, countRows(hostB.UUID), "unpinned keeps the upstream instance-wide cleanup")
}
