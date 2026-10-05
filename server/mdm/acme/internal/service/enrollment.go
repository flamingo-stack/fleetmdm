package service

import (
	"context"

	authz_ctx "github.com/fleetdm/fleet/v4/server/contexts/authz"
)

// >>> OPENFRAME(acme-enrollment): fork-specific ACME enrollment service — openframe/docs/acme-mdm.md
func (s *Service) NewACMEEnrollment(ctx context.Context, hostIdentifier string) (string, error) {
	// skipauth: No authorization check needed; caller is authenticated via DEP device identity.
	if az, ok := authz_ctx.FromContext(ctx); ok {
		az.SetChecked()
	}

	return s.store.NewEnrollment(ctx, hostIdentifier)
}

// <<< OPENFRAM(acme-enrollment)
