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

func ValidateServerURL(ctx context.Context, urlString string) error {
	// TODO - implement more robust URL validation here

	// no valid scheme provided
	if !(strings.HasPrefix(urlString, "http://") || strings.HasPrefix(urlString, "https://")) {
		return ctxerr.New(ctx, fleet.InvalidServerURLMsg)
	}

	// valid scheme provided - require host
	parsed, err := url.Parse(urlString)
	if err != nil {
		return ctxerr.Wrap(ctx, err)
	}
	if parsed.Host == "" {
		return ctxerr.New(ctx, fleet.InvalidServerURLMsg)
	}

	return nil
}

