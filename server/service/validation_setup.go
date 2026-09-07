package service

import (
	"context"
	"net/url"
	"strings"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
)

func (mw validationMiddleware) NewAppConfig(ctx context.Context, payload fleet.AppConfig) (*fleet.AppConfig, error) {
	invalid := &fleet.InvalidArgumentError{}
	var serverURLString string
	if payload.ServerSettings.ServerURL == "" {
		invalid.Append("server_url", "missing required argument")
	} else {
		serverURLString = cleanupURL(payload.ServerSettings.ServerURL)
	}
	if err := ValidateServerURL(ctx, serverURLString); err != nil {
		invalid.Append("server_url", err.Error())
	}
	if invalid.HasErrors() {
		return nil, ctxerr.Wrap(ctx, invalid)
	}
	return mw.Service.NewAppConfig(ctx, payload)
}

// ValidateServerURL validates that the given URL string is well-formed and
// contains a scheme (http/https) and a host.
//
// TODO(FLEETMDM-002) - implement more robust URL validation here, see
// https://github.com/fleetdm/fleet/issues for tracking further hardening
// (e.g. rejecting suspicious userinfo/host combinations).
func ValidateServerURL(ctx context.Context, urlString string) error {
	// no valid scheme provided
	if !(strings.HasPrefix(urlString, "http://") || strings.HasPrefix(urlString, "https://")) {
		return ctxerr.New(ctx, fleet.InvalidServerURLMsg)
	}

	// valid scheme provided - require host
	parsed, err := url.Parse(urlString)
	if err != nil {
		return err
	}
	if parsed.Host == "" {
		return ctxerr.New(ctx, fleet.InvalidServerURLMsg)
	}

	return nil
}

