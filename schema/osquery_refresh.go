// OPENFRAME(osquery-schema-search): refreshes the searchable osquery schema while preserving the embedded fallback.
package schema

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

const maxOsquerySchemaSize = 16 * 1024 * 1024

func RefreshOsquerySchema(ctx context.Context, client *http.Client, sourceURL string) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return 0, fmt.Errorf("build osquery schema request: %w", err)
	}
	request.Header.Set("User-Agent", "fleet-osquery-schema-refresh")

	response, err := client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("fetch osquery schema: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("fetch osquery schema: HTTP %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxOsquerySchemaSize+1))
	if err != nil {
		return 0, fmt.Errorf("read osquery schema: %w", err)
	}
	if len(body) > maxOsquerySchemaSize {
		return 0, fmt.Errorf("osquery schema exceeds %d bytes", maxOsquerySchemaSize)
	}

	tables, err := parseOsquerySchemaJSON(body)
	if err != nil {
		return 0, err
	}
	storeOsqueryTables(tables)
	return len(tables), nil
}
