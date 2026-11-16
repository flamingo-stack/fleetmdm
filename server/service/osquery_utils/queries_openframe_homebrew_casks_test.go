// OPENFRAME(homebrew-casks)
package osquery_utils

import (
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/require"
)

func TestSoftwareHomebrewCasks_macOnlyWithItsOwnSource(t *testing.T) {
	require.Equal(t, []string{"darwin"}, softwareHomebrewCasks.Platforms)
	require.Empty(t, softwareHomebrewCasks.Discovery)
	require.Contains(t, softwareHomebrewCasks.Query, "'homebrew_casks' AS source")
}

// a cask must be reported by exactly one of the two queries: this one takes those software_macos refuses
func TestSoftwareHomebrewCasks_conditionIsTheReverseOfSoftwareMacOS(t *testing.T) {
	appOfTheCask := "EXISTS (SELECT 1 FROM file WHERE file.path LIKE CONCAT(homebrew_packages.path, '/%%') " +
		"AND file.path LIKE '%.app%' LIMIT 1)"

	require.Contains(t, softwareHomebrewCasks.Query, "WHERE type = 'cask'\nAND "+appOfTheCask)
	require.Contains(t, softwareMacOS.Query, "WHERE type = 'cask'\nAND NOT "+appOfTheCask)
}

// the rows go through the regular software ingestion: a cask becomes a software row with its own source
func TestSoftwareHomebrewCasks_rowIsSoftware(t *testing.T) {
	s, err := fleet.SoftwareFromOsqueryRow("slack", "4.41.105", "homebrew_casks", "", "", "", "", "", "", "", "", "")
	require.NoError(t, err)
	require.Equal(t, "slack", s.Name)
	require.Equal(t, "homebrew_casks", s.Source)
}
