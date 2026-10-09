// OPENFRAME(mysql-multitenancy): tests for the shared-mode per-request tenant pin.
// The shared-mode gates (WithOpenframeTenant / openframePinHostTeam) read a cached env
// decision, so the tests exercise the mode-independent bodies (openframeTenantHandler,
// openframePinHostTeamShared) directly — same pattern as the pure-function tests in
// server/fleet/openframe_test.go.
package service

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTeamEnsurer struct {
	teamID uint
	err    error
	calls  int
}

func (f *fakeTeamEnsurer) EnsureOpenframeTeamID(_ context.Context, _ string) (uint, error) {
	f.calls++
	return f.teamID, f.err
}

func testTenantHandler(t *testing.T, ensurer *fakeTeamEnsurer) http.Handler {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return openframeTenantHandler(ensurer, slog.New(slog.DiscardHandler), next)
}

func TestOpenframeTenantHandlerPinsFromHeader(t *testing.T) {
	ensurer := &fakeTeamEnsurer{teamID: 42}
	var gotTeam uint
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTeam, gotOK = fleet.OpenframeTeamID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	h := openframeTenantHandler(ensurer, slog.New(slog.DiscardHandler), next)

	const tenantUUID = "3f1a9b2c-0000-4d5e-8f00-000000000001"
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/api/latest/fleet/hosts", nil)
		req.Header.Set("X-Tenant-Id", tenantUUID)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		require.True(t, gotOK, "request ctx must carry the tenant team pin")
		require.Equal(t, uint(42), gotTeam)
	}
	// uuid → team id never changes: resolved once, then served from the cache.
	assert.Equal(t, 1, ensurer.calls)
}

func TestOpenframeTenantHandlerFailClosed(t *testing.T) {
	t.Run("missing header on a control-plane path is rejected", func(t *testing.T) {
		ensurer := &fakeTeamEnsurer{teamID: 42}
		h := testTenantHandler(t, ensurer)
		req := httptest.NewRequest("GET", "/api/latest/fleet/hosts", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Zero(t, ensurer.calls)
	})

	t.Run("malformed tenant uuid is rejected and mints no team", func(t *testing.T) {
		ensurer := &fakeTeamEnsurer{teamID: 42}
		h := testTenantHandler(t, ensurer)
		req := httptest.NewRequest("GET", "/api/latest/fleet/hosts", nil)
		req.Header.Set("X-Tenant-Id", "not-a-uuid")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Zero(t, ensurer.calls, "a malformed uuid must never reach EnsureOpenframeTeamID")
	})

	t.Run("resolver error does not pass the request through", func(t *testing.T) {
		ensurer := &fakeTeamEnsurer{err: context.DeadlineExceeded}
		reached := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true })
		h := openframeTenantHandler(ensurer, slog.New(slog.DiscardHandler), next)
		req := httptest.NewRequest("GET", "/api/latest/fleet/hosts", nil)
		req.Header.Set("X-Tenant-Id", "3f1a9b2c-0000-4d5e-8f00-000000000001")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		require.NotEqual(t, http.StatusOK, rr.Code)
		assert.False(t, reached, "a request whose tenant cannot be resolved must not run")
	})
}

func TestOpenframeTenantHandlerAgentPathsExempt(t *testing.T) {
	ensurer := &fakeTeamEnsurer{teamID: 42}
	var pinnedOK bool
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		_, pinnedOK = fleet.OpenframeTeamID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	h := openframeTenantHandler(ensurer, slog.New(slog.DiscardHandler), next)

	// No header, agent-plane path → passes through unpinned; the tenant is derived later
	// from the authenticated host / enroll secret.
	req := httptest.NewRequest("POST", "/api/v1/osquery/config", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.True(t, reached)
	assert.False(t, pinnedOK)
	assert.Zero(t, ensurer.calls)
}

func TestOpenframeTenantExemptPath(t *testing.T) {
	exempt := []string{
		"/api/v1/osquery/enroll",
		"/api/osquery/log",
		"/api/osquery/carve/block",
		"/api/fleet/orbit/enroll",
		"/api/fleet/orbit/ping",
		"/api/latest/fleet/orbit/config",
		"/api/latest/fleet/device/token123/desktop",
		"/api/fleet/device/ping",
		"/api/mdm/apple/enroll",
		"/api/mdm/apple/installer",
		"/api/mdm/apple/account_driven_enroll",
		"/api/mdm/microsoft/discovery",
		"/api/latest/fleet/ota_enrollment",
	}
	for _, p := range exempt {
		assert.True(t, openframeTenantExemptPath(p), "expected exempt: %s", p)
	}

	notExempt := []string{
		"/api/latest/fleet/hosts",
		"/api/latest/fleet/hosts/1",
		"/api/latest/fleet/config",
		"/api/latest/fleet/software",
		"/api/latest/fleet/login",
		"/api/latest/fleet/queries",
		// User-authenticated admin MDM APIs — must still require X-Tenant-Id in shared mode
		// (these are under /api/{v}/fleet/mdm/, not /api/mdm/).
		"/api/latest/fleet/mdm/apple/commands",
		"/api/latest/fleet/mdm/apple/enrollment_profile",
		"/api/v1/fleet/mdm/commands",
		// Device-facing DEP paths that share their path with admin variants (method-only difference);
		// not exempt by path alone. Acceptable — OpenFrame does not use Apple/Windows MDM.
		"/api/latest/fleet/mdm/bootstrap",
		"/api/latest/fleet/mdm/setup/eula/token123",
	}
	for _, p := range notExempt {
		assert.False(t, openframeTenantExemptPath(p), "expected NOT exempt: %s", p)
	}
}

func TestOpenframePinHostTeamShared(t *testing.T) {
	ctx := context.Background()

	t.Run("host with a team pins the ctx", func(t *testing.T) {
		teamID := uint(7)
		got, err := openframePinHostTeamShared(ctx, &fleet.Host{TeamID: &teamID})
		require.NoError(t, err)
		id, ok := fleet.OpenframeTeamID(got)
		require.True(t, ok)
		assert.Equal(t, uint(7), id)
	})

	t.Run("host without a team fails auth", func(t *testing.T) {
		_, err := openframePinHostTeamShared(ctx, &fleet.Host{})
		require.Error(t, err)
		var authFailed *fleet.AuthFailedError
		assert.ErrorAs(t, err, &authFailed)
	})

	t.Run("nil host fails auth", func(t *testing.T) {
		_, err := openframePinHostTeamShared(ctx, nil)
		require.Error(t, err)
	})
}
