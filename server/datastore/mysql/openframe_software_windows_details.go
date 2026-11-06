// OPENFRAME(software-windows-details): stores what the OpenFrame osquery programs table reports about Windows
// software rows — openframe/docs/software-windows-details.md
package mysql

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
)

const openframeSoftwareWindowsDetailsBatchSize = 500

// OpenframeUpsertSoftwareWindowsDetails records the details of each software row, looked up by its raw checksum. A
// checksum with no software row yet is skipped: the row appears with the host's next software ingestion and the
// details with the next report.
func (ds *Datastore) OpenframeUpsertSoftwareWindowsDetails(ctx context.Context, detailsByChecksum map[string]fleet.SoftwareWindowsDetails) error {
	checksums := make([]string, 0, len(detailsByChecksum))
	for checksum := range detailsByChecksum {
		checksums = append(checksums, checksum)
	}
	for start := 0; start < len(checksums); start += openframeSoftwareWindowsDetailsBatchSize {
		end := min(start+openframeSoftwareWindowsDetailsBatchSize, len(checksums))
		if err := ds.upsertSoftwareWindowsDetailsBatch(ctx, checksums[start:end], detailsByChecksum); err != nil {
			return err
		}
	}
	return nil
}

func (ds *Datastore) upsertSoftwareWindowsDetailsBatch(ctx context.Context, checksums []string,
	detailsByChecksum map[string]fleet.SoftwareWindowsDetails,
) error {
	values := make([]string, 0, len(checksums))
	args := make([]any, 0, 7*len(checksums))
	for _, checksum := range checksums {
		d := detailsByChecksum[checksum]
		values = append(values, "SELECT UNHEX(?) AS checksum, ? AS arp_key, ? AS package_family_name, "+
			"? AS system_component, ? AS installer_type, ? AS install_scope, ? AS install_arch")
		args = append(args, hex.EncodeToString([]byte(checksum)), d.ArpKey, d.PackageFamilyName, d.SystemComponent,
			d.InstallerType, d.InstallScope, d.InstallArch)
	}
	stmt := `
INSERT INTO software_windows_details
  (software_id, arp_key, package_family_name, system_component, installer_type, install_scope, install_arch)
SELECT s.id, incoming.arp_key, incoming.package_family_name, incoming.system_component, incoming.installer_type,
  incoming.install_scope, incoming.install_arch
FROM software s
JOIN (` + strings.Join(values, " UNION ALL ") + `) incoming ON incoming.checksum = s.checksum
ON DUPLICATE KEY UPDATE
  arp_key = VALUES(arp_key), package_family_name = VALUES(package_family_name),
  system_component = VALUES(system_component), installer_type = VALUES(installer_type),
  install_scope = VALUES(install_scope), install_arch = VALUES(install_arch)`
	if _, err := ds.writer(ctx).ExecContext(ctx, stmt, args...); err != nil {
		return ctxerr.Wrap(ctx, err, "upsert software windows details")
	}
	return nil
}
