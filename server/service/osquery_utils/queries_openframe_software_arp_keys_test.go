// OPENFRAME(software-arp-keys)
package osquery_utils

import (
	"context"
	"log/slog"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mock"
	"github.com/stretchr/testify/require"
)

func TestDirectIngestSoftwareArpKeys_keyedByTheSoftwareWindowsChecksum(t *testing.T) {
	ds := new(mock.Store)
	var stored map[string]string
	ds.OpenframeUpsertSoftwareArpKeysFunc = func(ctx context.Context, arpKeys map[string]string) error {
		stored = arpKeys
		return nil
	}
	host := &fleet.Host{ID: 1, Platform: "windows"}
	rows := []map[string]string{{
		"name": "Google Chrome", "version": "130.0", "vendor": "Google LLC", "upgrade_code": "", "arp_key": "Google Chrome",
	}}

	require.NoError(t, directIngestSoftwareArpKeys(context.Background(), slog.Default(), host, ds, rows))

	want, err := fleet.SoftwareFromOsqueryRow("Google Chrome", "130.0", "programs", "Google LLC", `C:\Chrome`, "", "", "", "", "", "", "")
	require.NoError(t, err)
	MutateSoftwareOnIngestion(context.Background(), want, slog.Default())
	checksum, err := want.ComputeRawChecksum()
	require.NoError(t, err)
	require.Equal(t, map[string]string{string(checksum): "Google Chrome"}, stored)
}

func TestDirectIngestSoftwareArpKeys_noRows_noWrite(t *testing.T) {
	ds := new(mock.Store)
	require.NoError(t, directIngestSoftwareArpKeys(context.Background(), slog.Default(), &fleet.Host{ID: 1}, ds, nil))
	require.False(t, ds.OpenframeUpsertSoftwareArpKeysFuncInvoked)
}
