// OPENFRAME(software-arp-keys): reports the Uninstall registry subkey name of each Windows program —
// openframe/docs/software-arp-keys.md
package osquery_utils

import (
	"context"
	"log/slog"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
)

// Kept apart from software_windows: an osquery without the programs.arp_key column fails only this query.
var softwareWindowsArpKeys = DetailQuery{
	Query:            `SELECT name, version, publisher AS vendor, upgrade_code, arp_key FROM programs WHERE arp_key <> ''`,
	Platforms:        []string{"windows"},
	DirectIngestFunc: directIngestSoftwareArpKeys,
}

// The rows are matched to the host's software rows by the checksum software_windows gives them, so every field the
// checksum reads must be built the same way here.
func directIngestSoftwareArpKeys(ctx context.Context, logger *slog.Logger, host *fleet.Host, ds fleet.Datastore, rows []map[string]string) error {
	arpKeys := make(map[string]string, len(rows))
	for _, row := range rows {
		s, err := fleet.SoftwareFromOsqueryRow(row["name"], row["version"], "programs", row["vendor"],
			"", "", "", "", "", "", "", row["upgrade_code"])
		if err != nil {
			logger.DebugContext(ctx, "failed to parse software arp key row", "host_id", host.ID, "err", err)
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
		arpKeys[string(checksum)] = row["arp_key"]
	}
	if len(arpKeys) == 0 {
		return nil
	}
	return ds.OpenframeUpsertSoftwareArpKeys(ctx, arpKeys)
}
