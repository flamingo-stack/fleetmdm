// OPENFRAME(winget-packages)
package osquery_utils

import (
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/require"
)

func TestSoftwareWingetPackages_windowsOnlyAndDiscoveredByTable(t *testing.T) {
	require.Equal(t, []string{"windows"}, softwareWingetPackages.Platforms)
	require.Equal(t, discoveryTable("winget_packages"), softwareWingetPackages.Discovery)
	require.Contains(t, softwareWingetPackages.Query, "'winget_packages' AS source")
}

// the rows go through the regular software ingestion: a winget package becomes a software row with its own source
func TestSoftwareWingetPackages_rowIsSoftware(t *testing.T) {
	s, err := fleet.SoftwareFromOsqueryRow("7-Zip", "24.08", "winget_packages", "", "", "", "", "", "", "", "", "")
	require.NoError(t, err)
	require.Equal(t, "7-Zip", s.Name)
	require.Equal(t, "winget_packages", s.Source)
}
