package fleet

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
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
	if (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || !openframeValidHostPort(u.Host) ||
		strings.ContainsAny(settings.ServerURL, "?# \t\n\r") {
		return OpenframeInstanceSettings{}, fmt.Errorf("invalid FLEET_OPENFRAME_INSTANCE_SERVER_URL %q: must be an http(s) URL with a valid host and port and no query, fragment, credentials or whitespace", rawServerURL)
	}
	u.Host = strings.ToLower(u.Host)
	settings.ServerURL = u.String()
	return settings, nil
}

func openframeValidHostPort(hostPort string) bool {
	host, port := hostPort, ""
	if i := strings.LastIndex(hostPort, ":"); i >= 0 && !strings.HasSuffix(hostPort, "]") {
		host, port = hostPort[:i], hostPort[i+1:]
		if port == "" {
			return false
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		ip := net.ParseIP(host[1 : len(host)-1])
		return ip != nil && ip.To4() == nil
	}
	if host == "" || len(host) > 253 {
		return false
	}
	for label := range strings.SplitSeq(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
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
