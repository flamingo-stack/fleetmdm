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

func TestOpenframeHostSoftwareWindowsDetails(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	host1 := test.NewHost(t, ds, "host1", "", "host1key", "host1uuid", time.Now())
	host2 := test.NewHost(t, ds, "host2", "", "host2key", "host2uuid", time.Now())
	chrome := fleet.Software{Name: "Google Chrome", Version: "130.0", Source: "programs", Vendor: "Google LLC"}
	xaml := fleet.Software{Name: "Microsoft.UI.Xaml.2.8", Version: "8.2501.31001.0", Source: "programs", Vendor: "Microsoft Platform Extensions"}
	_, err := ds.UpdateHostSoftware(ctx, host1.ID, []fleet.Software{chrome, xaml})
	require.NoError(t, err)
	_, err = ds.UpdateHostSoftware(ctx, host2.ID, []fleet.Software{chrome})
	require.NoError(t, err)
	require.NoError(t, ds.SyncHostsSoftware(ctx, time.Now()))
	require.NoError(t, ds.SyncHostsSoftwareTitles(ctx, time.Now()))

	checksumOf := func(s fleet.Software) string {
		raw, err := s.ComputeRawChecksum()
		require.NoError(t, err)
		return string(raw)
	}
	// the same software row installed by the EXE on host1 and by the MSI on host2
	chromeExe := fleet.SoftwareWindowsDetails{ArpKey: ptr.String("Google Chrome"), SystemComponent: ptr.Bool(false),
		InstallerType: ptr.String("exe"), InstallScope: ptr.String("machine"), InstallArch: ptr.String("64")}
	chromeMsi := fleet.SoftwareWindowsDetails{ArpKey: ptr.String("{8A69D345-D564-463C-AFF1-A69D9E530F96}"),
		SystemComponent: ptr.Bool(false), InstallerType: ptr.String("msi"), InstallScope: ptr.String("machine"), InstallArch: ptr.String("64")}
	xamlDetails := fleet.SoftwareWindowsDetails{PackageFamilyName: ptr.String("Microsoft.UI.Xaml.2.8_8wekyb3d8bbwe"),
		InstallerType: ptr.String("msix")}
	unknown := fleet.Software{Name: "Not installed", Version: "1", Source: "programs"}
	require.NoError(t, ds.OpenframeReplaceHostSoftwareWindowsDetails(ctx, host1.ID, map[string]fleet.SoftwareWindowsDetails{
		checksumOf(chrome):  {ArpKey: ptr.String("stale")},
		checksumOf(xaml):    xamlDetails,
		checksumOf(unknown): {ArpKey: ptr.String("Unknown")},
	}))
	require.NoError(t, ds.OpenframeReplaceHostSoftwareWindowsDetails(ctx, host1.ID, map[string]fleet.SoftwareWindowsDetails{
		checksumOf(chrome): chromeExe,
		checksumOf(xaml):   xamlDetails,
	}))
	require.NoError(t, ds.OpenframeReplaceHostSoftwareWindowsDetails(ctx, host2.ID, map[string]fleet.SoftwareWindowsDetails{
		checksumOf(chrome): chromeMsi,
	}))

	var stored int
	require.NoError(t, ds.writer(ctx).GetContext(ctx, &stored, `SELECT COUNT(*) FROM host_software_windows_details`))
	require.Equal(t, 3, stored)

	installedOn := func(host *fleet.Host) map[string]*fleet.HostSoftwareInstalledVersion {
		host.Platform = "windows"
		hostSoftware, _, err := ds.ListHostSoftware(ctx, host, fleet.HostSoftwareTitleListOptions{ListOptions: fleet.ListOptions{PerPage: 10}})
		require.NoError(t, err)
		byName := map[string]*fleet.HostSoftwareInstalledVersion{}
		for _, s := range hostSoftware {
			require.Len(t, s.InstalledVersions, 1)
			byName[s.Name] = s.InstalledVersions[0]
		}
		return byName
	}
	onHost1 := installedOn(host1)
	require.Equal(t, chromeExe, onHost1["Google Chrome"].SoftwareWindowsDetails)
	require.Equal(t, "Google LLC", onHost1["Google Chrome"].Vendor)
	require.Equal(t, xamlDetails, onHost1["Microsoft.UI.Xaml.2.8"].SoftwareWindowsDetails)
	onHost2 := installedOn(host2)
	require.Equal(t, chromeMsi, onHost2["Google Chrome"].SoftwareWindowsDetails)

	// a program the host stops reporting loses its details; the rest stay
	require.NoError(t, ds.OpenframeReplaceHostSoftwareWindowsDetails(ctx, host1.ID, map[string]fleet.SoftwareWindowsDetails{
		checksumOf(chrome): chromeExe,
	}))
	require.NoError(t, ds.writer(ctx).GetContext(ctx, &stored, `SELECT COUNT(*) FROM host_software_windows_details`))
	require.Equal(t, 2, stored)
	require.Equal(t, fleet.SoftwareWindowsDetails{}, installedOn(host1)["Microsoft.UI.Xaml.2.8"].SoftwareWindowsDetails)

	// an empty report clears the host; the other host is untouched
	require.NoError(t, ds.OpenframeReplaceHostSoftwareWindowsDetails(ctx, host1.ID, map[string]fleet.SoftwareWindowsDetails{}))
	require.NoError(t, ds.writer(ctx).GetContext(ctx, &stored, `SELECT COUNT(*) FROM host_software_windows_details`))
	require.Equal(t, 1, stored)
	require.Equal(t, chromeMsi, installedOn(host2)["Google Chrome"].SoftwareWindowsDetails)

	// title versions carry the vendor Fleet stores and no per-install detail
	titles, _, _, err := ds.ListSoftwareTitles(ctx, fleet.SoftwareTitleListOptions{ListOptions: fleet.ListOptions{OrderKey: "name"}},
		fleet.TeamFilter{User: test.UserAdmin})
	require.NoError(t, err)
	require.Len(t, titles, 2)
	for _, title := range titles {
		if title.Name == "Google Chrome" {
			require.Equal(t, "Google LLC", title.Versions[0].Vendor)
		}
	}
}

// OPENFRAME(winget-packages): a winget_packages software row keeps the winget id of the host that reported it
func TestOpenframeHostSoftwareWindowsDetails_wingetID(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	host := test.NewHost(t, ds, "host1", "", "host1key", "host1uuid", time.Now())
	sevenZip := fleet.Software{Name: "7-Zip", Version: "24.08", Source: "winget_packages"}
	_, err := ds.UpdateHostSoftware(ctx, host.ID, []fleet.Software{sevenZip})
	require.NoError(t, err)
	require.NoError(t, ds.SyncHostsSoftware(ctx, time.Now()))
	require.NoError(t, ds.SyncHostsSoftwareTitles(ctx, time.Now()))
	raw, err := sevenZip.ComputeRawChecksum()
	require.NoError(t, err)

	require.NoError(t, ds.OpenframeReplaceHostSoftwareWindowsDetails(ctx, host.ID, map[string]fleet.SoftwareWindowsDetails{
		string(raw): {WingetID: ptr.String("7zip.7zip")},
	}))

	host.Platform = "windows"
	hostSoftware, _, err := ds.ListHostSoftware(ctx, host, fleet.HostSoftwareTitleListOptions{ListOptions: fleet.ListOptions{PerPage: 10}})
	require.NoError(t, err)
	require.Len(t, hostSoftware, 1)
	require.Len(t, hostSoftware[0].InstalledVersions, 1)
	require.Equal(t, fleet.SoftwareWindowsDetails{WingetID: ptr.String("7zip.7zip")}, hostSoftware[0].InstalledVersions[0].SoftwareWindowsDetails)
}
