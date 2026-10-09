package mysql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fleetdm/fleet/v4/server/activity/api"
	"github.com/fleetdm/fleet/v4/server/activity/internal/types"
	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	// >>> OPENFRAME(mysql-multitenancy): team pin helpers for CDC team_id stamping
	"github.com/fleetdm/fleet/v4/server/fleet"
	// <<< OPENFRAME(mysql-multitenancy)
	platform_mysql "github.com/fleetdm/fleet/v4/server/platform/mysql"
	"github.com/jmoiron/sqlx"
)

// NewActivity stores an activity record in the database.
// The webhook context key must be set in the context before calling this method.
func (ds *Datastore) NewActivity(
	ctx context.Context, user *api.User, activity api.ActivityDetails, details []byte, createdAt time.Time,
) error {
	ctx, span := tracer.Start(ctx, "activity.mysql.NewActivity")
	defer span.End()

	// Sanity check to ensure we processed activity webhook before storing the activity
	processed, _ := ctx.Value(types.ActivityWebhookContextKey).(bool)
	if !processed {
		return ctxerr.New(
			ctx, "activity webhook not processed. Please use svc.NewActivity instead of ds.NewActivity. This is a Fleet server bug.",
		)
	}

	var userID *uint
	var userName *string
	var userEmail *string
	var fleetInitiated bool
	var hostOnly bool

	if user != nil {
		// To support creating activities with users that were deleted. This can happen
		// for automatically installed software which uses the author of the upload as the author of
		// the installation.
		if user.ID != 0 && !user.Deleted {
			userID = &user.ID
		}
		userName = &user.Name
		userEmail = &user.Email
	}

	if automatableActivity, ok := activity.(types.AutomatableActivity); ok && automatableActivity.WasFromAutomation() {
		automationAuthor := types.ActivityAutomationAuthor
		userName = &automationAuthor
		fleetInitiated = true
	}

	if hostOnlyActivity, ok := activity.(types.ActivityHostOnly); ok && hostOnlyActivity.HostOnly() {
		hostOnly = true
	}

	cols := []string{"fleet_initiated", "user_id", "user_name", "activity_type", "details", "created_at", "host_only"}
	args := []any{
		fleetInitiated,
		userID,
		userName,
		activity.ActivityName(),
		details,
		createdAt,
		hostOnly,
	}
	// For system/automated activities (user == nil), user_email defaults to empty (not null).
	if userEmail != nil {
		args = append(args, userEmail)
		cols = append(cols, "user_email")
	}

	// >>> OPENFRAME(mysql-multitenancy): stamp the tenant team onto CDC-captured rows so the
	// shared-DB Debezium pipeline can resolve the tenant per event — openframe/docs/mysql-multitenancy-feature.md.
	// activity_past has no host reference, so the request's team pin is the only tenant signal
	// (user/team-level activities). Unpinned contexts (flag off, or background crons) keep the
	// original statement byte-identical and leave team_id NULL.
	teamID, teamPinned := fleet.OpenframeTeamID(ctx)
	if teamPinned {
		cols = append(cols, "team_id")
		args = append(args, teamID)
	}
	// <<< OPENFRAME(mysql-multitenancy)

	return platform_mysql.WithRetryTxx(ctx, ds.primary, func(tx sqlx.ExtContext) error {
		const insertActStmt = `INSERT INTO activity_past (%s) VALUES (%s)`
		sqlStmt := fmt.Sprintf(insertActStmt, strings.Join(cols, ","), strings.Repeat("?,", len(cols)-1)+"?")
		res, err := tx.ExecContext(ctx, sqlStmt, args...)
		if err != nil {
			return ctxerr.Wrap(ctx, err, "new activity")
		}

		// Insert into activity_host_past table if the activity is associated with hosts.
		// This supposes a reasonable amount of hosts per activity, to revisit if we
		// get in the 10K+.
		if ah, ok := activity.(types.ActivityHosts); ok {
			// >>> OPENFRAME(mysql-multitenancy): stamp each row with its host's team via a scalar
			// subselect (host activities can be written from unpinned cron contexts, and the host is
			// the authoritative tenant signal anyway) — engaged when the multitenancy flag is on,
			// or when the caller context is team-pinned. Flag off keeps the statement byte-identical.
			stampTeam := teamPinned || fleet.IsOpenframeMultitenancy()
			insertActHostStmt := `INSERT INTO activity_host_past (host_id, activity_id) VALUES `
			if stampTeam {
				insertActHostStmt = `INSERT INTO activity_host_past (host_id, activity_id, team_id) VALUES `
			}
			// <<< OPENFRAME(mysql-multitenancy)

			var sb strings.Builder
			if hostIDs := ah.HostIDs(); len(hostIDs) > 0 {
				sb.WriteString(insertActHostStmt)
				actID, _ := res.LastInsertId()
				for _, hid := range hostIDs {
					// >>> OPENFRAME(mysql-multitenancy)
					if stampTeam {
						sb.WriteString(fmt.Sprintf("(%d, %d, (SELECT team_id FROM hosts WHERE id = %d)),", hid, actID, hid))
						continue
					}
					// <<< OPENFRAME(mysql-multitenancy)
					sb.WriteString(fmt.Sprintf("(%d, %d),", hid, actID))
				}

				stmt := strings.TrimSuffix(sb.String(), ",")
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return ctxerr.Wrap(ctx, err, "insert host activity")
				}
			}
		}
		return nil
	}, ds.logger)
}
