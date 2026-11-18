// OPENFRAME(homebrew-casks)
package osquery_utils

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mock"
	"github.com/stretchr/testify/require"
)

func TestSoftwareHomebrewCasks_macOnlyWithItsOwnSource(t *testing.T) {
	require.Equal(t, []string{"darwin"}, softwareHomebrewCasks.Platforms)
	require.Empty(t, softwareHomebrewCasks.Discovery)
	require.Contains(t, softwareHomebrewCasks.Query, "'homebrew_casks' AS source")
}

// a cask must be reported by exactly one of the two queries: this one takes those software_macos refuses
func TestSoftwareHomebrewCasks_conditionIsTheReverseOfSoftwareMacOS(t *testing.T) {
	appOfTheCask := "EXISTS (SELECT 1 FROM file WHERE file.path LIKE homebrew_packages.path || '/%/%' " +
		"AND rtrim(file.path, '/') LIKE '%.app')"

	require.Contains(t, softwareHomebrewCasks.Query, "WHERE type = 'cask'\nAND "+appOfTheCask)
	require.Contains(t, softwareMacOS.Query, "WHERE type = 'cask'\nAND NOT "+appOfTheCask)
}

// a recursive file pattern follows symlinks out of the Caskroom, so neither query may walk below <cask>/<version>/
func TestSoftwareHomebrewCasks_neverWalksTheCaskFolder(t *testing.T) {
	for _, query := range []string{softwareHomebrewCasks.Query, softwareMacOS.Query} {
		require.NotContains(t, query, "%%")
		require.NotContains(t, query, "'%.app%'")
	}
}

// the software ingestion keeps a cask row whole: its own source, and the cask's folder as the installed path
func TestSoftwareHomebrewCasks_rowIngestedWithItsSourceAndPath(t *testing.T) {
	ds := new(mock.Store)
	host := fleet.Host{ID: 1, Platform: "darwin"}
	rows := []map[string]string{
		{"name": "Slack.app", "version": "4.41.105", "source": "apps", "bundle_identifier": "com.tinyspeck.slackmacgap", "installed_path": "/Applications/Slack.app"},
		{"name": "slack", "version": "4.41.105", "source": "homebrew_casks", "installed_path": "/opt/homebrew/Caskroom/slack/4.41.105"},
	}
	var ingested []fleet.Software
	ds.UpdateHostSoftwareFunc = func(ctx context.Context, hostID uint, software []fleet.Software) (*fleet.UpdateHostSoftwareDBResult, error) {
		ingested = software
		return nil, nil
	}
	var paths []string
	ds.UpdateHostSoftwareInstalledPathsFunc = func(ctx context.Context, hostID uint, sPaths map[string]struct{}, result *fleet.UpdateHostSoftwareDBResult) error {
		for path := range sPaths {
			paths = append(paths, path)
		}
		return nil
	}

	err := directIngestSoftware(t.Context(), slog.New(slog.DiscardHandler), &host, ds, rows)

	require.NoError(t, err)
	require.Len(t, ingested, 2)
	require.Equal(t, "slack", ingested[1].Name)
	require.Equal(t, "homebrew_casks", ingested[1].Source)
	require.Equal(t, "4.41.105", ingested[1].Version)
	require.Len(t, paths, 2)
	caskPath := "/opt/homebrew/Caskroom/slack/4.41.105" + fleet.SoftwareFieldSeparator
	require.Contains(t, strings.Join(paths, "\n"), caskPath)
}
