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

// seed inserts an enrollment directly, as one that existed before a clash was created around it.
func (f *openframeWindowsMDMFixture) seed(t *testing.T, hwID, deviceID, hostUUID string, teamID *uint) {
	ExecAdhocSQL(t, f.ds, func(q sqlx.ExtContext) error {
		_, err := q.ExecContext(t.Context(), `INSERT INTO mdm_windows_enrollments (mdm_device_id, mdm_hardware_id, device_state,
			device_type, device_name, enroll_type, enroll_user_id, enroll_proto_version, enroll_client_version, host_uuid, team_id)
			VALUES (?, ?, ?, 'CIMClient_Windows', 'pc', 'ProgrammaticEnrollment', '', '5.0', '10.0', ?, ?)`,
			deviceID, hwID, microsoft_mdm.MDMDeviceStateEnrolled, hostUUID, teamID)
		return err
	})
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
	hostA := f.host(t, "reenroll-a", "uuid-reenroll-a", f.teamA.ID)
	hostB := f.host(t, "reenroll-b", "uuid-reenroll-b", f.teamB.ID)
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

	err := f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostB.UUID, "hw-1", "dev-b", f.teamB.ID)
	var conflict *fleet.ConflictError
	require.ErrorAs(t, err, &conflict, "another tenant's hardware id is refused, not treated as new")
	require.Equal(t, 1, enrollments(), "tenant A's enrollment stays")
	require.Equal(t, 1, actions(hostA))
	require.Equal(t, 1, upcoming(hostA))

	require.NoError(t, f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostA.UUID, "hw-1", "dev-1", f.teamA.ID))
	require.Zero(t, enrollments())
	require.Zero(t, actions(hostA))
	require.Zero(t, upcoming(hostA))
	require.Equal(t, 1, actions(hostB), "tenant B's host keeps its lock state")
	require.Equal(t, 1, upcoming(hostB), "tenant B's host keeps its activities")

	require.NoError(t, f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostA.UUID, "hw-1", "dev-1", f.teamA.ID),
		"a first enrollment has nothing to remove")
}

// A device moved to another MSP: once the old tenant deletes its host, the new tenant's enrollment
// releases the old enrollment instead of being refused forever.
func TestOpenframeMDMWindowsReenrollmentAfterOldTenantDeletesHost(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()
	hostA := f.host(t, "moved-a", "uuid-moved", f.teamA.ID)
	f.enroll(t, "hw-moved", "dev-moved", hostA.UUID, &f.teamA.ID)
	var conflict *fleet.ConflictError

	hostB := f.host(t, "moved-b", "uuid-moved-b", f.teamB.ID)
	require.ErrorAs(t, f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostB.UUID, "hw-moved", "dev-moved-b", f.teamB.ID), &conflict,
		"while tenant A has the host, the device stays A's")

	require.NoError(t, f.ds.DeleteHost(ctx, hostA.ID))
	require.NoError(t, f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostB.UUID, "hw-moved", "dev-moved-b", f.teamB.ID),
		"once tenant A deleted the host, its enrollment is released")
	require.Zero(t, f.count(t, `SELECT COUNT(*) FROM mdm_windows_enrollments WHERE mdm_hardware_id = 'hw-moved'`))

	f.enroll(t, "hw-moved-2", "dev-moved-2", "uuid-moved-2", &f.teamA.ID)
	hostB2 := f.host(t, "moved-b2", "uuid-moved-2", f.teamB.ID)
	require.NoError(t, f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostB2.UUID, "hw-new", "dev-new", f.teamB.ID),
		"tenant A's released enrollment with the hardware uuid does not block tenant B")
}

// A refused re-enrollment leaves the device's working enrollment as it is.
func TestOpenframeMDMWindowsRefusedReenrollmentKeepsEnrollment(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()
	hostA := f.host(t, "keep-a", "uuid-keep", f.teamA.ID)
	f.hostState(t, hostA)
	f.enroll(t, "hw-keep", "dev-keep", hostA.UUID, &f.teamA.ID)
	f.host(t, "keep-b", hostA.UUID, f.teamB.ID)

	var conflict *fleet.ConflictError
	require.ErrorAs(t, f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostA.UUID, "hw-keep", "dev-keep", f.teamA.ID), &conflict)
	require.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM mdm_windows_enrollments WHERE mdm_hardware_id = 'hw-keep'`),
		"the enrollment survives the refusal")
	require.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM host_mdm_actions WHERE host_id = ?`, hostA.ID))
}

// Two enrollments that each pass the check before their insert: the one that finds the other already
// committed after its own insert removes itself, so at most one stays.
func TestOpenframeMDMWindowsConcurrentEnrollmentsKeepOne(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()
	hostA := f.host(t, "race-a", "uuid-race-a", f.teamA.ID)
	hostB := f.host(t, "race-b", "uuid-race-b", f.teamB.ID)
	require.NoError(t, f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostA.UUID, "hw-race-a", "dev-race", f.teamA.ID))
	require.NoError(t, f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostB.UUID, "hw-race-b", "dev-race", f.teamB.ID))

	f.enroll(t, "hw-race-a", "dev-race", hostA.UUID, &f.teamA.ID)
	err := f.ds.MDMWindowsInsertEnrolledDevice(ctx, &fleet.MDMWindowsEnrolledDevice{
		MDMDeviceID: "dev-race", MDMHardwareID: "hw-race-b", MDMDeviceState: microsoft_mdm.MDMDeviceStateEnrolled,
		MDMDeviceType: "CIMClient_Windows", MDMDeviceName: "DESKTOP-race", MDMEnrollType: "ProgrammaticEnrollment",
		MDMEnrollProtoVersion: "5.0", MDMEnrollClientVersion: "10.0", MDMNotInOOBE: true,
		HostUUID: hostB.UUID, OpenframeTeamID: &f.teamB.ID,
	})
	var conflict *fleet.ConflictError
	require.ErrorAs(t, err, &conflict, "the second enrollment with the same device id is refused after its insert")
	require.Zero(t, f.count(t, `SELECT COUNT(*) FROM mdm_windows_enrollments WHERE mdm_hardware_id = 'hw-race-b'`),
		"and removed again")
	require.Equal(t, 1, f.count(t, `SELECT COUNT(*) FROM mdm_windows_enrollments WHERE mdm_hardware_id = 'hw-race-a'`),
		"the first one stays")
}

func TestOpenframeMDMWindowsReenrollmentOfEnrollmentWithoutTenant(t *testing.T) {
	f := newOpenframeWindowsMDMFixture(t, CreateMySQLDS(t))
	ctx := t.Context()
	hostA := f.host(t, "legacy-a", "uuid-legacy", f.teamA.ID)
	hostB := f.host(t, "legacy-b", "uuid-legacy-b", f.teamB.ID)
	f.enroll(t, "hw-legacy", "dev-legacy", hostA.UUID, nil)
	f.enroll(t, "hw-unlinked", "dev-unlinked", "", nil)

	var conflict *fleet.ConflictError
	require.ErrorAs(t, f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostB.UUID, "hw-legacy", "dev-b", f.teamB.ID), &conflict,
		"an enrollment without a tenant belongs to the tenant of its host, not to tenant B")
	require.NoError(t, f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostA.UUID, "hw-unlinked", "dev-a2", f.teamA.ID),
		"an enrollment without a tenant or a host is released")
	require.NoError(t, f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostA.UUID, "hw-legacy", "dev-legacy", f.teamA.ID),
		"the tenant of its host re-enrolls the device")
	require.Zero(t, f.count(t, `SELECT COUNT(*) FROM mdm_windows_enrollments WHERE mdm_hardware_id IN ('hw-legacy', 'hw-unlinked')`))
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
	f.seed(t, "hw-b", hostA.UUID, hostB.UUID, &f.teamB.ID)
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

	prepare := func(hostUUID, deviceID string) error {
		return f.ds.OpenframeMDMWindowsPrepareEnrollment(ctx, hostUUID, "hw-new", deviceID, f.teamB.ID)
	}
	require.NoError(t, prepare(hostB.UUID, "dev-new"))

	require.ErrorAs(t, prepare(hostB.UUID, "dev-conf-a"), &conflict, "another device's device id is refused")
	require.ErrorAs(t, prepare(hostB.UUID, hostA.UUID), &conflict, "a device id that is another host's uuid is refused")
	require.NoError(t, prepare(hostB.UUID, hostB.UUID), "a device id equal to the host's own uuid is not another host's")
	require.ErrorAs(t, prepare(hostA.UUID, "dev-new"), &conflict, "a hardware uuid another tenant's enrollment holds is refused")
	require.ErrorContains(t, prepare("", "dev-new"), "no host")

	f.host(t, "conf-b-twin", hostB.UUID, f.teamA.ID)
	require.ErrorAs(t, prepare(hostB.UUID, "dev-new"), &conflict, "a hardware uuid another host has, in any tenant, is refused")
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
	f.seed(t, "hw-read", "dev-read", hostA.UUID, &f.teamA.ID)
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

	cmd := &fleet.MDMWindowsCommand{CommandUUID: uuid.NewString(), RawCommand: []byte("<Exec></Exec>"), TargetLocURI: "./test/uri"}
	require.NoError(t, f.ds.MDMWindowsInsertCommandForHosts(openframePinnedSharedCtx(ctx, f.teamA.ID), []string{hostA.UUID}, cmd))
	admin := fleet.TeamFilter{User: &fleet.User{GlobalRole: new(fleet.RoleAdmin)}}
	commands := func(teamID uint, hostIdentifier string) []*fleet.MDMCommand {
		got, _, _, err := f.ds.ListMDMCommands(openframePinnedSharedCtx(ctx, teamID), admin, &fleet.MDMCommandListOptions{
			ListOptions: fleet.ListOptions{PerPage: 10},
			Filters:     fleet.MDMCommandFilters{HostIdentifier: hostIdentifier},
		})
		require.NoError(t, err)
		return got
	}
	require.Len(t, commands(f.teamA.ID, ""), 1, "tenant A lists its device's command")
	require.Len(t, commands(f.teamA.ID, hostA.UUID), 1)
	for _, identifier := range []string{"", hostB.UUID} {
		for _, c := range commands(f.teamB.ID, identifier) {
			require.NotEqual(t, cmd.CommandUUID, c.CommandUUID, "tenant B does not list A's device's command on its host with A's uuid")
		}
	}
	_, err = f.ds.getMDMCommand(openframePinnedSharedCtx(ctx, f.teamA.ID), f.ds.reader(ctx), cmd.CommandUUID)
	require.NoError(t, err)
	_, err = f.ds.getMDMCommand(openframePinnedSharedCtx(ctx, f.teamB.ID), f.ds.reader(ctx), cmd.CommandUUID)
	require.True(t, fleet.IsNotFound(err), "tenant B can't read A's device's command by its uuid")
}
