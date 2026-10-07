package commonmdm

import (
	"fmt"
	"net/url"
	"path"
)

// ResolveURL resolves a relative path to a server URL (typically the Fleet
// server's). If cleanQuery is true, the query string part is cleared.
func ResolveURL(serverURL, relPath string, cleanQuery bool) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("parsing server URL: %w", err)
	}
	u.Path = path.Join(u.Path, relPath)
	if cleanQuery {
		u.RawQuery = ""
	}
	return u.String(), nil
}
