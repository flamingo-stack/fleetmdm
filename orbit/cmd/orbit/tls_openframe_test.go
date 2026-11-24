// OPENFRAME(agent-verify-server-tls): a stored --insecure must not turn off TLS verification in openframe mode, and osqueryd must get a CA bundle — openframe/docs/agent-verify-server-tls.md
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
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

	t.Run("failed write returns the error and still the bundle path", func(t *testing.T) {
		rootDir := filepath.Join(t.TempDir(), "missing")

		certPath, err := openframeOsqueryCertPath(rootDir, "")

		require.ErrorContains(t, err, "write osquery certs")
		require.Equal(t, filepath.Join(rootDir, "certs.pem"), certPath)
	})
}

func TestOrbitActionUsesOpenframeTLSHelpers(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "orbit.go", nil, 0)
	require.NoError(t, err)
	action := findFuncDecl(file, "orbitAction")
	require.NotNil(t, action, "orbitAction not found in orbit.go")

	drop := firstCallPos(action.Body, func(call *ast.CallExpr) bool { return isCallTo(call, "dropOpenframeInsecure") })
	insecureRead := firstCallPos(action.Body, isInsecureRead)
	require.True(t, drop.IsValid(), "orbitAction must call dropOpenframeInsecure")
	require.True(t, insecureRead.IsValid(), "orbitAction no longer reads --insecure")
	require.Less(t, int(drop), int(insecureRead), "dropOpenframeInsecure must run before orbitAction first reads --insecure")

	certBlock := blockAssigningFrom(action.Body, "openframeOsqueryCertPath")
	require.NotNil(t, certBlock, "orbitAction must call openframeOsqueryCertPath")
	require.True(t, containsStringLit(certBlock, `"--tls_server_certs"`), "the openframeOsqueryCertPath result must reach --tls_server_certs")
}

func findFuncDecl(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

func firstCallPos(root ast.Node, match func(*ast.CallExpr) bool) token.Pos {
	first := token.NoPos
	ast.Inspect(root, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok && match(call) && (!first.IsValid() || call.Pos() < first) {
			first = call.Pos()
		}
		return true
	})
	return first
}

func isCallTo(call *ast.CallExpr, name string) bool {
	ident, ok := call.Fun.(*ast.Ident)
	return ok && ident.Name == name
}

func isInsecureRead(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Bool" || len(call.Args) != 1 {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	return ok && lit.Value == `"insecure"`
}

func blockAssigningFrom(root ast.Node, funcName string) *ast.BlockStmt {
	var found *ast.BlockStmt
	ast.Inspect(root, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for _, stmt := range block.List {
			assign, ok := stmt.(*ast.AssignStmt)
			if !ok || len(assign.Rhs) != 1 {
				continue
			}
			if call, ok := assign.Rhs[0].(*ast.CallExpr); ok && isCallTo(call, funcName) {
				found = block
			}
		}
		return true
	})
	return found
}

func containsStringLit(root ast.Node, value string) bool {
	found := false
	ast.Inspect(root, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Value == value {
			found = true
		}
		return !found
	})
	return found
}

func TestOpenframeOsqueryCertsMatchPackaging(t *testing.T) {
	packagingCerts, err := os.ReadFile(filepath.Join("..", "..", "pkg", "packaging", "certs.pem"))
	require.NoError(t, err)

	require.Equal(t, packagingCerts, openframeOsqueryCerts, "copy orbit/pkg/packaging/certs.pem to orbit/cmd/orbit/openframe_certs.pem")
}
