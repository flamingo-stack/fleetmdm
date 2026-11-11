// OPENFRAME(software-windows-details)
package osquery_utils

import (
	"context"
	"log/slog"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mock"
	"github.com/fleetdm/fleet/v4/server/ptr"
	"github.com/stretchr/testify/require"
)

func TestDirectIngestSoftwareWindowsDetails_keyedByTheSoftwareWindowsChecksum(t *testing.T) {
	ds := new(mock.Store)
	var stored map[string]fleet.SoftwareWindowsDetails
	var storedFor uint
	ds.OpenframeReplaceHostSoftwareWindowsDetailsFunc = func(ctx context.Context, hostID uint, details map[string]fleet.SoftwareWindowsDetails) error {
		storedFor = hostID
		stored = details
		return nil
	}
	host := &fleet.Host{ID: 1, Platform: "windows"}
	rows := []map[string]string{
		{"name": "Google Chrome", "version": "130.0", "vendor": "Google LLC", "upgrade_code": "", "arp_key": "Google Chrome",
			"package_family_name": "", "system_component": "0", "installer_type": "exe", "install_scope": "machine", "install_arch": "64"},
		{"name": "Microsoft.UI.Xaml.2.8", "version": "8.2501.31001.0", "vendor": "Microsoft Platform Extensions", "upgrade_code": "",
			"arp_key": "", "package_family_name": "Microsoft.UI.Xaml.2.8_8wekyb3d8bbwe", "system_component": "", "installer_type": "msix",
			"install_scope": "", "install_arch": ""},
	}

	require.NoError(t, directIngestSoftwareWindowsDetails(context.Background(), slog.Default(), host, ds, rows))

	checksumOf := func(name, version, vendor string) string {
		s, err := fleet.SoftwareFromOsqueryRow(name, version, "programs", vendor, `C:\x`, "", "", "", "", "", "", "")
		require.NoError(t, err)
		MutateSoftwareOnIngestion(context.Background(), s, slog.Default())
		raw, err := s.ComputeRawChecksum()
		require.NoError(t, err)
		return string(raw)
	}
	require.Equal(t, uint(1), storedFor)
	require.Equal(t, map[string]fleet.SoftwareWindowsDetails{
		checksumOf("Google Chrome", "130.0", "Google LLC"): {ArpKey: ptr.String("Google Chrome"), SystemComponent: ptr.Bool(false),
			InstallerType: ptr.String("exe"), InstallScope: ptr.String("machine"), InstallArch: ptr.String("64")},
		checksumOf("Microsoft.UI.Xaml.2.8", "8.2501.31001.0", "Microsoft Platform Extensions"): {
			PackageFamilyName: ptr.String("Microsoft.UI.Xaml.2.8_8wekyb3d8bbwe"), InstallerType: ptr.String("msix")},
	}, stored)
}

// an empty report still runs: it clears what the host stored before
func TestDirectIngestSoftwareWindowsDetails_noRows_clearsTheHost(t *testing.T) {
	ds := new(mock.Store)
	var stored map[string]fleet.SoftwareWindowsDetails
	ds.OpenframeReplaceHostSoftwareWindowsDetailsFunc = func(ctx context.Context, hostID uint, details map[string]fleet.SoftwareWindowsDetails) error {
		stored = details
		return nil
	}
	require.NoError(t, directIngestSoftwareWindowsDetails(context.Background(), slog.Default(), &fleet.Host{ID: 1}, ds, nil))
	require.True(t, ds.OpenframeReplaceHostSoftwareWindowsDetailsFuncInvoked)
	require.Empty(t, stored)
}
