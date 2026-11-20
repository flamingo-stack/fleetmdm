// >>> OPENFRAME(mysql-multitenancy): MDM command endpoints scoped to the pinned team — openframe/docs/mysql-multitenancy-feature.md

package service

import (
	"context"

	"github.com/fleetdm/fleet/v4/server/fleet"
)

func openframeKeepPinnedTeamHosts(ctx context.Context, hosts []*fleet.Host) []*fleet.Host {
	teamID, ok := fleet.OpenframeTeamID(ctx)
	if !ok {
		return hosts
	}
	kept := make([]*fleet.Host, 0, len(hosts))
	for _, h := range hosts {
		if h.TeamID != nil && *h.TeamID == teamID {
			kept = append(kept, h)
		}
	}
	return kept
}

func openframePinnedHostUUIDs(ctx context.Context, requested []string, hosts []*fleet.Host) []string {
	if _, ok := fleet.OpenframeTeamID(ctx); !ok {
		return requested
	}
	uuids := make([]string, 0, len(hosts))
	for _, h := range hosts {
		uuids = append(uuids, h.UUID)
	}
	return uuids
}

func openframeKeepPinnedTeamCommandResults(
	ctx context.Context,
	results []*fleet.MDMCommandResult,
	pinnedHosts []*fleet.Host,
) ([]*fleet.MDMCommandResult, error) {
	if _, ok := fleet.OpenframeTeamID(ctx); !ok {
		return results, nil
	}
	own := make(map[string]struct{}, len(pinnedHosts))
	for _, h := range pinnedHosts {
		own[h.UUID] = struct{}{}
	}
	kept := make([]*fleet.MDMCommandResult, 0, len(results))
	for _, r := range results {
		if _, ok := own[r.HostUUID]; ok {
			kept = append(kept, r)
		}
	}
	if len(kept) == 0 && len(results) > 0 {
		return nil, newNotFoundError()
	}
	return kept, nil
}

// <<< OPENFRAME(mysql-multitenancy)
