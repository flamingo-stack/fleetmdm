package openframe

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"

	"github.com/rs/zerolog/log"
)

// openframeTokenRefreshErrorLogInterval controls how often (in number of
// consecutive errors) decrypt failures are logged, to avoid flooding logs.
const openframeTokenRefreshErrorLogInterval = 100

type OpenframeEncryptionService struct {
	encryptionKey   string
	decryptErrCount int
}

func NewOpenframeEncryptionService(encryptionKey string) *OpenframeEncryptionService {
	return &OpenframeEncryptionService{
		encryptionKey: encryptionKey,
	}
}

func (es *OpenframeEncryptionService) Decrypt(data string) ([]byte, error) {
	encryptedData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		es.decryptErrCount++
		if es.decryptErrCount % openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error decoding base64 data")
		}
		return nil, fmt.Errorf("decode base64 data: %w", err)
	}

	keyLen := len(es.encryptionKey)
	if keyLen != 16 && keyLen != 24 && keyLen != 32 {
		es.decryptErrCount++
		if es.decryptErrCount % openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Int("key_length", keyLen).Msg("Invalid AES key length")
		}
		return nil, fmt.Errorf("invalid AES key length %d: must be 16, 24, or 32 bytes", keyLen)
	}

	block, err := aes.NewCipher([]byte(es.encryptionKey))
	if err != nil {
		es.decryptErrCount++
		if es.decryptErrCount % openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error creating cipher")
		}
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	if len(encryptedData) < gcm.NonceSize() {
		return nil, fmt.Errorf("decrypt: ciphertext too short")
	}

	nonce := encryptedData[:gcm.NonceSize()]
	ciphertext := encryptedData[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		es.decryptErrCount++
		if es.decryptErrCount % openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error decrypting data")
		}
		return nil, fmt.Errorf("gcm open: %w", err)
	}
	es.decryptErrCount = 0

	return plaintext, nil
}
