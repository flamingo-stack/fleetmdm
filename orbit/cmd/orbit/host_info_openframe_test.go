// OPENFRAME(agent-host-info-diagnostics): the host-info row-count error must carry osqueryd's stderr and name WMI on Windows — openframe/docs/agent-host-info-diagnostics.md
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestHostInfoRowsError(t *testing.T) {
	const (
		rowsPrefix = "invalid number of rows from system info query: "
		wmiStderr  = "W1005 12:00:00.000000  4321 os_version.cpp:33] enum osquery::WmiError[0] (WmiRequest creation failed to connect to server: 0x800706ba)\r\n"
		wmiFlat    = "W1005 12:00:00.000000 4321 os_version.cpp:33] enum osquery::WmiError[0] (WmiRequest creation failed to connect to server: 0x800706ba)"
		sqlStderr  = "Error: no such table: os_version\n"
		sqlFlat    = "Error: no such table: os_version"
	)
	for _, tc := range []struct {
		name       string
		rows       int
		stderr     string
		exitedOK   bool
		goos       string
		wantWMI    bool
		wantStderr string
	}{
		{"windows, no rows, osqueryd warning", 0, wmiStderr, true, "windows", true, wmiFlat},
		{"windows, no rows, silent osqueryd", 0, "", true, "windows", true, ""},
		{"windows, no rows, failed query", 0, sqlStderr, false, "windows", false, sqlFlat},
		{"windows, too many rows", 2, "", true, "windows", false, ""},
		{"macos, no rows", 0, sqlStderr, true, "darwin", false, sqlFlat},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := hostInfoRowsError(tc.rows, tc.stderr, tc.exitedOK, tc.goos).Error()

			require.True(t, strings.HasPrefix(msg, rowsPrefix), msg)
			require.Equal(t, tc.wantWMI, strings.Contains(msg, "WMI (Win32_OperatingSystem)"), msg)
			require.NotContains(t, msg, "\n")
			if tc.wantStderr == "" {
				require.NotContains(t, msg, "osqueryd stderr")
			} else {
				require.True(t, strings.HasSuffix(msg, "; osqueryd stderr: "+tc.wantStderr), msg)
			}
		})
	}
}

func TestHostInfoRowsErrorTruncatesStderr(t *testing.T) {
	// The leading byte shifts the two-byte runes so the cut lands inside one.
	stderr := "a" + strings.Repeat("é", maxHostInfoStderrLen)

	msg := hostInfoRowsError(0, stderr, true, "windows").Error()

	require.True(t, strings.HasSuffix(msg, " ...(truncated)"), msg)
	require.Less(t, len(msg), maxHostInfoStderrLen+256)
	require.True(t, utf8.ValidString(msg))
}

func TestGetHostInfoNoRowsCarriesOsquerydStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake osqueryd is a shell script")
	}
	for _, exitCode := range []int{0, 1} {
		t.Run(fmt.Sprintf("exit %d", exitCode), func(t *testing.T) {
			dir := t.TempDir()
			fakeOsqueryd := filepath.Join(dir, "osqueryd")
			script := fmt.Sprintf("#!/bin/sh\necho '[]'\necho 'W1005 os_version.cpp:33] WmiRequest creation failed in ExecQuery: 0x80041010' >&2\nexit %d\n", exitCode)
			require.NoError(t, os.WriteFile(fakeOsqueryd, []byte(script), 0o755))

			info, err := getHostInfo(fakeOsqueryd, filepath.Join(dir, "db", "osquery.db"))

			require.Nil(t, info)
			require.ErrorContains(t, err, "invalid number of rows from system info query: 0")
			require.ErrorContains(t, err, "osqueryd stderr: W1005 os_version.cpp:33] WmiRequest creation failed in ExecQuery: 0x80041010")
		})
	}
}
