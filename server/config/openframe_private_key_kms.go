// >>> OPENFRAME(server-private-key-kms): server private key wrapped with Cloud KMS — openframe/docs/server-private-key-kms.md

package config

import (
	"errors"
	"regexp"
)

var openframeKMSKeyNameRegexp = regexp.MustCompile(`^projects/[^/]+/locations/[^/]+/keyRings/[^/]+/cryptoKeys/[^/]+$`)

func (s ServerConfig) validatePrivateKeyKMSKey(initFatal func(err error, msg string)) {
	if s.PrivateKeyKMSKey == "" {
		return
	}
	if s.PrivateKey != "" || s.PrivateKeySecretArn != "" {
		initFatal(errors.New("cannot specify private_key_kms_key together with private_key or private_key_arn"),
			"validate private key configuration")
		return
	}
	if !openframeKMSKeyNameRegexp.MatchString(s.PrivateKeyKMSKey) {
		initFatal(errors.New("private_key_kms_key must be projects/<project>/locations/<location>/keyRings/<ring>/cryptoKeys/<key>, without a key version"),
			"validate private key configuration")
	}
}

// <<< OPENFRAME(server-private-key-kms)
