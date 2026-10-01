package goose

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"sort"
)

var (
	ErrNoCurrentVersion = errors.New("no current version found")
	ErrNoNextVersion    = errors.New("no next version found")

	globalGoose = &Client{
		TableName: "goose_db_version",
		Dialect:   &PostgresDialect{},
	}
)

type Migrations []*Migration

// helpers so we can use pkg sort
func (ms Migrations) Len() int      { return len(ms) }
func (ms Migrations) Swap(i, j int) { ms[i], ms[j] = ms[j], ms[i] }
func (ms Migrations) Less(i, j int) bool {
	return ms[i].Version < ms[j].Version
}

// duplicateVersion returns an error describing the first duplicate migration
// version found in ms, or nil if there are none. Callers must invoke this
// after sorting, since sort.Interface implementations must not return errors
// or abort the process from within a comparator.
func (ms Migrations) duplicateVersion() error {
	for i := 1; i < len(ms); i++ {
		if ms[i].Version == ms[i-1].Version {
			return fmt.Errorf("goose: duplicate version %v detected:\n%v\n%v", ms[i].Version, ms[i-1].Source, ms[i].Source)
		}
	}
	return nil
}

func (ms Migrations) Current(current int64) (*Migration, error) {
	for i, migration := range ms {
		if migration.Version == current {
			return ms[i], nil
		}
	}

	return nil, ErrNoCurrentVersion
}

func (ms Migrations) Next(current int64) (*Migration, error) {
	for i, migration := range ms {
		if migration.Version > current {
			return ms[i], nil
		}
	}

	return nil, ErrNoNextVersion
}

func (ms Migrations) Last() (*Migration, error) {
	if len(ms) == 0 {
		return nil, ErrNoNextVersion
	}

	return ms[len(ms)-1], nil
}

func (ms Migrations) String() string {
	str := ""
	for _, m := range ms {
		str += fmt.Sprintln(m)
	}
	return str
}

// AddMigration adds a new go migration to the goose struct.
func (c *Client) AddMigration(up func(*sql.Tx) error, down func(*sql.Tx) error) {
	_, filename, _, _ := runtime.Caller(1)
	v, _ := NumericComponent(filename)
	migration := &Migration{Version: v, Next: -1, Previous: -1, UpFn: up, DownFn: down, Source: filename}

	c.Migrations = append(c.Migrations, migration)
}

// AddMigration exists for legacy support of the package global use of Goose.
// Calling the AddMigration method on a Goose struct is the preferred method.
func AddMigration(up func(*sql.Tx) error, down func(*sql.Tx) error) {
	// We can't just use globalGoose.AddMigration here because we need to
	// correctly record the caller.
	_, filename, _, _ := runtime.Caller(1)
	v, _ := NumericComponent(filename)
	migration := &Migration{Version: v, Next: -1, Previous: -1, UpFn: up, DownFn: down, Source: filename}

	globalGoose.Migrations = append(globalGoose.Migrations, migration)
}

// collect all the valid looking migration scripts in the
// migrations folder and go func registry, and key them by version
func (c *Client) collectMigrations(dirpath string, current, target int64) (Migrations, error) {
	var migrations Migrations

	// only load migrations from file if a path is explicitly provided
	if dirpath != "" {
		// extract the numeric component of each migration,
		// filter out any uninteresting files,
		// and ensure we only have one file per migration version.
		sqlMigrations, err := filepath.Glob(dirpath + "/*.sql")
		if err != nil {
			return nil, err
		}

		for _, file := range sqlMigrations {
			v, err := NumericComponent(file)
			if err != nil {
				return nil, err
			}
			if versionFilter(v, current, target) {
				migration := &Migration{Version: v, Next: -1, Previous: -1, Source: file}
				migrations = append(migrations, migration)
			}
		}
	}

	for _, migration := range c.Migrations {
		v, err := NumericComponent(migration.Source)
		if err != nil {
			return nil, err
		}
		if versionFilter(v, current, target) {
			migrations = append(migrations, migration)
		}
	}

	return sortAndConnectMigrations(migrations)
}

func sortAndConnectMigrations(migrations Migrations) (Migrations, error) {
	sort.Sort(migrations)

	if err := migrations.duplicateVersion(); err != nil {
		return nil, err
	}

	// now that we're sorted in the appropriate direction,
	// populate next and previous for each migration
	for i, m := range migrations {
		prev := int64(-1)
		if i > 0 {
			prev = migrations[i-1].Version
			migrations[i-1].Next = m.Version
		}
		migrations[i].Previous = prev
	}

	return migrations, nil
}

func versionFilter(v, current, target int64) bool {
	if target > current {
		return v > current && v <= target
	}

	if target < current {
		return v <= current && v > target
	}

	return false
}

// retrieve the current version for this DB.
// Create and initialize the DB version table if it doesn't exist.
func (c *Client) GetDBVersion(db *sql.DB) (int64, error) {
	rows, err := c.Dialect.dbVersionQuery(db, c.TableName)
	if err != nil {
		if isTableDoesNotExistErr(err) {
			return 0, c.createVersionTable(db)
		}
		return 0, err
	}
	defer rows.Close()

	// The most recent record for each migration specifies
	// whether it has been applied or rolled back.
	// The first version we find that has been applied is the current version.

	toSkip := make([]int64, 0)

	for rows.Next() {
		var row MigrationRecord
		if err = rows.Scan(&row.VersionId, &row.IsApplied); err != nil {
			log.Fatal("error scanning rows:", err) //nolint:gocritic // ignore exitAfterDefer
		}

		// have we already marked this version to be skipped?
		skip := false
		for _, v := range toSkip {
			if v == row.VersionId {
				skip = true
				break
			}
		}

		if skip {
			continue
		}

		// if version has been applied we're done
		if row.IsApplied {
			return row.VersionId, nil
		}

		// latest version of migration has not been applied.
		toSkip = append(toSkip, row.VersionId)
	}

	if err := rows.Err(); err != nil {
		return 0, err
	}

	// >>> OPENFRAME(migration-race): a concurrently-created-but-unseeded version
	// table must not crash the process. On MySQL the CREATE TABLE in
	// createVersionTable auto-commits before the version-0 INSERT, so a reader
	// racing the migration job on a fresh DB can observe the table existing but
	// empty (this also covers a version table whose rows were lost). Treat "no
	// applied row" as version 0 (nothing applied) and let the idempotent
	// migrations proceed/retry instead of panicking. — openframe/docs/migrations.md
	return 0, nil
	// <<< OPENFRAME(migration-race)
}

// isTableDoesNotExistErr reports whether err looks like it was caused by the
// goose version table not existing yet, as opposed to some other query
// failure (connection drop, permission error, deadlock, etc.) that should be
// surfaced to the caller rather than papered over with a CREATE TABLE.
func isTableDoesNotExistErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsAny(msg, []string{
		"does not exist",  // postgres
		"doesn't exist",   // mysql
		"no such table",   // sqlite
		"relation",        // postgres: relation "..." does not exist
	})
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if len(sub) > 0 && stringContains(s, sub) {
			return true
		}
	}
	return false
}

func stringContains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	n := len(sub)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= len(s); i++ {
		if s[i:i+n] == sub {
			return i
		}
	}
	return -1
}

// Create the goose_db_version table
// and insert the initial 0 value into it
func (c *Client) createVersionTable(db *sql.DB) error {
	txn, err := db.Begin()
	if err != nil {
		return err
	}

	d := c.Dialect

	if _, err := txn.Exec(d.createVersionTableSql(c.TableName)); err != nil {
		txn.Rollback() //nolint:errcheck
		return err
	}

	version := 0
	applied := true
	if _, err := txn.Exec(d.insertVersionSql(c.TableName), version, applied); err != nil {
		txn.Rollback() //nolint:errcheck
		return err
	}

	return txn.Commit()
}
