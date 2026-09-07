package ghapi

import (
	"bytes"
	"os/exec"

	"fleetdm/gm/pkg/logger"
)

// RunCommandAndReturnOutput runs a bash command, captures its output, and returns the output as a byte slice.
//
// Deprecated: This executes the given string via `bash -c`, which is prone to shell-injection
// if the command string is ever built from untrusted input. Prefer RunArgsAndReturnOutput, which
// executes a fixed argv without shell interpretation.
func RunCommandAndReturnOutput(command string) ([]byte, error) {
	logger.Debugf("Running COMMAND: %s", command)
	cmd := exec.Command("bash", "-c", command)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		logger.Errorf("Error running command: %s", out.String())
		return nil, err
	}
	return out.Bytes(), nil
}

// RunArgsAndReturnOutput runs a command given as an explicit argv (name plus arguments), captures
// its output, and returns the output as a byte slice. Unlike RunCommandAndReturnOutput, this does
// not invoke a shell, so caller-supplied argument values cannot be interpreted as shell syntax.
func RunArgsAndReturnOutput(name string, args ...string) ([]byte, error) {
	logger.Debugf("Running COMMAND: %s %v", name, args)
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		logger.Errorf("Error running command: %s", out.String())
		return nil, err
	}
	return out.Bytes(), nil
}
