package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// OPENFRAME(enroll-machine-identity): openframe/docs/enroll-machine-identity.md

func TestMoveToIdentifierSendsHeldKeyAndWritesNewKey(t *testing.T) {
	_, nodeKeyPath := newNodeKeyFile(t, "held-key\n")
	var got fleet.EnrollOrbitRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/orbit/enroll") {
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&got))
			writeEnrollResponse(t, w, "new-key")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	oc := newReenrollTestClient(t, srv.URL, nodeKeyPath)

	err := oc.MoveToIdentifier("machine-1")

	require.NoError(t, err)
	require.Equal(t, "held-key", got.PreviousOrbitNodeKey)
	require.Equal(t, "machine-1", got.OsqueryIdentifier)
	requireNodeKey(t, nodeKeyPath, "new-key")
	require.Equal(t, "machine-1", oc.hostInfo.OsqueryIdentifier)
	require.Empty(t, oc.hostInfo.PreviousOrbitNodeKey)
}

func TestMoveToIdentifierFailureKeepsIdentifierAndKey(t *testing.T) {
	_, nodeKeyPath := newNodeKeyFile(t, "held-key")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	oc := newReenrollTestClient(t, srv.URL, nodeKeyPath)

	err := oc.MoveToIdentifier("machine-1")

	require.Error(t, err)
	requireNodeKey(t, nodeKeyPath, "held-key")
	require.Empty(t, oc.hostInfo.OsqueryIdentifier)
	require.Empty(t, oc.hostInfo.PreviousOrbitNodeKey)
}

func TestMoveToIdentifierWithoutNodeKeyFails(t *testing.T) {
	_, nodeKeyPath := newNodeKeyFile(t, "  ")
	oc := newReenrollTestClient(t, "http://127.0.0.1:1", nodeKeyPath)

	err := oc.MoveToIdentifier("machine-1")

	require.ErrorContains(t, err, "empty")
}

func TestSupportsIdentifierMoveReadsServerCapability(t *testing.T) {
	for name, header := range map[string]string{"supported": string(fleet.CapabilityOpenframeIdentifierMove), "not supported": ""} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set(fleet.CapabilitiesHeader, header)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()
			oc := newReenrollTestClient(t, srv.URL, "")

			supported, err := oc.SupportsIdentifierMove()

			require.NoError(t, err)
			require.Equal(t, header != "", supported)
		})
	}
}
