// >>> OPENFRAME(windows-mdm-tenant): a Windows MDM enrollment belongs to the tenant it enrolled into — openframe/docs/mysql-multitenancy-feature.md

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	common_mysql "github.com/fleetdm/fleet/v4/server/platform/mysql"
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

// openframeWindowsUUIDHeldByOtherTenant is true when a host of a tenant other than the third argument
// holds the enrollment's host uuid (first argument) for the enrollment's tenant (second argument, NULL
// for an enrollment from before tenants were recorded, which any tenant's host with the uuid holds). An
// enrollment no host of its tenant holds any more (the host was deleted) is released.
const openframeWindowsUUIDHeldByOtherTenant = `EXISTS (SELECT 1 FROM hosts oh
	WHERE oh.uuid = ? AND oh.team_id <=> COALESCE(?, oh.team_id) AND NOT (oh.team_id <=> ?))`

// OpenframeMDMWindowsPrepareEnrollment readies, in one transaction, the enrollment of a Windows device
// into teamID through the host with hostUUID. Windows MDM sends commands and keeps profile statuses by
// the host's hardware uuid and by the device id the device chose, and the hardware id that names the
// enrollment to replace also comes from the device; none of them is unique across tenants. So it first
// refuses (ConflictError), leaving everything as it is, when another host of any tenant has hostUUID,
// another tenant's enrollment still held by its host has it, or the device id is another host's uuid
// or another device's. Then it removes the device's previous enrollment, named by its hardware id,
// when that enrollment is teamID's or released (MDMWindowsDeleteEnrolledDeviceOnReenrollment, with the
// host state cleared only on teamID's host); one another tenant's host still holds refuses it too.
func (ds *Datastore) OpenframeMDMWindowsPrepareEnrollment(ctx context.Context, hostUUID, mdmDeviceHWID, mdmDeviceID string, teamID uint) error {
	const (
		conflictStmt = `SELECT
			(SELECT COUNT(*) FROM hosts WHERE uuid = ?) AS hosts_with_uuid,
			EXISTS (SELECT 1 FROM mdm_windows_enrollments e
				WHERE e.host_uuid = ? AND e.mdm_hardware_id <> ? AND NOT (e.team_id <=> ?)
				AND EXISTS (SELECT 1 FROM hosts oh
					WHERE oh.uuid = e.host_uuid AND oh.team_id <=> COALESCE(e.team_id, oh.team_id) AND NOT (oh.team_id <=> ?))
			) AS uuid_enrolled_elsewhere,
			EXISTS (SELECT 1 FROM hosts WHERE uuid = ? AND uuid <> ?) AS device_id_is_host_uuid,
			EXISTS (SELECT 1 FROM mdm_windows_enrollments WHERE mdm_device_id = ? AND mdm_hardware_id <> ?) AS device_id_enrolled`
		loadStmt        = `SELECT host_uuid, team_id FROM mdm_windows_enrollments WHERE mdm_hardware_id = ? FOR UPDATE`
		heldStmt        = `SELECT ` + openframeWindowsUUIDHeldByOtherTenant
		delActionsStmt  = `DELETE FROM host_mdm_actions WHERE host_id = (SELECT id FROM hosts WHERE uuid = ? AND team_id = ? LIMIT 1)`
		delProfilesStmt = `DELETE FROM host_mdm_windows_profiles WHERE host_uuid = ?
			AND NOT EXISTS (SELECT 1 FROM hosts WHERE uuid = ? AND (team_id IS NULL OR team_id <> ?))`
		delSetupExpStmt = `DELETE ser FROM setup_experience_status_results ser
			JOIN hosts h ON ser.host_uuid = h.osquery_host_id OR ser.host_uuid = h.uuid
			WHERE h.uuid = ? AND h.team_id = ?`
		delUpcomingStmt = `DELETE ua FROM upcoming_activities ua JOIN hosts h ON h.id = ua.host_id WHERE h.uuid = ? AND h.team_id = ?`
		delStmt         = `DELETE FROM mdm_windows_enrollments WHERE mdm_hardware_id = ?`
	)
	if hostUUID == "" {
		return ctxerr.New(ctx, "Windows MDM enrollment has no host")
	}

	return ds.withRetryTxx(ctx, func(tx sqlx.ExtContext) error {
		var conflict struct {
			HostsWithUUID         int  `db:"hosts_with_uuid"`
			UUIDEnrolledElsewhere bool `db:"uuid_enrolled_elsewhere"`
			DeviceIDIsHostUUID    bool `db:"device_id_is_host_uuid"`
			DeviceIDEnrolled      bool `db:"device_id_enrolled"`
		}
		if err := sqlx.GetContext(ctx, tx, &conflict, conflictStmt,
			hostUUID, hostUUID, mdmDeviceHWID, teamID, teamID, mdmDeviceID, hostUUID, mdmDeviceID, mdmDeviceHWID); err != nil {
			return ctxerr.Wrap(ctx, err, "checking the Windows MDM enrollment's identifiers")
		}
		switch {
		case conflict.HostsWithUUID > 1:
			return ctxerr.Wrap(ctx, &fleet.ConflictError{Message: "another host has the Windows device's hardware UUID"})
		case conflict.UUIDEnrolledElsewhere:
			return ctxerr.Wrap(ctx, &fleet.ConflictError{Message: "the Windows device's hardware UUID is enrolled in another tenant"})
		case conflict.DeviceIDIsHostUUID:
			return ctxerr.Wrap(ctx, &fleet.ConflictError{Message: "the Windows device's MDM device id is another host's UUID"})
		case conflict.DeviceIDEnrolled:
			return ctxerr.Wrap(ctx, &fleet.ConflictError{Message: "the Windows device's MDM device id is already enrolled"})
		}

		var row struct {
			HostUUID string        `db:"host_uuid"`
			TeamID   sql.NullInt64 `db:"team_id"`
		}
		switch err := sqlx.GetContext(ctx, tx, &row, loadStmt, mdmDeviceHWID); {
		case errors.Is(err, sql.ErrNoRows):
			return nil
		case err != nil:
			return ctxerr.Wrap(ctx, err, "load MDMWindowsEnrolledDevice for re-enrollment")
		}
		var held bool
		if err := sqlx.GetContext(ctx, tx, &held, heldStmt, row.HostUUID, row.TeamID, teamID); err != nil {
			return ctxerr.Wrap(ctx, err, "check the tenant of the previous Windows MDM enrollment")
		}
		if held {
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

// openframeWindowsEnrollmentFence is the condition, on the enrollment aliased e, that keeps a Windows MDM
// command or profile of a shared-mode tenant on its own device. The keys commands are resolved by
// (host uuid, device id) come from the device and are not unique across tenants. A request pinned to a
// tenant reaches only that tenant's enrollments; an unpinned one (the profile cron, a device request)
// reaches only enrollments whose host uuid no host of another tenant has, so a uuid that two tenants
// share gets no command at all.
func openframeWindowsEnrollmentFence(ctx context.Context) (string, []any) {
	if teamID, ok := fleet.OpenframeTeamID(ctx); ok {
		return ` AND e.team_id = ?`, []any{teamID}
	}
	return ` AND e.team_id IS NOT NULL AND NOT EXISTS (
		SELECT 1 FROM hosts oh WHERE oh.uuid = e.host_uuid AND NOT (oh.team_id <=> e.team_id))`, nil
}

// openframeGetEnrollmentIDsByHostUUIDDB is getEnrollmentIDsByHostUUIDDB for shared mode.
func (ds *Datastore) openframeGetEnrollmentIDsByHostUUIDDB(ctx context.Context, tx sqlx.ExtContext, hostUUIDs []string) ([]uint, error) {
	fence, fenceArgs := openframeWindowsEnrollmentFence(ctx)
	var allIDs []uint
	err := common_mysql.BatchProcessSimple(hostUUIDs, windowsMDMCommandQueueBatchSize, func(batch []string) error {
		stmt, args, err := sqlx.In(
			`SELECT MAX(e.id) FROM mdm_windows_enrollments e WHERE e.host_uuid IN (?)`+fence+` GROUP BY e.host_uuid`, //nolint:gosec // constant fence
			append([]any{batch}, fenceArgs...)...)
		if err != nil {
			return err
		}
		var ids []uint
		if err := sqlx.SelectContext(ctx, tx, &ids, stmt, args...); err != nil {
			return err
		}
		allIDs = append(allIDs, ids...)
		return nil
	})
	return allIDs, err
}

// openframeGetEnrollmentIDsByHostUUIDOrDeviceIDDB is getEnrollmentIDsByHostUUIDOrDeviceIDDB for shared mode:
// within the fence, an enrollment of the host uuid wins over one whose device id merely equals it.
func (ds *Datastore) openframeGetEnrollmentIDsByHostUUIDOrDeviceIDDB(ctx context.Context, tx sqlx.ExtContext, hostUUIDsOrDeviceIDs []string) ([]uint, error) {
	fence, fenceArgs := openframeWindowsEnrollmentFence(ctx)
	stmt := `SELECT e.id FROM mdm_windows_enrollments e WHERE (e.host_uuid = ? OR e.mdm_device_id = ?)` + fence + //nolint:gosec // constant fence
		` ORDER BY e.host_uuid = ? DESC, e.created_at DESC, e.id DESC LIMIT 1`
	var allIDs []uint
	for _, id := range hostUUIDsOrDeviceIDs {
		args := append(append([]any{id, id}, fenceArgs...), id)
		var eid uint
		err := sqlx.GetContext(ctx, tx, &eid, stmt, args...)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, ctxerr.Wrap(ctx, err, "looking up enrollment ID")
		}
		allIDs = append(allIDs, eid)
	}
	return allIDs, nil
}

const openframeWindowsReconcileEnrollmentAnchor = `SELECT 1 FROM mdm_windows_enrollments mwe WHERE mwe.host_uuid = h.uuid`

// openframeWindowsReconcileHostsStmt keeps the Windows profile reconciler, in shared mode, on hosts whose
// enrollment is of their own tenant and whose uuid no host of another tenant has: the profile statuses
// it reads and writes are keyed by host uuid alone.
func openframeWindowsReconcileHostsStmt(ctx context.Context, stmt string) (string, error) {
	if !openframeSharedMode(ctx) {
		return stmt, nil
	}
	if strings.Count(stmt, openframeWindowsReconcileEnrollmentAnchor) != 1 {
		return "", ctxerr.New(ctx, "the Windows reconcile host query changed: re-apply the tenant fence")
	}
	return strings.Replace(stmt, openframeWindowsReconcileEnrollmentAnchor, openframeWindowsReconcileEnrollmentAnchor+
		` AND mwe.team_id <=> h.team_id) AND NOT EXISTS (
				SELECT 1 FROM hosts oh WHERE oh.uuid = h.uuid AND NOT (oh.team_id <=> h.team_id)`, 1), nil
}

// openframeWindowsEnrollmentOfHostTeam is the condition, on the enrollment aliased mwe joined to its host h by
// uuid, that in shared mode keeps only an enrollment of the host's own tenant, and of the request's tenant
// when it is pinned: the answer is keyed by uuid, which hosts of two tenants can share.
func openframeWindowsEnrollmentOfHostTeam(ctx context.Context) string {
	if !openframeSharedMode(ctx) {
		return ""
	}
	if teamID, ok := fleet.OpenframeTeamID(ctx); ok {
		return fmt.Sprintf(` AND mwe.team_id <=> h.team_id AND mwe.team_id = %d`, teamID)
	}
	return ` AND mwe.team_id <=> h.team_id`
}

const openframeWindowsCommandHostJoin = `INNER JOIN hosts h ON h.uuid = mwe.host_uuid`

// openframeWindowsCommandsStmt keeps, in shared mode, the MDM command listing's Windows rows on hosts of
// the enrollment's own tenant, and of the request's tenant when it is pinned: the enrollment is joined to
// its host by uuid, which hosts of two tenants can share, and the listing's team filter does not fence a
// pinned request.
func openframeWindowsCommandsStmt(ctx context.Context, stmt string) (string, error) {
	if !openframeSharedMode(ctx) {
		return stmt, nil
	}
	if strings.Count(stmt, openframeWindowsCommandHostJoin) != 2 {
		return "", ctxerr.New(ctx, "the Windows MDM command listing changed: re-apply the tenant fence")
	}
	fence := ` AND mwe.team_id <=> h.team_id`
	if teamID, ok := fleet.OpenframeTeamID(ctx); ok {
		fence += fmt.Sprintf(` AND mwe.team_id = %d`, teamID)
	}
	return strings.ReplaceAll(stmt, openframeWindowsCommandHostJoin, openframeWindowsCommandHostJoin+fence), nil
}

// <<< OPENFRAME(windows-mdm-tenant)
