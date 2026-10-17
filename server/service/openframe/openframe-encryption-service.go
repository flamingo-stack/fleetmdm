package openframe

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"

	"github.com/rs/zerolog/log"
)

type OpenframeEncryptionService struct {
	encryptionKey   string
	decryptErrCount int
}

func NewOpenframeEncryptionService(encryptionKey string) (*OpenframeEncryptionService, error) {
	switch len(encryptionKey) {
	case 16, 24, 32:
		// valid AES-128/192/256 key length
	default:
		return nil, fmt.Errorf("invalid encryption key length %d: must be 16, 24, or 32 bytes", len(encryptionKey))
	}
	return &OpenframeEncryptionService{
		encryptionKey: encryptionKey,
	}, nil
}

func (es *OpenframeEncryptionService) Decrypt(data string) ([]byte, error) {
	encryptedData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		es.decryptErrCount++
		if es.decryptErrCount % openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error decoding base64 data")
		}
		return nil, err
	}

	block, err := aes.NewCipher([]byte(es.encryptionKey))
	if err != nil {
		es.decryptErrCount++
		if es.decryptErrCount % openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error creating cipher")
		}
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(encryptedData) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce := encryptedData[:gcm.NonceSize()]
	ciphertext := encryptedData[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		es.decryptErrCount++
		if es.decryptErrCount % openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error decrypting data")
		}
		return nil, err
	}
	es.decryptErrCount = 0

	return plaintext, nil
}

