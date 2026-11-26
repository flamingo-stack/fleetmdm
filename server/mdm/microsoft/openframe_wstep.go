// >>> OPENFRAME(windows-wstep): Windows MDM WSTEP pair generated once and kept in mdm_config_assets — openframe/docs/mysql-multitenancy-feature.md

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"log/slog"
	"math/big"
	"time"

	"github.com/fleetdm/fleet/v4/server/config"
	"github.com/fleetdm/fleet/v4/server/datastore/mysql"
)

const (
	openframeWindowsWSTEPKeyBits  = 4096
	openframeWindowsWSTEPValidity = 10 * 365 * 24 * time.Hour
)

func openframeLoadWindowsWSTEP(
	ctx context.Context,
	cfg *config.FleetConfig,
	ds *mysql.Datastore,
	logger *slog.Logger,
	initFatal func(err error, msg string),
) {
	if cfg.MDM.IsMicrosoftWSTEPSet() {
		initFatal(errors.New("FLEET_OPENFRAME_INSTANCE_WINDOWS_MDM_ENABLED keeps the WSTEP pair in the database; unset the configured WSTEP certificate and key"),
			"loading the Windows MDM WSTEP pair")
		return
	}
	certPEM, keyPEM, err := ds.OpenframeLoadWindowsWSTEP(ctx, openframeNewWindowsWSTEPPair)
	if err != nil {
		initFatal(err, "loading the Windows MDM WSTEP pair")
		return
	}
	cfg.MDM.WindowsWSTEPIdentityCertBytes = string(certPEM)
	cfg.MDM.WindowsWSTEPIdentityKeyBytes = string(keyPEM)
	logger.InfoContext(ctx, "Windows MDM WSTEP pair loaded from the database")
}

func openframeNewWindowsWSTEPPair() (certPEM, keyPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, openframeWindowsWSTEPKeyBits)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "OpenFrame Windows MDM WSTEP CA", Organization: []string{"OpenFrame"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(openframeWindowsWSTEPValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM, nil
}

// <<< OPENFRAME(windows-wstep)
