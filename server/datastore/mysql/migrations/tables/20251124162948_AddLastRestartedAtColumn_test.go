package tables

import (
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestUp_20251124162948(t *testing.T) {
	db := applyUpToPrev(t)

	// uptimeNanos is the raw uptime value (in nanoseconds, as osquery reports it)
	// used for hosts that should have a computed last_restarted_at. It represents
	// roughly 27.6 days of uptime, expressed here as an explicit duration so the
	// same value used to seed the row is reused to compute the expected result,
	// rather than relying on a duplicated magic number.
	const uptimeDuration = 2388335 * time.Millisecond // ~27.6 days
	uptimeNanos := uptimeDuration.Nanoseconds()

	// detailUpdatedAt is the UTC timestamp used as the base for the migration's
	// last_restarted_at calculation (detail_updated_at - uptime).
	detailUpdatedAt := time.Date(2025, 11, 4, 23, 7, 56, 0, time.UTC)

	// Insert test hosts with various uptimes and detail_updated_at values.
	host1ID := execNoErrLastID(t, db, `INSERT INTO hosts (osquery_host_id, node_key, uuid, platform, uptime, detail_updated_at) VALUES (?, ?, ?, ?, ?, ?)`, "host1", "key1", "uuid1", "darwin", uptimeNanos, detailUpdatedAt.Format("2006-01-02 15:04:05"))
	host2ID := execNoErrLastID(t, db, `INSERT INTO hosts (osquery_host_id, node_key, uuid, platform, uptime, detail_updated_at) VALUES (?, ?, ?, ?, ?, ?)`, "host2", "key2", "uuid2", "darwin", 0, detailUpdatedAt.Format("2006-01-02 15:04:05"))
	host3ID := execNoErrLastID(t, db, `INSERT INTO hosts (osquery_host_id, node_key, uuid, platform, uptime, detail_updated_at) VALUES (?, ?, ?, ?, ?, ?)`, "host3", "key3", "uuid3", "darwin", uptimeNanos, nil)

	// Apply current migration.
	applyNext(t, db)

	var hosts []struct {
		HostID          string    `db:"id"`
		LastRestartedAt time.Time `db:"last_restarted_at"`
	}
	err := sqlx.Select(db, &hosts, `SELECT id, last_restarted_at FROM hosts ORDER BY id`)
	require.NoError(t, err)
	require.Len(t, hosts, 3)

	// This host has uptime and detail_updated_at, so we can calculate last_restarted_at.
	require.Equal(t, fmt.Sprint(host1ID), hosts[0].HostID)
	expectedRestartedAt1 := detailUpdatedAt.Add(-uptimeDuration)
	require.Equal(t, expectedRestartedAt1, hosts[0].LastRestartedAt)

	// This host has 0 uptime, so last_restarted_at should be zero time.
	require.Equal(t, fmt.Sprint(host2ID), hosts[1].HostID)
	expectedRestartedAt2 := time.Date(0o001, 1, 1, 0, 0, 0, 0, time.UTC)
	require.Equal(t, expectedRestartedAt2, hosts[1].LastRestartedAt)

	// This host has nil detail_updated_at, so last_restarted_at should be zero time.
	require.Equal(t, fmt.Sprint(host3ID), hosts[2].HostID)
	expectedRestartedAt3 := time.Date(0o001, 1, 1, 0, 0, 0, 0, time.UTC)
	require.Equal(t, expectedRestartedAt3, hosts[2].LastRestartedAt)
}
