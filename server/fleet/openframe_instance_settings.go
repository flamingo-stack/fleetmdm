package fleet

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type OpenframeInstanceSettings struct {
	ServerURL string
	OrgName   string
}

func OpenframeInstanceSettingsFromEnv() (OpenframeInstanceSettings, error) {
	return parseOpenframeInstanceSettings(
		os.Getenv("FLEET_OPENFRAME_INSTANCE_SERVER_URL"),
		os.Getenv("FLEET_OPENFRAME_INSTANCE_ORG_NAME"),
	)
}

func parseOpenframeInstanceSettings(rawServerURL, rawOrgName string) (OpenframeInstanceSettings, error) {
	settings := OpenframeInstanceSettings{
		ServerURL: strings.TrimRight(strings.TrimSpace(rawServerURL), "/"),
		OrgName:   strings.TrimSpace(rawOrgName),
	}
	if settings.ServerURL == "" {
		return settings, nil
	}
	u, err := url.Parse(settings.ServerURL)
	if err != nil {
		return OpenframeInstanceSettings{}, fmt.Errorf("invalid FLEET_OPENFRAME_INSTANCE_SERVER_URL %q: %w", rawServerURL, err)
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || strings.ContainsAny(settings.ServerURL, "?#") {
		return OpenframeInstanceSettings{}, fmt.Errorf("invalid FLEET_OPENFRAME_INSTANCE_SERVER_URL %q: must be an http(s) URL with a host and no query, fragment or credentials", rawServerURL)
	}
	return settings, nil
}

func (s OpenframeInstanceSettings) ApplyTo(cfg *AppConfig) {
	if s.ServerURL != "" {
		cfg.ServerSettings.ServerURL = s.ServerURL
	}
	if s.OrgName != "" {
		cfg.OrgInfo.OrgName = s.OrgName
	}
}

type openframeInstanceConfigCtxKey struct{}

func NewOpenframeInstanceConfigContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, openframeInstanceConfigCtxKey{}, true)
}

func OpenframeInstanceConfigContext(ctx context.Context) context.Context {
	if IsOpenframeSharedMode() {
		return NewOpenframeInstanceConfigContext(ctx)
	}
	return ctx
}

func OpenframeInstanceConfigFromContext(ctx context.Context) bool {
	instance, _ := ctx.Value(openframeInstanceConfigCtxKey{}).(bool)
	return instance
}

func OpenframeAppConfigTeamID(ctx context.Context) (uint, bool) {
	if OpenframeInstanceConfigFromContext(ctx) {
		return 0, false
	}
	return OpenframeTeamID(ctx)
}
