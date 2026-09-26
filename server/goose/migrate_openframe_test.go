// OPENFRAME(migration-race): regression test for GetDBVersion against an
// empty/unseeded version table — openframe/docs/migrations.md
//
// On a fresh DB the migration job (`fleet prepare db`) and the server
// (`fleet serve`) can start concurrently (the fork removed the migration Helm
// hook). MySQL auto-commits goose's CREATE TABLE before its version-0 INSERT,
// so one process can observe the version table existing but empty. Upstream
// goose `panic("unreachable")`s in that case; the fork returns version 0 so the
// idempotent migrations proceed/retry instead of crash-looping. This test pins
// that behavior. Pure logic — uses go-sqlmock, no live MySQL.
//
// KNOWN OPERATIONAL RISK (tracked, not fully resolved by this test): returning
// 0 here only mitigates the panic. It does not add a coordination barrier
// between `fleet prepare db` and `fleet serve`, so any caller of GetDBVersion
// that assumes a nonzero result implies "migrations have been seeded" can
// still be fooled during this same race window (version table exists, seed
// row not yet committed). See openframe/docs/migrations.md for the
// recommended fix (reinstate a migration-completion barrier, e.g. a Helm hook
// or init container, before `fleet serve` starts) — until that lands, treat
// GetDBVersion()==0 as ambiguous between "unmigrated" and "mid-race" in any
// new code path that depends on it.
package goose

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetDBVersion_EmptyVersionTableReturnsZeroNotPanic(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// Version table EXISTS (query succeeds) but returns ZERO rows — the race
	// window where CREATE TABLE has committed but the version-0 INSERT has not.
	mock.ExpectQuery("SELECT version_id, is_applied from migration_status_tables").
		WillReturnRows(sqlmock.NewRows([]string{"version_id", "is_applied"}))

	c := New("migration_status_tables", MySqlDialect{})

	// Must NOT panic, and must report version 0 (nothing applied yet).
	got, err := c.GetDBVersion(db)
	if err != nil {
		t.Fatalf("GetDBVersion returned error: %v", err)
	}
	if got != 0 {
		t.Fatalf("GetDBVersion = %d, want 0 for an empty version table", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}
