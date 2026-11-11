// OPENFRAME(winget-packages): the winget package id of a winget_packages software row reaches the host's windows
// details — openframe/docs/winget-packages.md
package service

import (
	"github.com/fleetdm/fleet/v4/server/fleet"
)

// Runs before the winget rows are moved into software_windows. A row is copied only when both queries ran, so a
// failed details query never turns into "every program lost its details".
func appendWingetRowsToWindowsDetails(results fleet.OsqueryDistributedQueryResults, statuses map[string]fleet.OsqueryStatus) {
	winget := hostDetailQueryPrefix + "software_winget_packages"
	details := hostDetailQueryPrefix + "software_windows_details"
	if statuses[winget] != fleet.StatusOK || statuses[details] != fleet.StatusOK {
		return
	}
	if _, ok := results[details]; !ok {
		return
	}
	for _, row := range results[winget] {
		if row["winget_id"] == "" {
			continue
		}
		results[details] = append(results[details], map[string]string{
			"name":      row["name"],
			"version":   row["version"],
			"vendor":    row["vendor"],
			"source":    row["source"],
			"winget_id": row["winget_id"],
		})
	}
}
