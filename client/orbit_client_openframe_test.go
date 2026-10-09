// OPENFRAME(agent-openframe-mode): unit tests for the orbit client's openframe
// wiring — openframe/docs/agent-openframe-mode.md
//
// Guards the two request-path fork edits an upstream refactor of the client
// would silently drop: the Bearer token injected from the auth manager on
// every request, and the /tools/agent/fleetmdm-server URL prefix that routes
// the agent through the OpenFrame gateway.
package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/service/openframe"
	"github.com/stretchr/testify/require"
)

// newOpenframeTestClient builds an OrbitClient in openframe mode wired to the
// given auth manager, mirroring newReenrollTestClient.
func newOpenframeTestClient(t *testing.T, serverURL string, mgr *openframe.OpenFrameAuthorizationManager) *OrbitClient {
	t.Helper()
	bc, err := NewBaseClient(serverURL, true, "", "", nil, fleet.CapabilityMap{}, nil)
	require.NoError(t, err)
	_, nodeKeyPath := newNodeKeyFile(t, "a-node-key")
	return &OrbitClient{
		BaseClient:      bc,
		nodeKeyFilePath: nodeKeyPath,
		enrollSecret:    "secret",
		hostInfo:        fleet.OrbitHostInfo{HardwareUUID: "uuid-1", Platform: "linux"},
		openFrameMode:   true,
		authManager:     mgr,
	}
}

func TestOrbitClientOpenframeBearerAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Run("openframe mode injects the manager's token", func(t *testing.T) {
		oc := newOpenframeTestClient(t, srv.URL, openframe.NewOpenFrameAuthorizationManagerWithToken("tok-1"))
		require.NoError(t, oc.Ping())
		require.Equal(t, "Bearer tok-1", gotAuth)
	})

	t.Run("a rotated token is used on the next request", func(t *testing.T) {
		mgr := openframe.NewOpenFrameAuthorizationManagerWithToken("tok-1")
		oc := newOpenframeTestClient(t, srv.URL, mgr)
		require.NoError(t, oc.Ping())
		mgr.UpdateToken("tok-2")
		require.NoError(t, oc.Ping())
		require.Equal(t, "Bearer tok-2", gotAuth)
	})

	t.Run("empty token sends no Authorization header", func(t *testing.T) {
		gotAuth = "sentinel"
		oc := newOpenframeTestClient(t, srv.URL, openframe.NewOpenFrameAuthorizationManager())
		require.NoError(t, oc.Ping())
		require.Empty(t, gotAuth)
	})

	t.Run("non-openframe mode sends no Authorization header", func(t *testing.T) {
		gotAuth = "sentinel"
		_, nodeKeyPath := newNodeKeyFile(t, "a-node-key")
		oc := newReenrollTestClient(t, srv.URL, nodeKeyPath)
		require.NoError(t, oc.Ping())
		require.Empty(t, gotAuth)
	})
}

func TestNewOrbitClientOpenframeURLPrefix(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hostInfo := fleet.OrbitHostInfo{HardwareUUID: "uuid-1", Platform: "linux"}

	t.Run("openframe mode routes through the tools-agent prefix", func(t *testing.T) {
		oc, err := NewOrbitClient(
			t.TempDir(), srv.URL, "", true, "secret", nil, hostInfo, nil, nil, "",
			true, openframe.NewOpenFrameAuthorizationManagerWithToken("tok"),
		)
		require.NoError(t, err)
		require.NoError(t, oc.Ping())
		require.Equal(t, "/tools/agent/fleetmdm-server/api/fleet/orbit/ping", gotPath)
	})

	t.Run("non-openframe mode uses the plain path", func(t *testing.T) {
		oc, err := NewOrbitClient(
			t.TempDir(), srv.URL, "", true, "secret", nil, hostInfo, nil, nil, "",
			false, nil,
		)
		require.NoError(t, err)
		require.NoError(t, oc.Ping())
		require.Equal(t, "/api/fleet/orbit/ping", gotPath)
	})
}
