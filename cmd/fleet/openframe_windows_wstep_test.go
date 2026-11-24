// OPENFRAME(windows-wstep)
package main

import (
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server"
	"github.com/fleetdm/fleet/v4/server/config"
	"github.com/fleetdm/fleet/v4/server/datastore/mysql/mysqltest"
	"github.com/stretchr/testify/require"
)

func TestOpenframeNewWindowsWSTEPPair(t *testing.T) {
	certPEM, keyPEM, err := openframeNewWindowsWSTEPPair()
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
	require.Equal(t, openframeWindowsWSTEPKeyBits, key.N.BitLen())

	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	require.True(t, cert.IsCA)
	require.NotZero(t, cert.KeyUsage&x509.KeyUsageCertSign)
	require.True(t, cert.NotAfter.After(time.Now().Add(9*365*24*time.Hour)))

	otherCert, _, err := openframeNewWindowsWSTEPPair()
	require.NoError(t, err)
	require.NotEqual(t, certPEM, otherCert)
}

func TestOpenframeLoadWindowsWSTEPRefusesConfiguredPair(t *testing.T) {
	certPEM, keyPEM, err := openframeNewWindowsWSTEPPair()
	require.NoError(t, err)
	cfg := config.FleetConfig{MDM: config.MDMConfig{
		WindowsWSTEPIdentityCertBytes: string(certPEM),
		WindowsWSTEPIdentityKeyBytes:  string(keyPEM),
	}}

	var fatal []error
	openframeLoadWindowsWSTEP(t.Context(), &cfg, nil, slog.New(slog.DiscardHandler), func(err error, _ string) { fatal = append(fatal, err) })
	require.Len(t, fatal, 1)
	require.ErrorContains(t, fatal[0], "unset the configured WSTEP certificate and key")
}

func TestOpenframeLoadWindowsWSTEPFromDatabase(t *testing.T) {
	ds := mysqltest.CreateMySQLDS(t)
	logger := slog.New(slog.DiscardHandler)
	var fatal []error
	initFatal := func(err error, _ string) { fatal = append(fatal, err) }

	var first config.FleetConfig
	openframeLoadWindowsWSTEP(t.Context(), &first, ds, logger, initFatal)
	require.Empty(t, fatal)
	require.True(t, first.MDM.IsMicrosoftWSTEPSet())
	_, _, _, err := first.MDM.MicrosoftWSTEP()
	require.NoError(t, err)

	var second config.FleetConfig
	openframeLoadWindowsWSTEP(t.Context(), &second, ds, logger, initFatal)
	require.Empty(t, fatal)
	require.Equal(t, first.MDM.WindowsWSTEPIdentityCertBytes, second.MDM.WindowsWSTEPIdentityCertBytes, "a restart reuses the stored pair")
	require.Equal(t, first.MDM.WindowsWSTEPIdentityKeyBytes, second.MDM.WindowsWSTEPIdentityKeyBytes)
}
