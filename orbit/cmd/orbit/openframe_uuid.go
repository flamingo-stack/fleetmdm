package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fleetdm/fleet/v4/orbit/pkg/constant"
	"github.com/fleetdm/fleet/v4/orbit/pkg/update"
	"github.com/google/uuid"
	"github.com/urfave/cli/v2"
)

// uuidCommand gets host UUID from osquery in OpenFrame mode
var uuidCommand = &cli.Command{
	Name:  "uuid",
	Usage: "Get the host hardware UUID in OpenFrame mode",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "json",
			Usage: "Output UUID in JSON format",
		},
		&cli.BoolFlag{
			Name:    "openframe-mode",
			Usage:   "Enable OpenFrame mode for osquery",
			EnvVars: []string{"ORBIT_OPENFRAME_MODE"},
		},
		&cli.StringFlag{
			Name:    "openframe-osquery-path",
			Usage:   "Custom path to osqueryd binary when using OpenFrame mode",
			EnvVars: []string{"ORBIT_OPENFRAME_OSQUERY_PATH"},
		},
	},
	Action: uuidAction,
}

func uuidAction(c *cli.Context) error {
	// Check that we're running in OpenFrame mode
	if !c.Bool("openframe-mode") {
		return fmt.Errorf("This command only works in OpenFrame mode.\nPlease run with --openframe-mode flag or set ORBIT_OPENFRAME_MODE environment variable")
	}

	// Set up root directory
	rootDir := c.String("root-dir")
	if rootDir == "" {
		rootDir = update.DefaultOptions.RootDirectory
		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to get orbit executable: %w", err)
		}
		if strings.HasPrefix(executable, "/var/lib/orbit") {
			rootDir = "/var/lib/orbit"
		}
	}

	var osquerydPath string

	// Get OpenFrame osqueryd path
	osquerydPath = c.String("openframe-osquery-path")
	if osquerydPath == "" {
		return fmt.Errorf("openframe-osquery-path must be specified when openframe-mode is enabled")
	}
	if _, err := os.Stat(osquerydPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("custom openframe osqueryd binary not found: %s", osquerydPath)
		} else {
			return fmt.Errorf("failed to check custom openframe osqueryd binary: %w", err)
		}
	}

	// Use temporary database for UUID query
	tmpDBPath := filepath.Join(os.TempDir(), fmt.Sprintf("orbit-uuid-%s", uuid.NewString()))
	defer os.RemoveAll(tmpDBPath)

	hostUUID, err := getHostUUID(osquerydPath, tmpDBPath)
	if err != nil {
		return fmt.Errorf("failed to get host UUID: %w", err)
	}

	if c.Bool("json") {
		fmt.Printf("{\"uuid\":\"%s\"}\n", hostUUID)
	} else {
		fmt.Println(hostUUID)
	}
	return nil
}

func getHostUUID(osqueryPath string, osqueryDBPath string) (string, error) {
	// Make sure parent directory exists (`osqueryd -S` doesn't create the parent directories).
	if err := os.MkdirAll(filepath.Dir(osqueryDBPath), constant.DefaultDirMode); err != nil {
		return "", err
	}
	const uuidQuery = `SELECT uuid FROM system_info`
	args := []string{
		"-S",
		"--database_path", osqueryDBPath,
		"--json", uuidQuery,
	}
	cmd := exec.Command(osqueryPath, args...)
	var (
		osquerydStdout bytes.Buffer
		osquerydStderr bytes.Buffer
	)
	cmd.Stdout = &osquerydStdout
	cmd.Stderr = &osquerydStderr

	var result []map[string]interface{}
	if err := cmd.Run(); err != nil {
		// Try to unmarshal the result even if there's an error (osquery exit status 78 issue)
		unmarshalErr := json.Unmarshal(osquerydStdout.Bytes(), &result)
		if unmarshalErr != nil {
			return "", fmt.Errorf("osqueryd failed: %w, output: %s, stderr: %s", err, osquerydStdout.String(), osquerydStderr.String())
		}
	} else {
		if err := json.Unmarshal(osquerydStdout.Bytes(), &result); err != nil {
			return "", fmt.Errorf("failed to parse osqueryd output: %w", err)
		}
	}

	if len(result) != 1 {
		return "", fmt.Errorf("expected 1 row from UUID query, got %d", len(result))
	}

	uuid, ok := result[0]["uuid"].(string)
	if !ok {
		return "", fmt.Errorf("UUID field not found or not a string")
	}

	return uuid, nil
}

