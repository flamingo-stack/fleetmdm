// OPENFRAME(agent-verify-server-tls): a stored --insecure must not turn off TLS verification in openframe mode, and osqueryd must get a CA bundle — openframe/docs/agent-verify-server-tls.md
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func TestDropOpenframeInsecure(t *testing.T) {
	for _, tc := range []struct {
		name         string
		args         []string
		env          map[string]string
		wantInsecure bool
	}{
		{
			name:         "openframe mode drops the --insecure flag",
			args:         []string{"--openframe-mode", "--insecure"},
			wantInsecure: false,
		},
		{
			name:         "openframe mode drops ORBIT_INSECURE",
			args:         []string{"--openframe-mode"},
			env:          map[string]string{"ORBIT_INSECURE": "true"},
			wantInsecure: false,
		},
		{
			name:         "openframe mode keeps --insecure when development allows it",
			args:         []string{"--openframe-mode", "--insecure"},
			env:          map[string]string{openframeAllowInsecureEnvVar: "true"},
			wantInsecure: true,
		},
		{
			name:         "openframe mode drops --insecure when the development variable is false",
			args:         []string{"--openframe-mode", "--insecure"},
			env:          map[string]string{openframeAllowInsecureEnvVar: "false"},
			wantInsecure: false,
		},
		{
			name:         "openframe mode drops --insecure when the development variable is not a boolean",
			args:         []string{"--openframe-mode", "--insecure"},
			env:          map[string]string{openframeAllowInsecureEnvVar: "yes please"},
			wantInsecure: false,
		},
		{
			name:         "upstream mode keeps --insecure",
			args:         []string{"--insecure"},
			wantInsecure: true,
		},
		{
			name:         "openframe mode without --insecure stays verified",
			args:         []string{"--openframe-mode"},
			wantInsecure: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ORBIT_INSECURE", "")
			t.Setenv(openframeAllowInsecureEnvVar, "")
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			var gotInsecure bool
			app := &cli.App{
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "insecure", EnvVars: []string{"ORBIT_INSECURE"}},
					&cli.BoolFlag{Name: "openframe-mode"},
				},
				Action: func(c *cli.Context) error {
					if err := dropOpenframeInsecure(c); err != nil {
						return err
					}
					gotInsecure = c.Bool("insecure")
					return nil
				},
			}

			require.NoError(t, app.Run(append([]string{"orbit"}, tc.args...)))
			require.Equal(t, tc.wantInsecure, gotInsecure)
		})
	}
}

func TestOpenframeOsqueryCertPath(t *testing.T) {
	t.Run("fleet certificate is passed through and nothing is written", func(t *testing.T) {
		rootDir := t.TempDir()

		certPath, err := openframeOsqueryCertPath(rootDir, "/etc/openframe/local-ca.pem")

		require.NoError(t, err)
		require.Equal(t, "/etc/openframe/local-ca.pem", certPath)
		require.NoFileExists(t, filepath.Join(rootDir, "certs.pem"))
	})

	t.Run("embedded bundle is written to the root dir", func(t *testing.T) {
		rootDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(rootDir, "certs.pem"), []byte("stale"), 0o644))

		certPath, err := openframeOsqueryCertPath(rootDir, "")

		require.NoError(t, err)
		require.Equal(t, filepath.Join(rootDir, "certs.pem"), certPath)
		written, err := os.ReadFile(certPath)
		require.NoError(t, err)
		require.Equal(t, openframeOsqueryCerts, written)
	})

	t.Run("missing root dir is an error", func(t *testing.T) {
		rootDir := filepath.Join(t.TempDir(), "missing")

		_, err := openframeOsqueryCertPath(rootDir, "")

		require.ErrorContains(t, err, "write osquery certs")
	})
}

func TestOpenframeOsqueryCertsMatchPackaging(t *testing.T) {
	packagingCerts, err := os.ReadFile(filepath.Join("..", "..", "pkg", "packaging", "certs.pem"))
	require.NoError(t, err)

	require.Equal(t, packagingCerts, openframeOsqueryCerts, "copy orbit/pkg/packaging/certs.pem to orbit/cmd/orbit/openframe_certs.pem")
}
