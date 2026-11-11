package mysql

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/jmoiron/sqlx"
)

const openframeAppleEnrollmentIntentTTLHours = 24

type openframeAppleEnrollmentIntent struct {
	TeamID  *uint  `db:"team_id"`
	IdPUUID string `db:"idp_uuid"`
}

func openframeTeamsDiffer(a, b *uint) bool {
	if a == nil || b == nil {
		return a != b
	}
	return *a != *b
}

func openframeRecordAppleEnrollmentIntent(ctx context.Context, tx sqlx.ExtContext, serial, uuid string, teamID *uint, idpUUID string) (bool, error) {
	if serial == "" || uuid == "" {
		return false, nil
	}
	var teams []*uint
	if err := sqlx.SelectContext(ctx, tx, &teams,
		`SELECT team_id FROM hosts WHERE hardware_serial = ? AND platform IN ('darwin', 'ios', 'ipados')`, serial); err != nil {
		return false, ctxerr.Wrap(ctx, err, "select apple hosts for enrollment intent")
	}
	for _, hostTeam := range teams {
		if openframeTeamsDiffer(hostTeam, teamID) {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO openframe_apple_enrollment_intents (hardware_serial, uuid, team_id, idp_uuid, created_at)
				VALUES (?, ?, ?, ?, NOW(6))
				ON DUPLICATE KEY UPDATE uuid = VALUES(uuid), team_id = VALUES(team_id), idp_uuid = VALUES(idp_uuid), created_at = VALUES(created_at)`,
				serial, uuid, teamID, idpUUID)
			if err != nil {
				return false, ctxerr.Wrap(ctx, err, "upsert apple enrollment intent")
			}
			return true, nil
		}
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM openframe_apple_enrollment_intents WHERE hardware_serial = ?`, serial)
	return false, ctxerr.Wrap(ctx, err, "clear apple enrollment intent")
}

func openframeTakeAppleEnrollmentIntent(ctx context.Context, tx sqlx.ExtContext, serial, uuid string) (openframeAppleEnrollmentIntent, bool, error) {
	var intent openframeAppleEnrollmentIntent
	err := sqlx.GetContext(ctx, tx, &intent, `
		SELECT team_id, idp_uuid FROM openframe_apple_enrollment_intents
		WHERE hardware_serial = ? AND uuid = ? AND created_at > NOW(6) - INTERVAL ? HOUR
		FOR UPDATE`,
		serial, uuid, openframeAppleEnrollmentIntentTTLHours)
	if errors.Is(err, sql.ErrNoRows) {
		return intent, false, nil
	}
	if err != nil {
		return intent, false, ctxerr.Wrap(ctx, err, "select apple enrollment intent")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM openframe_apple_enrollment_intents WHERE hardware_serial = ?`, serial); err != nil {
		return intent, false, ctxerr.Wrap(ctx, err, "consume apple enrollment intent")
	}
	return intent, true, nil
}

func openframeApplyAppleEnrollmentIntent(ctx context.Context, tx sqlx.ExtContext, hostID uint, serial, uuid string) (openframeAppleEnrollmentIntent, bool, error) {
	intent, found, err := openframeTakeAppleEnrollmentIntent(ctx, tx, serial, uuid)
	if err != nil || !found {
		return intent, false, err
	}
	var hostTeam *uint
	if err := sqlx.GetContext(ctx, tx, &hostTeam, `SELECT team_id FROM hosts WHERE id = ?`, hostID); err != nil {
		return intent, false, ctxerr.Wrap(ctx, err, "select apple host team for enrollment intent")
	}
	if !openframeTeamsDiffer(hostTeam, intent.TeamID) {
		return intent, false, nil
	}
	if err := deleteHosts(fleet.NewOpenframeHostTenantChangeContext(ctx), tx, []uint{hostID}); err != nil {
		return intent, false, ctxerr.Wrap(ctx, err, "delete the previous team's apple host")
	}
	return intent, true, nil
}

func openframeAssociateAppleEnrollmentIdP(ctx context.Context, tx sqlx.ExtContext, logger *slog.Logger, host *fleet.Host, idpUUID string) error {
	if idpUUID == "" {
		return nil
	}
	if err := associateHostMDMIdPAccountDB(ctx, tx, host.UUID, idpUUID); err != nil {
		return err
	}
	var account fleet.MDMIdPAccount
	if err := sqlx.GetContext(ctx, tx, &account,
		`SELECT uuid, username, fullname, email FROM mdm_idp_accounts WHERE uuid = ?`, idpUUID); err != nil {
		logger.ErrorContext(ctx, "failed to load mdm idp account for scim association",
			"host_uuid", host.UUID, "idp_uuid", idpUUID, "error", err)
		return nil
	}
	if err := maybeAssociateHostMDMIdPWithScimUser(ctx, tx, logger, host.ID, &account); err != nil {
		logger.ErrorContext(ctx, "failed to associate scim user with re-enrolled host",
			"host_uuid", host.UUID, "host_id", host.ID, "idp_uuid", idpUUID, "error", err)
	}
	return nil
}
