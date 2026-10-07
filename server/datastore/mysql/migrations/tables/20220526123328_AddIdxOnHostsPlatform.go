package tables

import (
	"database/sql"

	"github.com/pkg/errors"
)

func init() {
	MigrationClient.AddMigration(Up_20220526123328, Down_20220526123328)
}

func Up_20220526123328(tx *sql.Tx) error {
	// Idempotent migration.
	if !indexExistsTx(tx, "hosts", "hosts_platform_idx") {
		stm := "CREATE INDEX hosts_platform_idx ON hosts (platform);"

		if _, err := tx.Exec(stm); err != nil {
			return errors.Wrap(err, "creating hosts index")
		}
	}

	return nil
}

func Down_20220526123328(tx *sql.Tx) error {
	return nil
}

