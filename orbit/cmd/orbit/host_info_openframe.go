// OPENFRAME(agent-host-info-diagnostics): host-info row-count error that carries osqueryd's stderr and names WMI on Windows — openframe/docs/agent-host-info-diagnostics.md
package main

import (
	"errors"
	"fmt"
	"strings"
)

// maxHostInfoStderrLen bounds the stderr carried in the error, which is logged again on every orbit restart.
const maxHostInfoStderrLen = 2048

func hostInfoRowsError(rows int, osquerydStderr string, osquerydExitedOK bool, goos string) error {
	msg := fmt.Sprintf("invalid number of rows from system info query: %d", rows)
	// A failed query also prints an empty result, and then the cause is in stderr, not necessarily WMI.
	if rows == 0 && osquerydExitedOK && goos == "windows" {
		msg += "; on Windows this means osquery got no OS data from WMI (Win32_OperatingSystem)"
	}
	if stderr := compactOsquerydOutput(osquerydStderr); stderr != "" {
		msg += "; osqueryd stderr: " + stderr
	}
	return errors.New(msg)
}

func compactOsquerydOutput(output string) string {
	output = strings.Join(strings.Fields(output), " ")
	if len(output) > maxHostInfoStderrLen {
		output = strings.ToValidUTF8(output[:maxHostInfoStderrLen], "") + " ...(truncated)"
	}
	return output
}
