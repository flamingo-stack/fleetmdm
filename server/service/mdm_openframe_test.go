package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mdm/nanomdm/mdm"
	"github.com/fleetdm/fleet/v4/server/mock"
	mdmmock "github.com/fleetdm/fleet/v4/server/mock/mdm"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/stretchr/testify/require"
)

const (
	openframePinnedTeam  = uint(7)
	openframeForeignTeam = uint(8)
)

var (
	openframeOwnHost     = &fleet.Host{ID: 1, UUID: "own-uuid", Platform: "ios", TeamID: new(openframePinnedTeam)}
	openframeForeignHost = &fleet.Host{ID: 2, UUID: "foreign-uuid", Platform: "ios", TeamID: new(openframeForeignTeam)}
)

func openframeHostsByUUID(uuids []string) []*fleet.Host {
	known := map[string]*fleet.Host{openframeOwnHost.UUID: openframeOwnHost, openframeForeignHost.UUID: openframeForeignHost}
	var hosts []*fleet.Host
	for _, u := range uuids {
		if h, ok := known[u]; ok {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

func openframeProfileListCommand() string {
	raw := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Command</key>
	<dict>
		<key>RequestType</key>
		<string>ProfileList</string>
	</dict>
	<key>CommandUUID</key>
	<string>%s</string>
</dict>
</plist>`, "cmd-uuid")
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func TestOpenframeRunMDMCommandPinned(t *testing.T) {
	newSvc := func(t *testing.T) (fleet.Service, context.Context, *[]string, *bool) {
		svc, ctx, ds, opts := setupAppleMDMService(t, &fleet.LicenseInfo{Tier: fleet.TierFree})
		ds.ListHostsLiteByUUIDsFunc = func(ctx context.Context, filter fleet.TeamFilter, uuids []string) ([]*fleet.Host, error) {
			return openframeHostsByUUID(uuids), nil
		}
		ds.AreHostsConnectedToFleetMDMFunc = func(ctx context.Context, hosts []*fleet.Host) (map[string]bool, error) {
			connected := make(map[string]bool, len(hosts))
			for _, h := range hosts {
				connected[h.UUID] = true
			}
			return connected, nil
		}
		ds.AppConfigFunc = func(ctx context.Context) (*fleet.AppConfig, error) {
			return &fleet.AppConfig{MDM: fleet.MDM{EnabledAndConfigured: true}}, nil
		}
		var enqueued []string
		var invoked bool
		storage := opts.MDMStorage.(*mdmmock.MDMAppleStore)
		storage.EnqueueCommandFunc = func(ctx context.Context, ids []string, cmd *mdm.CommandWithSubtype) (map[string]error, error) {
			invoked = true
			enqueued = append(enqueued, ids...)
			return nil, nil
		}
		return svc, test.UserContext(ctx, test.UserAdmin), &enqueued, &invoked
	}

	t.Run("pinned: a foreign host in the request is not enqueued", func(t *testing.T) {
		// setup
		svc, ctx, enqueued, _ := newSvc(t)
		ctx = fleet.NewOpenframeTeamContext(ctx, openframePinnedTeam)

		// execution
		_, err := svc.RunMDMCommand(ctx, openframeProfileListCommand(), []string{openframeOwnHost.UUID, openframeForeignHost.UUID})

		// verifications
		require.NoError(t, err)
		require.Equal(t, []string{openframeOwnHost.UUID}, *enqueued)
	})

	t.Run("pinned: only foreign hosts is not found", func(t *testing.T) {
		// setup
		svc, ctx, _, invoked := newSvc(t)
		ctx = fleet.NewOpenframeTeamContext(ctx, openframePinnedTeam)

		// execution
		_, err := svc.RunMDMCommand(ctx, openframeProfileListCommand(), []string{openframeForeignHost.UUID})

		// verifications
		require.Error(t, err)
		require.Contains(t, err.Error(), "No hosts targeted")
		require.False(t, *invoked)
	})

	t.Run("pinned: deprecated Apple enqueue drops the foreign host", func(t *testing.T) {
		// setup
		svc, ctx, enqueued, _ := newSvc(t)
		ctx = fleet.NewOpenframeTeamContext(ctx, openframePinnedTeam)

		// execution
		_, err := svc.EnqueueMDMAppleCommand(ctx, openframeProfileListCommand(), []string{openframeOwnHost.UUID, openframeForeignHost.UUID})

		// verifications
		require.NoError(t, err)
		require.Equal(t, []string{openframeOwnHost.UUID}, *enqueued)
	})

	t.Run("unpinned: every requested host is enqueued", func(t *testing.T) {
		// setup
		svc, ctx, enqueued, _ := newSvc(t)

		// execution
		_, err := svc.RunMDMCommand(ctx, openframeProfileListCommand(), []string{openframeOwnHost.UUID, openframeForeignHost.UUID})

		// verifications
		require.NoError(t, err)
		require.ElementsMatch(t, []string{openframeOwnHost.UUID, openframeForeignHost.UUID}, *enqueued)
	})
}

func TestOpenframeGetMDMCommandResultsPinned(t *testing.T) {
	newSvc := func(t *testing.T, resultHosts ...*fleet.Host) (fleet.Service, context.Context) {
		ds := new(mock.Store)
		ds.GetMDMCommandPlatformFunc = func(ctx context.Context, commandUUID string) (string, error) {
			return "darwin", nil
		}
		ds.GetMDMAppleCommandRequestTypeFunc = func(ctx context.Context, commandUUID string) (string, error) {
			return "ProfileList", nil
		}
		ds.GetMDMAppleCommandResultsFunc = func(ctx context.Context, commandUUID, hostUUID string) ([]*fleet.MDMCommandResult, error) {
			results := make([]*fleet.MDMCommandResult, 0, len(resultHosts))
			for _, h := range resultHosts {
				results = append(results, &fleet.MDMCommandResult{HostUUID: h.UUID, CommandUUID: commandUUID, Status: "Acknowledged", RequestType: "ProfileList"})
			}
			return results, nil
		}
		ds.ListHostsLiteByUUIDsFunc = func(ctx context.Context, filter fleet.TeamFilter, uuids []string) ([]*fleet.Host, error) {
			return openframeHostsByUUID(uuids), nil
		}
		svc, ctx := newTestService(t, ds, nil, nil)
		return svc, test.UserContext(ctx, test.UserAdmin)
	}
	hostUUIDs := func(results []*fleet.MDMCommandResult) []string {
		out := make([]string, 0, len(results))
		for _, r := range results {
			out = append(out, r.HostUUID)
		}
		return out
	}
	getters := map[string]func(fleet.Service, context.Context) ([]*fleet.MDMCommandResult, error){
		"commands/results": func(svc fleet.Service, ctx context.Context) ([]*fleet.MDMCommandResult, error) {
			return svc.GetMDMCommandResults(ctx, "cmd-uuid", "")
		},
		"mdm/apple/commandresults": func(svc fleet.Service, ctx context.Context) ([]*fleet.MDMCommandResult, error) {
			return svc.GetMDMAppleCommandResults(ctx, "cmd-uuid")
		},
	}

	for name, get := range getters {
		t.Run(name+" pinned: only the team's results", func(t *testing.T) {
			// setup
			svc, ctx := newSvc(t, openframeOwnHost, openframeForeignHost)
			ctx = fleet.NewOpenframeTeamContext(ctx, openframePinnedTeam)

			// execution
			results, err := get(svc, ctx)

			// verifications
			require.NoError(t, err)
			require.Equal(t, []string{openframeOwnHost.UUID}, hostUUIDs(results))
		})

		t.Run(name+" pinned: a command of another tenant is not found", func(t *testing.T) {
			// setup
			svc, ctx := newSvc(t, openframeForeignHost)
			ctx = fleet.NewOpenframeTeamContext(ctx, openframePinnedTeam)

			// execution
			_, err := get(svc, ctx)

			// verifications
			require.True(t, fleet.IsNotFound(err))
		})

		t.Run(name+" unpinned: every result", func(t *testing.T) {
			// setup
			svc, ctx := newSvc(t, openframeOwnHost, openframeForeignHost)

			// execution
			results, err := get(svc, ctx)

			// verifications
			require.NoError(t, err)
			require.ElementsMatch(t, []string{openframeOwnHost.UUID, openframeForeignHost.UUID}, hostUUIDs(results))
		})
	}
}
