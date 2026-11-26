// OPENFRAME(windows-wstep)
package main

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server"
	"github.com/fleetdm/fleet/v4/server/config"
	microsoft_mdm "github.com/fleetdm/fleet/v4/server/mdm/microsoft"
	"github.com/stretchr/testify/require"
)

func TestOpenframeNewWSTEPPair(t *testing.T) {
	certPEM, keyPEM, err := microsoft_mdm.OpenframeNewWSTEPPair()
	require.NoError(t, err)

	cfg := config.MDMConfig{
		WindowsWSTEPIdentityCertBytes: string(certPEM),
		WindowsWSTEPIdentityKeyBytes:  string(keyPEM),
	}
	require.True(t, cfg.IsMicrosoftWSTEPSet())
	_, _, _, err = cfg.MicrosoftWSTEP()
	require.NoError(t, err, "Fleet accepts the generated pair")

	key, err := server.DecodePrivateKeyPEM(keyPEM)
	require.NoError(t, err, "the WSTEP signer reads the key as PKCS#1")
	require.Equal(t, microsoft_mdm.OpenframeWSTEPKeyBits, key.N.BitLen())

	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.True(t, cert.IsCA)
	require.NotZero(t, cert.KeyUsage&x509.KeyUsageCertSign)
	require.True(t, cert.NotAfter.After(time.Now().Add(9*365*24*time.Hour)))

	otherCert, _, err := microsoft_mdm.OpenframeNewWSTEPPair()
	require.NoError(t, err)
	require.NotEqual(t, certPEM, otherCert)
}

func TestOpenframeCheckWindowsWSTEP(t *testing.T) {
	certPEM, keyPEM, err := microsoft_mdm.OpenframeNewWSTEPPair()
	require.NoError(t, err)

	cfg := config.FleetConfig{}
	cfg.Server.PrivateKey = "a-server-private-key-of-32-bytes"
	require.NoError(t, openframeCheckWindowsWSTEP(&cfg))

	cfg.MDM.WindowsWSTEPIdentityCertBytes = string(certPEM)
	cfg.MDM.WindowsWSTEPIdentityKeyBytes = string(keyPEM)
	require.ErrorContains(t, openframeCheckWindowsWSTEP(&cfg), "unset the configured WSTEP certificate and key")

	cfg = config.FleetConfig{}
	require.ErrorContains(t, openframeCheckWindowsWSTEP(&cfg), "needs the server private key")
}
