// OPENFRAME(managed-queries): queries twin of policies_openframe_managed_test.go — verifies that
// `queries.openframe_managed` keeps a query out of the listing and its counts while leaving by-id
// reads intact — openframe/docs/managed-queries.md
package mysql

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/require"
)

func TestOpenframeManagedQueries(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := context.Background()

	newQuery := func(name string, managed bool) *fleet.Query {
		q, err := ds.NewQuery(ctx, &fleet.Query{
			Name:             name,
			Query:            "SELECT 1",
			Saved:            true,
			Logging:          fleet.LoggingSnapshot,
			OpenframeManaged: managed,
		})
		require.NoError(t, err)
		return q
	}

	visible := newQuery("openframe-managed-visible", false)
	require.False(t, visible.OpenframeManaged)

	managed := newQuery("openframe-managed-platform", true)
	require.True(t, managed.OpenframeManaged, "create must round-trip the flag")

	queryNames := func(queries []*fleet.Query) []string {
		names := make([]string, 0, len(queries))
		for _, q := range queries {
			names = append(names, q.Name)
		}
		return names
	}

	t.Run("excluded from the listing and its counts", func(t *testing.T) {
		queries, total, _, _, err := ds.ListQueries(ctx, fleet.ListQueryOptions{})
		require.NoError(t, err)
		require.Equal(t, []string{visible.Name}, queryNames(queries))
		require.Equal(t, 1, total, "managed queries must not inflate the count")
	})

	t.Run("included when opted in", func(t *testing.T) {
		opts := fleet.ListQueryOptions{IncludeOpenframeManaged: true}
		queries, total, _, _, err := ds.ListQueries(ctx, opts)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{visible.Name, managed.Name}, queryNames(queries))
		require.Equal(t, 2, total, "opt-in must surface managed queries in the count too")
	})

	t.Run("still readable by id", func(t *testing.T) {
		got, err := ds.Query(ctx, managed.ID)
		require.NoError(t, err)
		require.True(t, got.OpenframeManaged)
	})

	t.Run("unmanaging brings it back", func(t *testing.T) {
		got, err := ds.Query(ctx, managed.ID)
		require.NoError(t, err)

		got.OpenframeManaged = false
		require.NoError(t, ds.SaveQuery(ctx, got, false, false))

		queries, total, _, _, err := ds.ListQueries(ctx, fleet.ListQueryOptions{})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{visible.Name, managed.Name}, queryNames(queries))
		require.Equal(t, 2, total)

		got.OpenframeManaged = true
		require.NoError(t, ds.SaveQuery(ctx, got, false, false))

		queries, total, _, _, err = ds.ListQueries(ctx, fleet.ListQueryOptions{})
		require.NoError(t, err)
		require.Equal(t, []string{visible.Name}, queryNames(queries))
		require.Equal(t, 1, total)
	})
}
