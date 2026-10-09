// OPENFRAME(host-assignments, mysql-multitenancy): unit tests for the
// openframe migration pipeline's registration and idempotency helpers —
// openframe/docs/migrations.md
//
// The deep MySQL-backed idempotency tests live in
// server/datastore/mysql/migrations_openframe_test.go; these cover what runs
// without Docker: that every migration file's init() actually registered
// (an upstream sync dropping one is a silent data-loss bug — the table never
// gets created on fresh installs) and the information_schema probe helpers.
package openframe

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestAllOpenframeMigrationsRegistered(t *testing.T) {
	want := []int64{
		20260301000001, // AddPolicyHostsJoinTable
		20260301000002, // AddQueryHostsJoinTable
		20260620000001, // ScopeLabelUniqueNameToTeam
		20260626000001, // ScopeHostIdentityUniqueToTeam
		20260629000001, // AddTeamsOpenframeTenantUUID
		20260722000001, // AddTeamIdToCdcTables
		20260818000001, // AddPoliciesOpenframeManagedColumn
		20260818000002, // AddQueriesOpenframeManagedColumn
		20260831000001, // SeedGlobalAppConfigRow
	}

	var got []int64
	for _, m := range MigrationClient.Migrations {
		got = append(got, m.Version)
		require.NotNil(t, m.UpFn, "migration %d has no Up function", m.Version)
	}
	require.ElementsMatch(t, want, got,
		"a migration file exists whose init() did not register, or the expected list is stale — update both together")
}

func TestMigrationClientUsesOwnVersionTable(t *testing.T) {
	// The openframe pipeline must never share goose state with Fleet's own
	// migrations: same table would mean colliding version numbers.
	require.Equal(t, "migration_status_openframe", MigrationClient.TableName)
}

func newMockTx(t *testing.T) (*sql.Tx, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	return tx, mock
}

func TestIndexExists(t *testing.T) {
	cases := []struct {
		name  string
		count int
		want  bool
	}{
		{"present", 1, true},
		{"absent", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx, mock := newMockTx(t)
			mock.ExpectQuery("information_schema.STATISTICS").
				WithArgs("hosts", "idx_test").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tc.count))

			got, err := indexExists(tx, "hosts", "idx_test")
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}

	t.Run("query error propagates", func(t *testing.T) {
		tx, mock := newMockTx(t)
		mock.ExpectQuery("information_schema.STATISTICS").
			WillReturnError(errors.New("boom"))

		_, err := indexExists(tx, "hosts", "idx_test")
		require.Error(t, err)
	})
}

func TestColumnExists(t *testing.T) {
	cases := []struct {
		name  string
		count int
		want  bool
	}{
		{"present", 1, true},
		{"absent", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx, mock := newMockTx(t)
			mock.ExpectQuery("information_schema.COLUMNS").
				WithArgs("teams", "openframe_tenant_uuid").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tc.count))

			got, err := columnExists(tx, "teams", "openframe_tenant_uuid")
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}

	t.Run("query error propagates", func(t *testing.T) {
		tx, mock := newMockTx(t)
		mock.ExpectQuery("information_schema.COLUMNS").
			WillReturnError(errors.New("boom"))

		_, err := columnExists(tx, "teams", "openframe_tenant_uuid")
		require.Error(t, err)
	})
}
