// >>> OPENFRAME(windows-mdm-tenant): a Windows MDM enrollment belongs to the tenant it enrolled into — openframe/docs/mysql-multitenancy-feature.md

package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/jmoiron/sqlx"
)

// OpenframeMDMWindowsEnrollmentTeamID returns the tenant team the enrollment enrolled into, or nil when it
// has none recorded (enrolled before the column existed, or outside shared mode).
func (ds *Datastore) OpenframeMDMWindowsEnrollmentTeamID(ctx context.Context, enrollmentID uint) (*uint, error) {
	var teamID sql.NullInt64
	err := sqlx.GetContext(ctx, ds.reader(ctx), &teamID,
		`SELECT team_id FROM mdm_windows_enrollments WHERE id = ?`, enrollmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ctxerr.Wrap(ctx, notFound("MDMWindowsEnrolledDevice").WithID(enrollmentID))
	}
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "selecting the tenant of a Windows MDM enrollment")
	}
	if !teamID.Valid || teamID.Int64 <= 0 {
		return nil, nil
	}
	return new(uint(teamID.Int64)), nil
}

// OpenframeMDMWindowsDeleteEnrolledDeviceOnReenrollment is MDMWindowsDeleteEnrolledDeviceOnReenrollment for
// a device re-enrolling into teamID. The hardware id comes from the device, so the enrollment it names is
// removed only when it belongs to teamID: its team_id, or for a row without one, a host of teamID with
// the row's host uuid. An enrollment of another tenant is left as it is and the error is a
// ConflictError. The host state cleared with it is that of teamID's host only.
func (ds *Datastore) OpenframeMDMWindowsDeleteEnrolledDeviceOnReenrollment(ctx context.Context, mdmDeviceHWID string, teamID uint) error {
	const (
		loadStmt        = `SELECT host_uuid, team_id FROM mdm_windows_enrollments WHERE mdm_hardware_id = ? FOR UPDATE`
		hostOfTeamStmt  = `SELECT EXISTS (SELECT 1 FROM hosts WHERE uuid = ? AND team_id = ?)`
		delActionsStmt  = `DELETE FROM host_mdm_actions WHERE host_id = (SELECT id FROM hosts WHERE uuid = ? AND team_id = ? LIMIT 1)`
		delProfilesStmt = `DELETE FROM host_mdm_windows_profiles WHERE host_uuid = ?
			AND NOT EXISTS (SELECT 1 FROM hosts WHERE uuid = ? AND (team_id IS NULL OR team_id <> ?))`
		delSetupExpStmt = `DELETE ser FROM setup_experience_status_results ser
			JOIN hosts h ON ser.host_uuid = h.osquery_host_id OR ser.host_uuid = h.uuid
			WHERE h.uuid = ? AND h.team_id = ?`
		delUpcomingStmt = `DELETE ua FROM upcoming_activities ua JOIN hosts h ON h.id = ua.host_id WHERE h.uuid = ? AND h.team_id = ?`
		delStmt         = `DELETE FROM mdm_windows_enrollments WHERE mdm_hardware_id = ?`
	)

	return ds.withRetryTxx(ctx, func(tx sqlx.ExtContext) error {
		var row struct {
			HostUUID string        `db:"host_uuid"`
			TeamID   sql.NullInt64 `db:"team_id"`
		}
		switch err := sqlx.GetContext(ctx, tx, &row, loadStmt, mdmDeviceHWID); {
		case errors.Is(err, sql.ErrNoRows):
			return ctxerr.Wrap(ctx, notFound("MDMWindowsEnrolledDevice"))
		case err != nil:
			return ctxerr.Wrap(ctx, err, "load MDMWindowsEnrolledDevice for re-enrollment")
		}

		owned := row.TeamID.Valid && row.TeamID.Int64 == int64(teamID)
		if !row.TeamID.Valid && row.HostUUID != "" {
			if err := sqlx.GetContext(ctx, tx, &owned, hostOfTeamStmt, row.HostUUID, teamID); err != nil {
				return ctxerr.Wrap(ctx, err, "check the host of a Windows MDM enrollment without a tenant")
			}
		}
		if !owned {
			return ctxerr.Wrap(ctx, &fleet.ConflictError{Message: "the Windows device is enrolled in another tenant"})
		}

		if row.HostUUID != "" {
			if _, err := tx.ExecContext(ctx, delActionsStmt, row.HostUUID, teamID); err != nil {
				return ctxerr.Wrap(ctx, err, "delete host_mdm_actions for host")
			}
			if _, err := tx.ExecContext(ctx, delProfilesStmt, row.HostUUID, row.HostUUID, teamID); err != nil {
				return ctxerr.Wrap(ctx, err, "delete host_mdm_windows_profiles for host")
			}
			if _, err := tx.ExecContext(ctx, delSetupExpStmt, row.HostUUID, teamID); err != nil {
				return ctxerr.Wrap(ctx, err, "delete setup_experience_status_results for host")
			}
			if _, err := tx.ExecContext(ctx, delUpcomingStmt, row.HostUUID, teamID); err != nil {
				return ctxerr.Wrap(ctx, err, "delete upcoming_activities for host")
			}
		}

		if _, err := tx.ExecContext(ctx, delStmt, mdmDeviceHWID); err != nil {
			return ctxerr.Wrap(ctx, err, "delete MDMWindowsEnrolledDevice")
		}
		return nil
	})
}

// OpenframeUpdateMDMWindowsEnrollmentHost is UpdateMDMWindowsEnrollmentsHostUUID for shared mode: the
// device id comes from the device, so the enrollment is linked to the host only when both belong to the
// same tenant. The host is named by id, which, unlike its uuid, no host of another tenant shares.
func (ds *Datastore) OpenframeUpdateMDMWindowsEnrollmentHost(ctx context.Context, hostID uint, mdmDeviceID string) (bool, error) {
	const stmt = `UPDATE mdm_windows_enrollments e
		JOIN hosts h ON h.id = ?
		SET e.host_uuid = h.uuid
		WHERE e.mdm_device_id = ? AND e.host_uuid <> h.uuid AND e.team_id = h.team_id`
	res, err := ds.writer(ctx).ExecContext(ctx, stmt, hostID, mdmDeviceID)
	if err != nil {
		return false, ctxerr.Wrap(ctx, err, "setting host_uuid for windows enrollment of the host's tenant")
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return false, ctxerr.Wrap(ctx, err, "checking rows affected when setting host_uuid for windows enrollment")
	}
	return aff > 0, nil
}

// <<< OPENFRAME(windows-mdm-tenant)
