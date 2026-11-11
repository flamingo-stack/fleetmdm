// OPENFRAME(software-windows-details)
package service

import (
	"context"
	"log/slog"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/service/osquery_utils"
	"github.com/stretchr/testify/require"
)

func TestPreProcessSoftwareResults_windowsDetailsFollowTheSoftwareRenames(t *testing.T) {
	software := hostDetailQueryPrefix + "software_windows"
	details := hostDetailQueryPrefix + "software_windows_details"
	acrobatDC := hostDetailQueryPrefix + "software_windows_acrobat_dc"
	results := fleet.OsqueryDistributedQueryResults{
		software: {{"name": "Adobe Acrobat (64-bit)", "version": "25.1", "source": "programs", "vendor": "Adobe"}},
		details: {
			{"name": "Adobe Acrobat (64-bit)", "version": "25.1", "vendor": "Adobe", "arp_key": "{AC76BA86-1033-FF00-7760-BC15014EA700}"},
			{"name": "Microsoft Edge", "version": "142.0", "vendor": "Microsoft Corporation", "arp_key": "Microsoft Edge"},
		},
		acrobatDC: {{"1": "1"}},
	}
	statuses := map[string]fleet.OsqueryStatus{software: fleet.StatusOK, details: fleet.StatusOK, acrobatDC: fleet.StatusOK}

	preProcessSoftwareResults(context.Background(), &fleet.Host{ID: 1, Platform: "windows"}, results, statuses, nil,
		osquery_utils.SoftwareOverrideQueries, slog.New(slog.DiscardHandler))

	require.Equal(t, "Adobe Acrobat DC (64-bit)", results[software][0]["name"])
	require.Len(t, results[details], 2)
	require.Equal(t, "Adobe Acrobat DC (64-bit)", results[details][0]["name"])
	require.Equal(t, "{AC76BA86-1033-FF00-7760-BC15014EA700}", results[details][0]["arp_key"])
	require.Equal(t, "Microsoft Edge", results[details][1]["name"])
}

func TestPreProcessSoftwareResults_windowsDetailsKeepOnlyRowsWithAKey(t *testing.T) {
	software := hostDetailQueryPrefix + "software_windows"
	details := hostDetailQueryPrefix + "software_windows_details"
	scan := hostDetailQueryPrefix + "software_windows_program_files_scan"
	results := fleet.OsqueryDistributedQueryResults{
		software: {{"name": "Microsoft Edge", "version": "142.0", "source": "programs", "vendor": "Microsoft Corporation", "installed_path": `C:\Program Files\Edge`}},
		details:  {{"name": "Microsoft Edge", "version": "142.0", "vendor": "Microsoft Corporation", "arp_key": "Microsoft Edge"}},
		scan: {{"path": `C:\Program Files\Tool\tool.exe`, "filename": "tool.exe", "file_version": "1.0.0.0",
			"product_version": "1.0", "size": "10"}},
	}
	statuses := map[string]fleet.OsqueryStatus{software: fleet.StatusOK, details: fleet.StatusOK, scan: fleet.StatusOK}

	preProcessSoftwareResults(context.Background(), &fleet.Host{ID: 1, Platform: "windows"}, results, statuses, nil,
		osquery_utils.SoftwareOverrideQueries, slog.New(slog.DiscardHandler))

	require.GreaterOrEqual(t, len(results[software]), 1)
	require.Len(t, results[details], 1)
	require.Equal(t, "Microsoft Edge", results[details][0]["arp_key"])
}
