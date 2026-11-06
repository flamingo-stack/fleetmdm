// OPENFRAME(software-windows-details)
package mysql

import (
	"context"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/ptr"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/stretchr/testify/require"
)

func TestOpenframeSoftwareWindowsDetails(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	host := test.NewHost(t, ds, "host1", "", "host1key", "host1uuid", time.Now())
	chrome := fleet.Software{Name: "Google Chrome", Version: "130.0", Source: "programs", Vendor: "Google LLC"}
	xaml := fleet.Software{Name: "Microsoft.UI.Xaml.2.8", Version: "8.2501.31001.0", Source: "programs", Vendor: "Microsoft Platform Extensions"}
	_, err := ds.UpdateHostSoftware(ctx, host.ID, []fleet.Software{chrome, xaml})
	require.NoError(t, err)
	require.NoError(t, ds.SyncHostsSoftware(ctx, time.Now()))
	require.NoError(t, ds.SyncHostsSoftwareTitles(ctx, time.Now()))

	checksumOf := func(s fleet.Software) string {
		raw, err := s.ComputeRawChecksum()
		require.NoError(t, err)
		return string(raw)
	}
	chromeDetails := fleet.SoftwareWindowsDetails{ArpKey: ptr.String("Google Chrome"), SystemComponent: ptr.Bool(false),
		InstallerType: ptr.String("exe"), InstallScope: ptr.String("machine"), InstallArch: ptr.String("64")}
	xamlDetails := fleet.SoftwareWindowsDetails{PackageFamilyName: ptr.String("Microsoft.UI.Xaml.2.8_8wekyb3d8bbwe"),
		InstallerType: ptr.String("msix")}
	unknown := fleet.Software{Name: "Not installed", Version: "1", Source: "programs"}
	require.NoError(t, ds.OpenframeUpsertSoftwareWindowsDetails(ctx, map[string]fleet.SoftwareWindowsDetails{
		checksumOf(chrome):  {ArpKey: ptr.String("stale")},
		checksumOf(unknown): {ArpKey: ptr.String("Unknown")},
	}))
	require.NoError(t, ds.OpenframeUpsertSoftwareWindowsDetails(ctx, map[string]fleet.SoftwareWindowsDetails{
		checksumOf(chrome): chromeDetails,
		checksumOf(xaml):   xamlDetails,
	}))

	var stored int
	require.NoError(t, ds.writer(ctx).GetContext(ctx, &stored, `SELECT COUNT(*) FROM software_windows_details`))
	require.Equal(t, 2, stored)

	titles, _, _, err := ds.ListSoftwareTitles(ctx, fleet.SoftwareTitleListOptions{ListOptions: fleet.ListOptions{OrderKey: "name"}},
		fleet.TeamFilter{User: test.UserAdmin})
	require.NoError(t, err)
	require.Len(t, titles, 2)
	versions := map[string]fleet.SoftwareVersion{}
	for _, title := range titles {
		require.Len(t, title.Versions, 1)
		versions[title.Name] = title.Versions[0]
	}
	require.Equal(t, chromeDetails, versions["Google Chrome"].SoftwareWindowsDetails)
	require.Equal(t, "Google LLC", versions["Google Chrome"].Vendor)
	require.Equal(t, xamlDetails, versions["Microsoft.UI.Xaml.2.8"].SoftwareWindowsDetails)

	host.Platform = "windows"
	hostSoftware, _, err := ds.ListHostSoftware(ctx, host, fleet.HostSoftwareTitleListOptions{ListOptions: fleet.ListOptions{PerPage: 10}})
	require.NoError(t, err)
	byName := map[string]*fleet.HostSoftwareInstalledVersion{}
	for _, s := range hostSoftware {
		require.Len(t, s.InstalledVersions, 1)
		byName[s.Name] = s.InstalledVersions[0]
	}
	require.Equal(t, chromeDetails, byName["Google Chrome"].SoftwareWindowsDetails)
	require.Equal(t, "Microsoft Platform Extensions", byName["Microsoft.UI.Xaml.2.8"].Vendor)
}
