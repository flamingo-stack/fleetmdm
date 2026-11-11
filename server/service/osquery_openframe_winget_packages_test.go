// OPENFRAME(winget-packages)
package service

import (
	"context"
	"log/slog"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/service/osquery_utils"
	"github.com/stretchr/testify/require"
)

func TestPreProcessSoftwareResults_wingetPackagesJoinTheWindowsSoftware(t *testing.T) {
	software := hostDetailQueryPrefix + "software_windows"
	details := hostDetailQueryPrefix + "software_windows_details"
	winget := hostDetailQueryPrefix + "software_winget_packages"
	results := fleet.OsqueryDistributedQueryResults{
		software: {{"name": "7-Zip 24.08 (x64)", "version": "24.08", "source": "programs", "vendor": "Igor Pavlov"}},
		details:  {{"name": "7-Zip 24.08 (x64)", "version": "24.08", "vendor": "Igor Pavlov", "arp_key": "7-Zip"}},
		winget:   {{"name": "7-Zip", "version": "24.08", "source": "winget_packages", "vendor": "", "installed_path": ""}},
	}
	statuses := map[string]fleet.OsqueryStatus{software: fleet.StatusOK, details: fleet.StatusOK, winget: fleet.StatusOK}

	preProcessSoftwareResults(context.Background(), &fleet.Host{ID: 1, Platform: "windows"}, results, statuses, nil,
		osquery_utils.SoftwareOverrideQueries, slog.New(slog.DiscardHandler))

	require.Equal(t, []map[string]string{
		{"name": "7-Zip 24.08 (x64)", "version": "24.08", "source": "programs", "vendor": "Igor Pavlov"},
		{"name": "7-Zip", "version": "24.08", "source": "winget_packages", "vendor": "", "installed_path": ""},
	}, results[software])
	require.Equal(t, []map[string]string{
		{"name": "7-Zip 24.08 (x64)", "version": "24.08", "vendor": "Igor Pavlov", "arp_key": "7-Zip"},
	}, results[details])
	require.NotContains(t, results, winget)
}

// an osquery without the table does not run the query; the Windows software is ingested as before
func TestPreProcessSoftwareResults_noWingetTable_softwareUnchanged(t *testing.T) {
	software := hostDetailQueryPrefix + "software_windows"
	results := fleet.OsqueryDistributedQueryResults{
		software: {{"name": "7-Zip 24.08 (x64)", "version": "24.08", "source": "programs", "vendor": "Igor Pavlov"}},
	}
	statuses := map[string]fleet.OsqueryStatus{software: fleet.StatusOK}

	preProcessSoftwareResults(context.Background(), &fleet.Host{ID: 1, Platform: "windows"}, results, statuses, nil,
		osquery_utils.SoftwareOverrideQueries, slog.New(slog.DiscardHandler))

	require.Equal(t, []map[string]string{
		{"name": "7-Zip 24.08 (x64)", "version": "24.08", "source": "programs", "vendor": "Igor Pavlov"},
	}, results[software])
}
