// OPENFRAME(windows-mdm-tenant)
package mysql

import (
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestOpenframeHostTeamIDsByUUID(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := t.Context()
	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-uuid-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-uuid-b"})
	require.NoError(t, err)

	host := func(name, uuid string, teamID *uint) {
		h := test.NewHost(t, ds, name, "", name+"-key", name+"-osq", time.Now())
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			_, err := q.ExecContext(ctx, `UPDATE hosts SET uuid = ?, team_id = ? WHERE id = ?`, uuid, teamID, h.ID)
			return err
		})
	}
	host("only-a", "uuid-only-a", &teamA.ID)
	host("twice-a-1", "uuid-twice-a", &teamA.ID)
	host("twice-a-2", "uuid-twice-a", &teamA.ID)
	host("shared-a", "uuid-shared", &teamA.ID)
	host("shared-b", "uuid-shared", &teamB.ID)
	host("no-team", "uuid-no-team", nil)

	for uuid, want := range map[string][]uint{
		"uuid-only-a":  {teamA.ID},
		"uuid-twice-a": {teamA.ID},
		"uuid-shared":  {teamA.ID, teamB.ID},
		"uuid-no-team": {0},
		"uuid-gone":    nil,
	} {
		got, err := ds.OpenframeHostTeamIDsByUUID(ctx, uuid)
		require.NoError(t, err, uuid)
		require.ElementsMatch(t, want, got, uuid)
	}
}
