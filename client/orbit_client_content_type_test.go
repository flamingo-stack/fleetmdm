package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/require"
)

// >>> OPENFRAME(agent-json-content-type): guards the header against an upstream refactor of the
// request builders, which would drop it with no git conflict — openframe/docs/agent-json-content-type.md

func TestOrbitClientJSONContentType(t *testing.T) {
	var gotMethod, gotContentType string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotContentType = r.Method, r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)

		if strings.HasSuffix(r.URL.Path, "/orbit/enroll") {
			writeEnrollResponse(t, w, "a-node-key")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, nodeKeyPath := newNodeKeyFile(t, "existing-key")

	t.Run("request with a body declares application/json", func(t *testing.T) {
		key, err := newReenrollTestClient(t, srv.URL, nodeKeyPath).enroll()
		require.NoError(t, err)
		require.Equal(t, "a-node-key", key)

		require.Equal(t, http.MethodPost, gotMethod)
		require.NotEmpty(t, gotBody)
		require.Equal(t, "application/json", gotContentType)
	})

	t.Run("bodyless request declares no content type", func(t *testing.T) {
		gotContentType = "sentinel"
		require.NoError(t, newReenrollTestClient(t, srv.URL, nodeKeyPath).Ping())

		require.Equal(t, http.MethodHead, gotMethod)
		require.Empty(t, gotContentType)
	})
}

func TestDeviceClientJSONContentType(t *testing.T) {
	var gotContentType string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dc, err := NewDeviceClient(srv.URL, true, "", nil, "")
	require.NoError(t, err)

	t.Run("request with a body declares application/json", func(t *testing.T) {
		require.NoError(t, dc.ReportError("a-token", fleet.FleetdError{ErrorSource: "orbit"}))

		require.NotEmpty(t, gotBody)
		require.Equal(t, "application/json", gotContentType)
	})

	t.Run("bodyless request declares no content type", func(t *testing.T) {
		gotContentType = "sentinel"
		require.NoError(t, dc.CheckToken("a-token"))

		require.Empty(t, gotContentType)
	})
}

// <<< OPENFRAME(agent-json-content-type)
