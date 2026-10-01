package openframe

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"sync"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/rs/zerolog/log"
)

type OpenframeEncryptionService struct {
	encryptionKey   string
	decryptErrCount int
	decryptErrMu    sync.Mutex
}

func NewOpenframeEncryptionService(encryptionKey string) *OpenframeEncryptionService {
	return &OpenframeEncryptionService{
		encryptionKey: encryptionKey,
	}
}

func (es *OpenframeEncryptionService) incrDecryptErrCount() int {
	es.decryptErrMu.Lock()
	defer es.decryptErrMu.Unlock()
	es.decryptErrCount++
	return es.decryptErrCount
}

func (es *OpenframeEncryptionService) resetDecryptErrCount() {
	es.decryptErrMu.Lock()
	defer es.decryptErrMu.Unlock()
	es.decryptErrCount = 0
}

func (es *OpenframeEncryptionService) Decrypt(ctx context.Context, data string) ([]byte, error) {
	encryptedData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		count := es.incrDecryptErrCount()
		if count%openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error decoding base64 data")
		}
		return nil, ctxerr.Wrap(ctx, err, "decode base64 data")
	}

	switch len(es.encryptionKey) {
	case 16, 24, 32:
	default:
		count := es.incrDecryptErrCount()
		err := ctxerr.New(ctx, fmt.Sprintf("invalid encryption key length: got %d bytes, want 16, 24, or 32", len(es.encryptionKey)))
		if count%openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Invalid encryption key length")
		}
		return nil, err
	}

	block, err := aes.NewCipher([]byte(es.encryptionKey))
	if err != nil {
		count := es.incrDecryptErrCount()
		if count%openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error creating cipher")
		}
		return nil, ctxerr.Wrap(ctx, err, "create cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "create gcm")
	}

	if len(encryptedData) < gcm.NonceSize() {
		return nil, ctxerr.New(ctx, "ciphertext too short")
	}

	nonce := encryptedData[:gcm.NonceSize()]
	ciphertext := encryptedData[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		count := es.incrDecryptErrCount()
		if count%openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error decrypting data")
		}
		return nil, ctxerr.Wrap(ctx, err, "decrypt data")
	}
	es.resetDecryptErrCount()

	return plaintext, nil
}
