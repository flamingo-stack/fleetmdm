// OPENFRAME(homebrew-casks)
package service

import (
	"context"
	"log/slog"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/service/osquery_utils"
	"github.com/stretchr/testify/require"
)

func TestPreProcessSoftwareResults_homebrewCasksJoinTheMacSoftware(t *testing.T) {
	software := hostDetailQueryPrefix + "software_macos"
	casks := hostDetailQueryPrefix + "software_homebrew_casks"
	results := fleet.OsqueryDistributedQueryResults{
		software: {{"name": "Slack.app", "version": "4.41.105", "source": "apps", "bundle_identifier": "com.tinyspeck.slackmacgap"}},
		casks:    {{"name": "slack", "version": "4.41.105", "source": "homebrew_casks", "installed_path": "/opt/homebrew/Caskroom/slack/4.41.105"}},
	}
	statuses := map[string]fleet.OsqueryStatus{software: fleet.StatusOK, casks: fleet.StatusOK}

	preProcessSoftwareResults(context.Background(), &fleet.Host{ID: 1, Platform: "darwin"}, results, statuses, nil,
		osquery_utils.SoftwareOverrideQueries, slog.New(slog.DiscardHandler))

	require.Equal(t, []map[string]string{
		{"name": "Slack.app", "version": "4.41.105", "source": "apps", "bundle_identifier": "com.tinyspeck.slackmacgap"},
		{"name": "slack", "version": "4.41.105", "source": "homebrew_casks", "installed_path": "/opt/homebrew/Caskroom/slack/4.41.105"},
	}, results[software])
	require.NotContains(t, results, casks)
}

// a failed cask query must not cost the host its software: the Mac software is ingested as before
func TestPreProcessSoftwareResults_homebrewCasksQueryFailed_softwareUnchanged(t *testing.T) {
	software := hostDetailQueryPrefix + "software_macos"
	casks := hostDetailQueryPrefix + "software_homebrew_casks"
	results := fleet.OsqueryDistributedQueryResults{
		software: {{"name": "Slack.app", "version": "4.41.105", "source": "apps", "bundle_identifier": "com.tinyspeck.slackmacgap"}},
	}
	statuses := map[string]fleet.OsqueryStatus{software: fleet.StatusOK, casks: fleet.OsqueryStatus(1)}

	preProcessSoftwareResults(context.Background(), &fleet.Host{ID: 1, Platform: "darwin"}, results, statuses, nil,
		osquery_utils.SoftwareOverrideQueries, slog.New(slog.DiscardHandler))

	require.Equal(t, []map[string]string{
		{"name": "Slack.app", "version": "4.41.105", "source": "apps", "bundle_identifier": "com.tinyspeck.slackmacgap"},
	}, results[software])
}
