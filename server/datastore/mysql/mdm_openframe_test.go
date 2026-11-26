package mysql

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOpenframeMDMCommandFences(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := t.Context()

	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "tenant-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "tenant-b"})
	require.NoError(t, err)

	newHost := func(name string, team *fleet.Team) *fleet.Host {
		h, err := ds.NewHost(ctx, &fleet.Host{
			Hostname:       name,
			OsqueryHostID:  new("osq-" + name),
			NodeKey:        new("nk-" + name),
			UUID:           uuid.NewString(),
			Platform:       "ios",
			HardwareSerial: "SERIAL-" + name,
			TeamID:         &team.ID,
		})
		require.NoError(t, err)
		nanoEnroll(t, ds, h, false)
		return h
	}
	hostA := newHost("iphone-a", teamA)
	hostB := newHost("iphone-b", teamB)

	commander, _ := createMDMAppleCommanderAndStorage(t, ds)
	enqueue := func(reqType string, hosts ...*fleet.Host) string {
		cmdUUID := uuid.NewString()
		ids := make([]string, 0, len(hosts))
		for _, h := range hosts {
			ids = append(ids, h.UUID)
		}
		require.NoError(t, commander.EnqueueCommand(ctx, ids, createRawAppleCmd(reqType, cmdUUID)))
		return cmdUUID
	}
	cmdA := enqueue("ProfileList", hostA)
	cmdB := enqueue("ProfileList", hostB)
	cmdShared := enqueue("InstalledApplicationList", hostA, hostB)

	ctxA := fleet.NewOpenframeTeamContext(ctx, teamA.ID)
	admin := fleet.TeamFilter{User: test.UserAdmin}
	list := func(c context.Context, filter fleet.TeamFilter, filters fleet.MDMCommandFilters) []*fleet.MDMCommand {
		cmds, _, _, err := ds.ListMDMCommands(c, filter, &fleet.MDMCommandListOptions{
			ListOptions: fleet.ListOptions{PerPage: 100},
			Filters:     filters,
		})
		require.NoError(t, err)
		return cmds
	}
	pairs := func(cmds []*fleet.MDMCommand) [][2]string {
		out := make([][2]string, 0, len(cmds))
		for _, c := range cmds {
			out = append(out, [2]string{c.CommandUUID, c.HostUUID})
		}
		return out
	}

	t.Run("pinned list returns only the team's hosts", func(t *testing.T) {
		// execution
		cmds := list(ctxA, admin, fleet.MDMCommandFilters{})

		// verifications
		require.ElementsMatch(t, [][2]string{{cmdA, hostA.UUID}, {cmdShared, hostA.UUID}}, pairs(cmds))
	})

	t.Run("pinned list with a request type filter", func(t *testing.T) {
		// execution
		cmds := list(ctxA, admin, fleet.MDMCommandFilters{RequestType: "ProfileList"})

		// verifications
		require.ElementsMatch(t, [][2]string{{cmdA, hostA.UUID}}, pairs(cmds))
	})

	t.Run("pinned list ignores an explicit foreign team", func(t *testing.T) {
		// execution
		cmds := list(ctxA, fleet.TeamFilter{User: test.UserAdmin, TeamID: &teamB.ID}, fleet.MDMCommandFilters{})

		// verifications
		require.Empty(t, cmds)
	})

	t.Run("pinned list by a foreign host identifier is empty", func(t *testing.T) {
		// execution
		byUUID := list(ctxA, admin, fleet.MDMCommandFilters{HostIdentifier: hostB.UUID})
		bySerial := list(ctxA, admin, fleet.MDMCommandFilters{HostIdentifier: hostB.HardwareSerial})

		// verifications
		require.Empty(t, byUUID)
		require.Empty(t, bySerial)
	})

	t.Run("pinned list by an own host identifier", func(t *testing.T) {
		// execution
		cmds := list(ctxA, admin, fleet.MDMCommandFilters{HostIdentifier: hostA.HardwareSerial})

		// verifications
		require.ElementsMatch(t, [][2]string{{cmdA, hostA.UUID}, {cmdShared, hostA.UUID}}, pairs(cmds))
	})

	t.Run("unpinned list is unchanged", func(t *testing.T) {
		// execution
		cmds := list(ctx, admin, fleet.MDMCommandFilters{})

		// verifications
		require.ElementsMatch(t, [][2]string{
			{cmdA, hostA.UUID}, {cmdB, hostB.UUID}, {cmdShared, hostA.UUID}, {cmdShared, hostB.UUID},
		}, pairs(cmds))
	})

	t.Run("pinned host identifiers: a foreign host is not found", func(t *testing.T) {
		// execution
		own, ownErr := ds.GetHostMDMIdentifiers(ctxA, hostA.UUID, admin)
		_, foreignErr := ds.GetHostMDMIdentifiers(ctxA, hostB.UUID, admin)
		unpinned, unpinnedErr := ds.GetHostMDMIdentifiers(ctx, hostB.UUID, admin)

		// verifications
		require.NoError(t, ownErr)
		require.Len(t, own, 1)
		require.Equal(t, hostA.UUID, own[0].UUID)
		require.True(t, fleet.IsNotFound(foreignErr))
		require.NoError(t, unpinnedErr)
		require.Len(t, unpinned, 1)
		require.Equal(t, hostB.UUID, unpinned[0].UUID)
	})
}
