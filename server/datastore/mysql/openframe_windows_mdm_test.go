// OPENFRAME(windows-mdm-tenant)
package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	microsoft_mdm "github.com/fleetdm/fleet/v4/server/mdm/microsoft"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

type openframeWindowsMDMFixture struct {
	ds           *Datastore
	teamA, teamB *fleet.Team
}

// newOpenframeWindowsMDMFixture takes the test's own datastore: CreateMySQLDS names the database after
// its caller.
func newOpenframeWindowsMDMFixture(t *testing.T, ds *Datastore) *openframeWindowsMDMFixture {
	ctx := t.Context()
	require.NoError(t, ds.MigrateOpenframe(ctx))
	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-windows-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-windows-b"})
	require.NoError(t, err)
	return &openframeWindowsMDMFixture{ds: ds, teamA: teamA, teamB: teamB}
}

func (f *openframeWindowsMDMFixture) host(t *testing.T, name, hostUUID string, teamID uint) *fleet.Host {
	h := test.NewHost(t, f.ds, name, "", name+"-key", name+"-osq", time.Now())
	ExecAdhocSQL(t, f.ds, func(q sqlx.ExtContext) error {
		_, err := q.ExecContext(t.Context(), `UPDATE hosts SET uuid = ?, team_id = ?, platform = 'windows' WHERE id = ?`, hostUUID, teamID, h.ID)
		return err
	})
	h.UUID, h.TeamID, h.Platform = hostUUID, &teamID, "windows"
	return h
}

func (f *openframeWindowsMDMFixture) enroll(t *testing.T, hwID, deviceID, hostUUID string, teamID *uint) {
	require.NoError(t, f.ds.MDMWindowsInsertEnrolledDevice(t.Context(), &fleet.MDMWindowsEnrolledDevice{
		MDMDeviceID:            deviceID,
		MDMHardwareID:          hwID,
		MDMDeviceState:         microsoft_mdm.MDMDeviceStateEnrolled,
		MDMDeviceType:          "CIMClient_Windows",
		MDMDeviceName:          "DESKTOP-" + hwID,
		MDMEnrollType:          "ProgrammaticEnrollment",
		MDMEnrollUserID:        "",
		MDMEnrollProtoVersion:  "5.0",
		MDMEnrollClientVersion: "10.0.19045.2965",
		MDMNotInOOBE:           true,
		HostUUID:               hostUUID,
		OpenframeTeamID:        teamID,
	}))
}

// hostState adds the host state a re-enrollment clears: a lock action and an upcoming activity.
func (f *openframeWindowsMDMFixture) hostState(t *testing.T, h *fleet.Host) {
	ExecAdhocSQL(t, f.ds, func(q sqlx.ExtContext) error {
		if _, err := q.ExecContext(t.Context(),
			`INSERT INTO host_mdm_actions (host_id, lock_ref, fleet_platform) VALUES (?, ?, 'windows')`, h.ID, uuid.NewString()); err != nil {
			return err
		}
		_, err := q.ExecContext(t.Context(),
			`INSERT INTO upcoming_activities (host_id, priority, activity_type, execution_id, payload) VALUES (?, 1, 'script', ?, JSON_OBJECT())`,
			h.ID, uuid.NewString())
		return err
	})
}

func (f *openframeWindowsMDMFixture) count(t *testing.T, stmt string, args ...any) int {
	var n int
	require.NoError(t, sqlx.GetContext(t.Context(), f.ds.reader(t.Context()), &n, stmt, args...))
	return n
}

func (f *openframeWindowsMDMFixture) enrollmentHostUUID(t *testing.T, deviceID string) string {
	var hostUUID string
	require.NoError(t, sqlx.GetContext(t.Context(), f.ds.reader(t.Context()), &hostUUID,
		`SELECT host_uuid FROM mdm_windows_enrollments WHERE mdm_device_id = ?`, deviceID))
	return hostUUID
}

func (f *openframeWindowsMDMFixture) enrollmentID(t *testing.T, hwID string) uint {
	var id uint
	require.NoError(t, sqlx.GetContext(t.Context(), f.ds.reader(t.Context()), &id,
		`SELECT id FROM mdm_windows_enrollments WHERE mdm_hardware_id = ?`, hwID))
	return id
}

func TestOpenframeMDMWindowsEnrollmentRecordsTenant(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()

	f.enroll(t, "hw-a", "dev-a", "uuid-a", &f.teamA.ID)
	got, err := f.ds.OpenframeMDMWindowsEnrollmentTeamID(ctx, f.enrollmentID(t, "hw-a"))
	require.NoError(t, err)
	require.Equal(t, &f.teamA.ID, got)

	f.enroll(t, "hw-legacy", "dev-legacy", "uuid-a", nil)
	got, err = f.ds.OpenframeMDMWindowsEnrollmentTeamID(ctx, f.enrollmentID(t, "hw-legacy"))
	require.NoError(t, err)
	require.Nil(t, got, "an enrollment without a tenant reports none")

	_, err = f.ds.OpenframeMDMWindowsEnrollmentTeamID(ctx, 999999)
	require.True(t, fleet.IsNotFound(err))
}

func TestOpenframeMDMWindowsReenrollmentStaysInTenant(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()
	hostA := f.host(t, "reenroll-a", "uuid-shared", f.teamA.ID)
	hostB := f.host(t, "reenroll-b", "uuid-shared", f.teamB.ID)
	f.hostState(t, hostA)
	f.hostState(t, hostB)
	f.enroll(t, "hw-1", "dev-1", hostA.UUID, &f.teamA.ID)

	actions := func(h *fleet.Host) int {
		return f.count(t, `SELECT COUNT(*) FROM host_mdm_actions WHERE host_id = ?`, h.ID)
	}
	upcoming := func(h *fleet.Host) int {
		return f.count(t, `SELECT COUNT(*) FROM upcoming_activities WHERE host_id = ?`, h.ID)
	}
	enrollments := func() int {
		return f.count(t, `SELECT COUNT(*) FROM mdm_windows_enrollments WHERE mdm_hardware_id = 'hw-1'`)
	}

	err := f.ds.OpenframeMDMWindowsDeleteEnrolledDeviceOnReenrollment(ctx, "hw-1", f.teamB.ID)
	var conflict *fleet.ConflictError
	require.ErrorAs(t, err, &conflict, "another tenant's hardware id is refused, not treated as new")
	require.False(t, fleet.IsNotFound(err))
	require.Equal(t, 1, enrollments(), "tenant A's enrollment stays")
	require.Equal(t, 1, actions(hostA))
	require.Equal(t, 1, upcoming(hostA))

	require.NoError(t, f.ds.OpenframeMDMWindowsDeleteEnrolledDeviceOnReenrollment(ctx, "hw-1", f.teamA.ID))
	require.Zero(t, enrollments())
	require.Zero(t, actions(hostA))
	require.Zero(t, upcoming(hostA))
	require.Equal(t, 1, actions(hostB), "the same uuid in tenant B keeps its lock state")
	require.Equal(t, 1, upcoming(hostB), "the same uuid in tenant B keeps its activities")

	err = f.ds.OpenframeMDMWindowsDeleteEnrolledDeviceOnReenrollment(ctx, "hw-1", f.teamA.ID)
	require.True(t, fleet.IsNotFound(err), "nothing left to delete")
}

func TestOpenframeMDMWindowsReenrollmentOfEnrollmentWithoutTenant(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()
	hostA := f.host(t, "legacy-a", "uuid-legacy", f.teamA.ID)
	f.enroll(t, "hw-legacy", "dev-legacy", hostA.UUID, nil)
	f.enroll(t, "hw-unlinked", "dev-unlinked", "", nil)

	var conflict *fleet.ConflictError
	require.ErrorAs(t, f.ds.OpenframeMDMWindowsDeleteEnrolledDeviceOnReenrollment(ctx, "hw-legacy", f.teamB.ID), &conflict,
		"an enrollment without a tenant belongs to the tenant of its host, not to tenant B")
	require.ErrorAs(t, f.ds.OpenframeMDMWindowsDeleteEnrolledDeviceOnReenrollment(ctx, "hw-unlinked", f.teamA.ID), &conflict,
		"an enrollment without a tenant or a host belongs to no tenant")
	require.NoError(t, f.ds.OpenframeMDMWindowsDeleteEnrolledDeviceOnReenrollment(ctx, "hw-legacy", f.teamA.ID),
		"the tenant of its host re-enrolls the device")
	require.Zero(t, f.count(t, `SELECT COUNT(*) FROM mdm_windows_enrollments WHERE mdm_hardware_id = 'hw-legacy'`))
}

func TestOpenframeMDMWindowsLinkStaysInTenant(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()
	hostA := f.host(t, "link-a", "uuid-link-a", f.teamA.ID)
	hostB := f.host(t, "link-b", "uuid-link-b", f.teamB.ID)
	f.enroll(t, "hw-link", "dev-link", "", &f.teamA.ID)
	f.enroll(t, "hw-link-legacy", "dev-link-legacy", "", nil)

	updated, err := f.ds.OpenframeUpdateMDMWindowsEnrollmentHost(ctx, hostB.ID, "dev-link")
	require.NoError(t, err)
	require.False(t, updated, "a host of tenant B can't take tenant A's enrollment")
	require.Empty(t, f.enrollmentHostUUID(t, "dev-link"))

	updated, err = f.ds.OpenframeUpdateMDMWindowsEnrollmentHost(ctx, hostA.ID, "dev-link")
	require.NoError(t, err)
	require.True(t, updated)
	require.Equal(t, hostA.UUID, f.enrollmentHostUUID(t, "dev-link"))

	updated, err = f.ds.OpenframeUpdateMDMWindowsEnrollmentHost(ctx, hostA.ID, "dev-link")
	require.NoError(t, err)
	require.False(t, updated, "linking the same host again changes nothing, as upstream")

	updated, err = f.ds.OpenframeUpdateMDMWindowsEnrollmentHost(ctx, hostB.ID, "dev-link")
	require.NoError(t, err)
	require.False(t, updated, "a linked enrollment is not moved to another tenant's host")
	require.Equal(t, hostA.UUID, f.enrollmentHostUUID(t, "dev-link"))

	updated, err = f.ds.OpenframeUpdateMDMWindowsEnrollmentHost(ctx, hostA.ID, "dev-link-legacy")
	require.NoError(t, err)
	require.False(t, updated, "an enrollment without a tenant is linked to no host")
}

func openframeSharedCtx(ctx context.Context) context.Context {
	return context.WithValue(ctx, openframeSharedModeCtxKey{}, true)
}

func openframePinnedSharedCtx(ctx context.Context, teamID uint) context.Context {
	return openframeSharedCtx(fleet.NewOpenframeTeamContext(ctx, teamID))
}

func (f *openframeWindowsMDMFixture) enrollmentIDs(t *testing.T, ctx context.Context, byUUIDOnly bool, ids ...string) []uint {
	var (
		got []uint
		err error
	)
	if byUUIDOnly {
		got, err = f.ds.getEnrollmentIDsByHostUUIDDB(ctx, f.ds.writer(ctx), ids)
	} else {
		got, err = f.ds.getEnrollmentIDsByHostUUIDOrDeviceIDDB(ctx, f.ds.writer(ctx), ids)
	}
	require.NoError(t, err)
	return got
}

// A tenant-B host claiming tenant A's hardware uuid, and a tenant-B enrollment whose device id is that uuid,
// must not receive tenant A's commands, and tenant B's commands must not reach tenant A's device.
func TestOpenframeMDMWindowsCommandsStayInTenant(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()
	hostA := f.host(t, "cmd-a", "uuid-victim", f.teamA.ID)
	f.enroll(t, "hw-a", "dev-a", hostA.UUID, &f.teamA.ID)
	enrollmentA := f.enrollmentID(t, "hw-a")
	pinnedA, pinnedB := openframePinnedSharedCtx(ctx, f.teamA.ID), openframePinnedSharedCtx(ctx, f.teamB.ID)

	for _, byUUIDOnly := range []bool{true, false} {
		require.Equal(t, []uint{enrollmentA}, f.enrollmentIDs(t, pinnedA, byUUIDOnly, hostA.UUID), "tenant A reaches its device")
		require.Equal(t, []uint{enrollmentA}, f.enrollmentIDs(t, openframeSharedCtx(ctx), byUUIDOnly, hostA.UUID),
			"an unpinned command reaches a uuid only one tenant has")
	}

	f.host(t, "cmd-b-fake", hostA.UUID, f.teamB.ID)
	for _, byUUIDOnly := range []bool{true, false} {
		require.Empty(t, f.enrollmentIDs(t, pinnedB, byUUIDOnly, hostA.UUID), "tenant B's command for its host with A's uuid reaches no device")
		require.Equal(t, []uint{enrollmentA}, f.enrollmentIDs(t, pinnedA, byUUIDOnly, hostA.UUID), "tenant A still reaches its device")
		require.Empty(t, f.enrollmentIDs(t, openframeSharedCtx(ctx), byUUIDOnly, hostA.UUID),
			"an unpinned command reaches no device of a uuid two tenants share")
		require.Equal(t, []uint{enrollmentA}, f.enrollmentIDs(t, ctx, byUUIDOnly, hostA.UUID), "outside shared mode as upstream")
	}
	cmd := &fleet.MDMWindowsCommand{CommandUUID: uuid.NewString(), RawCommand: []byte("<Exec></Exec>"), TargetLocURI: "./test/uri"}
	require.NoError(t, f.ds.MDMWindowsInsertCommandForHosts(pinnedB, []string{hostA.UUID}, cmd))
	require.Zero(t, f.count(t, `SELECT COUNT(*) FROM windows_mdm_command_queue WHERE command_uuid = ?`, cmd.CommandUUID),
		"tenant B's wipe or command for its host with A's uuid is queued for no device")

	hostB := f.host(t, "cmd-b", "uuid-attacker", f.teamB.ID)
	f.enroll(t, "hw-b", hostA.UUID, hostB.UUID, &f.teamB.ID)
	enrollmentB := f.enrollmentID(t, "hw-b")
	require.Equal(t, []uint{enrollmentA}, f.enrollmentIDs(t, pinnedA, false, hostA.UUID),
		"a tenant-B enrollment whose device id is A's host uuid does not take A's commands")
	require.Equal(t, []uint{enrollmentB}, f.enrollmentIDs(t, ctx, false, hostA.UUID),
		"upstream resolves the newest enrollment, whatever its tenant")

}

func TestOpenframeMDMWindowsReconcileSkipsSharedUUID(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()
	shared := openframeSharedCtx(ctx)
	hostA := f.host(t, "rec-a", "uuid-rec", f.teamA.ID)
	f.enroll(t, "hw-rec", "dev-rec", hostA.UUID, &f.teamA.ID)
	require.NoError(t, f.ds.SetOrUpdateMDMData(ctx, hostA.ID, false, true, "https://a.example.com", false, fleet.WellKnownMDMFleet, "", false))

	got, err := f.ds.GetWindowsMDMHostForReconcile(shared, hostA.UUID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, hostA.ID, got.HostID)

	hostB := f.host(t, "rec-b-fake", hostA.UUID, f.teamB.ID)
	require.NoError(t, f.ds.SetOrUpdateMDMData(ctx, hostB.ID, false, true, "https://b.example.com", false, fleet.WellKnownMDMFleet, "", false))
	got, err = f.ds.GetWindowsMDMHostForReconcile(shared, hostA.UUID)
	require.NoError(t, err)
	require.Nil(t, got, "the reconciler skips a uuid two tenants share")

	listed, _, _, _, err := f.ds.GetWindowsProfileReconcileSnapshot(shared, "", 100)
	require.NoError(t, err)
	require.Empty(t, listed, "neither tenant's host with the shared uuid is reconciled")

	got, err = f.ds.GetWindowsMDMHostForReconcile(ctx, hostA.UUID)
	require.NoError(t, err)
	require.NotNil(t, got, "outside shared mode as upstream")
}

func TestOpenframeMDMWindowsEnrollConflict(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()
	hostA := f.host(t, "conf-a", "uuid-conf-a", f.teamA.ID)
	hostB := f.host(t, "conf-b", "uuid-conf-b", f.teamB.ID)
	f.enroll(t, "hw-conf-a", "dev-conf-a", hostA.UUID, &f.teamA.ID)
	var conflict *fleet.ConflictError

	require.NoError(t, f.ds.OpenframeWindowsMDMEnrollConflict(ctx, hostB.UUID, "dev-new", f.teamB.ID))

	require.ErrorAs(t, f.ds.OpenframeWindowsMDMEnrollConflict(ctx, hostB.UUID, "dev-conf-a", f.teamB.ID), &conflict,
		"another device's device id is refused")
	require.ErrorAs(t, f.ds.OpenframeWindowsMDMEnrollConflict(ctx, hostB.UUID, hostA.UUID, f.teamB.ID), &conflict,
		"a device id that is another host's uuid is refused")
	require.NoError(t, f.ds.OpenframeWindowsMDMEnrollConflict(ctx, hostB.UUID, hostB.UUID, f.teamB.ID),
		"a device id equal to the host's own uuid is not another host's")
	require.ErrorAs(t, f.ds.OpenframeWindowsMDMEnrollConflict(ctx, hostA.UUID, "dev-new", f.teamB.ID), &conflict,
		"a hardware uuid another tenant's enrollment holds is refused")

	f.host(t, "conf-b-twin", hostB.UUID, f.teamA.ID)
	require.ErrorAs(t, f.ds.OpenframeWindowsMDMEnrollConflict(ctx, hostB.UUID, "dev-new", f.teamB.ID), &conflict,
		"a hardware uuid another host has, in any tenant, is refused")
}

func TestOpenframeMDMWindowsResetStaysInTenant(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()
	hostA := f.host(t, "reset-a", "uuid-reset", f.teamA.ID)
	hostB := f.host(t, "reset-b", "uuid-reset", f.teamB.ID)
	for _, h := range []*fleet.Host{hostA, hostB} {
		ExecAdhocSQL(t, f.ds, func(q sqlx.ExtContext) error {
			if _, err := q.ExecContext(ctx, `UPDATE hosts SET platform = 'windows' WHERE id = ?`, h.ID); err != nil {
				return err
			}
			_, err := q.ExecContext(ctx, `INSERT INTO host_disk_encryption_keys (host_id, base64_encrypted) VALUES (?, 'x')`, h.ID)
			return err
		})
	}
	keys := func(h *fleet.Host) int {
		return f.count(t, `SELECT COUNT(*) FROM host_disk_encryption_keys WHERE host_id = ?`, h.ID)
	}

	require.NoError(t, f.ds.MDMResetEnrollment(openframePinnedSharedCtx(ctx, f.teamB.ID), hostB.UUID, false))
	require.Zero(t, keys(hostB), "tenant B's own host is reset")
	require.Equal(t, 1, keys(hostA), "tenant A's host with the same uuid keeps its BitLocker key")
}

func TestOpenframeMDMWindowsHostReadsStayInTenant(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()
	hostA := f.host(t, "read-a", "uuid-read", f.teamA.ID)
	hostB := f.host(t, "read-b-fake", hostA.UUID, f.teamB.ID)
	f.enroll(t, "hw-read", "dev-read", hostA.UUID, &f.teamA.ID)
	for _, h := range []*fleet.Host{hostA, hostB} {
		require.NoError(t, f.ds.SetOrUpdateMDMData(ctx, h.ID, false, true, "https://x.example.com", false, fleet.WellKnownMDMFleet, "", false))
		ExecAdhocSQL(t, f.ds, func(q sqlx.ExtContext) error {
			_, err := q.ExecContext(ctx, `UPDATE hosts SET platform = 'windows' WHERE id = ?`, h.ID)
			return err
		})
		h.Platform = "windows"
	}
	profileA, err := f.ds.NewMDMWindowsConfigProfile(ctx, fleet.MDMWindowsConfigProfile{
		TeamID: &f.teamA.ID, Name: "tenant-a-wifi",
		SyncML: []byte(`<Replace><Item><Target><LocURI>./Device/Vendor/MSFT/Policy/Config/A</LocURI></Target></Item></Replace>`),
	}, nil)
	require.NoError(t, err)
	ExecAdhocSQL(t, f.ds, func(q sqlx.ExtContext) error {
		_, err := q.ExecContext(ctx, `INSERT INTO host_mdm_windows_profiles (host_uuid, profile_uuid, profile_name, command_uuid, status, operation_type)
			VALUES (?, ?, ?, ?, 'failed', 'install')`, hostA.UUID, profileA.ProfileUUID, profileA.Name, uuid.NewString())
		return err
	})

	profiles, err := f.ds.GetHostMDMWindowsProfiles(openframePinnedSharedCtx(ctx, f.teamA.ID), hostA.UUID)
	require.NoError(t, err)
	require.Len(t, profiles, 1, "tenant A sees its profile")
	profiles, err = f.ds.GetHostMDMWindowsProfiles(openframePinnedSharedCtx(ctx, f.teamB.ID), hostB.UUID)
	require.NoError(t, err)
	require.Empty(t, profiles, "tenant B's host with A's uuid does not show A's profile")

	connected, err := f.ds.IsHostConnectedToFleetMDM(openframeSharedCtx(ctx), hostA)
	require.NoError(t, err)
	require.True(t, connected)
	connected, err = f.ds.IsHostConnectedToFleetMDM(openframeSharedCtx(ctx), hostB)
	require.NoError(t, err)
	require.False(t, connected, "tenant B's host is not connected through A's enrollment")
	byUUID, err := f.ds.AreHostsConnectedToFleetMDM(openframePinnedSharedCtx(ctx, f.teamB.ID), []*fleet.Host{hostB})
	require.NoError(t, err)
	require.False(t, byUUID[hostB.UUID], "tenant B's host list does not show its host connected through A's enrollment")
	byUUID, err = f.ds.AreHostsConnectedToFleetMDM(openframePinnedSharedCtx(ctx, f.teamA.ID), []*fleet.Host{hostA})
	require.NoError(t, err)
	require.True(t, byUUID[hostA.UUID])
	connected, err = f.ds.IsHostConnectedToFleetMDM(ctx, hostB)
	require.NoError(t, err)
	require.True(t, connected, "outside shared mode as upstream")
}
