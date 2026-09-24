package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunOsquerySchemaRefreshRunsImmediatelyAndOnInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		go runOsquerySchemaRefresh(ctx, time.Hour, discardOsquerySchemaRefreshLogger(), func(context.Context) (int, error) {
			return int(calls.Add(1)), nil
		})

		synctest.Wait()
		require.Equal(t, int32(1), calls.Load())

		time.Sleep(time.Hour + time.Nanosecond)
		synctest.Wait()
		require.Equal(t, int32(2), calls.Load())
	})
}

func TestRunOsquerySchemaRefreshRetriesAfterFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		go runOsquerySchemaRefresh(ctx, time.Hour, discardOsquerySchemaRefreshLogger(), func(context.Context) (int, error) {
			if calls.Add(1) == 1 {
				return 0, errors.New("schema source unavailable")
			}
			return 1, nil
		})

		synctest.Wait()
		require.Equal(t, int32(1), calls.Load())

		time.Sleep(time.Hour + time.Nanosecond)
		synctest.Wait()
		require.Equal(t, int32(2), calls.Load())
	})
}

func TestRunOsquerySchemaRefreshStopsWhenContextIsCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		ctx, cancel := context.WithCancel(t.Context())

		go runOsquerySchemaRefresh(ctx, time.Hour, discardOsquerySchemaRefreshLogger(), func(context.Context) (int, error) {
			return int(calls.Add(1)), nil
		})

		synctest.Wait()
		cancel()
		synctest.Wait()
		time.Sleep(time.Hour + time.Nanosecond)
		synctest.Wait()
		require.Equal(t, int32(1), calls.Load())
	})
}

func discardOsquerySchemaRefreshLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
