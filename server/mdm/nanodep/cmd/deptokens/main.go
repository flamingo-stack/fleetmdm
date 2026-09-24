package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/fleetdm/fleet/v4/server/mdm/nanodep/tokenpki"
	"golang.org/x/crypto/pbkdf2"
)

const (
	defaultCN   = "deptokens"
	defaultDays = 1

	pemEncryptedType = "ENCRYPTED RSA PRIVATE KEY"
	pbkdf2Iterations = 100000
	pbkdf2SaltLen    = 16
)

// overridden by -ldflags -X
var version = "unknown"

func main() {
	var (
		flCert     = flag.String("cert", "cert.pem", "path to certificate")
		flKey      = flag.String("key", "cert.key", "path to key")
		flPassword = flag.String("password", "", "password to encrypt/decrypt private key with")
		flTokens   = flag.String("token", "", "path to tokens")
		flForce    = flag.Bool("f", false, "force overwriting the keypair")
		flVersion  = flag.Bool("version", false, "print version")
	)
	flag.Parse()

	if *flVersion {
		fmt.Println(version)
		return
	}

	var err error
	if *flTokens == "" {
		if *flPassword == "" {
			fmt.Println("WARNING: no password provided, private key will be saved in clear text")
		}
		err = generateKeyPair(*flCert, *flKey, *flPassword, *flForce)
		if err == nil {
			fmt.Printf("wrote %s, %s\n", *flCert, *flKey)
		}
	} else {
		var jsonBytes []byte
		jsonBytes, err = decryptTokens(*flTokens, *flCert, *flKey, *flPassword)
		if err == nil {
			_, _ = os.Stdout.Write(jsonBytes)
		}
	}
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}

// encodeEncryptedKeyPEM generates a PEM structure for key optionally
// encrypting it with password. Encryption, when used, is performed with
// AES-256-GCM using a PBKDF2-derived key, rather than the legacy/deprecated
// PEM encryption (which relies on 3DES/PBKDF1 and is not considered secure).
func encodeEncryptedKeyPEM(key *rsa.PrivateKey, password string) ([]byte, error) {
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	var block *pem.Block
	if password == "" {
		block = &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: keyBytes,
		}
	} else {
		salt := make([]byte, pbkdf2SaltLen)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, err
		}
		derivedKey := pbkdf2.Key([]byte(password), salt, pbkdf2Iterations, 32, sha256.New)
		gcmCipher, err := aes.NewCipher(derivedKey)
		if err != nil {
			return nil, err
		}
		gcm, err := cipher.NewGCM(gcmCipher)
		if err != nil {
			return nil, err
		}
		nonce := make([]byte, gcm.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return nil, err
		}
		ciphertext := gcm.Seal(nil, nonce, keyBytes, nil)
		// layout: salt || nonce || ciphertext
		payload := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
		payload = append(payload, salt...)
		payload = append(payload, nonce...)
		payload = append(payload, ciphertext...)
		block = &pem.Block{
			Type:  pemEncryptedType,
			Bytes: payload,
		}
	}
	return pem.EncodeToMemory(block), nil
}

// decodeEncryptedKeyPEM decodes an private key in pemBytes optionally
// decrypting it with password. Supports both the modern AES-256-GCM format
// written by encodeEncryptedKeyPEM and legacy x509 PEM-encrypted blocks for
// backwards compatibility with previously generated keys.
func decodeEncryptedKeyPEM(pemBytes []byte, password string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	switch block.Type {
	case "RSA PRIVATE KEY":
		keyBytes := block.Bytes
		if x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck // retained for backwards compatibility with legacy encrypted keys
			if password == "" {
				return nil, errors.New("no password supplied for encrypted PEM")
			}
			var err error
			keyBytes, err = x509.DecryptPEMBlock(block, []byte(password)) //nolint:staticcheck // retained for backwards compatibility with legacy encrypted keys
			if err != nil {
				return nil, err
			}
		}
		return x509.ParsePKCS1PrivateKey(keyBytes)
	case pemEncryptedType:
		if password == "" {
			return nil, errors.New("no password supplied for encrypted PEM")
		}
		if len(block.Bytes) < pbkdf2SaltLen {
			return nil, errors.New("invalid encrypted PEM: too short")
		}
		salt := block.Bytes[:pbkdf2SaltLen]
		rest := block.Bytes[pbkdf2SaltLen:]
		derivedKey := pbkdf2.Key([]byte(password), salt, pbkdf2Iterations, 32, sha256.New)
		gcmCipher, err := aes.NewCipher(derivedKey)
		if err != nil {
			return nil, err
		}
		gcm, err := cipher.NewGCM(gcmCipher)
		if err != nil {
			return nil, err
		}
		if len(rest) < gcm.NonceSize() {
			return nil, errors.New("invalid encrypted PEM: too short")
		}
		nonce := rest[:gcm.NonceSize()]
		ciphertext := rest[gcm.NonceSize():]
		keyBytes, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return nil, err
		}
		return x509.ParsePKCS1PrivateKey(keyBytes)
	default:
		return nil, errors.New("PEM type is not RSA PRIVATE KEY")
	}
}

// generateKeyPair creates and saves a keypair checking whether they exist first.
func generateKeyPair(certFile, keyFile, password string, force bool) error {
	if !force {
		_, err := os.Stat(certFile)
		certExists := err == nil
		_, err = os.Stat(keyFile)
		keyExists := err == nil
		if keyExists || certExists {
			return errors.New("cert or key already exist, not overwriting")
		}
	}
	key, cert, err := tokenpki.SelfSignedRSAKeypair(defaultCN, defaultDays)
	if err != nil {
		return fmt.Errorf("generating keypair: %w", err)
	}
	err = os.WriteFile(certFile, tokenpki.PEMCertificate(cert.Raw), 0644)
	if err != nil {
		return fmt.Errorf("writing cert: %w", err)
	}
	keyPEM, err := encodeEncryptedKeyPEM(key, password)
	if err == nil {
		err = os.WriteFile(keyFile, keyPEM, 0600)
	}
	if err != nil {
		return fmt.Errorf("writing key: %w", err)
	}
	return nil
}

// decryptTokens reads tokenFile from disk and decrypts it using certFile and keyfile (with optional password).
func decryptTokens(tokenFile, certFile, keyFile, password string) ([]byte, error) {
	keyBytes, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	key, err := decodeEncryptedKeyPEM(keyBytes, password)
	if err != nil {
		return nil, err
	}
	tokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, err
	}
	certBytes, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	cert, err := tokenpki.CertificateFromPEM(certBytes)
	if err != nil {
		return nil, err
	}
	return tokenpki.DecryptTokenJSON(tokenBytes, cert, key)
}
