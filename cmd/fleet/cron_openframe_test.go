// OPENFRAME(query-results-ttl): unit test for the query_results TTL cleanup
// cron — openframe/docs/query-results-ttl-cleanup.md
//
// This was the only piece of the query-results-ttl slug with no test at all:
// the schedule construction and the cutoff math (now - TTL) that decides which
// rows die. mock.Store stands in for the locker/stats store, so no MySQL.
package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/config"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mock"
	"github.com/stretchr/testify/require"
)

func TestNewQueryResultsTTLCleanupSchedule(t *testing.T) {
	const ttl = 24 * time.Hour

	ds := new(mock.Store)
	ds.LockFunc = func(ctx context.Context, name, owner string, expiration time.Duration) (bool, error) {
		return true, nil
	}
	ds.UnlockFunc = func(ctx context.Context, name, owner string) error { return nil }
	// A completed run one hour ago makes the schedule due on its first tick.
	ds.GetLatestCronStatsFunc = func(ctx context.Context, name string) ([]fleet.CronStats, error) {
		return []fleet.CronStats{{
			ID:        1,
			StatsType: fleet.CronStatsTypeScheduled,
			Name:      name,
			Status:    fleet.CronStatsStatusCompleted,
			CreatedAt: time.Now().Add(-time.Hour),
			UpdatedAt: time.Now().Add(-time.Hour),
		}}, nil
	}
	ds.InsertCronStatsFunc = func(ctx context.Context, statsType fleet.CronStatsType, name, instance string, status fleet.CronStatsStatus) (int, error) {
		return 1, nil
	}
	ds.UpdateCronStatsFunc = func(ctx context.Context, id int, status fleet.CronStatsStatus, cronErrors *fleet.CronScheduleErrors) error {
		return nil
	}

	cutoffCh := make(chan time.Time, 1)
	ds.CleanupExpiredQueryResultsFunc = func(ctx context.Context, expiredBefore time.Time) (int64, error) {
		select {
		case cutoffCh <- expiredBefore:
		default:
		}
		return 5, nil
	}

	cfg := config.FleetConfig{}
	cfg.Server.QueryResultsTTL = ttl
	// The schedule floors sub-second intervals to 1s; use 1s explicitly so the
	// test's timing expectations match what actually runs.
	cfg.Server.QueryResultsCleanupInterval = time.Second

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	s, err := newQueryResultsTTLCleanupSchedule(ctx, "test_instance", ds, &cfg, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	require.Equal(t, string(fleet.CronQueryResultsTTLCleanup), s.Name())

	s.Start()

	select {
	case cutoff := <-cutoffCh:
		// The job must delete rows older than now - TTL; allow generous slack
		// for scheduling delay.
		require.WithinDuration(t, time.Now().Add(-ttl), cutoff, time.Minute)
	case <-time.After(10 * time.Second):
		t.Fatal("the TTL cleanup job never ran")
	}

	cancel()
	select {
	case <-s.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("schedule did not stop")
	}
}
