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
}

func TestOpenframeConfigEnvOverrides(t *testing.T) {
	cfg := loadTestConfig(t, map[string]string{
		"FLEET_REDIS_KEY_PREFIX":                      "tenant-a",
		"FLEET_SERVER_QUERY_RESULTS_TTL":              "24h",
		"FLEET_SERVER_QUERY_RESULTS_CLEANUP_INTERVAL": "30m",
	})

	require.Equal(t, "tenant-a", cfg.Redis.KeyPrefix)
	require.Equal(t, 24*time.Hour, cfg.Server.QueryResultsTTL)
	require.Equal(t, 30*time.Minute, cfg.Server.QueryResultsCleanupInterval)
}

func TestOpenframeConfigRejectsRemovedOsquerySchemaRefreshFlags(t *testing.T) {
	for _, flag := range []string{
		"--osquery_schema_refresh_enabled=true",
		"--osquery_schema_refresh_interval=24h",
		"--osquery_schema_refresh_url=https://schema.example.test/osquery.json",
	} {
		t.Run(flag, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.PersistentFlags().StringP("config", "c", "", "Path to a configuration file")
			NewManager(cmd)

			err := cmd.ParseFlags([]string{flag})

			require.ErrorContains(t, err, "unknown flag")
		})
	}
}
