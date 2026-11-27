// OPENFRAME(windows-mdm-tenant)
package mysql

import (
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
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
		_, err := q.ExecContext(t.Context(), `UPDATE hosts SET uuid = ?, team_id = ? WHERE id = ?`, hostUUID, teamID, h.ID)
		return err
	})
	h.UUID, h.TeamID = hostUUID, &teamID
	return h
}

func (f *openframeWindowsMDMFixture) enroll(t *testing.T, hwID, deviceID, hostUUID string, teamID *uint) {
	require.NoError(t, f.ds.MDMWindowsInsertEnrolledDevice(t.Context(), &fleet.MDMWindowsEnrolledDevice{
		MDMDeviceID:            deviceID,
		MDMHardwareID:          hwID,
		MDMDeviceState:         "ds",
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
