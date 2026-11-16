package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerConfigValidatePrivateKeyKMSKey(t *testing.T) {
	t.Parallel()

	const keyName = "projects/p/locations/europe-west1/keyRings/fleet/cryptoKeys/server-private-key"

	cases := []struct {
		name    string
		cfg     ServerConfig
		wantErr string
	}{
		{name: "unset", cfg: ServerConfig{}},
		{name: "valid key name", cfg: ServerConfig{PrivateKeyKMSKey: keyName}},
		{
			name:    "with private_key",
			cfg:     ServerConfig{PrivateKeyKMSKey: keyName, PrivateKey: strings.Repeat("x", 32)},
			wantErr: "together with private_key",
		},
		{
			name:    "with private_key_arn",
			cfg:     ServerConfig{PrivateKeyKMSKey: keyName, PrivateKeySecretArn: "test-secret-arn-placeholder"},
			wantErr: "together with private_key",
		},
		{
			name:    "key version",
			cfg:     ServerConfig{PrivateKeyKMSKey: keyName + "/cryptoKeyVersions/1"},
			wantErr: "without a key version",
		},
		{
			name:    "short name",
			cfg:     ServerConfig{PrivateKeyKMSKey: "server-private-key"},
			wantErr: "without a key version",
		},
		{
			name:    "empty segment",
			cfg:     ServerConfig{PrivateKeyKMSKey: "projects//locations/l/keyRings/r/cryptoKeys/k"},
			wantErr: "without a key version",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []error
			tc.cfg.Validate(func(err error, msg string) { got = append(got, err) })
			if tc.wantErr == "" {
				require.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			require.ErrorContains(t, got[0], tc.wantErr)
		})
	}
}
