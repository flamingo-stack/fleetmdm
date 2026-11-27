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
	ctx := t.Context()
	require.NoError(t, ds.MigrateOpenframe(ctx))
	teamA, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-wstep-a"})
	require.NoError(t, err)
	teamB, err := ds.NewTeam(ctx, &fleet.Team{Name: "openframe-wstep-b"})
	require.NoError(t, err)

	windowsHost := func(t *testing.T, teamID uint, name string) *fleet.Host {
		h := test.NewHost(t, ds, name, "", name+"-key", name+"-uuid", time.Now())
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			_, err := q.ExecContext(ctx, `UPDATE hosts SET platform = 'windows', team_id = ? WHERE id = ?`, teamID, h.ID)
			return err
		})
		h.TeamID = &teamID
		return h
	}
	enroll := func(t *testing.T, h *fleet.Host) {
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			_, err := q.ExecContext(ctx, `
				INSERT INTO mdm_windows_enrollments (mdm_device_id, mdm_hardware_id, device_state, device_type, device_name,
					enroll_type, enroll_user_id, enroll_proto_version, enroll_client_version, host_uuid, team_id)
				VALUES (?, ?, 'enrolled', 'CIMClient_Windows', 'pc', 'ProgrammaticEnrollment', '', '5.0', '10.0', ?, ?)`,
				h.UUID+"-dev", h.UUID+"-hw", h.UUID, h.TeamID)
			return err
		})
	}
	reset := func(t *testing.T) {
		TruncateTables(t, ds, "mdm_config_assets", "mdm_windows_enrollments", "host_disk_encryption_keys")
	}

	t.Run("each tenant generates once and reloads its own pair", func(t *testing.T) {
		reset(t)
		var calls atomic.Int32

		certA, keyA, err := ds.OpenframeLoadWindowsWSTEP(ctx, teamA.ID, openframeFakeWSTEPPair(&calls))
		require.NoError(t, err)
		certB, keyB, err := ds.OpenframeLoadWindowsWSTEP(ctx, teamB.ID, openframeFakeWSTEPPair(&calls))
		require.NoError(t, err)
		require.NotEqual(t, certA, certB)
		require.NotEqual(t, keyA, keyB)

		again, keyAgain, err := ds.OpenframeLoadWindowsWSTEP(ctx, teamA.ID, openframeFakeWSTEPPair(&calls))
		require.NoError(t, err)
		require.Equal(t, certA, again)
		require.Equal(t, keyA, keyAgain)
		require.EqualValues(t, 2, calls.Load(), "a stored pair is never regenerated")

		got, gotKey, err := ds.OpenframeGetWindowsWSTEP(ctx, teamB.ID)
		require.NoError(t, err)
		require.Equal(t, certB, got)
		require.Equal(t, keyB, gotKey)

		var stored []byte
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			return sqlx.GetContext(ctx, q, &stored, `SELECT value FROM mdm_config_assets WHERE name = ?`,
				fmt.Sprintf("openframe_windows_wstep_key/%d", teamA.ID))
		})
		require.False(t, bytes.Contains(stored, keyA), "the key is stored encrypted")
	})

	t.Run("get never generates", func(t *testing.T) {
		reset(t)
		_, _, err := ds.OpenframeGetWindowsWSTEP(ctx, teamA.ID)
		require.True(t, fleet.IsNotFound(err))
	})

	t.Run("concurrent first enrollments of a tenant agree on one pair", func(t *testing.T) {
		reset(t)
		var calls atomic.Int32
		const starts = 7
		certs := make([]string, starts)
		var wg sync.WaitGroup
		for i := range starts {
			wg.Go(func() {
				cert, _, err := ds.OpenframeLoadWindowsWSTEP(ctx, teamA.ID, openframeFakeWSTEPPair(&calls))
				require.NoError(t, err)
				certs[i] = string(cert)
			})
		}
		wg.Wait()
		for _, c := range certs {
			require.Equal(t, certs[0], c)
		}
	})

	t.Run("refuses a new pair over the tenant's enrolled devices, not another tenant's", func(t *testing.T) {
		reset(t)
		enroll(t, windowsHost(t, teamA.ID, "openframe-wstep-enrolled-a"))
		var calls atomic.Int32

		_, _, err := ds.OpenframeLoadWindowsWSTEP(ctx, teamA.ID, openframeFakeWSTEPPair(&calls))
		require.ErrorContains(t, err, "already enrolled")
		require.Zero(t, calls.Load())
		_, _, err = ds.OpenframeGetWindowsWSTEP(ctx, teamA.ID)
		require.True(t, fleet.IsNotFound(err), "nothing is stored")

		_, _, err = ds.OpenframeLoadWindowsWSTEP(ctx, teamB.ID, openframeFakeWSTEPPair(&calls))
		require.NoError(t, err, "tenant B's first enrollment is not blocked by tenant A's devices")
	})

	t.Run("refuses a new pair over the tenant's escrowed BitLocker keys", func(t *testing.T) {
		reset(t)
		h := windowsHost(t, teamB.ID, "openframe-wstep-key-b")
		ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
			_, err := q.ExecContext(ctx, `INSERT INTO host_disk_encryption_keys (host_id, base64_encrypted) VALUES (?, 'x')`, h.ID)
			return err
		})
		var calls atomic.Int32
		_, _, err := ds.OpenframeLoadWindowsWSTEP(ctx, teamB.ID, openframeFakeWSTEPPair(&calls))
		require.ErrorContains(t, err, "escrowed BitLocker keys")
		require.Zero(t, calls.Load())

		_, _, err = ds.OpenframeLoadWindowsWSTEP(ctx, teamA.ID, openframeFakeWSTEPPair(&calls))
		require.NoError(t, err)
	})

	t.Run("refuses half a pair", func(t *testing.T) {
		reset(t)
		require.NoError(t, ds.InsertMDMConfigAssets(ctx, []fleet.MDMConfigAsset{
			{Name: fleet.MDMAssetName(fmt.Sprintf("openframe_windows_wstep_cert/%d", teamA.ID)), Value: []byte("cert-only")},
		}, nil))
		var calls atomic.Int32
		_, _, err := ds.OpenframeLoadWindowsWSTEP(ctx, teamA.ID, openframeFakeWSTEPPair(&calls))
		require.ErrorContains(t, err, "only one half")
		require.Zero(t, calls.Load())
	})

	t.Run("refuses no team", func(t *testing.T) {
		reset(t)
		var calls atomic.Int32
		_, _, err := ds.OpenframeLoadWindowsWSTEP(ctx, 0, openframeFakeWSTEPPair(&calls))
		require.Error(t, err)
		require.Zero(t, calls.Load())
	})
}
