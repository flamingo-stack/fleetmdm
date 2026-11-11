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
		winget:   {{"name": "7-Zip", "version": "24.08", "source": "winget_packages", "vendor": "", "installed_path": "", "winget_id": "7zip.7zip"}},
	}
	statuses := map[string]fleet.OsqueryStatus{software: fleet.StatusOK, details: fleet.StatusOK, winget: fleet.StatusOK}

	preProcessSoftwareResults(context.Background(), &fleet.Host{ID: 1, Platform: "windows"}, results, statuses, nil,
		osquery_utils.SoftwareOverrideQueries, slog.New(slog.DiscardHandler))

	require.Equal(t, []map[string]string{
		{"name": "7-Zip 24.08 (x64)", "version": "24.08", "source": "programs", "vendor": "Igor Pavlov"},
		{"name": "7-Zip", "version": "24.08", "source": "winget_packages", "vendor": "", "installed_path": "", "winget_id": "7zip.7zip"},
	}, results[software])
	require.Equal(t, []map[string]string{
		{"name": "7-Zip 24.08 (x64)", "version": "24.08", "vendor": "Igor Pavlov", "arp_key": "7-Zip"},
		{"name": "7-Zip", "version": "24.08", "vendor": "", "source": "winget_packages", "winget_id": "7zip.7zip"},
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

// a failed details query must not turn into details holding only the winget rows: the replace would drop every program
func TestPreProcessSoftwareResults_detailsQueryFailed_noWingetDetails(t *testing.T) {
	software := hostDetailQueryPrefix + "software_windows"
	details := hostDetailQueryPrefix + "software_windows_details"
	winget := hostDetailQueryPrefix + "software_winget_packages"
	results := fleet.OsqueryDistributedQueryResults{
		software: {{"name": "7-Zip 24.08 (x64)", "version": "24.08", "source": "programs", "vendor": "Igor Pavlov"}},
		winget:   {{"name": "7-Zip", "version": "24.08", "source": "winget_packages", "vendor": "", "winget_id": "7zip.7zip"}},
	}
	statuses := map[string]fleet.OsqueryStatus{software: fleet.StatusOK, details: fleet.OsqueryStatus(1), winget: fleet.StatusOK}

	preProcessSoftwareResults(context.Background(), &fleet.Host{ID: 1, Platform: "windows"}, results, statuses, nil,
		osquery_utils.SoftwareOverrideQueries, slog.New(slog.DiscardHandler))

	require.NotContains(t, results, details)
}

// the Acrobat override filters the details rows by key; a winget row keeps its place through it
func TestPreProcessSoftwareResults_wingetDetailsSurviveTheOverrides(t *testing.T) {
	software := hostDetailQueryPrefix + "software_windows"
	details := hostDetailQueryPrefix + "software_windows_details"
	winget := hostDetailQueryPrefix + "software_winget_packages"
	acrobatDC := hostDetailQueryPrefix + "software_windows_acrobat_dc"
	results := fleet.OsqueryDistributedQueryResults{
		software:  {{"name": "Adobe Acrobat (64-bit)", "version": "25.1", "source": "programs", "vendor": "Adobe"}},
		details:   {{"name": "Adobe Acrobat (64-bit)", "version": "25.1", "vendor": "Adobe", "arp_key": "{AC76BA86-1033-FF00-7760-BC15014EA700}"}},
		winget:    {{"name": "7-Zip", "version": "24.08", "source": "winget_packages", "vendor": "", "winget_id": "7zip.7zip"}},
		acrobatDC: {{"1": "1"}},
	}
	statuses := map[string]fleet.OsqueryStatus{software: fleet.StatusOK, details: fleet.StatusOK, winget: fleet.StatusOK,
		acrobatDC: fleet.StatusOK}

	preProcessSoftwareResults(context.Background(), &fleet.Host{ID: 1, Platform: "windows"}, results, statuses, nil,
		osquery_utils.SoftwareOverrideQueries, slog.New(slog.DiscardHandler))

	require.Len(t, results[details], 2)
	require.Equal(t, "Adobe Acrobat DC (64-bit)", results[details][0]["name"])
	require.Equal(t, "7zip.7zip", results[details][1]["winget_id"])
}
