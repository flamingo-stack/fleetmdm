// OPENFRAME(server-private-key-kms)
package mysql

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

const openframeTestKMSKey = "projects/p/locations/l/keyRings/r/cryptoKeys/k"

type openframeFakeKeyWrapper struct {
	encrypts   atomic.Int32
	decrypts   atomic.Int32
	decryptErr error
}

func (w *openframeFakeKeyWrapper) Encrypt(_ context.Context, keyName string, plaintext []byte) ([]byte, error) {
	w.encrypts.Add(1)
	return append([]byte(keyName+"|"), plaintext...), nil
}

func (w *openframeFakeKeyWrapper) Decrypt(_ context.Context, keyName string, ciphertext []byte) ([]byte, error) {
	w.decrypts.Add(1)
	if w.decryptErr != nil {
		return nil, w.decryptErr
	}
	prefix := []byte(keyName + "|")
	if !bytes.HasPrefix(ciphertext, prefix) {
		return nil, errors.New("wrapped with another key")
	}
	return bytes.Clone(ciphertext[len(prefix):]), nil
}

func openframeFixedKey(key string) func() (string, error) {
	return func() (string, error) { return key, nil }
}

func TestOpenframeLoadServerPrivateKey(t *testing.T) {
	ds := CreateMySQLDS(t)

	cases := []struct {
		name string
		fn   func(t *testing.T, ds *Datastore)
	}{
		{"CreatesAndReloads", testOpenframeServerPrivateKeyCreatesAndReloads},
		{"KeyNameMismatch", testOpenframeServerPrivateKeyKeyNameMismatch},
		{"DecryptError", testOpenframeServerPrivateKeyDecryptError},
		{"RefusesOverEncryptedData", testOpenframeServerPrivateKeyRefusesOverEncryptedData},
		{"ConcurrentStartsAgree", testOpenframeServerPrivateKeyConcurrentStartsAgree},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer TruncateTables(t, ds)
			c.fn(t, ds)
		})
	}
}

func testOpenframeServerPrivateKeyCreatesAndReloads(t *testing.T, ds *Datastore) {
	ctx := t.Context()
	db := ds.writer(ctx)
	wrapper := &openframeFakeKeyWrapper{}

	key, err := OpenframeLoadServerPrivateKey(ctx, db, wrapper, openframeTestKMSKey, openframeFixedKey("first-key-0123456789abcdef0123456789"))
	require.NoError(t, err)
	require.Equal(t, "first-key-0123456789abcdef0123456789", key)
	require.EqualValues(t, 1, wrapper.encrypts.Load())

	var stored openframeServerPrivateKeyRow
	require.NoError(t, sqlx.GetContext(ctx, db, &stored, `SELECT kms_key_name, wrapped_key FROM openframe_server_private_key WHERE id = 1`))
	require.Equal(t, openframeTestKMSKey, stored.KMSKeyName)
	require.Equal(t, openframeTestKMSKey+"|first-key-0123456789abcdef0123456789", string(stored.WrappedKey))

	key, err = OpenframeLoadServerPrivateKey(ctx, db, wrapper, openframeTestKMSKey, func() (string, error) {
		t.Fatal("a stored key must not be regenerated")
		return "", nil
	})
	require.NoError(t, err)
	require.Equal(t, "first-key-0123456789abcdef0123456789", key)
	require.EqualValues(t, 1, wrapper.encrypts.Load())
}

func testOpenframeServerPrivateKeyKeyNameMismatch(t *testing.T, ds *Datastore) {
	ctx := t.Context()
	db := ds.writer(ctx)
	wrapper := &openframeFakeKeyWrapper{}

	_, err := OpenframeLoadServerPrivateKey(ctx, db, wrapper, openframeTestKMSKey, openframeFixedKey("key-a"))
	require.NoError(t, err)

	other := "projects/p/locations/l/keyRings/r/cryptoKeys/other"
	_, err = OpenframeLoadServerPrivateKey(ctx, db, wrapper, other, openframeFixedKey("key-b"))
	require.ErrorContains(t, err, "wrapped with "+openframeTestKMSKey+", but private_key_kms_key is "+other)
	require.EqualValues(t, 1, wrapper.decrypts.Load())

	var count int
	require.NoError(t, sqlx.GetContext(ctx, db, &count, `SELECT COUNT(*) FROM openframe_server_private_key`))
	require.Equal(t, 1, count)
}

func testOpenframeServerPrivateKeyDecryptError(t *testing.T, ds *Datastore) {
	ctx := t.Context()
	db := ds.writer(ctx)

	_, err := OpenframeLoadServerPrivateKey(ctx, db, &openframeFakeKeyWrapper{}, openframeTestKMSKey, openframeFixedKey("key-a"))
	require.NoError(t, err)

	failing := &openframeFakeKeyWrapper{decryptErr: errors.New("permission denied")}
	_, err = OpenframeLoadServerPrivateKey(ctx, db, failing, openframeTestKMSKey, openframeFixedKey("key-b"))
	require.ErrorContains(t, err, "permission denied")
	require.Zero(t, failing.encrypts.Load())
}

func testOpenframeServerPrivateKeyRefusesOverEncryptedData(t *testing.T, ds *Datastore) {
	ctx := t.Context()
	db := ds.writer(ctx)

	for _, stmt := range []string{
		`INSERT INTO secret_variables (name, value) VALUES ('OPENFRAME_TEST', 'ciphertext')`,
		`INSERT INTO mdm_config_assets (name, value, md5_checksum) VALUES ('apns_key', 'ciphertext', UNHEX(MD5('ciphertext')))`,
	} {
		_, err := db.ExecContext(ctx, stmt)
		require.NoError(t, err)
	}

	wrapper := &openframeFakeKeyWrapper{}
	_, err := OpenframeLoadServerPrivateKey(ctx, db, wrapper, openframeTestKMSKey, openframeFixedKey("key-a"))
	require.ErrorContains(t, err, "mdm_config_assets, secret_variables already hold data encrypted with another key")
	require.Zero(t, wrapper.encrypts.Load())

	var count int
	require.NoError(t, sqlx.GetContext(ctx, db, &count, `SELECT COUNT(*) FROM openframe_server_private_key`))
	require.Zero(t, count)
}

func testOpenframeServerPrivateKeyConcurrentStartsAgree(t *testing.T, ds *Datastore) {
	ctx := t.Context()
	db := ds.writer(ctx)
	wrapper := &openframeFakeKeyWrapper{}

	const servers = 7
	keys := make([]string, servers)
	errs := make([]error, servers)
	var wg sync.WaitGroup
	for i := range servers {
		wg.Go(func() {
			keys[i], errs[i] = OpenframeLoadServerPrivateKey(ctx, db, wrapper, openframeTestKMSKey,
				openframeFixedKey(fmt.Sprintf("candidate-%d-0123456789abcdef0123456789", i)))
		})
	}
	wg.Wait()

	for i := range servers {
		require.NoError(t, errs[i])
		require.Equal(t, keys[0], keys[i])
	}
	var count int
	require.NoError(t, sqlx.GetContext(ctx, db, &count, `SELECT COUNT(*) FROM openframe_server_private_key`))
	require.Equal(t, 1, count)
}

func TestOpenframeLoadServerPrivateKeyMissingTable(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	db := sqlx.NewDb(sqlDB, "mysql")

	mock.ExpectQuery("SELECT kms_key_name, wrapped_key FROM openframe_server_private_key").
		WillReturnError(&mysql.MySQLError{Number: 1146, Message: "Table 'fleet.openframe_server_private_key' doesn't exist"})

	_, err = OpenframeLoadServerPrivateKey(t.Context(), db, &openframeFakeKeyWrapper{}, openframeTestKMSKey, openframeFixedKey("key"))
	require.ErrorContains(t, err, "run fleet prepare db first")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrateOpenframeServerPrivateKeyTableMatchesSchema(t *testing.T) {
	ds := CreateMySQLDS(t)
	ctx := t.Context()
	db := ds.writer(ctx)

	showCreate := func() string {
		var row struct {
			Table  string `db:"Table"`
			Create string `db:"Create Table"`
		}
		require.NoError(t, sqlx.GetContext(ctx, db, &row, `SHOW CREATE TABLE openframe_server_private_key`))
		return row.Create
	}
	fromSchema := showCreate()

	_, err := db.ExecContext(ctx, `DROP TABLE openframe_server_private_key`)
	require.NoError(t, err)
	require.NoError(t, ds.MigrateOpenframe(ctx))
	require.Equal(t, fromSchema, showCreate())

	_, err = db.ExecContext(ctx, `INSERT INTO openframe_server_private_key (id, kms_key_name, wrapped_key) VALUES (2, 'k', 'w')`)
	require.ErrorContains(t, err, "ck_openframe_server_private_key_single_row")
}
