package osquery_utils

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mock"
	"github.com/stretchr/testify/require"
)

// OPENFRAME(enroll-machine-identity): openframe/docs/enroll-machine-identity.md
func TestLinkWindowsHostMDMEnrollmentOpenframeSharedUUID(t *testing.T) {
	newDS := func(refuse error) *mock.Store {
		ds := new(mock.Store)
		ds.OpenframeRefuseSharedHardwareUUIDFunc = func(ctx context.Context, hostID uint, hardwareUUID string) error {
			return refuse
		}
		ds.UpdateMDMWindowsEnrollmentsHostUUIDFunc = func(ctx context.Context, hostUUID string, mdmDeviceID string) (bool, error) {
			return false, nil
		}
		return ds
	}
	logger := slog.New(slog.DiscardHandler)
	openframe := fleet.NewOpenframeModeContext(context.Background())

	t.Run("a host whose uuid another host of its team shares is not linked", func(t *testing.T) {
		ds := newDS(errors.New("MDM enrollment refused"))

		_, err := LinkWindowsHostMDMEnrollment(openframe, logger, ds, 7, "uuid-1", "device-1")

		require.ErrorContains(t, err, "MDM enrollment refused")
		require.False(t, ds.UpdateMDMWindowsEnrollmentsHostUUIDFuncInvoked)
	})

	t.Run("a host alone with its uuid is linked", func(t *testing.T) {
		ds := newDS(nil)

		_, err := LinkWindowsHostMDMEnrollment(openframe, logger, ds, 7, "uuid-1", "device-1")

		require.NoError(t, err)
		require.True(t, ds.UpdateMDMWindowsEnrollmentsHostUUIDFuncInvoked)
	})

	t.Run("without OpenFrame mode nothing is checked, as upstream", func(t *testing.T) {
		ds := newDS(errors.New("MDM enrollment refused"))

		_, err := LinkWindowsHostMDMEnrollment(context.Background(), logger, ds, 7, "uuid-1", "device-1")

		require.NoError(t, err)
		require.False(t, ds.OpenframeRefuseSharedHardwareUUIDFuncInvoked)
	})
}

// OPENFRAME(windows-mdm-tenant): openframe/docs/mysql-multitenancy-feature.md
func TestOpenframeUpdateWindowsEnrollmentHost(t *testing.T) {
	newDS := func() *mock.Store {
		ds := new(mock.Store)
		ds.OpenframeUpdateMDMWindowsEnrollmentHostFunc = func(ctx context.Context, hostID uint, mdmDeviceID string) (bool, error) {
			require.Equal(t, uint(7), hostID)
			require.Equal(t, "device-1", mdmDeviceID)
			return true, nil
		}
		ds.UpdateMDMWindowsEnrollmentsHostUUIDFunc = func(ctx context.Context, hostUUID string, mdmDeviceID string) (bool, error) {
			require.Equal(t, "uuid-1", hostUUID)
			return true, nil
		}
		return ds
	}

	ds := newDS()
	updated, err := OpenframeUpdateWindowsEnrollmentHost(t.Context(), ds, 7, "uuid-1", "device-1", true)
	require.NoError(t, err)
	require.True(t, updated)
	require.True(t, ds.OpenframeUpdateMDMWindowsEnrollmentHostFuncInvoked, "shared mode links within the host's tenant")
	require.False(t, ds.UpdateMDMWindowsEnrollmentsHostUUIDFuncInvoked)

	ds = newDS()
	_, err = OpenframeUpdateWindowsEnrollmentHost(t.Context(), ds, 7, "uuid-1", "device-1", false)
	require.NoError(t, err)
	require.True(t, ds.UpdateMDMWindowsEnrollmentsHostUUIDFuncInvoked, "outside shared mode as upstream")
	require.False(t, ds.OpenframeUpdateMDMWindowsEnrollmentHostFuncInvoked)
}
