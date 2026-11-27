// >>> OPENFRAME(windows-mdm-tenant): a Windows MDM enrollment is linked only to a host of its tenant — openframe/docs/mysql-multitenancy-feature.md

package osquery_utils

import (
	"context"

	"github.com/fleetdm/fleet/v4/server/fleet"
)

// OpenframeUpdateWindowsEnrollmentHost links the Windows MDM enrollment with this device id to the host.
// The device id comes from the device, so in shared mode the link is made only when the enrollment
// belongs to the host's tenant; otherwise it is UpdateMDMWindowsEnrollmentsHostUUID as upstream.
func OpenframeUpdateWindowsEnrollmentHost(ctx context.Context, ds fleet.Datastore, hostID uint, hostUUID, mdmDeviceID string, shared bool) (bool, error) {
	if shared {
		return ds.OpenframeUpdateMDMWindowsEnrollmentHost(ctx, hostID, mdmDeviceID)
	}
	return ds.UpdateMDMWindowsEnrollmentsHostUUID(ctx, hostUUID, mdmDeviceID)
}

// <<< OPENFRAME(windows-mdm-tenant)
