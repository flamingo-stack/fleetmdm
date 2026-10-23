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
		es.decryptErrCount++
		if es.decryptErrCount % openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error creating GCM")
		}
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

