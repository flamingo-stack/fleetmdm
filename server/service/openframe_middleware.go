// OPENFRAME(mysql-multitenancy): per-request tenant pinning for the shared multi-tenant Fleet
// topology, active only in shared mode.
package service

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/google/uuid"
)

// openframeTenantHeader is the trusted tenant UUID the OpenFrame gateway injects (client-supplied
// copies are stripped upstream; fleet-service is reachable only via the gateway).
const openframeTenantHeader = "X-Tenant-Id"

type openframeTeamEnsurer interface {
	EnsureOpenframeTeamID(ctx context.Context, tenantUUID string) (uint, error)
}

// WithOpenframeTenant pins each control-plane request to the team named by the X-Tenant-Id header.
// Outside shared mode it returns next unchanged (zero overhead); in shared mode a non-exempt
// request without a resolvable tenant is rejected (fail closed).
func WithOpenframeTenant(ds openframeTeamEnsurer, logger *slog.Logger, next http.Handler) http.Handler {
	if !fleet.IsOpenframeSharedMode() {
		return next
	}
	return openframeTenantHandler(ds, logger, next)
}

// openframeTenantHandler is split out so it can be tested without toggling the cached shared-mode env.
func openframeTenantHandler(ds openframeTeamEnsurer, logger *slog.Logger, next http.Handler) http.Handler {
	var teamIDByTenantUUID sync.Map // tenant UUID → team id uint (a tenant's team id never changes)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Agent/device/MDM planes carry no gateway header — their tenant comes from the
		// authenticated host / enroll secret (openframePinHostTeam, the enrollment pins).
		if openframeTenantExemptPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		tenantUUID := strings.TrimSpace(r.Header.Get(openframeTenantHeader))
		if tenantUUID == "" {
			logger.WarnContext(ctx, "openframe shared mode: rejecting request without tenant header",
				"path", r.URL.Path, "remote_addr", r.RemoteAddr)
			encodeError(ctx, fleet.NewAuthRequiredError("missing tenant"), w)
			return
		}

		teamID, cached := teamIDByTenantUUID.Load(tenantUUID)
		if !cached {
			// Validate before resolving: EnsureOpenframeTeamID would mint a team for any string.
			if _, err := uuid.Parse(tenantUUID); err != nil {
				logger.WarnContext(ctx, "openframe shared mode: rejecting request with malformed tenant header",
					"path", r.URL.Path, "remote_addr", r.RemoteAddr)
				encodeError(ctx, fleet.NewAuthRequiredError("invalid tenant"), w)
				return
			}
			id, err := ds.EnsureOpenframeTeamID(ctx, tenantUUID)
			if err != nil {
				logger.ErrorContext(ctx, "openframe shared mode: resolving tenant team",
					"tenant_uuid", tenantUUID, "err", err)
				encodeError(ctx, err, w)
				return
			}
			teamIDByTenantUUID.Store(tenantUUID, id)
			teamID = id
		}

		next.ServeHTTP(w, r.WithContext(fleet.NewOpenframeTeamContext(ctx, teamID.(uint))))
	})
}

// openframeTenantExemptPath matches the agent/device/MDM-enrollment planes, which derive their
// tenant from the authenticated principal (host node key / enroll secret / device cert) rather
// than the gateway X-Tenant-Id header.
//
// The MDM marker is "/api/mdm/" — the device enrollment/management endpoints (Apple
// /api/mdm/apple/{enroll,installer,account_driven_enroll}, Microsoft /api/mdm/microsoft…). It is
// deliberately NOT the broad "/mdm/": the user-authenticated admin MDM APIs live under
// /api/{v}/fleet/mdm/… (e.g. .../mdm/apple/commands, .../mdm/apple/enrollment_profile) and MUST
// still require X-Tenant-Id in shared mode. The raw device protocol (/mdm/apple/scep, /mdm/apple/mdm,
// SCEP proxy) is served on the root mux and never reaches this wrapper.
//
// Residual: a few device-facing DEP endpoints live under /api/{v}/fleet/mdm/ too (GET
// .../mdm/bootstrap download, GET .../mdm/setup/eula/{token}) and share their path with admin
// variants that differ only by HTTP method — so path-only matching cannot exempt them without also
// exempting the admin route. They are NOT exempted here (they'd need X-Tenant-Id). This is
// acceptable because OpenFrame does not use Apple/Windows MDM; if it ever does, make this exemption
// method-aware. See openframe/docs/agent-ingestion-isolation.md.
func openframeTenantExemptPath(path string) bool {
	for _, marker := range []string{"/osquery/", "/fleet/orbit/", "/fleet/device/", "/api/mdm/", "/fleet/ota_enrollment"} {
		if strings.Contains(path, marker) {
			return true
		}
	}
	return false
}

// openframePinHostTeam scopes ctx to the authenticated host's team in shared mode; a host with no
// team fails auth (fail closed) rather than running unscoped. No-op outside shared mode.
func openframePinHostTeam(ctx context.Context, host *fleet.Host) (context.Context, error) {
	if !fleet.IsOpenframeSharedMode() {
		return ctx, nil
	}
	return openframePinHostTeamShared(ctx, host)
}

func openframePinHostTeamShared(ctx context.Context, host *fleet.Host) (context.Context, error) {
	if host == nil || host.TeamID == nil || *host.TeamID == 0 {
		return ctx, fleet.NewAuthFailedError("openframe shared mode: authenticated host has no team")
	}
	return fleet.NewOpenframeTeamContext(ctx, *host.TeamID), nil
}
