// >>> OPENFRAME(windows-wstep): the WSTEP CA a tenant gets with its first Windows enrollment — openframe/docs/mysql-multitenancy-feature.md

package microsoft_mdm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

const (
	OpenframeWSTEPKeyBits  = 4096
	OpenframeWSTEPValidity = 10 * 365 * 24 * time.Hour
)

// OpenframeNewWSTEPPair generates a self-signed WSTEP CA: the certificate PEM and its PKCS#1
// private key PEM, the form NewCertManager and the BitLocker key decryption read.
func OpenframeNewWSTEPPair() (certPEM, keyPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, OpenframeWSTEPKeyBits)
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
		NotAfter:              now.Add(OpenframeWSTEPValidity),
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
