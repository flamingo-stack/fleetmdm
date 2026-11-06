// OPENFRAME(software-windows-details): reports what the OpenFrame osquery programs table knows about each Windows
// program beyond the columns Fleet keeps — openframe/docs/software-windows-details.md
package osquery_utils

import (
	"context"
	"log/slog"
	"strings"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
)

// Kept apart from software_windows: an osquery without these programs columns fails only this query.
var softwareWindowsDetails = DetailQuery{
	Query: `SELECT name, version, publisher AS vendor, upgrade_code, arp_key, package_family_name, system_component,
  installer_type, install_scope, install_arch
FROM programs WHERE arp_key <> '' OR package_family_name <> ''`,
	Platforms:        []string{"windows"},
	DirectIngestFunc: directIngestSoftwareWindowsDetails,
}

// The rows are matched to the host's software rows by the checksum software_windows gives them, so every field the
// checksum reads must be built the same way here.
func directIngestSoftwareWindowsDetails(ctx context.Context, logger *slog.Logger, host *fleet.Host, ds fleet.Datastore, rows []map[string]string) error {
	details := make(map[string]fleet.SoftwareWindowsDetails, len(rows))
	for _, row := range rows {
		s, err := fleet.SoftwareFromOsqueryRow(row["name"], row["version"], "programs", row["vendor"],
			"", "", "", "", "", "", "", row["upgrade_code"])
		if err != nil {
			logger.DebugContext(ctx, "failed to parse software windows details row", "host_id", host.ID, "err", err)
			continue
		}
		MutateSoftwareOnIngestion(ctx, s, logger)
		if shouldRemoveSoftware(host, s) {
			continue
		}
		checksum, err := s.ComputeRawChecksum()
		if err != nil {
			return ctxerr.Wrap(ctx, err, "compute software checksum")
		}
		details[string(checksum)] = windowsDetailsOf(row)
	}
	if len(details) == 0 {
		return nil
	}
	return ds.OpenframeUpsertSoftwareWindowsDetails(ctx, details)
}

func windowsDetailsOf(row map[string]string) fleet.SoftwareWindowsDetails {
	return fleet.SoftwareWindowsDetails{
		ArpKey:            nonEmpty(row["arp_key"]),
		PackageFamilyName: nonEmpty(row["package_family_name"]),
		SystemComponent:   flag(row["system_component"]),
		InstallerType:     nonEmpty(row["installer_type"]),
		InstallScope:      nonEmpty(row["install_scope"]),
		InstallArch:       nonEmpty(row["install_arch"]),
	}
}

func nonEmpty(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func flag(value string) *bool {
	switch strings.TrimSpace(value) {
	case "1":
		on := true
		return &on
	case "0":
		off := false
		return &off
	}
	return nil
}
