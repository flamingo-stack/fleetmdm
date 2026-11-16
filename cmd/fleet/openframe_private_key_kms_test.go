// OPENFRAME(server-private-key-kms)
package main

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/config"
	"github.com/fleetdm/fleet/v4/server/datastore/mysql/mysqltest"
	"github.com/fleetdm/fleet/v4/server/platform/mysql/testing_utils"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/cloudkms/v1"
	"google.golang.org/api/option"
)

const openframeTestKMSKeyName = "projects/p/locations/l/keyRings/r/cryptoKeys/k"

type openframeFakeKMSServer struct {
	calls           atomic.Int32
	failFirst       int32
	failStatus      int
	corruptCRC      bool
	corruptPlainCRC bool
	skipVerifyFlag  bool
}

func (f *openframeFakeKMSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := f.calls.Add(1)
	if n <= f.failFirst {
		w.WriteHeader(f.failStatus)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": f.failStatus, "message": "injected"}})
		return
	}
	wantAAD := base64.StdEncoding.EncodeToString([]byte(openframeServerPrivateKeyAAD))
	switch {
	case r.URL.Path == "/v1/"+openframeTestKMSKeyName+":encrypt":
		var req cloudkms.EncryptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AdditionalAuthenticatedData != wantAAD {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		pt, _ := base64.StdEncoding.DecodeString(req.Plaintext)
		ct := append([]byte("wrapped:"), slices.Clone(pt)...)
		slices.Reverse(ct[len("wrapped:"):])
		crc := openframeCRC32CValue(ct)
		if f.corruptCRC {
			crc++
		}
		_ = json.NewEncoder(w).Encode(cloudkms.EncryptResponse{
			Name:                    openframeTestKMSKeyName + "/cryptoKeyVersions/1",
			Ciphertext:              base64.StdEncoding.EncodeToString(ct),
			CiphertextCrc32c:        crc,
			VerifiedPlaintextCrc32c: !f.skipVerifyFlag && req.PlaintextCrc32c == openframeCRC32CValue(pt),
			VerifiedAdditionalAuthenticatedDataCrc32c: true,
		})
	case r.URL.Path == "/v1/"+openframeTestKMSKeyName+":decrypt":
		var req cloudkms.DecryptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AdditionalAuthenticatedData != wantAAD {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ct, _ := base64.StdEncoding.DecodeString(req.Ciphertext)
		if req.CiphertextCrc32c != openframeCRC32CValue(ct) || !strings.HasPrefix(string(ct), "wrapped:") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		pt := slices.Clone(ct[len("wrapped:"):])
		slices.Reverse(pt)
		crc := openframeCRC32CValue(pt)
		if f.corruptPlainCRC {
			crc++
		}
		_ = json.NewEncoder(w).Encode(cloudkms.DecryptResponse{
			Plaintext:       base64.StdEncoding.EncodeToString(pt),
			PlaintextCrc32c: crc,
		})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func openframeFakeKMSOptions(t *testing.T, fake *openframeFakeKMSServer) []option.ClientOption {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return []option.ClientOption{
		option.WithEndpoint(srv.URL + "/"),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
	}
}

func newOpenframeTestCloudKMS(t *testing.T, fake *openframeFakeKMSServer) openframeCloudKMS {
	t.Helper()
	svc, err := cloudkms.NewService(t.Context(), openframeFakeKMSOptions(t, fake)...)
	require.NoError(t, err)
	return openframeCloudKMS{svc: svc, maxElapsed: 2 * time.Second}
}

func TestOpenframeCloudKMSRoundTrip(t *testing.T) {
	fake := &openframeFakeKMSServer{}
	kms := newOpenframeTestCloudKMS(t, fake)

	wrapped, err := kms.Encrypt(t.Context(), openframeTestKMSKeyName, []byte("server-private-key"))
	require.NoError(t, err)
	require.NotContains(t, string(wrapped), "server-private-key")

	plain, err := kms.Decrypt(t.Context(), openframeTestKMSKeyName, wrapped)
	require.NoError(t, err)
	require.Equal(t, "server-private-key", string(plain))
	require.EqualValues(t, 2, fake.calls.Load())
}

func TestOpenframeCloudKMSRetriesTransientErrors(t *testing.T) {
	fake := &openframeFakeKMSServer{failFirst: 2, failStatus: http.StatusServiceUnavailable}
	kms := newOpenframeTestCloudKMS(t, fake)

	_, err := kms.Encrypt(t.Context(), openframeTestKMSKeyName, []byte("server-private-key"))
	require.NoError(t, err)
	require.EqualValues(t, 3, fake.calls.Load())
}

func TestOpenframeCloudKMSDoesNotRetryPermanentErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusPreconditionFailed} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			fake := &openframeFakeKMSServer{failFirst: 100, failStatus: status}
			kms := newOpenframeTestCloudKMS(t, fake)

			_, err := kms.Decrypt(t.Context(), openframeTestKMSKeyName, []byte("wrapped:yek"))
			require.Error(t, err)
			require.EqualValues(t, 1, fake.calls.Load())
		})
	}
}

func TestOpenframeCloudKMSRejectsUnverifiedResponses(t *testing.T) {
	t.Run("ciphertext checksum", func(t *testing.T) {
		fake := &openframeFakeKMSServer{corruptCRC: true}
		kms := newOpenframeTestCloudKMS(t, fake)
		_, err := kms.Encrypt(t.Context(), openframeTestKMSKeyName, []byte("server-private-key"))
		require.ErrorContains(t, err, "ciphertext checksum mismatch")
		require.Greater(t, fake.calls.Load(), int32(1))
	})
	t.Run("plaintext checksum", func(t *testing.T) {
		fake := &openframeFakeKMSServer{corruptPlainCRC: true}
		kms := newOpenframeTestCloudKMS(t, fake)
		_, err := kms.Decrypt(t.Context(), openframeTestKMSKeyName, []byte("wrapped:yek"))
		require.ErrorContains(t, err, "plaintext checksum mismatch")
		require.Greater(t, fake.calls.Load(), int32(1))
	})
	t.Run("request checksums not verified", func(t *testing.T) {
		fake := &openframeFakeKMSServer{skipVerifyFlag: true}
		kms := newOpenframeTestCloudKMS(t, fake)
		_, err := kms.Encrypt(t.Context(), openframeTestKMSKeyName, []byte("server-private-key"))
		require.ErrorContains(t, err, "did not verify the request checksums")
	})
}

func TestOpenframeNewServerPrivateKey(t *testing.T) {
	a, err := openframeNewServerPrivateKey()
	require.NoError(t, err)
	b, err := openframeNewServerPrivateKey()
	require.NoError(t, err)

	require.Len(t, a, 44)
	raw, err := base64.StdEncoding.DecodeString(a)
	require.NoError(t, err)
	require.Len(t, raw, 32)
	require.NotEqual(t, a, b)
}

func TestOpenframeServerPrivateKeyFromKMS(t *testing.T) {
	const dbName = "openframe_server_private_key_kms_boot"
	ds := mysqltest.CreateNamedMySQLDS(t, dbName)
	logger := slog.New(slog.DiscardHandler)

	cfg := config.FleetConfig{
		Mysql: config.MysqlConfig{
			Address:  testing_utils.TestAddress,
			Username: testing_utils.TestUsername,
			Password: testing_utils.TestPassword,
			Database: dbName,
		},
		Server: config.ServerConfig{PrivateKeyKMSKey: openframeTestKMSKeyName},
	}
	var fatal []error
	initFatal := func(err error, msg string) { fatal = append(fatal, err) }

	fake := &openframeFakeKMSServer{}
	kmsOpts := openframeFakeKMSOptions(t, fake)

	first := openframeServerPrivateKeyFromKMS(t.Context(), cfg, logger, initFatal, kmsOpts...)
	require.Empty(t, fatal)
	require.Len(t, first, 44)

	var stored struct {
		KMSKeyName string `db:"kms_key_name"`
		WrappedKey []byte `db:"wrapped_key"`
	}
	mysqltest.ExecAdhocSQL(t, ds, func(q sqlx.ExtContext) error {
		return sqlx.GetContext(t.Context(), q, &stored, `SELECT kms_key_name, wrapped_key FROM openframe_server_private_key WHERE id = 1`)
	})
	require.Equal(t, openframeTestKMSKeyName, stored.KMSKeyName)
	require.NotContains(t, string(stored.WrappedKey), first)

	second := openframeServerPrivateKeyFromKMS(t.Context(), cfg, logger, initFatal, kmsOpts...)
	require.Empty(t, fatal)
	require.Equal(t, first, second)

	cfg.Server.PrivateKeyKMSKey = openframeTestKMSKeyName + "-other"
	require.Empty(t, openframeServerPrivateKeyFromKMS(t.Context(), cfg, logger, initFatal, kmsOpts...))
	require.Len(t, fatal, 1)
	require.ErrorContains(t, fatal[0], "but private_key_kms_key is "+openframeTestKMSKeyName+"-other")
}
