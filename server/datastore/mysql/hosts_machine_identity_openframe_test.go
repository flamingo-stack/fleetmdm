package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/ptr"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// OPENFRAME(enroll-machine-identity): runs only under MYSQL_TEST=1 — openframe/docs/enroll-machine-identity.md
func TestOpenframeMachineIdentity(t *testing.T) {
	ds := CreateMySQLDS(t)
	plain := context.Background()
	ctx := fleet.NewOpenframeModeContext(plain)

	enrolledHost := func(t *testing.T, uuid, identifier, orbitNodeKey string, teamID *uint, seenAt time.Time) *fleet.Host {
		h, err := ds.NewHost(ctx, &fleet.Host{
			DetailUpdatedAt: time.Now(),
			LabelUpdatedAt:  time.Now(),
			PolicyUpdatedAt: time.Now(),
			SeenTime:        time.Now(),
			OsqueryHostID:   ptr.String(identifier),
			NodeKey:         ptr.String("nk-" + identifier),
			UUID:            uuid,
			Hostname:        "host-" + identifier,
			Platform:        "windows",
			TeamID:          teamID,
		})
		require.NoError(t, err)
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			if _, err := q.ExecContext(ctx, `UPDATE hosts SET orbit_node_key = ? WHERE id = ?`, orbitNodeKey, h.ID); err != nil {
				return err
			}
			if _, err := q.ExecContext(ctx, `INSERT INTO host_seen_times (host_id, seen_time) VALUES (?, ?) ON DUPLICATE KEY UPDATE seen_time = VALUES(seen_time)`, h.ID, seenAt); err != nil {
				return err
			}
			_, err := q.ExecContext(ctx, `INSERT INTO openframe_host_enroll_owners (host_id, instance_id) VALUES (?, ?)`, h.ID, "instance-"+orbitNodeKey)
			return err
		})
		return h
	}
	ownerOf := func(t *testing.T, hostID uint) string {
		var instanceID string
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			return sqlx.GetContext(ctx, q, &instanceID, `SELECT instance_id FROM openframe_host_enroll_owners WHERE host_id = ?`, hostID)
		})
		return instanceID
	}
	forgetOwner := func(t *testing.T, hostID uint) {
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			_, err := q.ExecContext(ctx, `DELETE FROM openframe_host_enroll_owners WHERE host_id = ?`, hostID)
			return err
		})
	}
	longAgo := time.Now().Add(-24 * time.Hour)

	type enrollment struct {
		uuid, identifier, nodeKey, previousNodeKey, instance string
		teamID                                               *uint
		window                                               time.Duration
	}
	enrollOrbit := func(ctx context.Context, e enrollment) (*fleet.Host, error) {
		return ds.EnrollOrbit(ctx,
			fleet.WithEnrollOrbitHostInfo(fleet.OrbitHostInfo{
				HardwareUUID:         e.uuid,
				OsqueryIdentifier:    e.identifier,
				PreviousOrbitNodeKey: e.previousNodeKey,
				InstanceID:           e.instance,
				Hostname:             "enrolling-" + e.nodeKey,
				Platform:             "windows",
			}),
			fleet.WithEnrollOrbitNodeKey(e.nodeKey),
			fleet.WithEnrollOrbitTeamID(e.teamID),
			fleet.WithEnrollOrbitLiveOwnerWindow(e.window),
		)
	}
	mustEnroll := func(t *testing.T, ctx context.Context, e enrollment) *fleet.Host {
		h, err := enrollOrbit(ctx, e)
		require.NoError(t, err)
		return h
	}
	hostOf := func(t *testing.T, hostID uint) *fleet.Host {
		h, err := ds.Host(ctx, hostID)
		require.NoError(t, err)
		require.NotNil(t, h.OsqueryHostID)
		return h
	}

	t.Run("the agent holding the host moves it to its new identifier without a refetch", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-move-1", "uuid-move-1", "onk-move-1", nil, longAgo)

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-move-1", identifier: "machine-move-1", nodeKey: "onk-move-1b", previousNodeKey: "onk-move-1"})

		require.Equal(t, existing.ID, enrolled.ID)
		moved := hostOf(t, existing.ID)
		require.Equal(t, "machine-move-1", *moved.OsqueryHostID)
		require.False(t, moved.RefetchRequested)
	})

	t.Run("the other machine of a pair, whose key was taken over, gets its own host", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-pair-1", "uuid-pair-1", "onk-pair-a", nil, longAgo)
		first := mustEnroll(t, ctx, enrollment{uuid: "uuid-pair-1", identifier: "machine-pair-a", nodeKey: "onk-pair-a2", previousNodeKey: "onk-pair-a"})

		second := mustEnroll(t, ctx, enrollment{uuid: "uuid-pair-1", identifier: "machine-pair-b", nodeKey: "onk-pair-b2", previousNodeKey: "onk-pair-b-stale"})

		require.Equal(t, existing.ID, first.ID)
		require.NotEqual(t, first.ID, second.ID)
		require.Equal(t, "machine-pair-a", *hostOf(t, first.ID).OsqueryHostID)
		require.Equal(t, "machine-pair-b", *hostOf(t, second.ID).OsqueryHostID)
	})

	t.Run("a reinstalled agent enrolls with its identifier and keeps the host", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-reinstall-1", "machine-reinstall-1", "onk-reinstall-1", nil, longAgo)

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-reinstall-1", identifier: "machine-reinstall-1", nodeKey: "onk-reinstall-1b"})

		require.Equal(t, existing.ID, enrolled.ID)
	})

	t.Run("moving back to the hardware uuid keeps the host", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-back-1", "machine-back-1", "onk-back-1", nil, longAgo)

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-back-1", nodeKey: "onk-back-1b", previousNodeKey: "onk-back-1"})

		require.Equal(t, existing.ID, enrolled.ID)
		require.Equal(t, "uuid-back-1", *hostOf(t, existing.ID).OsqueryHostID)
	})

	t.Run("a key held by a host with another hardware uuid proves nothing", func(t *testing.T) {
		other := enrolledHost(t, "uuid-foreign-key-other", "uuid-foreign-key-other", "onk-foreign-key", nil, longAgo)

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-foreign-key-1", identifier: "machine-foreign-key-1", nodeKey: "onk-foreign-key-b", previousNodeKey: "onk-foreign-key"})

		require.NotEqual(t, other.ID, enrolled.ID)
		require.Equal(t, "uuid-foreign-key-other", *hostOf(t, other.ID).OsqueryHostID)
	})

	t.Run("an identifier another host already has is not taken from it", func(t *testing.T) {
		held := enrolledHost(t, "uuid-taken-1", "uuid-taken-1", "onk-taken-held", nil, longAgo)
		taker := enrolledHost(t, "uuid-taken-1", "machine-taken-1", "onk-taken-taker", nil, longAgo)

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-taken-1", identifier: "machine-taken-1", nodeKey: "onk-taken-b", previousNodeKey: "onk-taken-held"})

		require.Equal(t, taker.ID, enrolled.ID)
		require.Equal(t, "uuid-taken-1", *hostOf(t, held.ID).OsqueryHostID)
	})

	t.Run("a key held by a host of another team proves nothing", func(t *testing.T) {
		teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "machine-identity-a"})
		require.NoError(t, err)
		teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "machine-identity-b"})
		require.NoError(t, err)
		foreign := enrolledHost(t, "uuid-team-1", "uuid-team-1", "onk-team-a", &teamA.ID, longAgo)
		pinned := fleet.NewOpenframeTeamContext(ctx, teamB.ID)

		enrolled := mustEnroll(t, pinned, enrollment{uuid: "uuid-team-1", identifier: "machine-team-1", nodeKey: "onk-team-b", previousNodeKey: "onk-team-a", teamID: &teamB.ID})

		require.NotEqual(t, foreign.ID, enrolled.ID)
		require.Equal(t, "uuid-team-1", *hostOf(t, foreign.ID).OsqueryHostID)
	})

	t.Run("without OpenFrame mode the previous key is ignored, as upstream", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-plain-1", "uuid-plain-1", "onk-plain-1", nil, longAgo)

		enrolled := mustEnroll(t, plain, enrollment{uuid: "uuid-plain-1", identifier: "machine-plain-1", nodeKey: "onk-plain-1b", previousNodeKey: "onk-plain-1"})

		require.NotEqual(t, existing.ID, enrolled.ID)
		require.Equal(t, "uuid-plain-1", *hostOf(t, existing.ID).OsqueryHostID)
	})

	enrollOsquery := func(ctx context.Context, uuid, identifier, nodeKey, instance string, window time.Duration) (*fleet.Host, error) {
		return ds.EnrollOsquery(ctx,
			fleet.WithEnrollOsqueryHostID(identifier),
			fleet.WithEnrollOsqueryHardwareUUID(uuid),
			fleet.WithEnrollOsqueryNodeKey(nodeKey),
			fleet.WithEnrollOsqueryInstanceID(instance),
			fleet.WithEnrollOsqueryLiveOwnerWindow(window),
		)
	}
	now := time.Now()
	window := 5 * time.Minute

	t.Run("orbit: a second machine is refused while the owner is live", func(t *testing.T) {
		enrolledHost(t, "uuid-live-1", "uuid-live-1", "onk-live-1", nil, now)

		_, err := enrollOrbit(ctx, enrollment{uuid: "uuid-live-1", nodeKey: "onk-live-1b", instance: "instance-other", window: window})

		require.ErrorContains(t, err, "enrolled by another live agent")
	})

	t.Run("orbit: the same machine re-enrolls while its osquery is live", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-live-2", "uuid-live-2", "onk-live-2", nil, now)

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-live-2", nodeKey: "onk-live-2b", instance: "instance-onk-live-2", window: window})

		require.Equal(t, existing.ID, enrolled.ID)
	})

	t.Run("orbit: a reinstall goes through once the owner went quiet, and owns the host", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-live-3", "uuid-live-3", "onk-live-3", nil, now.Add(-time.Hour))

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-live-3", nodeKey: "onk-live-3b", instance: "instance-new", window: window})

		require.Equal(t, existing.ID, enrolled.ID)
		require.Equal(t, "instance-new", ownerOf(t, existing.ID))
	})

	t.Run("orbit: a host whose owner is not known yet is taken over and owned", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-live-4", "uuid-live-4", "onk-live-4", nil, now)
		forgetOwner(t, existing.ID)

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-live-4", nodeKey: "onk-live-4b", instance: "instance-first", window: window})

		require.Equal(t, existing.ID, enrolled.ID)
		require.Equal(t, "instance-first", ownerOf(t, existing.ID))
	})

	t.Run("orbit: an agent that sends no instance id is not refused", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-live-5", "uuid-live-5", "onk-live-5", nil, now)

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-live-5", nodeKey: "onk-live-5b", window: window})

		require.Equal(t, existing.ID, enrolled.ID)
	})

	t.Run("orbit: with no window a second machine takes over, as upstream", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-live-6", "uuid-live-6", "onk-live-6", nil, now)

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-live-6", nodeKey: "onk-live-6b", instance: "instance-other"})

		require.Equal(t, existing.ID, enrolled.ID)
	})

	t.Run("orbit: the agent holding the host is never refused", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-live-7", "uuid-live-7", "onk-live-7", nil, now)

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-live-7", identifier: "machine-live-7", nodeKey: "onk-live-7b", previousNodeKey: "onk-live-7", instance: "instance-other", window: window})

		require.Equal(t, existing.ID, enrolled.ID)
	})

	t.Run("orbit: a host already on the machine id is not refused", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-live-8", "machine-live-8", "onk-live-8", nil, now)

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-live-8", identifier: "machine-live-8", nodeKey: "onk-live-8b", instance: "instance-other", window: window})

		require.Equal(t, existing.ID, enrolled.ID)
	})

	t.Run("osquery: a second machine is refused while the owner is live", func(t *testing.T) {
		enrolledHost(t, "uuid-osq-1", "uuid-osq-1", "onk-osq-1", nil, now)

		_, err := enrollOsquery(ctx, "uuid-osq-1", "uuid-osq-1", "nk-osq-1b", "instance-other", window)

		require.ErrorContains(t, err, "enrolled by another live agent")
	})

	t.Run("osquery: the same machine re-enrolls while live", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-osq-2", "uuid-osq-2", "onk-osq-2", nil, now)

		enrolled, err := enrollOsquery(ctx, "uuid-osq-2", "uuid-osq-2", "nk-osq-2b", "instance-onk-osq-2", window)

		require.NoError(t, err)
		require.Equal(t, existing.ID, enrolled.ID)
	})

	t.Run("osquery: a reinstall goes through once the owner went quiet", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-osq-3", "uuid-osq-3", "onk-osq-3", nil, now.Add(-time.Hour))

		enrolled, err := enrollOsquery(ctx, "uuid-osq-3", "uuid-osq-3", "nk-osq-3b", "instance-new", window)

		require.NoError(t, err)
		require.Equal(t, existing.ID, enrolled.ID)
		require.Equal(t, "instance-new", ownerOf(t, existing.ID))
	})

	t.Run("osquery: the machine-id owner takes its host back from a stale key", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-osq-4", "machine-osq-4", "onk-osq-4", nil, now)

		enrolled, err := enrollOsquery(ctx, "uuid-osq-4", "machine-osq-4", "nk-osq-4b", "instance-other", window)

		require.NoError(t, err)
		require.Equal(t, existing.ID, enrolled.ID)
	})

	t.Run("osquery: without OpenFrame mode nothing is refused, as upstream", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-osq-5", "uuid-osq-5", "onk-osq-5", nil, now)

		enrolled, err := enrollOsquery(plain, "uuid-osq-5", "uuid-osq-5", "nk-osq-5b", "instance-other", window)

		require.NoError(t, err)
		require.Equal(t, existing.ID, enrolled.ID)
	})

	t.Run("Windows MDM: a host whose uuid another host of its team shares is refused", func(t *testing.T) {
		first := enrolledHost(t, "uuid-mdm-shared", "machine-mdm-a", "onk-mdm-a", nil, longAgo)
		second := enrolledHost(t, "uuid-mdm-shared", "machine-mdm-b", "onk-mdm-b", nil, longAgo)

		require.ErrorContains(t, ds.OpenframeRefuseSharedHardwareUUID(ctx, first.ID, "uuid-mdm-shared"), "MDM enrollment refused")
		require.ErrorContains(t, ds.OpenframeRefuseSharedHardwareUUID(ctx, second.ID, "uuid-mdm-shared"), "MDM enrollment refused")
	})

	t.Run("Windows MDM: a tenant with one host is not refused for another tenant's pair", func(t *testing.T) {
		teamX, err := ds.NewTeam(ctx, &fleet.Team{Name: "machine-identity-mdm-x"})
		require.NoError(t, err)
		teamY, err := ds.NewTeam(ctx, &fleet.Team{Name: "machine-identity-mdm-y"})
		require.NoError(t, err)
		xFirst := enrolledHost(t, "uuid-mdm-vendor", "machine-mdm-x1", "onk-mdm-x1", &teamX.ID, longAgo)
		enrolledHost(t, "uuid-mdm-vendor", "machine-mdm-x2", "onk-mdm-x2", &teamX.ID, longAgo)
		y := enrolledHost(t, "uuid-mdm-vendor", "machine-mdm-y", "onk-mdm-y", &teamY.ID, longAgo)

		require.NoError(t, ds.OpenframeRefuseSharedHardwareUUID(ctx, y.ID, "uuid-mdm-vendor"))
		require.ErrorContains(t, ds.OpenframeRefuseSharedHardwareUUID(ctx, xFirst.ID, "uuid-mdm-vendor"), "MDM enrollment refused")
	})

	t.Run("Windows MDM: a host alone with its uuid is not refused", func(t *testing.T) {
		alone := enrolledHost(t, "uuid-mdm-alone", "machine-mdm-alone", "onk-mdm-alone", nil, longAgo)

		require.NoError(t, ds.OpenframeRefuseSharedHardwareUUID(ctx, alone.ID, "uuid-mdm-alone"))
	})

	t.Run("Apple MDM: a uuid every host of which is in one team is refused", func(t *testing.T) {
		enrolledHost(t, "uuid-apple-shared", "machine-apple-a", "onk-apple-a", nil, longAgo)
		enrolledHost(t, "uuid-apple-shared", "machine-apple-b", "onk-apple-b", nil, longAgo)

		require.ErrorContains(t, ds.MDMResetEnrollment(ctx, "uuid-apple-shared", false), "MDM enrollment refused")
	})

	t.Run("Apple MDM: a uuid shared across teams is left to upstream", func(t *testing.T) {
		teamX, err := ds.NewTeam(ctx, &fleet.Team{Name: "machine-identity-apple-x"})
		require.NoError(t, err)
		teamY, err := ds.NewTeam(ctx, &fleet.Team{Name: "machine-identity-apple-y"})
		require.NoError(t, err)
		enrolledHost(t, "uuid-apple-teams", "machine-apple-x1", "onk-apple-x1", &teamX.ID, longAgo)
		enrolledHost(t, "uuid-apple-teams", "machine-apple-x2", "onk-apple-x2", &teamX.ID, longAgo)
		enrolledHost(t, "uuid-apple-teams", "machine-apple-y", "onk-apple-y", &teamY.ID, longAgo)

		err = ds.MDMResetEnrollment(ctx, "uuid-apple-teams", false)

		if err != nil {
			require.NotContains(t, err.Error(), "MDM enrollment refused")
		}
	})

	t.Run("Apple MDM: without OpenFrame mode a shared uuid is left to upstream", func(t *testing.T) {
		enrolledHost(t, "uuid-apple-plain", "machine-apple-plain-a", "onk-apple-plain-a", nil, longAgo)
		enrolledHost(t, "uuid-apple-plain", "machine-apple-plain-b", "onk-apple-plain-b", nil, longAgo)

		err := ds.MDMResetEnrollment(plain, "uuid-apple-plain", false)

		if err != nil {
			require.NotContains(t, err.Error(), "MDM enrollment refused")
		}
	})
}

// OPENFRAME(enroll-machine-identity): openframe/docs/enroll-machine-identity.md
func TestMigrateOpenframeHostEnrollOwnersTableMatchesSchema(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := t.Context()
	db := ds.writer(ctx)

	showCreate := func() string {
		var row struct {
			Table  string `db:"Table"`
			Create string `db:"Create Table"`
		}
		require.NoError(t, sqlx.GetContext(ctx, db, &row, `SHOW CREATE TABLE openframe_host_enroll_owners`))
		return row.Create
	}
	fromSchema := showCreate()

	_, err := db.ExecContext(ctx, `DROP TABLE openframe_host_enroll_owners`)
	require.NoError(t, err)
	require.NoError(t, ds.MigrateOpenframe(ctx))
	require.Equal(t, fromSchema, showCreate())
}
