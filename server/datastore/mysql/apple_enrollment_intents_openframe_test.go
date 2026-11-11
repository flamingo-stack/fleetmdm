package mysql

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

type openframeAppleHostRow struct {
	ID     uint  `db:"id"`
	TeamID *uint `db:"team_id"`
}

func TestOpenframeAppleReenrollIntoAnotherTenant(t *testing.T) {
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
	otaWithIdP := func(ctx context.Context, team *uint, idpUUID string, info fleet.MDMAppleMachineInfo) {
		require.NoError(t, ds.IngestMDMAppleDeviceFromOTAEnrollment(ctx, team, idpUUID, info))
	}
	ota := func(ctx context.Context, team *uint, info fleet.MDMAppleMachineInfo) {
		otaWithIdP(ctx, team, "", info)
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
	authenticate := func(ctx context.Context, info fleet.MDMAppleMachineInfo) {
		require.NoError(t, ds.MDMAppleUpsertHost(ctx, &fleet.Host{
			HardwareSerial: info.Serial, UUID: info.UDID, HardwareModel: info.Product, Platform: "ios",
		}, false))
	}
	hosts := func(serial string) []openframeAppleHostRow {
		var rows []openframeAppleHostRow
		require.NoError(t, sqlx.SelectContext(base, ds.reader(base), &rows,
			`SELECT id, team_id FROM hosts WHERE hardware_serial = ? ORDER BY id`, serial))
		return rows
	}
	intents := func(serial string) int {
		var n int
		require.NoError(t, sqlx.GetContext(base, ds.reader(base), &n,
			`SELECT COUNT(*) FROM openframe_apple_enrollment_intents WHERE hardware_serial = ?`, serial))
		return n
	}
	enrollInA := func(ctx context.Context, info fleet.MDMAppleMachineInfo) uint {
		ota(ctx, &teamA.ID, info)
		authenticate(ctx, info)
		rows := hosts(info.Serial)
		require.Len(t, rows, 1)
		require.Equal(t, &teamA.ID, rows[0].TeamID)
		return rows[0].ID
	}

	t.Run("the check-in moves the device into the other tenant as a new host", func(t *testing.T) {
		info := newDevice("SERIAL-MOVE")
		hostA := enrollInA(shared, info)

		ota(shared, &teamB.ID, info)
		require.Equal(t, 1, intents(info.Serial))
		rows := hosts(info.Serial)
		require.Len(t, rows, 1)
		require.Equal(t, &teamA.ID, rows[0].TeamID, "the OTA request alone does not touch the previous tenant's host")

		authenticate(shared, info)
		rows = hosts(info.Serial)
		require.Len(t, rows, 1)
		require.NotEqual(t, hostA, rows[0].ID)
		require.Equal(t, &teamB.ID, rows[0].TeamID)
		require.Equal(t, 0, intents(info.Serial), "the intent is consumed")

		authenticate(shared, info)
		rows = hosts(info.Serial)
		require.Len(t, rows, 1)
		require.Equal(t, &teamB.ID, rows[0].TeamID, "a later check-in keeps the new host")
	})

	t.Run("enrolling again through the same tenant clears the intent", func(t *testing.T) {
		info := newDevice("SERIAL-BACK")
		hostA := enrollInA(shared, info)

		ota(shared, &teamB.ID, info)
		ota(shared, &teamA.ID, info)
		require.Equal(t, 0, intents(info.Serial))

		authenticate(shared, info)
		rows := hosts(info.Serial)
		require.Len(t, rows, 1)
		require.Equal(t, hostA, rows[0].ID)
		require.Equal(t, &teamA.ID, rows[0].TeamID)
	})

	t.Run("an expired intent is ignored", func(t *testing.T) {
		info := newDevice("SERIAL-EXPIRED")
		hostA := enrollInA(shared, info)

		ota(shared, &teamB.ID, info)
		_, err := ds.writer(base).ExecContext(base,
			`UPDATE openframe_apple_enrollment_intents SET created_at = NOW(6) - INTERVAL 25 HOUR WHERE hardware_serial = ?`, info.Serial)
		require.NoError(t, err)

		authenticate(shared, info)
		rows := hosts(info.Serial)
		require.Len(t, rows, 1)
		require.Equal(t, hostA, rows[0].ID)
		require.Equal(t, &teamA.ID, rows[0].TeamID)
	})

	t.Run("an intent for another UDID is ignored", func(t *testing.T) {
		info := newDevice("SERIAL-UDID")
		hostA := enrollInA(shared, info)

		other := info
		other.UDID = "UDID-SOMEONE-ELSE"
		ota(shared, &teamB.ID, other)
		require.Equal(t, 1, intents(info.Serial))
		var uuid string
		require.NoError(t, sqlx.GetContext(base, ds.reader(base), &uuid, `SELECT uuid FROM hosts WHERE id = ?`, hostA))
		require.Equal(t, info.UDID, uuid, "an OTA request through another tenant does not rewrite the host's UDID")

		authenticate(shared, info)
		rows := hosts(info.Serial)
		require.Len(t, rows, 1)
		require.Equal(t, hostA, rows[0].ID)
		require.Equal(t, &teamA.ID, rows[0].TeamID)
	})

	t.Run("the IdP account follows the tenant the device enrolls into", func(t *testing.T) {
		info := newDevice("SERIAL-IDP")
		userA := idpAccount("user-a@tenant-a.example")
		userB := idpAccount("user-b@tenant-b.example")
		otaWithIdP(shared, &teamA.ID, userA, info)
		authenticate(shared, info)
		require.Equal(t, userA, idpOf(info.UDID))

		otaWithIdP(shared, &teamB.ID, userB, info)
		require.Equal(t, userA, idpOf(info.UDID), "the OTA request alone keeps the previous tenant's IdP link")

		authenticate(shared, info)
		rows := hosts(info.Serial)
		require.Len(t, rows, 1)
		require.Equal(t, &teamB.ID, rows[0].TeamID)
		require.Equal(t, userB, idpOf(info.UDID))
	})

	t.Run("the previous tenant's IdP link is dropped when the new enrollment has none", func(t *testing.T) {
		info := newDevice("SERIAL-IDP-NONE")
		userA := idpAccount("user-a2@tenant-a.example")
		otaWithIdP(shared, &teamA.ID, userA, info)
		authenticate(shared, info)
		require.Equal(t, userA, idpOf(info.UDID))

		ota(shared, &teamB.ID, info)
		authenticate(shared, info)
		require.Empty(t, idpOf(info.UDID))
	})

	t.Run("a device new to Fleet records no intent", func(t *testing.T) {
		info := newDevice("SERIAL-NEW")
		ota(shared, &teamB.ID, info)
		require.Equal(t, 0, intents(info.Serial))
		authenticate(shared, info)
		rows := hosts(info.Serial)
		require.Len(t, rows, 1)
		require.Equal(t, &teamB.ID, rows[0].TeamID)
	})

	t.Run("upstream keeps the device's host and team", func(t *testing.T) {
		info := newDevice("SERIAL-UPSTREAM")
		notShared := context.WithValue(base, openframeSharedModeCtxKey{}, false)
		hostA := enrollInA(notShared, info)

		ota(notShared, &teamB.ID, info)
		require.Equal(t, 0, intents(info.Serial))
		authenticate(notShared, info)
		rows := hosts(info.Serial)
		require.Len(t, rows, 1)
		require.Equal(t, hostA, rows[0].ID)
		require.Equal(t, &teamA.ID, rows[0].TeamID)
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
