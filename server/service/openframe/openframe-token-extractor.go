package openframe

import (
	"os"
	"sync/atomic"

	"github.com/rs/zerolog/log"
)

type OpenframeTokenExtractor struct {
	encryptionService *OpenframeEncryptionService
	tokenFilePath     string
	readErrCount      int64
}

func NewOpenframeTokenExtractor(encryptionService *OpenframeEncryptionService, tokenFilePath string) *OpenframeTokenExtractor {
	log.Info().Msgf("Token file path: %s", tokenFilePath)
	return &OpenframeTokenExtractor{
		encryptionService: encryptionService,
		tokenFilePath:     tokenFilePath,
	}
}

func (te *OpenframeTokenExtractor) ExtractToken() (string, error) {
	encryptedData, err := os.ReadFile(te.tokenFilePath)
	if err != nil {
		count := atomic.AddInt64(&te.readErrCount, 1)
		if count%openframeTokenRefreshErrorLogInterval == 1 {
			log.Error().Err(err).Msg("Error reading token file")
		}
		return "", err
	}
	atomic.StoreInt64(&te.readErrCount, 0)

	decryptedData, err := te.encryptionService.Decrypt(string(encryptedData))
	if err != nil {
		log.Error().Err(err).Msg("Error decrypting data")
		return "", err
	}

	token := string(decryptedData)
	return token, nil
}
