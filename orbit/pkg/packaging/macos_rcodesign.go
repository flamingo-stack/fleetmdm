package packaging

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/fleetdm/fleet/v4/pkg/retry"
	"github.com/fleetdm/fleet/v4/pkg/secure"
)

func rSign(pkgPath, cert string) error {
	pemFile, err := os.CreateTemp("", "cert-*.pem")
	if err != nil {
		return fmt.Errorf("creating cert temp file: %s", err)
	}
	pemPath := pemFile.Name()
	defer os.Remove(pemPath)
	if err := pemFile.Chmod(0o600); err != nil {
		pemFile.Close()
		return fmt.Errorf("setting cert temp file permissions: %s", err)
	}
	if _, err := pemFile.WriteString(cert); err != nil {
		pemFile.Close()
		return fmt.Errorf("writing cert data: %s", err)
	}
	if err := pemFile.Close(); err != nil {
		return fmt.Errorf("closing cert temp file: %s", err)
	}

	return retry.Do(func() error {
		var outBuf bytes.Buffer
		cmd := exec.Command(
			"rcodesign",
			"sign",
			pkgPath,
			"--pem-source", pemPath,
		)
		cmd.Stdout = &outBuf
		cmd.Stderr = &outBuf
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("rcodesign: %w: %s", err, outBuf.String())
		}
		return nil
	}, retry.WithMaxAttempts(3))
}

func rNotarizeStaple(pkg, apiKeyID, apiKeyIssuer, apiKeyContent string) error {
	path, err := writeAPIKeys(apiKeyIssuer, apiKeyID, apiKeyContent)
	defer os.Remove(path)
	if err != nil {
		return fmt.Errorf("writing API keys: %s", err)
	}

	return retry.Do(func() error {
		var outBuf bytes.Buffer
		cmd := exec.Command("rcodesign",
			"notarize",
			pkg,
			"--api-issuer", apiKeyIssuer,
			"--api-key", apiKeyID,
			"--staple",
		)
		cmd.Stdout = &outBuf
		cmd.Stderr = &outBuf
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("rcodesign notarize: %w: %s", err, outBuf.String())
		}
		return nil
	}, retry.WithMaxAttempts(3))
}

func writeAPIKeys(issuer, id, content string) (string, error) {
	homedir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home dir: %s", err)
	}

	// The underliying tools (rcodesign and Transporter) expect to find a
	// certificate key in this path.
	path := filepath.Join(homedir, ".appstoreconnect", "private_keys")
	if err = secure.MkdirAll(path, 0o700); err != nil {
		return "", fmt.Errorf("creating private keys dir: %s", err)
	}

	keyPath := filepath.Join(path, fmt.Sprintf("AuthKey_%s.p8", id))
	if err = os.WriteFile(keyPath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("writing api key contents: %s", err)
	}

	return keyPath, nil
}
