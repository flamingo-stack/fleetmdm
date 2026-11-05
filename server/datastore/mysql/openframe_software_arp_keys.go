// OPENFRAME(software-arp-keys): stores the Uninstall registry subkey name of Windows software rows —
// openframe/docs/software-arp-keys.md
package mysql

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
)

const openframeSoftwareArpKeysBatchSize = 500

// OpenframeUpsertSoftwareArpKeys records the ARP key of each software row, looked up by its raw checksum. A checksum
// with no software row yet is skipped: the row appears with the host's next software ingestion and the key with the
// next arp-keys report.
func (ds *Datastore) OpenframeUpsertSoftwareArpKeys(ctx context.Context, arpKeysByChecksum map[string]string) error {
	checksums := make([]string, 0, len(arpKeysByChecksum))
	for checksum := range arpKeysByChecksum {
		checksums = append(checksums, checksum)
	}
	for start := 0; start < len(checksums); start += openframeSoftwareArpKeysBatchSize {
		end := min(start+openframeSoftwareArpKeysBatchSize, len(checksums))
		if err := ds.upsertSoftwareArpKeysBatch(ctx, checksums[start:end], arpKeysByChecksum); err != nil {
			return err
		}
	}
	return nil
}

func (ds *Datastore) upsertSoftwareArpKeysBatch(ctx context.Context, checksums []string, arpKeysByChecksum map[string]string) error {
	values := make([]string, 0, len(checksums))
	args := make([]any, 0, 2*len(checksums))
	for _, checksum := range checksums {
		values = append(values, "SELECT UNHEX(?) AS checksum, ? AS arp_key")
		args = append(args, hex.EncodeToString([]byte(checksum)), arpKeysByChecksum[checksum])
	}
	stmt := `
INSERT INTO software_arp_keys (software_id, arp_key)
SELECT s.id, incoming.arp_key
FROM software s
JOIN (` + strings.Join(values, " UNION ALL ") + `) incoming ON incoming.checksum = s.checksum
ON DUPLICATE KEY UPDATE arp_key = VALUES(arp_key)`
	if _, err := ds.writer(ctx).ExecContext(ctx, stmt, args...); err != nil {
		return ctxerr.Wrap(ctx, err, "upsert software arp keys")
	}
	return nil
}
