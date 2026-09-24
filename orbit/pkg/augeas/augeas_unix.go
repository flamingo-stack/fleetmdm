//go:build !windows
// +build !windows

package augeas

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed lenses
var lenses embed.FS

func CopyLenses(installPath string) (string, error) {
	outPath := filepath.Join(installPath, "lenses")

	err := os.RemoveAll(outPath)
	if err != nil {
		return "", fmt.Errorf("remove existing lenses dir: %w", err)
	}
	err = os.MkdirAll(outPath, 0o755)
	if err != nil {
		return "", fmt.Errorf("create lenses dir: %w", err)
	}
	entries, err := lenses.ReadDir("lenses")
	if err != nil {
		return "", fmt.Errorf("read embedded lenses dir: %w", err)
	}
	for _, entry := range entries {
		src, err := lenses.Open(filepath.Join("lenses", entry.Name()))
		if err != nil {
			return "", fmt.Errorf("open embedded lens %s: %w", entry.Name(), err)
		}
		dest, err := os.OpenFile(filepath.Join(outPath, entry.Name()), os.O_CREATE|os.O_WRONLY, 0o644) // nolint:gosec // G302
		if err != nil {
			return "", fmt.Errorf("create lens file %s: %w", entry.Name(), err)
		}
		_, err = io.Copy(dest, src)
		if err != nil {
			return "", fmt.Errorf("copy lens file %s: %w", entry.Name(), err)
		}
		err = src.Close()
		if err != nil {
			return "", fmt.Errorf("close embedded lens %s: %w", entry.Name(), err)
		}
		err = dest.Close()
		if err != nil {
			return "", fmt.Errorf("close lens file %s: %w", entry.Name(), err)
		}
	}

	return outPath, nil
}
