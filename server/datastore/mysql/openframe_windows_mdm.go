// >>> OPENFRAME(windows-mdm-tenant): the tenant of a Windows MDM device's host — openframe/docs/mysql-multitenancy-feature.md

package mysql

import (
	"context"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/jmoiron/sqlx"
)

// OpenframeHostTeamIDsByUUID returns the distinct team ids of the hosts with this exact UUID, 0 for a host
// without a team. One id is the device's tenant; several mean hosts of different tenants share the UUID.
func (ds *Datastore) OpenframeHostTeamIDsByUUID(ctx context.Context, hostUUID string) ([]uint, error) {
	var teamIDs []uint
	if err := sqlx.SelectContext(ctx, ds.reader(ctx), &teamIDs,
		`SELECT DISTINCT COALESCE(team_id, 0) FROM hosts WHERE uuid = ?`, hostUUID); err != nil {
		return nil, ctxerr.Wrap(ctx, err, "selecting the teams of the hosts with a uuid")
	}
	return teamIDs, nil
}

// <<< OPENFRAME(windows-mdm-tenant)
