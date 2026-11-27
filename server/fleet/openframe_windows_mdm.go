// >>> OPENFRAME(windows-mdm-tenant): the tenant's public Windows MDM address — openframe/docs/mysql-multitenancy-feature.md

package fleet

import (
	"errors"
	"net/url"
	"strings"
)

var errOpenframeWindowsServerURL = errors.New("must be an https URL with a valid host and no query, fragment, credentials or whitespace")

// ParseOpenframeWindowsServerURL checks a tenant's mdm.windows_server_url and returns it with a
// lower-case host and no trailing slash.
func ParseOpenframeWindowsServerURL(raw string) (string, error) {
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	if s == "" || strings.ContainsAny(s, "?# \t\n\r") {
		return "", errOpenframeWindowsServerURL
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.User != nil || !openframeValidHostPort(u.Host) {
		return "", errOpenframeWindowsServerURL
	}
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

// <<< OPENFRAME(windows-mdm-tenant)
