package schema

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const refreshedSchemaFixture = `[
  {
    "name": "openframe_refresh_probe",
    "platforms": ["all"],
    "description": "OpenFrame schema refresh probe table",
    "columns": [
      {
        "name": "probe_value",
        "type": "text",
        "description": "Probe value",
        "notes": null,
        "required": false,
        "hidden": null
      }
    ],
    "examples": null,
    "notes": null,
    "evented": false,
    "cacheable": false
  }
]`

func preserveOsquerySchema(t *testing.T) {
	t.Helper()
	tables, err := currentOsqueryTables()
	require.NoError(t, err)
	t.Cleanup(func() {
		storeOsqueryTables(tables)
	})
}

func TestRefreshOsquerySchemaReplacesSearchableSnapshot(t *testing.T) {
	preserveOsquerySchema(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(refreshedSchemaFixture))
	}))
	t.Cleanup(server.Close)

	count, err := RefreshOsquerySchema(t.Context(), server.Client(), server.URL)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	tables, err := SearchOsqueryTables("openframe refresh probe", "all", 5)
	require.NoError(t, err)
	require.Len(t, tables, 1)
	require.Equal(t, "openframe_refresh_probe", tables[0].Name)
	require.Equal(t, "probe_value", tables[0].Columns[0].Name)
}

func TestRefreshOsquerySchemaFailureKeepsPreviousSnapshot(t *testing.T) {
	preserveOsquerySchema(t)
	validServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(refreshedSchemaFixture))
	}))
	t.Cleanup(validServer.Close)
	_, err := RefreshOsquerySchema(t.Context(), validServer.Client(), validServer.URL)
	require.NoError(t, err)

	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "http error", status: http.StatusBadGateway, body: `{"message":"unavailable"}`},
		{name: "malformed json", status: http.StatusOK, body: `{`},
		{name: "empty schema", status: http.StatusOK, body: `[]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)

			_, err := RefreshOsquerySchema(t.Context(), server.Client(), server.URL)
			require.Error(t, err)

			tables, searchErr := SearchOsqueryTables("openframe refresh probe", "all", 5)
			require.NoError(t, searchErr)
			require.Len(t, tables, 1)
			require.Equal(t, "openframe_refresh_probe", tables[0].Name)
		})
	}
}

func TestRefreshOsquerySchemaRejectsInvalidStructureAndKeepsPreviousSnapshot(t *testing.T) {
	preserveOsquerySchema(t)
	baseline, err := parseOsquerySchemaJSON([]byte(refreshedSchemaFixture))
	require.NoError(t, err)

	tests := []struct {
		name string
		body string
	}{
		{name: "missing table name", body: `[{"name":"","columns":[{"name":"value","type":"text"}]}]`},
		{name: "missing columns", body: `[{"name":"broken_table","columns":[]}]`},
		{name: "duplicate table name", body: `[{"name":"duplicate","columns":[{"name":"one","type":"text"}]},{"name":"duplicate","columns":[{"name":"two","type":"text"}]}]`},
		{name: "missing column name", body: `[{"name":"broken_table","columns":[{"name":"","type":"text"}]}]`},
		{name: "missing column type", body: `[{"name":"broken_table","columns":[{"name":"value","type":""}]}]`},
		{name: "duplicate column name", body: `[{"name":"broken_table","columns":[{"name":"value","type":"text"},{"name":"value","type":"integer"}]}]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storeOsqueryTables(baseline)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)

			_, err := RefreshOsquerySchema(t.Context(), server.Client(), server.URL)
			require.Error(t, err)

			tables, searchErr := SearchOsqueryTables("openframe refresh probe", "all", 5)
			require.NoError(t, searchErr)
			require.Len(t, tables, 1)
			require.Equal(t, "openframe_refresh_probe", tables[0].Name)
		})
	}
}

func TestRefreshOsquerySchemaRejectsOversizedResponse(t *testing.T) {
	preserveOsquerySchema(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxOsquerySchemaSize+1)))
	}))
	t.Cleanup(server.Close)

	_, err := RefreshOsquerySchema(t.Context(), server.Client(), server.URL)
	require.ErrorContains(t, err, "exceeds")
}

func TestRefreshOsquerySchemaUsesRequestContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := RefreshOsquerySchema(ctx, http.DefaultClient, "https://schema.example.test/osquery.json")
	require.ErrorIs(t, err, context.Canceled)
}
