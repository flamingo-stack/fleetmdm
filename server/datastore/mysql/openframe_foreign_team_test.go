// OPENFRAME(mysql-multitenancy): unit test for the explicit-team rejection
// helper — openframe/docs/mysql-multitenancy-feature.md
//
// openframeForeignTeam backs every "caller passed an explicit fleet_id for
// another tenant" fence; it is pure context logic, so it runs without
// MYSQL_TEST and catches a sync that breaks the pin plumbing.
package mysql

import (
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/require"
)

func TestOpenframeForeignTeam(t *testing.T) {
	ctx := t.Context()

	// Unpinned process: no tenant scope, so no team is foreign (upstream
	// behavior must stay unchanged). This assumes no earlier test in this
	// package pinned the process via fleet.SetOpenframeTeamID or the
	// FLEET_OPENFRAME_* env vars — the pin and the env decision are cached
	// process-globals.
	require.False(t, openframeForeignTeam(ctx, 1))
	require.False(t, openframeForeignTeam(ctx, 0))

	pinned := fleet.NewOpenframeTeamContext(ctx, 7)
	require.False(t, openframeForeignTeam(pinned, 7), "own team is never foreign")
	require.True(t, openframeForeignTeam(pinned, 8), "another tenant's team must be foreign")
	require.True(t, openframeForeignTeam(pinned, 0))
}
