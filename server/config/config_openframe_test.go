// OPENFRAME(redis-key-prefix, query-results-ttl): unit tests for the
// fork-added config flags — openframe/docs/redis-key-prefix.md,
// openframe/docs/query-results-ttl-cleanup.md
//
// Both flags are registered inside large upstream flag blocks; a merge that
// re-generates that block can drop a registration with no compile error,
// leaving the tenant prefix or the TTL silently at their zero values in every
// deployment. These tests pin the defaults and the env-var plumbing.
package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/pkg/testutils"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func loadTestConfig(t *testing.T, env map[string]string) FleetConfig {
	t.Helper()
	// viper also reads the ambient environment; start from a clean one so a
	// developer's local FLEET_* vars can't skew the assertions.
	testutils.SaveEnv(t)
	os.Clearenv()
	for k, v := range env {
		t.Setenv(k, v)
	}

	cmd := &cobra.Command{}
	cmd.PersistentFlags().StringP("config", "c", "", "Path to a configuration file")
	man := NewManager(cmd)
	return man.LoadConfig()
}

func TestOpenframeConfigDefaults(t *testing.T) {
	cfg := loadTestConfig(t, nil)

	require.Empty(t, cfg.Redis.KeyPrefix, "key prefix must default to off (single-tenant Redis)")
	require.Equal(t, 60*24*time.Hour, cfg.Server.QueryResultsTTL)
	require.Equal(t, 1*time.Hour, cfg.Server.QueryResultsCleanupInterval)
	require.False(t, cfg.Osquery.SchemaRefreshEnabled)
	require.Equal(t, 24*time.Hour, cfg.Osquery.SchemaRefreshInterval)
	require.Equal(t, "https://raw.githubusercontent.com/fleetdm/fleet/2cb8509c210a7443f715bc9c2b64fc7bc85fd5f7/schema/osquery_fleet_schema.json", cfg.Osquery.SchemaRefreshURL)
}

func TestOpenframeConfigEnvOverrides(t *testing.T) {
	cfg := loadTestConfig(t, map[string]string{
		"FLEET_REDIS_KEY_PREFIX":                      "tenant-a",
		"FLEET_SERVER_QUERY_RESULTS_TTL":              "24h",
		"FLEET_SERVER_QUERY_RESULTS_CLEANUP_INTERVAL": "30m",
		"FLEET_OSQUERY_SCHEMA_REFRESH_ENABLED":        "true",
		"FLEET_OSQUERY_SCHEMA_REFRESH_INTERVAL":       "6h",
		"FLEET_OSQUERY_SCHEMA_REFRESH_URL":            "https://schema.example.test/osquery.json",
	})

	require.Equal(t, "tenant-a", cfg.Redis.KeyPrefix)
	require.Equal(t, 24*time.Hour, cfg.Server.QueryResultsTTL)
	require.Equal(t, 30*time.Minute, cfg.Server.QueryResultsCleanupInterval)
	require.True(t, cfg.Osquery.SchemaRefreshEnabled)
	require.Equal(t, 6*time.Hour, cfg.Osquery.SchemaRefreshInterval)
	require.Equal(t, "https://schema.example.test/osquery.json", cfg.Osquery.SchemaRefreshURL)
}

func TestOpenframeConfigYamlOverridesOsquerySchemaRefresh(t *testing.T) {
	testutils.SaveEnv(t)
	os.Clearenv()

	cmd := &cobra.Command{}
	cmd.PersistentFlags().StringP("config", "c", "", "Path to a configuration file")
	man := NewManager(cmd)
	man.viper.SetConfigType("yaml")
	require.NoError(t, man.viper.ReadConfig(strings.NewReader(`
osquery:
  schema_refresh_enabled: true
  schema_refresh_interval: 12h
  schema_refresh_url: https://schema.example.test/fleet.json
`)))

	cfg := man.LoadConfig()
	require.True(t, cfg.Osquery.SchemaRefreshEnabled)
	require.Equal(t, 12*time.Hour, cfg.Osquery.SchemaRefreshInterval)
	require.Equal(t, "https://schema.example.test/fleet.json", cfg.Osquery.SchemaRefreshURL)
}

func TestOsquerySchemaRefreshConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      OsqueryConfig
		errorReason string
	}{
		{
			name: "enabled with zero interval",
			config: OsqueryConfig{
				HostIdentifier:        "provided",
				SchemaRefreshEnabled:  true,
				SchemaRefreshInterval: 0,
				SchemaRefreshURL:      "https://schema.example.test/osquery.json",
			},
			errorReason: "interval must be greater than zero",
		},
		{
			name: "enabled with unsupported URL scheme",
			config: OsqueryConfig{
				HostIdentifier:        "provided",
				SchemaRefreshEnabled:  true,
				SchemaRefreshInterval: time.Hour,
				SchemaRefreshURL:      "file:///tmp/osquery.json",
			},
			errorReason: "URL must use http or https",
		},
		{
			name: "disabled ignores refresh settings",
			config: OsqueryConfig{
				HostIdentifier:        "provided",
				SchemaRefreshEnabled:  false,
				SchemaRefreshInterval: 0,
				SchemaRefreshURL:      "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var validationErr error
			tt.config.Validate(func(err error, _ string) {
				validationErr = err
			})
			if tt.errorReason == "" {
				require.NoError(t, validationErr)
				return
			}
			require.ErrorContains(t, validationErr, tt.errorReason)
		})
	}
}
