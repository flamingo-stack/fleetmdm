package mysql

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestOpenframeAppleOTAIntoAnotherTenant(t *testing.T) {
	ds := CreateMySQLDS(t)
	base := context.Background()
	shared := context.WithValue(base, openframeSharedModeCtxKey{}, true)

	teamA, err := ds.NewTeam(base, &fleet.Team{Name: "tenant-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(base, &fleet.Team{Name: "tenant-b"})
	require.NoError(t, err)

	newDevice := func(serial string) fleet.MDMAppleMachineInfo {
		return fleet.MDMAppleMachineInfo{Serial: serial, UDID: "UDID-" + serial, Product: "iPhone15,2"}
	}
	ota := func(ctx context.Context, team *uint, idpUUID string, info fleet.MDMAppleMachineInfo) error {
		return ds.IngestMDMAppleDeviceFromOTAEnrollment(ctx, team, idpUUID, info)
	}
	authenticate := func(ctx context.Context, info fleet.MDMAppleMachineInfo) {
		require.NoError(t, ds.MDMAppleUpsertHost(ctx, &fleet.Host{
			HardwareSerial: info.Serial, UUID: info.UDID, HardwareModel: info.Product, Platform: "ios",
		}, false))
	}
	type hostRow struct {
		ID     uint   `db:"id"`
		UUID   string `db:"uuid"`
		TeamID *uint  `db:"team_id"`
	}
	hosts := func(serial string) []hostRow {
		var rows []hostRow
		require.NoError(t, sqlx.SelectContext(base, ds.reader(base), &rows,
			`SELECT id, uuid, team_id FROM hosts WHERE hardware_serial = ? ORDER BY id`, serial))
		return rows
	}
	idpAccount := func(email string) string {
		require.NoError(t, ds.InsertMDMIdPAccount(base, &fleet.MDMIdPAccount{Username: email, Fullname: email, Email: email}))
		acct, err := ds.GetMDMIdPAccountByEmail(base, email)
		require.NoError(t, err)
		return acct.UUID
	}
	idpOf := func(hostUUID string) string {
		acct, err := ds.GetMDMIdPAccountByHostUUID(base, hostUUID)
		require.NoError(t, err)
		if acct == nil {
			return ""
		}
		return acct.UUID
	}
	requireConflict := func(err error) {
		var conflict fleet.ConflictError
		require.ErrorAs(t, err, &conflict)
	}
	enrollInA := func(ctx context.Context, info fleet.MDMAppleMachineInfo, idpUUID string) hostRow {
		require.NoError(t, ota(ctx, &teamA.ID, idpUUID, info))
		authenticate(ctx, info)
		rows := hosts(info.Serial)
		require.Len(t, rows, 1)
		require.Equal(t, &teamA.ID, rows[0].TeamID)
		return rows[0]
	}

	t.Run("another tenant's OTA request is refused and changes nothing", func(t *testing.T) {
		info := newDevice("SERIAL-REFUSED")
		userA := idpAccount("user-a@tenant-a.example")
		userB := idpAccount("user-b@tenant-b.example")
		hostA := enrollInA(shared, info, userA)

		forged := info
		forged.UDID = "UDID-FORGED"
		requireConflict(ota(shared, &teamB.ID, userB, forged))
		requireConflict(ota(shared, &teamB.ID, "", info))

		require.Equal(t, []hostRow{hostA}, hosts(info.Serial), "the host keeps its id, UDID and team")
		require.Equal(t, userA, idpOf(info.UDID), "the host keeps its IdP link")
	})

	t.Run("a host of another tenant on any platform with the same serial is protected", func(t *testing.T) {
		const serial = "SERIAL-WINDOWS"
		windowsHost, err := ds.NewHost(base, &fleet.Host{
			Hostname: "windows-a", Platform: "windows", HardwareSerial: serial, UUID: "WINDOWS-UUID-A",
			OsqueryHostID: new("windows-a"), NodeKey: new("windows-a"), TeamID: &teamA.ID,
		})
		require.NoError(t, err)

		requireConflict(ota(shared, &teamB.ID, "", newDevice(serial)))
		rows := hosts(serial)
		require.Equal(t, []hostRow{{ID: windowsHost.ID, UUID: "WINDOWS-UUID-A", TeamID: &teamA.ID}}, rows)
	})

	t.Run("another tenant's request with the device's UDID and another serial is refused", func(t *testing.T) {
		info := newDevice("SERIAL-UDID-OWNER")
		hostA := enrollInA(shared, info, "")

		other := info
		other.Serial = "SERIAL-UDID-OTHER"
		requireConflict(ota(shared, &teamB.ID, "", other))
		require.Empty(t, hosts(other.Serial))
		require.Equal(t, []hostRow{hostA}, hosts(info.Serial))
	})

	t.Run("the host's own tenant can enroll the device again", func(t *testing.T) {
		info := newDevice("SERIAL-SAME")
		hostA := enrollInA(shared, info, "")
		require.NoError(t, ota(shared, &teamA.ID, "", info))
		authenticate(shared, info)
		require.Equal(t, []hostRow{hostA}, hosts(info.Serial))
	})

	t.Run("after the previous tenant deletes the host the device enrolls into the new one", func(t *testing.T) {
		info := newDevice("SERIAL-TRANSFER")
		hostA := enrollInA(shared, info, "")
		require.NoError(t, ds.DeleteHost(base, hostA.ID))

		require.NoError(t, ota(shared, &teamB.ID, "", info))
		authenticate(shared, info)
		rows := hosts(info.Serial)
		require.Len(t, rows, 1)
		require.NotEqual(t, hostA.ID, rows[0].ID)
		require.Equal(t, &teamB.ID, rows[0].TeamID)
	})

	t.Run("a device new to Fleet enrolls", func(t *testing.T) {
		info := newDevice("SERIAL-NEW")
		require.NoError(t, ota(shared, &teamB.ID, "", info))
		rows := hosts(info.Serial)
		require.Len(t, rows, 1)
		require.Equal(t, &teamB.ID, rows[0].TeamID)
	})

	t.Run("upstream accepts the request and keeps the device's host and team", func(t *testing.T) {
		info := newDevice("SERIAL-UPSTREAM")
		notShared := context.WithValue(base, openframeSharedModeCtxKey{}, false)
		hostA := enrollInA(notShared, info, "")

		require.NoError(t, ota(notShared, &teamB.ID, "", info))
		authenticate(notShared, info)
		require.Equal(t, []hostRow{hostA}, hosts(info.Serial))
	})
}

func TestOpenframeTeamsDiffer(t *testing.T) {
	a, b := new(uint(5)), new(uint(6))
	require.False(t, openframeTeamsDiffer(nil, nil))
	require.True(t, openframeTeamsDiffer(a, nil))
	require.True(t, openframeTeamsDiffer(nil, b))
	require.True(t, openframeTeamsDiffer(a, b))
	require.False(t, openframeTeamsDiffer(a, new(uint(5))))
}
