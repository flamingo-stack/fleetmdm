package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fleetdm/fleet/v4/orbit/pkg/constant"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

var cleanupCommand = &cli.Command{
	Name:  "cleanup",
	Usage: "Clean up all orbit data, logs, secrets and stop osqueryd process in OpenFrame mode",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "openframe-mode",
			Usage:   "Enable OpenFrame mode for cleanup",
			EnvVars: []string{"ORBIT_OPENFRAME_MODE"},
		},
		&cli.StringFlag{
			Name:    "openframe-osquery-path",
			Usage:   "Custom path to osqueryd binary when using OpenFrame mode",
			EnvVars: []string{"ORBIT_OPENFRAME_OSQUERY_PATH"},
		},
	},
	Action: cleanupAction,
}

func cleanupAction(c *cli.Context) error {
	// Check that we're running in OpenFrame mode
	if !c.Bool("openframe-mode") {
		return fmt.Errorf("This command only works in OpenFrame mode.\nPlease run with --openframe-mode flag or set ORBIT_OPENFRAME_MODE environment variable")
	}

	rootDir := c.String("root-dir")
	if rootDir == "" {
		rootDir = getDefaultRootDir()
	}

	fmt.Println("🧹 Starting OpenFrame cleanup...")
	results := &cleanupResults{}

	// Stop osqueryd process in OpenFrame mode
	osquerydPath := c.String("openframe-osquery-path")
	if err := stopOsqueryd(osquerydPath, results); err != nil {
		log.Error().Err(err).Msg("Failed to stop osqueryd")
	}

	// Clean all files
	cleanLogFiles(rootDir, results)
	cleanCacheFiles(rootDir, results)
	cleanSecretFiles(rootDir, results)

	// Print results
	printResults(results)

	if len(results.errors) > 0 {
		return fmt.Errorf("cleanup completed with %d errors", len(results.errors))
	}

	return nil
}

type cleanupResults struct {
	filesRemoved    []string
	processesKilled []string
	errors          []error
}

// getDefaultRootDir returns the default root directory based on OS
func getDefaultRootDir() string {
	switch runtime.GOOS {
	case "darwin":
		return "/opt/orbit"
	case "windows":
		return filepath.Join(os.Getenv("ProgramFiles"), "Orbit")
	default: // linux
		return "/opt/orbit"
	}
}

// stopOsqueryd stops the osqueryd process in OpenFrame mode
func stopOsqueryd(osquerydPath string, results *cleanupResults) error {
	fmt.Println("🛑 Stopping osqueryd process...")

	switch runtime.GOOS {
	case "darwin", "linux":
		cmd := exec.Command("pkill", "osqueryd")
		if err := cmd.Run(); err != nil {
			// Process might not be running, that's okay
			log.Debug().Err(err).Msg("pkill osqueryd returned error (process might not be running)")
		}
		results.processesKilled = append(results.processesKilled, "osqueryd")
		fmt.Println("  Stopped osqueryd process")
	case "windows":
		cmd := exec.Command("taskkill", "/F", "/IM", "osqueryd.exe")
		if err := cmd.Run(); err != nil {
			// Process might not be running, that's okay
			log.Debug().Err(err).Msg("taskkill osqueryd.exe returned error (process might not be running)")
		}
		results.processesKilled = append(results.processesKilled, "osqueryd.exe")
		fmt.Println("  Stopped osqueryd.exe process")
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return nil
}

// cleanLogFiles removes log files
func cleanLogFiles(rootDir string, results *cleanupResults) {
	fmt.Println("🗑️  Cleaning log files...")

	logPaths := getLogPaths(rootDir)
	for _, path := range logPaths {
		removePathIfExists(path, results)
	}
}

// cleanCacheFiles removes cache and temporary files
func cleanCacheFiles(rootDir string, results *cleanupResults) {
	fmt.Println("🗑️  Cleaning cache files...")

	cachePaths := getCachePaths(rootDir)
	for _, path := range cachePaths {
		removePathIfExists(path, results)
	}
}

// cleanSecretFiles removes secrets and enrollment data
func cleanSecretFiles(rootDir string, results *cleanupResults) {
	fmt.Println("🗑️  Cleaning secrets and enrollment data...")

	secretPaths := getSecretPaths(rootDir)
	for _, path := range secretPaths {
		removePathIfExists(path, results)
	}
}

// getLogPaths returns paths to log files
func getLogPaths(rootDir string) []string {
	paths := []string{
		filepath.Join(rootDir, "osquery_log"),
		filepath.Join(rootDir, "orbit.stderr.log"),
		filepath.Join(rootDir, "orbit.stdout.log"),
	}

	switch runtime.GOOS {
	case "darwin", "linux":
		paths = append(paths, "/var/log/orbit")
	case "windows":
		systemProfile := filepath.Join(os.Getenv("SystemRoot"), "system32", "config", "systemprofile")
		paths = append(paths, filepath.Join(systemProfile, "AppData", "Local", "FleetDM", "Orbit", "Logs"))
	}

	return paths
}

// getCachePaths returns paths to cache and temporary files
func getCachePaths(rootDir string) []string {
	paths := []string{
		filepath.Join(rootDir, "shell"),           // Temporary shell data
		filepath.Join(rootDir, "update-metadata"), // TUF update cache
		filepath.Join(rootDir, "updates.json"),    // Old update metadata
	}

	// Find and add .old files
	filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(path, ".old") {
			paths = append(paths, path)
		}
		return nil
	})

	return paths
}

// getSecretPaths returns paths to secrets and enrollment data
func getSecretPaths(rootDir string) []string {
	return []string{
		filepath.Join(rootDir, constant.OrbitNodeKeyFileName),
		filepath.Join(rootDir, constant.DesktopTokenFileName),
		filepath.Join(rootDir, constant.OsqueryEnrollSecretFileName),
		filepath.Join(rootDir, constant.ServerOverridesFileName),
		filepath.Join(rootDir, "osquery.db"),
		filepath.Join(rootDir, "osquery.db-wal"),
		filepath.Join(rootDir, "osquery.db-shm"),
	}
}

// removePathIfExists removes a path if it exists
func removePathIfExists(path string, results *cleanupResults) {
	// Check if path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	fmt.Printf("  Removing: %s\n", path)
	if err := os.RemoveAll(path); err != nil {
		log.Error().Err(err).Msgf("Failed to remove: %s", path)
		results.errors = append(results.errors, err)
	} else {
		results.filesRemoved = append(results.filesRemoved, path)
	}
}

// printResults prints cleanup results
func printResults(results *cleanupResults) {
	fmt.Println()
	fmt.Println("=" + strings.Repeat("=", 50))

	fmt.Printf("✅ Cleaned %d files/directories\n", len(results.filesRemoved))
	fmt.Printf("✅ Stopped %d processes\n", len(results.processesKilled))

	if len(results.errors) > 0 {
		fmt.Printf("⚠️  %d errors occurred\n", len(results.errors))
	}

	fmt.Println("=" + strings.Repeat("=", 50))
	fmt.Println()

	if len(results.errors) == 0 {
		fmt.Println("🎉 OpenFrame cleanup completed successfully!")
	}
}
