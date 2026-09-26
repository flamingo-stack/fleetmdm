package tables

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/goose"
	"github.com/jmoiron/sqlx"
)

var MigrationClient = goose.New("migration_status_tables", goose.MySqlDialect{})

// can override in tests
var (
	outputTo         io.Writer = os.Stderr
	progressInterval           = time.Second * 5
)

type migrationStep func(tx *sql.Tx) error

func basicMigrationStep(statement string, errorMessage string) migrationStep {
	return func(tx *sql.Tx) error {
		_, err := tx.Exec(statement)
		if err != nil {
			return fmt.Errorf("%s: %w", errorMessage, err)
		}
		return nil
	}
}

func basicMigrationStepWithArgs(statement string, args []any, errorMessage string) migrationStep {
	return func(tx *sql.Tx) error {
		_, err := tx.Exec(statement, args...)
		if err != nil {
			return fmt.Errorf("%s: %w", errorMessage, err)
		}
		return nil
	}
}

type (
	getTotalCountFn       func(tx *sql.Tx) (uint64, error)
	incrementCountFn      func()
	executeWithProgressFn func(tx *sql.Tx, increment incrementCountFn) error
)

func incrementalMigrationStep(count getTotalCountFn, execute executeWithProgressFn) migrationStep {
	return func(tx *sql.Tx) error {
		total, err := count(tx)
		if err != nil {
			return err
		}
		if total == 0 { // skip no-ops to avoid divide by zero
			return nil
		}

		atomicCurrent := atomic.Uint64{}

		// Every five seconds, echo the % progress of the executor
		// Since we output once the migration step is complete, we need an extra channel to indicate when both the step
		// and the "step complete" output are com0plete
		stepComplete := make(chan struct{})
		outputComplete := make(chan struct{})
		go func() {
			ticker := time.NewTicker(progressInterval)
			defer ticker.Stop()
			defer close(outputComplete)
			for {
				select {
				case <-stepComplete:
					_, _ = fmt.Fprint(outputTo, "    100% complete\n")
					return
				case <-ticker.C:
					current := atomicCurrent.Load()
					switch {
					case current >= total:
						_, _ = fmt.Fprint(outputTo, "    Almost done...\n")
					default:
						_, _ = fmt.Fprintf(outputTo, "    %d%% complete\n", (100*current)/total)
					}
				}
			}
		}()

		err = execute(tx, func() {
			atomicCurrent.Add(1)
		})
		close(stepComplete)
		<-outputComplete // Wait for the goroutine to complete
		return err
	}
}

func withSteps(steps []migrationStep, tx *sql.Tx) error {
	stepCount := len(steps)
	for i, step := range steps {
		if stepCount > 1 {
			_, _ = fmt.Fprintf(outputTo, "  Step %d of %d\n", i+1, stepCount)
		}
		if err := step(tx); err != nil {
			return err
		}
	}
	return nil
}

func fkExists(tx *sql.Tx, table, name string) bool {
	var count int
	err := tx.QueryRow(`
SELECT COUNT(1)
FROM information_schema.REFERENTIAL_CONSTRAINTS
WHERE CONSTRAINT_SCHEMA = DATABASE()
AND TABLE_NAME = ?
AND CONSTRAINT_NAME = ?
	`, table, name).Scan(&count)
	if err != nil {
		return false
	}

	return count > 0
}

func constraintExists(tx *sql.Tx, table, name string) bool {
	var count int
	err := tx.QueryRow(`
SELECT COUNT(1)
FROM information_schema.TABLE_CONSTRAINTS
WHERE CONSTRAINT_SCHEMA = DATABASE()
AND TABLE_NAME = ?
AND CONSTRAINT_NAME = ?
	`, table, name).Scan(&count)
	if err != nil {
		return false
	}

	return count > 0
}

func columnExists(tx *sql.Tx, table, column string) bool {
	return columnsExists(tx, table, column)
}

func columnsExists(tx *sql.Tx, table string, columns ...string) bool {
	if len(columns) == 0 {
		return false
	}
	inColumns := strings.TrimRight(strings.Repeat("?,", len(columns)), ",")
	args := make([]interface{}, 0, len(columns)+1)
	args = append(args, table)
	for _, column := range columns {
		args = append(args, column)
	}

	var count int
	err := tx.QueryRow(
		fmt.Sprintf(`
SELECT
    count(*)
FROM
    information_schema.columns
WHERE
    TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = ?
    AND COLUMN_NAME IN (%s)
`, inColumns), args...,
	).Scan(&count)
	if err != nil {
		return false
	}

	return count == len(columns)
}

func tableExists(tx *sql.Tx, table string) bool {
	var count int
	err := tx.QueryRow(
		`
SELECT
    count(*)
FROM
    information_schema.columns
WHERE
    TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = ?
`,
		table,
	).Scan(&count)
	if err != nil {
		return false
	}

	return count > 0
}

func indexExists(tx *sqlx.DB, table, index string) bool {
	var count int
	err := tx.QueryRow(`
SELECT COUNT(1)
FROM INFORMATION_SCHEMA.STATISTICS
WHERE table_schema = DATABASE()
AND table_name = ?
AND index_name = ?
`, table, index).Scan(&count)
	if err != nil {
		return false
	}

	return count > 0
}

func indexExistsTx(tx *sql.Tx, table, index string) bool {
	var count int
	err := tx.QueryRow(`
SELECT COUNT(1)
FROM INFORMATION_SCHEMA.STATISTICS
WHERE table_schema = DATABASE()
AND table_name = ?
AND index_name = ?
`, table, index).Scan(&count)
	if err != nil {
		return false
	}

	return count > 0
}

// updateAppConfigJSON updates the `json_value` stored in the `app_config_json` after applying the
// supplied callback to the current config object.
func updateAppConfigJSON(tx *sql.Tx, fn func(config *fleet.AppConfig) error) error {
	var raw []byte
	row := tx.QueryRow(`SELECT json_value FROM app_config_json LIMIT 1`)
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("select app_config_json: %w", err)
	}

	var config fleet.AppConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return fmt.Errorf("unmarshal app_config_json: %w", err)
	}

	if err := fn(&config); err != nil {
		return fmt.Errorf("callback app_config_json: %w", err)
	}

	b, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal updated app_config_json: %w", err)
	}

	const updateStmt = `UPDATE app_config_json SET json_value = ? WHERE id = 1`
	if _, err := tx.Exec(updateStmt, b); err != nil {
		return fmt.Errorf("update app_config_json: %w", err)
	}

	return nil
}
