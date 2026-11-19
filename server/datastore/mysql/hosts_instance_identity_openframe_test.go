package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/ptr"
	"github.com/stretchr/testify/require"
)

// TestOpenframeInstanceIdentity covers OPENFRAME(instance-identity): an agent that enrolls with an
// instance identifier takes over the host its device enrolled as by hardware UUID, and a second
// device sharing that UUID gets a host of its own instead of overwriting it.
// Runs only under MYSQL_TEST=1.
func TestOpenframeInstanceIdentity(t *testing.T) {
	ds := CreateMySQLDS(t)
	plain := context.Background()
	ctx := fleet.NewOpenframeModeContext(plain)

	uuidModeHost := func(t *testing.T, uuid string, teamID *uint) *fleet.Host {
		h, err := ds.NewHost(ctx, &fleet.Host{
			DetailUpdatedAt: time.Now(),
			LabelUpdatedAt:  time.Now(),
			PolicyUpdatedAt: time.Now(),
			SeenTime:        time.Now(),
			OsqueryHostID:   ptr.String(uuid),
			NodeKey:         ptr.String("nk-" + uuid),
			UUID:            uuid,
			Hostname:        "host-" + uuid,
			Platform:        "windows",
			TeamID:          teamID,
		})
		require.NoError(t, err)
		return h
	}

	enrollOrbit := func(ctx context.Context, t *testing.T, uuid, instanceID, nodeKey string, teamID *uint) *fleet.Host {
		h, err := ds.EnrollOrbit(ctx,
			fleet.WithEnrollOrbitHostInfo(fleet.OrbitHostInfo{
				HardwareUUID:      uuid,
				OsqueryIdentifier: instanceID,
				Hostname:          "host-" + instanceID,
				Platform:          "windows",
			}),
			fleet.WithEnrollOrbitNodeKey(nodeKey),
			fleet.WithEnrollOrbitTeamID(teamID),
		)
		require.NoError(t, err)
		return h
	}

	osqueryHostIDOf := func(t *testing.T, hostID uint) string {
		h, err := ds.Host(ctx, hostID)
		require.NoError(t, err)
		require.NotNil(t, h.OsqueryHostID)
		return *h.OsqueryHostID
	}

	t.Run("orbit with an instance id takes over the uuid-mode host of its device", func(t *testing.T) {
		existing := uuidModeHost(t, "uuid-adopt-1", nil)

		enrolled := enrollOrbit(ctx, t, "uuid-adopt-1", "instance-adopt-1", "onk-adopt-1", nil)

		require.Equal(t, existing.ID, enrolled.ID)
		require.Equal(t, "instance-adopt-1", osqueryHostIDOf(t, existing.ID))
	})

	t.Run("a second device sharing the uuid gets its own host", func(t *testing.T) {
		existing := uuidModeHost(t, "uuid-shared-1", nil)
		first := enrollOrbit(ctx, t, "uuid-shared-1", "instance-shared-a", "onk-shared-a", nil)

		second := enrollOrbit(ctx, t, "uuid-shared-1", "instance-shared-b", "onk-shared-b", nil)

		require.Equal(t, existing.ID, first.ID)
		require.NotEqual(t, first.ID, second.ID)
		require.Equal(t, "instance-shared-a", osqueryHostIDOf(t, first.ID))
		require.Equal(t, "instance-shared-b", osqueryHostIDOf(t, second.ID))
	})

	t.Run("re-enrolling with the same instance id keeps the adopted host", func(t *testing.T) {
		existing := uuidModeHost(t, "uuid-again-1", nil)
		enrollOrbit(ctx, t, "uuid-again-1", "instance-again-a", "onk-again-a", nil)
		enrollOrbit(ctx, t, "uuid-again-1", "instance-again-b", "onk-again-b", nil)

		again := enrollOrbit(ctx, t, "uuid-again-1", "instance-again-a", "onk-again-a2", nil)

		require.Equal(t, existing.ID, again.ID)
	})

	t.Run("osquery enrolling with an instance id takes over the uuid-mode host", func(t *testing.T) {
		existing := uuidModeHost(t, "uuid-osq-1", nil)

		enrolled, err := ds.EnrollOsquery(ctx,
			fleet.WithEnrollOsqueryHostID("instance-osq-1"),
			fleet.WithEnrollOsqueryHardwareUUID("uuid-osq-1"),
			fleet.WithEnrollOsqueryNodeKey("nk-osq-1"),
		)
		require.NoError(t, err)

		require.Equal(t, existing.ID, enrolled.ID)
		require.Equal(t, "instance-osq-1", osqueryHostIDOf(t, existing.ID))
	})

	t.Run("a uuid-mode host of another tenant is not taken over", func(t *testing.T) {
		teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "instance-identity-a"})
		require.NoError(t, err)
		teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "instance-identity-b"})
		require.NoError(t, err)
		foreign := uuidModeHost(t, "uuid-tenant-1", &teamA.ID)

		enrolled := enrollOrbit(fleet.NewOpenframeTeamContext(ctx, teamB.ID), t, "uuid-tenant-1", "instance-tenant-1", "onk-tenant-1", &teamB.ID)

		require.NotEqual(t, foreign.ID, enrolled.ID)
		require.Equal(t, "uuid-tenant-1", osqueryHostIDOf(t, foreign.ID))
	})

	t.Run("without OpenFrame mode an instance id gets a new host, as upstream", func(t *testing.T) {
		existing := uuidModeHost(t, "uuid-upstream-1", nil)

		enrolled := enrollOrbit(plain, t, "uuid-upstream-1", "instance-upstream-1", "onk-upstream-1", nil)

		require.NotEqual(t, existing.ID, enrolled.ID)
		require.Equal(t, "uuid-upstream-1", osqueryHostIDOf(t, existing.ID))
	})
}
