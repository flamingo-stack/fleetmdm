// OPENFRAME(software-windows-details): stores what the OpenFrame osquery programs table reports about the programs
// installed on a host — openframe/docs/software-windows-details.md
package mysql

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/jmoiron/sqlx"
)

const openframeHostSoftwareWindowsDetailsBatchSize = 500

// OpenframeReplaceHostSoftwareWindowsDetails makes the host's stored details equal to the report: each software row,
// looked up by its raw checksum, gets the reported details and rows the host no longer reports are removed. A checksum
// with no software row yet is skipped: the row appears with the host's next software ingestion and the details with
// the next report.
func (ds *Datastore) OpenframeReplaceHostSoftwareWindowsDetails(ctx context.Context, hostID uint,
	detailsByChecksum map[string]fleet.SoftwareWindowsDetails,
) error {
	checksums := make([]string, 0, len(detailsByChecksum))
	for checksum := range detailsByChecksum {
		checksums = append(checksums, checksum)
	}
	return ds.withRetryTxx(ctx, func(tx sqlx.ExtContext) error {
		for start := 0; start < len(checksums); start += openframeHostSoftwareWindowsDetailsBatchSize {
			end := min(start+openframeHostSoftwareWindowsDetailsBatchSize, len(checksums))
			if err := upsertHostSoftwareWindowsDetails(ctx, tx, hostID, checksums[start:end], detailsByChecksum); err != nil {
				return err
			}
		}
		return deleteUnreportedHostSoftwareWindowsDetails(ctx, tx, hostID, checksums)
	})
}

func upsertHostSoftwareWindowsDetails(ctx context.Context, tx sqlx.ExtContext, hostID uint, checksums []string,
	detailsByChecksum map[string]fleet.SoftwareWindowsDetails,
) error {
	if len(checksums) == 0 {
		return nil
	}
	values := make([]string, 0, len(checksums))
	args := make([]any, 0, 1+8*len(checksums))
	args = append(args, hostID)
	for _, checksum := range checksums {
		d := detailsByChecksum[checksum]
		values = append(values, "SELECT UNHEX(?) AS checksum, ? AS arp_key, ? AS package_family_name, "+
			"? AS system_component, ? AS installer_type, ? AS install_scope, ? AS install_arch, ? AS winget_id")
		args = append(args, hex.EncodeToString([]byte(checksum)), d.ArpKey, d.PackageFamilyName, d.SystemComponent,
			d.InstallerType, d.InstallScope, d.InstallArch, d.WingetID)
	}
	stmt := `
INSERT INTO host_software_windows_details
  (host_id, software_id, arp_key, package_family_name, system_component, installer_type, install_scope, install_arch,
  winget_id)
SELECT ?, s.id, incoming.arp_key, incoming.package_family_name, incoming.system_component, incoming.installer_type,
  incoming.install_scope, incoming.install_arch, incoming.winget_id
FROM software s
JOIN (` + strings.Join(values, " UNION ALL ") + `) incoming ON incoming.checksum = s.checksum
ON DUPLICATE KEY UPDATE
  arp_key = VALUES(arp_key), package_family_name = VALUES(package_family_name),
  system_component = VALUES(system_component), installer_type = VALUES(installer_type),
  install_scope = VALUES(install_scope), install_arch = VALUES(install_arch), winget_id = VALUES(winget_id)`
	if _, err := tx.ExecContext(ctx, stmt, args...); err != nil {
		return ctxerr.Wrap(ctx, err, "upsert host software windows details")
	}
	return nil
}

// the delete keys on the checksums the host reported, so a software row that does not exist yet simply keeps nothing
func deleteUnreportedHostSoftwareWindowsDetails(ctx context.Context, tx sqlx.ExtContext, hostID uint, checksums []string) error {
	stmt := `DELETE FROM host_software_windows_details WHERE host_id = ?`
	args := []any{hostID}
	if len(checksums) > 0 {
		raw := make([][]byte, 0, len(checksums))
		for _, checksum := range checksums {
			raw = append(raw, []byte(checksum))
		}
		var err error
		stmt, args, err = sqlx.In(stmt+` AND software_id NOT IN (SELECT id FROM software WHERE checksum IN (?))`, hostID, raw)
		if err != nil {
			return ctxerr.Wrap(ctx, err, "build delete of unreported host software windows details")
		}
	}
	if _, err := tx.ExecContext(ctx, stmt, args...); err != nil {
		return ctxerr.Wrap(ctx, err, "delete unreported host software windows details")
	}
	return nil
}
