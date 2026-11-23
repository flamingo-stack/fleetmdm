// OPENFRAME(agent-verify-server-tls): in openframe mode a stored --insecure no longer turns off TLS verification, and osqueryd gets a CA bundle — openframe/docs/agent-verify-server-tls.md
package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

const openframeAllowInsecureEnvVar = "ORBIT_OPENFRAME_ALLOW_INSECURE"

//go:embed openframe_certs.pem
var openframeOsqueryCerts []byte

func dropOpenframeInsecure(c *cli.Context) error {
	if !c.Bool("openframe-mode") || !c.Bool("insecure") {
		return nil
	}
	if allowed, _ := strconv.ParseBool(os.Getenv(openframeAllowInsecureEnvVar)); allowed {
		log.Warn().Msgf("openframe mode: TLS certificate verification is off because %s is set", openframeAllowInsecureEnvVar)
		return nil
	}
	log.Warn().Msg("openframe mode: ignoring --insecure, TLS certificates are verified")
	return c.Set("insecure", "false")
}

func openframeOsqueryCertPath(rootDir, fleetCertificate string) (string, error) {
	if fleetCertificate != "" {
		return fleetCertificate, nil
	}
	certPath := filepath.Join(rootDir, "certs.pem")
	if err := os.WriteFile(certPath, openframeOsqueryCerts, 0o644); err != nil {
		return "", fmt.Errorf("write osquery certs: %w", err)
	}
	return certPath, nil
}
