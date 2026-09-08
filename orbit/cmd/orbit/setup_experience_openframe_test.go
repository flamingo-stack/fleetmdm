// OPENFRAME(agent-skip-setup-experience): processSetupExperience must not register the status poller after a not-enabled (402) init — openframe/docs/agent-skip-setup-experience.md
package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	fleetclient "github.com/fleetdm/fleet/v4/client"
	setupexperience "github.com/fleetdm/fleet/v4/orbit/pkg/setup_experience"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/require"
)

func TestProcessSetupExperienceRegistersPollerOnlyWhenEnabled(t *testing.T) {
	for _, tc := range []struct {
		name       string
		initStatus int
		initBody   string
		wantPoller bool
	}{
		{"init 402 (no Premium license)", http.StatusPaymentRequired, `{"error":"Requires Fleet Premium license"}`, false},
		{"init ok, disabled", http.StatusOK, `{"result":{"enabled":false}}`, false},
		{"init ok, enabled", http.StatusOK, `{"result":{"enabled":true}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var statusCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(fleet.CapabilitiesHeader, string(fleet.CapabilityWebSetupExperience))
				switch r.URL.Path {
				case "/api/fleet/orbit/enroll":
					_, _ = w.Write([]byte(`{"orbit_node_key":"node-key"}`))
				case "/api/fleet/orbit/config":
					_, _ = w.Write([]byte(`{}`))
				case "/api/fleet/orbit/setup_experience/init":
					w.WriteHeader(tc.initStatus)
					_, _ = w.Write([]byte(tc.initBody))
				case "/api/fleet/orbit/setup_experience/status":
					statusCalls.Add(1)
					w.WriteHeader(http.StatusPaymentRequired)
					_, _ = w.Write([]byte(`{"error":"Requires Fleet Premium license"}`))
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer srv.Close()

			rootDir := t.TempDir()
			oc, err := fleetclient.NewOrbitClient(
				rootDir, srv.URL, "", true, "secret", nil,
				fleet.OrbitHostInfo{HardwareUUID: "uuid-1", Platform: "linux"},
				nil, nil, "", false, nil,
			)
			require.NoError(t, err)
			// Ping learns the server capabilities (web_setup_experience) from the response header.
			require.NoError(t, oc.Ping())

			require.NoError(t, processSetupExperience(oc, rootDir, func() error { return nil }))

			info, err := setupexperience.ReadSetupExperienceStatusFile(rootDir)
			require.NoError(t, err)
			require.NotNil(t, info)
			require.Equal(t, tc.wantPoller, info.Enabled)
			require.Nil(t, info.TimeFinished)

			if tc.wantPoller {
				require.Len(t, oc.ConfigReceivers, 1)
				return
			}
			require.Empty(t, oc.ConfigReceivers)
			require.NoError(t, oc.RunConfigReceivers())
			require.Zero(t, statusCalls.Load(), "a disabled setup experience must never poll the status endpoint")
		})
	}
}
