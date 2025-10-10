package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fleetdm/fleet/v4/orbit/pkg/constant"
	"github.com/fleetdm/fleet/v4/orbit/pkg/update"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"
)

var cleanupCommand = &cli.Command{
	Name:  "cleanup",
	Usage: "Clean up orbit data, logs, and configuration (does not touch osquery process)",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "all",
			Usage: "Clean all data including enrollment secrets and node keys",
		},
		&cli.BoolFlag{
			Name:  "logs",
			Usage: "Clean only log files",
		},
		&cli.BoolFlag{
			Name:  "cache",
			Usage: "Clean only cache and temporary files",
		},
		&cli.BoolFlag{
			Name:  "secrets",
			Usage: "Clean enrollment secrets and node keys",
		},
		&cli.BoolFlag{
			Name:  "registry",
			Usage: "Clean Windows registry entries (Windows only)",
		},
		&cli.BoolFlag{
			Name:  "service",
			Usage: "Stop and remove orbit service (does not stop osqueryd)",
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "Show what would be deleted without actually deleting",
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "Skip confirmation prompt",
		},
	},
	Action: cleanupAction,
}

func cleanupAction(c *cli.Context) error {
	// Check for root/admin privileges
	if !hasAdminPrivileges() {
		return fmt.Errorf("This command requires administrator/root privileges.\nPlease run with sudo (macOS/Linux) or as Administrator (Windows)")
	}

	rootDir := c.String("root-dir")
	if rootDir == "" {
		rootDir = getDefaultRootDir()
	}

	dryRun := c.Bool("dry-run")
	force := c.Bool("force")
	cleanAll := c.Bool("all")
	cleanLogs := c.Bool("logs")
	cleanCache := c.Bool("cache")
	cleanSecrets := c.Bool("secrets")
	cleanRegistry := c.Bool("registry")
	cleanService := c.Bool("service")

	// If nothing specified, show help
	if !cleanAll && !cleanLogs && !cleanCache && !cleanSecrets && !cleanRegistry && !cleanService {
		return cli.ShowSubcommandHelp(c)
	}

	// Confirmation prompt
	if !force && !dryRun {
		fmt.Print("⚠️  This will delete orbit data. Continue? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Cleanup cancelled.")
			return nil
		}
	}

	if dryRun {
		fmt.Println("🔍 DRY RUN - No files will be deleted\n")
	}

	results := &cleanupResults{}

	// Stop service and processes (but NOT osqueryd!)
	if cleanAll || cleanService {
		if err := stopOrbitService(dryRun, results); err != nil {
			log.Error().Err(err).Msg("Failed to stop orbit service")
		}
	}

	// Clean files
	if cleanAll || cleanLogs {
		cleanLogFiles(rootDir, dryRun, results)
	}

	if cleanAll || cleanCache {
		cleanCacheFiles(rootDir, dryRun, results)
	}

	if cleanAll || cleanSecrets {
		cleanSecretFiles(rootDir, dryRun, results)
	}

	// Platform-specific cleanup
	if cleanAll || cleanRegistry {
		if runtime.GOOS == "windows" {
			cleanWindowsRegistry(dryRun, results)
		}
	}

	if cleanAll || cleanService {
		cleanServiceFiles(dryRun, results)
	}

	// Print results
	printResults(dryRun, results)

	if len(results.errors) > 0 {
		return fmt.Errorf("cleanup completed with %d errors", len(results.errors))
	}

	return nil
}

type cleanupResults struct {
	filesRemoved    []string
	servicesRemoved []string
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

// hasAdminPrivileges checks if running with admin/root privileges
func hasAdminPrivileges() bool {
	switch runtime.GOOS {
	case "windows":
		// Check if running as administrator
		cmd := exec.Command("net", "session")
		err := cmd.Run()
		return err == nil
	default: // unix-like
		return os.Geteuid() == 0
	}
}

// stopOrbitService stops orbit service and fleet-desktop, but NOT osqueryd
func stopOrbitService(dryRun bool, results *cleanupResults) error {
	fmt.Println("🛑 Stopping orbit service and fleet-desktop...")

	switch runtime.GOOS {
	case "darwin":
		return stopOrbitServiceMacOS(dryRun, results)
	case "linux":
		return stopOrbitServiceLinux(dryRun, results)
	case "windows":
		return stopOrbitServiceWindows(dryRun, results)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func stopOrbitServiceMacOS(dryRun bool, results *cleanupResults) error {
	commands := [][]string{
		{"launchctl", "stop", "com.fleetdm.orbit"},
		{"launchctl", "unload", "/Library/LaunchDaemons/com.fleetdm.orbit.plist"},
		{"pkill", "fleet-desktop"},
	}

	for _, cmd := range commands {
		if dryRun {
			fmt.Printf("  Would run: %s\n", strings.Join(cmd, " "))
		} else {
			c := exec.Command(cmd[0], cmd[1:]...)
			_ = c.Run() // Ignore errors (service might not be running)
		}
	}

	results.servicesRemoved = append(results.servicesRemoved, "com.fleetdm.orbit")
	return nil
}

func stopOrbitServiceLinux(dryRun bool, results *cleanupResults) error {
	commands := [][]string{
		{"systemctl", "stop", "orbit.service"},
		{"systemctl", "disable", "orbit.service"},
		{"pkill", "-f", "fleet-desktop"},
	}

	for _, cmd := range commands {
		if dryRun {
			fmt.Printf("  Would run: %s\n", strings.Join(cmd, " "))
		} else {
			c := exec.Command(cmd[0], cmd[1:]...)
			_ = c.Run() // Ignore errors
		}
	}

	results.servicesRemoved = append(results.servicesRemoved, "orbit.service")
	return nil
}

func stopOrbitServiceWindows(dryRun bool, results *cleanupResults) error {
	// Stop Windows Service
	if dryRun {
		fmt.Println("  Would run: Stop-Service -Name 'Fleet osquery'")
		fmt.Println("  Would run: Stop-Process -Name fleet-desktop")
	} else {
		// Stop service
		cmd := exec.Command("net", "stop", "Fleet osquery")
		_ = cmd.Run()

		// Kill fleet-desktop
		cmd = exec.Command("taskkill", "/F", "/IM", "fleet-desktop.exe")
		_ = cmd.Run()
	}

	results.servicesRemoved = append(results.servicesRemoved, "Fleet osquery")
	return nil
}

// cleanLogFiles removes log files
func cleanLogFiles(rootDir string, dryRun bool, results *cleanupResults) {
	fmt.Println("🗑️  Cleaning log files...")

	logPaths := getLogPaths(rootDir)
	for _, path := range logPaths {
		removePathIfExists(path, dryRun, results)
	}
}

// cleanCacheFiles removes cache and temporary files
func cleanCacheFiles(rootDir string, dryRun bool, results *cleanupResults) {
	fmt.Println("🗑️  Cleaning cache files...")

	cachePaths := getCachePaths(rootDir)
	for _, path := range cachePaths {
		removePathIfExists(path, dryRun, results)
	}
}

// cleanSecretFiles removes secrets and enrollment data
func cleanSecretFiles(rootDir string, dryRun bool, results *cleanupResults) {
	fmt.Println("🗑️  Cleaning secrets and enrollment data...")

	secretPaths := getSecretPaths(rootDir)
	for _, path := range secretPaths {
		removePathIfExists(path, dryRun, results)
	}
}

// cleanServiceFiles removes service configuration files
func cleanServiceFiles(dryRun bool, results *cleanupResults) {
	fmt.Println("🗑️  Cleaning service configuration files...")

	switch runtime.GOOS {
	case "darwin":
		paths := []string{
			"/Library/LaunchDaemons/com.fleetdm.orbit.plist",
			"/usr/local/bin/orbit",
		}
		for _, path := range paths {
			removePathIfExists(path, dryRun, results)
		}

		// Forget package
		if dryRun {
			fmt.Println("  Would run: pkgutil --forget com.fleetdm.orbit.base.pkg")
		} else {
			cmd := exec.Command("pkgutil", "--forget", "com.fleetdm.orbit.base.pkg")
			_ = cmd.Run()
		}

	case "linux":
		paths := []string{
			"/usr/lib/systemd/system/orbit.service",
			"/etc/default/orbit",
			"/usr/local/bin/orbit",
		}
		for _, path := range paths {
			removePathIfExists(path, dryRun, results)
		}

		// Reload systemd
		if !dryRun {
			cmd := exec.Command("systemctl", "daemon-reload")
			_ = cmd.Run()
		}

	case "windows":
		// Windows service removal is handled in stopOrbitService
	}
}

// cleanWindowsRegistry removes Windows registry entries
func cleanWindowsRegistry(dryRun bool, results *cleanupResults) {
	if runtime.GOOS != "windows" {
		return
	}

	fmt.Println("🗑️  Cleaning Windows registry...")

	registryCommands := []string{
		// Remove uninstall entry
		`Get-ChildItem "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall" | Where-Object { (Get-ItemProperty $_.PSPath).DisplayName -eq "Fleet osquery" } | Remove-Item -Recurse -Force`,
		// Remove service entry
		`Remove-Item "HKLM:\SYSTEM\CurrentControlSet\Services\Fleet osquery" -Recurse -Force -ErrorAction SilentlyContinue`,
	}

	for _, regCmd := range registryCommands {
		if dryRun {
			fmt.Printf("  Would run PowerShell: %s\n", regCmd)
		} else {
			cmd := exec.Command("powershell", "-Command", regCmd)
			_ = cmd.Run()
		}
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
func removePathIfExists(path string, dryRun bool, results *cleanupResults) {
	// Check if path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	if dryRun {
		fmt.Printf("  Would remove: %s\n", path)
		results.filesRemoved = append(results.filesRemoved, path)
	} else {
		fmt.Printf("  Removing: %s\n", path)
		if err := os.RemoveAll(path); err != nil {
			log.Error().Err(err).Msgf("Failed to remove: %s", path)
			results.errors = append(results.errors, err)
		} else {
			results.filesRemoved = append(results.filesRemoved, path)
		}
	}
}

// printResults prints cleanup results
func printResults(dryRun bool, results *cleanupResults) {
	fmt.Println()
	fmt.Println("=" + strings.Repeat("=", 50))

	if dryRun {
		fmt.Printf("✅ Would clean %d files/directories\n", len(results.filesRemoved))
		fmt.Printf("✅ Would remove %d services\n", len(results.servicesRemoved))
	} else {
		fmt.Printf("✅ Cleaned %d files/directories\n", len(results.filesRemoved))
		fmt.Printf("✅ Removed %d services\n", len(results.servicesRemoved))
	}

	if len(results.errors) > 0 {
		fmt.Printf("⚠️  %d errors occurred\n", len(results.errors))
	}

	fmt.Println("=" + strings.Repeat("=", 50))
	fmt.Println()

	if !dryRun && len(results.errors) == 0 {
		fmt.Println("🎉 Cleanup completed successfully!")
		fmt.Println()
		fmt.Println("Note: osqueryd process was NOT stopped (as requested)")
	}
}

