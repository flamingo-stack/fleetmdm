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

	enrolledHost := func(t *testing.T, uuid, identifier, orbitNodeKey string, teamID *uint, lastEnrolledAt time.Time) *fleet.Host {
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
			_, err := q.ExecContext(ctx, `UPDATE hosts SET orbit_node_key = ?, last_enrolled_at = ? WHERE id = ?`, orbitNodeKey, lastEnrolledAt, h.ID)
			return err
		})
		return h
	}
	longAgo := time.Now().Add(-24 * time.Hour)

	type enrollment struct {
		uuid, identifier, nodeKey, previousNodeKey string
		teamID                                     *uint
		cooldown                                   time.Duration
	}
	enrollOrbit := func(ctx context.Context, e enrollment) (*fleet.Host, error) {
		return ds.EnrollOrbit(ctx,
			fleet.WithEnrollOrbitHostInfo(fleet.OrbitHostInfo{
				HardwareUUID:         e.uuid,
				OsqueryIdentifier:    e.identifier,
				PreviousOrbitNodeKey: e.previousNodeKey,
				Hostname:             "enrolling-" + e.nodeKey,
				Platform:             "windows",
			}),
			fleet.WithEnrollOrbitNodeKey(e.nodeKey),
			fleet.WithEnrollOrbitTeamID(e.teamID),
			fleet.WithEnrollOrbitCooldown(e.cooldown),
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

	t.Run("a takeover within the cooldown is refused", func(t *testing.T) {
		enrolledHost(t, "uuid-cooldown-1", "uuid-cooldown-1", "onk-cooldown-a", nil, time.Now())

		_, err := enrollOrbit(ctx, enrollment{uuid: "uuid-cooldown-1", nodeKey: "onk-cooldown-b", cooldown: 10 * time.Minute})

		require.ErrorContains(t, err, "enrolling too often")
	})

	t.Run("a takeover after the cooldown, or with no cooldown, goes through", func(t *testing.T) {
		old := enrolledHost(t, "uuid-cooldown-2", "uuid-cooldown-2", "onk-cooldown-2a", nil, longAgo)
		recent := enrolledHost(t, "uuid-cooldown-3", "uuid-cooldown-3", "onk-cooldown-3a", nil, time.Now())

		afterCooldown := mustEnroll(t, ctx, enrollment{uuid: "uuid-cooldown-2", nodeKey: "onk-cooldown-2b", cooldown: 10 * time.Minute})
		noCooldown := mustEnroll(t, ctx, enrollment{uuid: "uuid-cooldown-3", nodeKey: "onk-cooldown-3b"})

		require.Equal(t, old.ID, afterCooldown.ID)
		require.Equal(t, recent.ID, noCooldown.ID)
	})

	t.Run("the agent holding the host is not held to the cooldown", func(t *testing.T) {
		existing := enrolledHost(t, "uuid-cooldown-4", "uuid-cooldown-4", "onk-cooldown-4", nil, time.Now())

		enrolled := mustEnroll(t, ctx, enrollment{uuid: "uuid-cooldown-4", identifier: "machine-cooldown-4", nodeKey: "onk-cooldown-4b", previousNodeKey: "onk-cooldown-4", cooldown: 10 * time.Minute})

		require.Equal(t, existing.ID, enrolled.ID)
	})

	t.Run("an MDM enrollment of a uuid two hosts of one team share is refused", func(t *testing.T) {
		enrolledHost(t, "uuid-mdm-shared", "machine-mdm-a", "onk-mdm-a", nil, longAgo)
		enrolledHost(t, "uuid-mdm-shared", "machine-mdm-b", "onk-mdm-b", nil, longAgo)

		_, windowsErr := ds.UpdateMDMWindowsEnrollmentsHostUUID(ctx, "uuid-mdm-shared", "mdm-device-shared")
		appleErr := ds.MDMResetEnrollment(ctx, "uuid-mdm-shared", false)

		require.ErrorContains(t, windowsErr, "MDM enrollment refused")
		require.ErrorContains(t, appleErr, "MDM enrollment refused")
	})

	t.Run("hosts of different teams sharing a uuid do not block MDM", func(t *testing.T) {
		teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "machine-identity-mdm-a"})
		require.NoError(t, err)
		teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "machine-identity-mdm-b"})
		require.NoError(t, err)
		enrolledHost(t, "uuid-mdm-teams", "machine-mdm-teams-a", "onk-mdm-teams-a", &teamA.ID, longAgo)
		enrolledHost(t, "uuid-mdm-teams", "machine-mdm-teams-b", "onk-mdm-teams-b", &teamB.ID, longAgo)

		_, err = ds.UpdateMDMWindowsEnrollmentsHostUUID(ctx, "uuid-mdm-teams", "mdm-device-teams")

		require.NoError(t, err)
	})

	t.Run("without OpenFrame mode a shared uuid does not block MDM, as upstream", func(t *testing.T) {
		enrolledHost(t, "uuid-mdm-plain", "machine-mdm-plain-a", "onk-mdm-plain-a", nil, longAgo)
		enrolledHost(t, "uuid-mdm-plain", "machine-mdm-plain-b", "onk-mdm-plain-b", nil, longAgo)

		_, err := ds.UpdateMDMWindowsEnrollmentsHostUUID(plain, "uuid-mdm-plain", "mdm-device-plain")

		require.NoError(t, err)
	})
}
