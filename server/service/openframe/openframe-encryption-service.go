package openframe

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

type OpenframeEncryptionService struct {
	encryptionKey   string
	decryptErrCount atomic.Int64
}

func NewOpenframeEncryptionService(encryptionKey string) *OpenframeEncryptionService {
	return &OpenframeEncryptionService{
		encryptionKey: encryptionKey,
	}
}

func (es *OpenframeEncryptionService) Decrypt(data string) ([]byte, error) {
	encryptedData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		count := es.decryptErrCount.Add(1)
		if count % openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error decoding base64 data")
		}
		return nil, fmt.Errorf("decoding base64 data: %w", err)
	}

	block, err := aes.NewCipher([]byte(es.encryptionKey))
	if err != nil {
		count := es.decryptErrCount.Add(1)
		if count % openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error creating cipher")
		}
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	if len(encryptedData) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce := encryptedData[:gcm.NonceSize()]
	ciphertext := encryptedData[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		count := es.decryptErrCount.Add(1)
		if count % openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error decrypting data")
		}
		return nil, fmt.Errorf("decrypting data: %w", err)
	}
	es.decryptErrCount.Store(0)

	return plaintext, nil
}
