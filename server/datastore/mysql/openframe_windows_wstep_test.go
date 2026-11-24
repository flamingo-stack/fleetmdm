// OPENFRAME(windows-wstep)
package mysql

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func openframeFakeWSTEPPair(calls *atomic.Int32) func() ([]byte, []byte, error) {
	return func() ([]byte, []byte, error) {
		n := calls.Add(1)
		return fmt.Appendf(nil, "cert-%d", n), fmt.Appendf(nil, "key-%d", n), nil
	}
}

func TestOpenframeLoadWindowsWSTEP(t *testing.T) {
	ds := CreateMySQLDS(t)

	t.Run("generates once and reloads the stored pair", func(t *testing.T) {
		TruncateTables(t, ds, "mdm_config_assets")
		ctx := t.Context()
		var calls atomic.Int32

		cert, key, err := ds.OpenframeLoadWindowsWSTEP(ctx, openframeFakeWSTEPPair(&calls))
		require.NoError(t, err)
		require.Equal(t, "cert-1", string(cert))
		require.Equal(t, "key-1", string(key))

		var stored []byte
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			return sqlx.GetContext(ctx, q, &stored, `SELECT value FROM mdm_config_assets WHERE name = ?`, OpenframeMDMAssetWindowsWSTEPKey)
		})
		require.False(t, bytes.Contains(stored, []byte("key-1")), "the key is stored encrypted")

		cert2, key2, err := ds.OpenframeLoadWindowsWSTEP(ctx, openframeFakeWSTEPPair(&calls))
		require.NoError(t, err)
		require.Equal(t, cert, cert2)
		require.Equal(t, key, key2)
		require.EqualValues(t, 1, calls.Load(), "a stored pair is never regenerated")
	})

	t.Run("concurrent starts agree on one pair", func(t *testing.T) {
		TruncateTables(t, ds, "mdm_config_assets")
		ctx := t.Context()
		var calls atomic.Int32
		const starts = 7
		certs := make([]string, starts)
		var wg sync.WaitGroup
		for i := range starts {
			wg.Go(func() {
				cert, _, err := ds.OpenframeLoadWindowsWSTEP(ctx, openframeFakeWSTEPPair(&calls))
				require.NoError(t, err)
				certs[i] = string(cert)
			})
		}
		wg.Wait()
		for _, c := range certs {
			require.Equal(t, certs[0], c)
		}
	})

	t.Run("refuses a new pair over enrolled Windows devices", func(t *testing.T) {
		TruncateTables(t, ds, "mdm_config_assets", "mdm_windows_enrollments")
		ctx := t.Context()
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			_, err := q.ExecContext(ctx, `
				INSERT INTO mdm_windows_enrollments (mdm_device_id, mdm_hardware_id, device_state, device_type, device_name,
					enroll_type, enroll_user_id, enroll_proto_version, enroll_client_version, host_uuid)
				VALUES ('dev-1', 'hw-1', 'enrolled', 'CIMClient_Windows', 'pc', 'ProgrammaticEnrollment', '', '5.0', '10.0', 'uuid-1')`)
			return err
		})
		var calls atomic.Int32
		_, _, err := ds.OpenframeLoadWindowsWSTEP(ctx, openframeFakeWSTEPPair(&calls))
		require.ErrorContains(t, err, "already enrolled")
		require.Zero(t, calls.Load())
		_, err = ds.GetAllMDMConfigAssetsByName(ctx, []fleet.MDMAssetName{OpenframeMDMAssetWindowsWSTEPCert}, nil)
		require.True(t, fleet.IsNotFound(err), "nothing is stored")
	})

	t.Run("refuses a new pair over escrowed Windows BitLocker keys", func(t *testing.T) {
		TruncateTables(t, ds, "mdm_config_assets", "mdm_windows_enrollments")
		ctx := t.Context()
		h := test.NewHost(t, ds, "openframe-wstep-win", "", "openframe-wstep-key", "openframe-wstep-uuid", time.Now())
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			if _, err := q.ExecContext(ctx, `UPDATE hosts SET platform = 'windows' WHERE id = ?`, h.ID); err != nil {
				return err
			}
			_, err := q.ExecContext(ctx, `INSERT INTO host_disk_encryption_keys (host_id, base64_encrypted) VALUES (?, 'x')`, h.ID)
			return err
		})
		var calls atomic.Int32
		_, _, err := ds.OpenframeLoadWindowsWSTEP(ctx, openframeFakeWSTEPPair(&calls))
		require.ErrorContains(t, err, "escrowed BitLocker keys")
		require.Zero(t, calls.Load())
	})

	t.Run("refuses half a pair", func(t *testing.T) {
		TruncateTables(t, ds, "mdm_config_assets", "mdm_windows_enrollments", "host_disk_encryption_keys")
		ctx := t.Context()
		require.NoError(t, ds.InsertMDMConfigAssets(ctx, []fleet.MDMConfigAsset{
			{Name: OpenframeMDMAssetWindowsWSTEPCert, Value: []byte("cert-only")},
		}, nil))
		var calls atomic.Int32
		_, _, err := ds.OpenframeLoadWindowsWSTEP(ctx, openframeFakeWSTEPPair(&calls))
		require.ErrorContains(t, err, "only one half")
		require.Zero(t, calls.Load())
	})
}
