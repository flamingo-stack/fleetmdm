package mysql

import (
	"context"
	"log/slog"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/jmoiron/sqlx"
)

func openframeTeamsDiffer(a, b *uint) bool {
	if a == nil || b == nil {
		return a != b
	}
	return *a != *b
}

func openframeRefuseAppleOTAForOtherTeam(ctx context.Context, tx sqlx.QueryerContext, logger *slog.Logger, serial string, teamID *uint) error {
	if serial == "" {
		return nil
	}
	var hosts []struct {
		ID     uint  `db:"id"`
		TeamID *uint `db:"team_id"`
	}
	if err := sqlx.SelectContext(ctx, tx, &hosts,
		`SELECT id, team_id FROM hosts WHERE hardware_serial = ? AND platform IN ('darwin', 'ios', 'ipados')`, serial); err != nil {
		return ctxerr.Wrap(ctx, err, "select apple hosts for ota enrollment")
	}
	for _, h := range hosts {
		if openframeTeamsDiffer(h.TeamID, teamID) {
			logger.InfoContext(ctx, "refusing ota enrollment: the device's host belongs to another team",
				"host_id", h.ID, "host_team_id", fleet.OpenframeTeamLogValue(h.TeamID), "enroll_team_id", fleet.OpenframeTeamLogValue(teamID))
			return ctxerr.Wrap(ctx, fleet.ConflictError{Message: "This device is enrolled in another organization. Remove it there before enrolling it here."})
		}
	}
	return nil
}
