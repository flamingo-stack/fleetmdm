// OPENFRAME(software-arp-keys)
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

func TestOpenframeSoftwareArpKeys(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	host := test.NewHost(t, ds, "host1", "", "host1key", "host1uuid", time.Now())
	chrome := fleet.Software{Name: "Google Chrome", Version: "130.0", Source: "programs", Vendor: "Google LLC"}
	git := fleet.Software{Name: "Git", Version: "2.47.0", Source: "programs", Vendor: "The Git Development Community"}
	_, err := ds.UpdateHostSoftware(ctx, host.ID, []fleet.Software{chrome, git})
	require.NoError(t, err)
	require.NoError(t, ds.SyncHostsSoftware(ctx, time.Now()))
	require.NoError(t, ds.SyncHostsSoftwareTitles(ctx, time.Now()))

	checksumOf := func(s fleet.Software) string {
		raw, err := s.ComputeRawChecksum()
		require.NoError(t, err)
		return string(raw)
	}
	unknown := fleet.Software{Name: "Not installed", Version: "1", Source: "programs"}
	require.NoError(t, ds.OpenframeUpsertSoftwareArpKeys(ctx, map[string]string{
		checksumOf(chrome):  "Google Chrome",
		checksumOf(unknown): "Unknown",
	}))
	require.NoError(t, ds.OpenframeUpsertSoftwareArpKeys(ctx, map[string]string{checksumOf(git): "Git_is1"}))

	var stored int
	require.NoError(t, ds.writer(ctx).GetContext(ctx, &stored, `SELECT COUNT(*) FROM software_arp_keys`))
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
	require.Equal(t, ptr.String("Google Chrome"), versions["Google Chrome"].ArpKey)
	require.Equal(t, "Google LLC", versions["Google Chrome"].Vendor)
	require.Equal(t, ptr.String("Git_is1"), versions["Git"].ArpKey)

	host.Platform = "windows"
	hostSoftware, _, err := ds.ListHostSoftware(ctx, host, fleet.HostSoftwareTitleListOptions{ListOptions: fleet.ListOptions{PerPage: 10}})
	require.NoError(t, err)
	byName := map[string]*fleet.HostSoftwareInstalledVersion{}
	for _, s := range hostSoftware {
		require.Len(t, s.InstalledVersions, 1)
		byName[s.Name] = s.InstalledVersions[0]
	}
	require.Equal(t, ptr.String("Google Chrome"), byName["Google Chrome"].ArpKey)
	require.Equal(t, "The Git Development Community", byName["Git"].Vendor)
}
