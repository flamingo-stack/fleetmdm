// >>> OPENFRAME(server-private-key-kms): server private key wrapped with Cloud KMS — openframe/docs/server-private-key-kms.md

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

type OpenframeKeyWrapper interface {
	Encrypt(ctx context.Context, keyName string, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, keyName string, ciphertext []byte) ([]byte, error)
}

const openframeTablesWithEncryptedDataQuery = `
SELECT t.name FROM (
  SELECT 'abm_tokens' AS name, EXISTS (SELECT 1 FROM abm_tokens) AS present
  UNION ALL SELECT 'ca_config_assets', EXISTS (SELECT 1 FROM ca_config_assets)
  UNION ALL SELECT 'certificate_authorities', EXISTS (SELECT 1 FROM certificate_authorities)
  UNION ALL SELECT 'host_managed_local_account_passwords', EXISTS (SELECT 1 FROM host_managed_local_account_passwords)
  UNION ALL SELECT 'host_recovery_key_passwords', EXISTS (SELECT 1 FROM host_recovery_key_passwords)
  UNION ALL SELECT 'mdm_config_assets', EXISTS (SELECT 1 FROM mdm_config_assets)
  UNION ALL SELECT 'secret_variables', EXISTS (SELECT 1 FROM secret_variables)
  UNION ALL SELECT 'vpp_tokens', EXISTS (SELECT 1 FROM vpp_tokens)
) t
WHERE t.present
ORDER BY t.name`

type openframeServerPrivateKeyRow struct {
	KMSKeyName string `db:"kms_key_name"`
	WrappedKey []byte `db:"wrapped_key"`
}

func OpenframeLoadServerPrivateKey(
	ctx context.Context,
	db sqlx.ExtContext,
	wrapper OpenframeKeyWrapper,
	kmsKeyName string,
	newKey func() (string, error),
) (string, error) {
	row, err := openframeReadServerPrivateKey(ctx, db)
	switch {
	case err == nil:
		return openframeUnwrapServerPrivateKey(ctx, wrapper, kmsKeyName, row)
	case !errors.Is(err, sql.ErrNoRows):
		return "", err
	}

	var tables []string
	if err := sqlx.SelectContext(ctx, db, &tables, openframeTablesWithEncryptedDataQuery); err != nil {
		return "", ctxerr.Wrap(ctx, err, "checking for data encrypted with the server private key")
	}
	if len(tables) > 0 {
		return "", ctxerr.Errorf(ctx, "no wrapped server private key is stored, but %s already hold data encrypted with another key; "+
			"a new key would make that data unreadable", strings.Join(tables, ", "))
	}

	key, err := newKey()
	if err != nil {
		return "", ctxerr.Wrap(ctx, err, "generating server private key")
	}
	wrapped, err := wrapper.Encrypt(ctx, kmsKeyName, []byte(key))
	if err != nil {
		return "", ctxerr.Wrapf(ctx, err, "wrapping server private key with %s", kmsKeyName)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT IGNORE INTO openframe_server_private_key (id, kms_key_name, wrapped_key) VALUES (1, ?, ?)`,
		kmsKeyName, wrapped,
	); err != nil {
		return "", ctxerr.Wrap(ctx, err, "storing wrapped server private key")
	}

	row, err = openframeReadServerPrivateKey(ctx, db)
	if err != nil {
		return "", err
	}
	return openframeUnwrapServerPrivateKey(ctx, wrapper, kmsKeyName, row)
}

func openframeReadServerPrivateKey(ctx context.Context, db sqlx.QueryerContext) (*openframeServerPrivateKeyRow, error) {
	var row openframeServerPrivateKeyRow
	err := sqlx.GetContext(ctx, db, &row, `SELECT kms_key_name, wrapped_key FROM openframe_server_private_key WHERE id = 1`)
	if err == nil {
		return &row, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1146 {
		return nil, ctxerr.New(ctx, "table openframe_server_private_key does not exist; run fleet prepare db first")
	}
	return nil, ctxerr.Wrap(ctx, err, "reading wrapped server private key")
}

func openframeUnwrapServerPrivateKey(
	ctx context.Context,
	wrapper OpenframeKeyWrapper,
	kmsKeyName string,
	row *openframeServerPrivateKeyRow,
) (string, error) {
	if row.KMSKeyName != kmsKeyName {
		return "", ctxerr.Errorf(ctx, "the server private key is wrapped with %s, but private_key_kms_key is %s", row.KMSKeyName, kmsKeyName)
	}
	key, err := wrapper.Decrypt(ctx, kmsKeyName, row.WrappedKey)
	if err != nil {
		return "", ctxerr.Wrapf(ctx, err, "unwrapping server private key with %s", kmsKeyName)
	}
	return string(key), nil
}

// <<< OPENFRAME(server-private-key-kms)
