// OPENFRAME(osquery-schema-search): runs configurable per-replica osquery schema refreshes.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/fleetdm/fleet/v4/schema"
	configpkg "github.com/fleetdm/fleet/v4/server/config"
)

const osquerySchemaRefreshTimeout = 15 * time.Second

type osquerySchemaRefreshFunc func(context.Context) (int, error)

func startOsquerySchemaRefresh(ctx context.Context, config configpkg.OsqueryConfig, logger *slog.Logger) {
	if !config.SchemaRefreshEnabled {
		return
	}

	client := &http.Client{Timeout: osquerySchemaRefreshTimeout}
	go runOsquerySchemaRefresh(ctx, config.SchemaRefreshInterval, logger, func(ctx context.Context) (int, error) {
		return schema.RefreshOsquerySchema(ctx, client, config.SchemaRefreshURL)
	})
}

func runOsquerySchemaRefresh(
	ctx context.Context,
	interval time.Duration,
	logger *slog.Logger,
	refresh osquerySchemaRefreshFunc,
) {
	refreshAndLog := func() {
		count, err := refresh(ctx)
		if err != nil {
			logger.WarnContext(ctx, "osquery schema refresh failed; keeping previous schema", "err", err)
			return
		}
		logger.InfoContext(ctx, "osquery schema refreshed", "tables", count)
	}

	refreshAndLog()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshAndLog()
		}
	}
}
