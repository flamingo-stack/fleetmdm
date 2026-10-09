package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/ptr"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/stretchr/testify/require"
)

// TestOpenframeEnrollmentTeamIsolation verifies the OPENFRAME(mysql-multitenancy) change to
// matchHostDuringEnrollment: when the request is scoped to a tenant team (via context, or
// FLEET_OPENFRAME_TEAM_ID in production), an agent enrolling under that team must NOT be matched
// to (and hijack) a host that belongs to a different team but shares a hardware serial. The
// serial-match path is the exploitable cross-tenant vector (hardware_serial is not unique).
// Runs only under MYSQL_TEST=1.
func TestOpenframeEnrollmentTeamIsolation(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "tenant-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "tenant-b"})
	require.NoError(t, err)

	const sharedSerial = "SHARED-SERIAL-123"

	// DEP-style pre-created Apple host in a team, with the shared serial.
	makeAppleHost := func(team *fleet.Team, uuid, osqueryID string) *fleet.Host {
		h, err := ds.NewHost(ctx, &fleet.Host{
			DetailUpdatedAt: time.Now(),
			LabelUpdatedAt:  time.Now(),
			PolicyUpdatedAt: time.Now(),
			SeenTime:        time.Now(),
			OsqueryHostID:   ptr.String(osqueryID),
			NodeKey:         ptr.String("nk-" + osqueryID),
			UUID:            uuid,
			Hostname:        "host-" + osqueryID,
			Platform:        "darwin",
			HardwareSerial:  sharedSerial,
			TeamID:          &team.ID,
		})
		require.NoError(t, err)
		return h
	}

	t.Run("baseline (no team scope): cross-team serial match occurs", func(t *testing.T) {
		hostA := makeAppleHost(teamA, "uuid-base-A", "osq-base-A")

		enrolled, err := ds.EnrollOsquery(ctx,
			fleet.WithEnrollOsqueryHostID("osq-base-B"),
			fleet.WithEnrollOsqueryHardwareSerial(sharedSerial),
			fleet.WithEnrollOsqueryNodeKey("nk-base-B"),
			fleet.WithEnrollOsqueryMDMEnabled(true),
			fleet.WithEnrollOsqueryTeamID(&teamB.ID),
		)
		require.NoError(t, err)
		// Legacy behavior: team B's enroll matched team A's host (the hijack this fix prevents).
		require.Equal(t, hostA.ID, enrolled.ID)
	})

	t.Run("team-scoped: enrollment cannot match another tenant's host", func(t *testing.T) {
		hostA := makeAppleHost(teamA, "uuid-of-A", "osq-of-A")

		// Scope the enroll to team B via context (mirrors a team-B-pinned process).
		enrolled, err := ds.EnrollOsquery(fleet.NewOpenframeTeamContext(ctx, teamB.ID),
			fleet.WithEnrollOsqueryHostID("osq-of-B"),
			fleet.WithEnrollOsqueryHardwareSerial(sharedSerial),
			fleet.WithEnrollOsqueryNodeKey("nk-of-B"),
			fleet.WithEnrollOsqueryMDMEnabled(true),
			fleet.WithEnrollOsqueryTeamID(&teamB.ID),
		)
		require.NoError(t, err)

		// Must NOT hijack team A's host; a distinct host is created for team B.
		require.NotEqual(t, hostA.ID, enrolled.ID)

		// Team A's host is untouched (its osquery identifier was not overwritten).
		reloadedA, err := ds.Host(ctx, hostA.ID)
		require.NoError(t, err)
		require.NotNil(t, reloadedA.OsqueryHostID)
		require.Equal(t, "osq-of-A", *reloadedA.OsqueryHostID)
		require.NotNil(t, reloadedA.TeamID)
		require.Equal(t, teamA.ID, *reloadedA.TeamID)
	})
}

// TestOpenframeSameDeviceEnrollsIntoTwoTeams verifies migration 20260626000001: once
// osquery-identity uniqueness is scoped to (team_id, osquery_host_id), the SAME device (same
// osquery_host_id / uuid) can enroll into two different tenant teams — each gets its own host
// row. Under the upstream global UNIQUE(osquery_host_id) the second enroll's INSERT would fail
// with a duplicate-key error. Runs only under MYSQL_TEST=1.
func TestOpenframeSameDeviceEnrollsIntoTwoTeams(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	// Apply the OpenFrame migrations so osquery_host_id is unique per (team_id, osquery_host_id).
	require.NoError(t, ds.MigrateOpenframe(ctx))

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "dup-tenant-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "dup-tenant-b"})
	require.NoError(t, err)

	const sharedID = "DEVICE-OSQUERY-ID-XYZ"

	// Same device identifier enrolls under team A (process pinned to A via context).
	enrolledA, err := ds.EnrollOsquery(fleet.NewOpenframeTeamContext(ctx, teamA.ID),
		fleet.WithEnrollOsqueryHostID(sharedID),
		fleet.WithEnrollOsqueryHardwareSerial("SERIAL-A"),
		fleet.WithEnrollOsqueryNodeKey("nk-dup-A"),
		fleet.WithEnrollOsqueryTeamID(&teamA.ID),
	)
	require.NoError(t, err)

	// The same device identifier enrolls under team B. With the team-scoped unique this must
	// succeed and create a distinct host (rather than fail on a global duplicate-key).
	enrolledB, err := ds.EnrollOsquery(fleet.NewOpenframeTeamContext(ctx, teamB.ID),
		fleet.WithEnrollOsqueryHostID(sharedID),
		fleet.WithEnrollOsqueryHardwareSerial("SERIAL-B"),
		fleet.WithEnrollOsqueryNodeKey("nk-dup-B"),
		fleet.WithEnrollOsqueryTeamID(&teamB.ID),
	)
	require.NoError(t, err)

	require.NotEqual(t, enrolledA.ID, enrolledB.ID, "same device must get a distinct host per team")

	hostA, err := ds.Host(ctx, enrolledA.ID)
	require.NoError(t, err)
	require.NotNil(t, hostA.TeamID)
	require.Equal(t, teamA.ID, *hostA.TeamID)

	hostB, err := ds.Host(ctx, enrolledB.ID)
	require.NoError(t, err)
	require.NotNil(t, hostB.TeamID)
	require.Equal(t, teamB.ID, *hostB.TeamID)
}

// TestOpenframeHostByIDTeamFence verifies the OPENFRAME(mysql-multitenancy) fence on the by-id host
// getters (Host / HostLite / HostByIdentifier): a team-scoped process can read its own host but gets
// NotFound (→ 404) for another tenant's host, even though /hosts/{id} has no team param and the
// caller is effectively global-admin. Unpinned reads are unchanged. Runs only under MYSQL_TEST=1.
func TestOpenframeHostByIDTeamFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "byid-tenant-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "byid-tenant-b"})
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

	hostA := mk(teamA, "byid-A")
	hostB := mk(teamB, "byid-B")

	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)

	t.Run("Host: own ok, foreign NotFound", func(t *testing.T) {
		got, err := ds.Host(ctxA, hostA.ID)
		require.NoError(t, err)
		require.Equal(t, hostA.ID, got.ID)

		_, err = ds.Host(ctxA, hostB.ID)
		require.True(t, fleet.IsNotFound(err), "foreign host by id must be NotFound, got %v", err)
	})

	t.Run("HostLite: own ok, foreign NotFound", func(t *testing.T) {
		got, err := ds.HostLite(ctxA, hostA.ID)
		require.NoError(t, err)
		require.Equal(t, hostA.ID, got.ID)

		_, err = ds.HostLite(ctxA, hostB.ID)
		require.True(t, fleet.IsNotFound(err), "foreign host lite by id must be NotFound, got %v", err)
	})

	t.Run("HostByIdentifier: own ok, foreign NotFound", func(t *testing.T) {
		got, err := ds.HostByIdentifier(ctxA, "byid-A")
		require.NoError(t, err)
		require.Equal(t, hostA.ID, got.ID)

		_, err = ds.HostByIdentifier(ctxA, "byid-B")
		require.True(t, fleet.IsNotFound(err), "foreign host by identifier must be NotFound, got %v", err)
	})

	t.Run("HostLiteByIdentifier/ByID: own ok, foreign NotFound", func(t *testing.T) {
		got, err := ds.HostLiteByIdentifier(ctxA, "byid-A")
		require.NoError(t, err)
		require.Equal(t, hostA.ID, got.ID)

		_, err = ds.HostLiteByIdentifier(ctxA, "byid-B")
		require.True(t, fleet.IsNotFound(err), "foreign host-lite by identifier must be NotFound, got %v", err)

		_, err = ds.HostLiteByID(ctxA, hostB.ID)
		require.True(t, fleet.IsNotFound(err), "foreign host-lite by id must be NotFound, got %v", err)
	})

	t.Run("ListHostsLiteByIDs: foreign ids drop out", func(t *testing.T) {
		hosts, err := ds.ListHostsLiteByIDs(ctxA, []uint{hostA.ID, hostB.ID})
		require.NoError(t, err)
		require.Len(t, hosts, 1)
		require.Equal(t, hostA.ID, hosts[0].ID)
	})

	t.Run("HostIDsByIdentifier: foreign identifiers resolve to nothing", func(t *testing.T) {
		adminFilter := fleet.TeamFilter{User: test.UserAdmin}
		ids, err := ds.HostIDsByIdentifier(ctxA, adminFilter, []string{"byid-A", "byid-B", "host-byid-B"})
		require.NoError(t, err)
		require.Equal(t, []uint{hostA.ID}, ids)
	})

	t.Run("unpinned baseline: foreign host still readable", func(t *testing.T) {
		got, err := ds.Host(ctx, hostB.ID)
		require.NoError(t, err)
		require.Equal(t, hostB.ID, got.ID)

		lite, err := ds.HostLiteByIdentifier(ctx, "byid-B")
		require.NoError(t, err)
		require.Equal(t, hostB.ID, lite.ID)

		hosts, err := ds.ListHostsLiteByIDs(ctx, []uint{hostA.ID, hostB.ID})
		require.NoError(t, err)
		require.Len(t, hosts, 2)

		ids, err := ds.HostIDsByIdentifier(ctx, fleet.TeamFilter{User: test.UserAdmin}, []string{"byid-A", "byid-B"})
		require.NoError(t, err)
		require.Len(t, ids, 2)
	})
}

// TestOpenframeListHostsTeamFence verifies the OPENFRAME(mysql-multitenancy) fence in
// applyHostFilters: ListHosts / CountHosts scope to the process's pinned team even when the caller's
// TeamFilter is a global-admin (matches all teams) — the live SDK searchHosts path. Runs only under
// MYSQL_TEST=1.
func TestOpenframeListHostsTeamFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	// Global-admin filter: matches all teams, so only the OpenFrame fence restricts the result.
	filter := fleet.TeamFilter{User: &fleet.User{GlobalRole: ptr.String(fleet.RoleAdmin)}}

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "lh-tenant-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "lh-tenant-b"})
	require.NoError(t, err)

	mk := func(team *fleet.Team, key string) *fleet.Host {
		h, err := ds.NewHost(ctx, &fleet.Host{
			DetailUpdatedAt: time.Now(), LabelUpdatedAt: time.Now(), PolicyUpdatedAt: time.Now(), SeenTime: time.Now(),
			OsqueryHostID: ptr.String(key), NodeKey: ptr.String("nk-" + key), UUID: key, Hostname: "host-" + key,
			Platform: "darwin", TeamID: &team.ID,
		})
		require.NoError(t, err)
		return h
	}
	hostA := mk(teamA, "lh-A")
	hostB := mk(teamB, "lh-B")

	// Baseline (unpinned): global-admin sees both.
	all, err := ds.ListHosts(ctx, filter, fleet.HostListOptions{})
	require.NoError(t, err)
	require.Len(t, all, 2)
	n, err := ds.CountHosts(ctx, filter, fleet.HostListOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, n)

	// Pinned to team A: only team A's host, despite the global-admin filter.
	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)
	got, err := ds.ListHosts(ctxA, filter, fleet.HostListOptions{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, hostA.ID, got[0].ID)

	n, err = ds.CountHosts(ctxA, filter, fleet.HostListOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// Team B's host is invisible to the team-A-pinned process.
	for _, h := range got {
		require.NotEqual(t, hostB.ID, h.ID)
	}
}

// TestOpenframeDeleteHostsTeamFence verifies the OPENFRAME(mysql-multitenancy) fence in
// deleteHosts: a team-scoped request deleting a mix of its own and another tenant's host ids
// only deletes its own. Runs only under MYSQL_TEST=1.
func TestOpenframeDeleteHostsTeamFence(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "del-tenant-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "del-tenant-b"})
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

	hostA := mk(teamA, "del-A")
	hostB := mk(teamB, "del-B")

	// Scope to team A, then try to delete both A (owned) and B (other tenant).
	require.NoError(t, ds.DeleteHosts(fleet.NewOpenframeTeamContext(ctx, teamA.ID), []uint{hostA.ID, hostB.ID}))

	// A is deleted; B (another tenant) is untouched.
	_, err = ds.Host(ctx, hostA.ID)
	require.True(t, fleet.IsNotFound(err))

	gotB, err := ds.Host(ctx, hostB.ID)
	require.NoError(t, err)
	require.Equal(t, hostB.ID, gotB.ID)
}
