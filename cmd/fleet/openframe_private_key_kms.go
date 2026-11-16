// >>> OPENFRAME(server-private-key-kms): server private key wrapped with Cloud KMS — openframe/docs/server-private-key-kms.md

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"hash/crc32"
	"log/slog"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/fleetdm/fleet/v4/server/config"
	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/datastore/mysql"
	"google.golang.org/api/cloudkms/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const openframeServerPrivateKeyAAD = "openframe/fleet/server-private-key/v1"

var openframeCRC32C = crc32.MakeTable(crc32.Castagnoli)

func openframeServerPrivateKeyFromKMS(
	ctx context.Context,
	cfg config.FleetConfig,
	logger *slog.Logger,
	initFatal func(err error, msg string),
	kmsOpts ...option.ClientOption,
) string {
	svc, err := cloudkms.NewService(ctx, kmsOpts...)
	if err != nil {
		initFatal(err, "create Cloud KMS client")
		return ""
	}
	conns, err := mysql.NewDBConnections(cfg.Mysql, mysql.Logger(logger))
	if err != nil {
		initFatal(err, "connect to MySQL to load the server private key")
		return ""
	}
	defer conns.Primary.Close()

	wrapper := openframeCloudKMS{svc: svc, maxElapsed: time.Minute}
	key, err := mysql.OpenframeLoadServerPrivateKey(ctx, conns.Primary, wrapper, cfg.Server.PrivateKeyKMSKey, openframeNewServerPrivateKey)
	if err != nil {
		initFatal(err, "load server private key through Cloud KMS")
		return ""
	}
	logger.InfoContext(ctx, "server private key loaded through Cloud KMS", "kms_key", cfg.Server.PrivateKeyKMSKey)
	return key
}

func openframeNewServerPrivateKey() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

type openframeCloudKMS struct {
	svc        *cloudkms.Service
	maxElapsed time.Duration
}

func (k openframeCloudKMS) Encrypt(ctx context.Context, keyName string, plaintext []byte) ([]byte, error) {
	req := &cloudkms.EncryptRequest{
		Plaintext:                         base64.StdEncoding.EncodeToString(plaintext),
		PlaintextCrc32c:                   openframeCRC32CValue(plaintext),
		AdditionalAuthenticatedData:       base64.StdEncoding.EncodeToString([]byte(openframeServerPrivateKeyAAD)),
		AdditionalAuthenticatedDataCrc32c: openframeCRC32CValue([]byte(openframeServerPrivateKeyAAD)),
	}
	var ciphertext []byte
	err := k.retry(ctx, func() error {
		resp, err := k.svc.Projects.Locations.KeyRings.CryptoKeys.Encrypt(keyName, req).Context(ctx).Do()
		if err != nil {
			return err
		}
		if !resp.VerifiedPlaintextCrc32c || !resp.VerifiedAdditionalAuthenticatedDataCrc32c {
			return errors.New("encrypt response did not verify the request checksums")
		}
		out, err := base64.StdEncoding.DecodeString(resp.Ciphertext)
		if err != nil {
			return backoff.Permanent(ctxerr.Wrap(ctx, err, "decode Cloud KMS ciphertext"))
		}
		if openframeCRC32CValue(out) != resp.CiphertextCrc32c {
			return errors.New("encrypt response ciphertext checksum mismatch")
		}
		ciphertext = out
		return nil
	})
	return ciphertext, err
}

func (k openframeCloudKMS) Decrypt(ctx context.Context, keyName string, ciphertext []byte) ([]byte, error) {
	req := &cloudkms.DecryptRequest{
		Ciphertext:                        base64.StdEncoding.EncodeToString(ciphertext),
		CiphertextCrc32c:                  openframeCRC32CValue(ciphertext),
		AdditionalAuthenticatedData:       base64.StdEncoding.EncodeToString([]byte(openframeServerPrivateKeyAAD)),
		AdditionalAuthenticatedDataCrc32c: openframeCRC32CValue([]byte(openframeServerPrivateKeyAAD)),
	}
	var plaintext []byte
	err := k.retry(ctx, func() error {
		resp, err := k.svc.Projects.Locations.KeyRings.CryptoKeys.Decrypt(keyName, req).Context(ctx).Do()
		if err != nil {
			return err
		}
		out, err := base64.StdEncoding.DecodeString(resp.Plaintext)
		if err != nil {
			return backoff.Permanent(ctxerr.Wrap(ctx, err, "decode Cloud KMS plaintext"))
		}
		if openframeCRC32CValue(out) != resp.PlaintextCrc32c {
			return errors.New("decrypt response plaintext checksum mismatch")
		}
		plaintext = out
		return nil
	})
	return plaintext, err
}

func (k openframeCloudKMS) retry(ctx context.Context, call func() error) error {
	policy := backoff.NewExponentialBackOff()
	policy.InitialInterval = 500 * time.Millisecond
	policy.MaxElapsedTime = k.maxElapsed
	return backoff.Retry(func() error {
		err := call()
		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) && openframeKMSPermanentStatus(apiErr.Code) {
			return backoff.Permanent(err)
		}
		return err
	}, backoff.WithContext(policy, ctx))
}

func openframeKMSPermanentStatus(code int) bool {
	switch code {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusPreconditionFailed:
		return true
	}
	return false
}

func openframeCRC32CValue(b []byte) int64 {
	return int64(crc32.Checksum(b, openframeCRC32C))
}

// <<< OPENFRAME(server-private-key-kms)
