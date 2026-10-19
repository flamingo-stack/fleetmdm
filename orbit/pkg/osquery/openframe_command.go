package osquery

import "strings"

const openframeSecretFlag = "--openframe-secret"

// Note: omitOpenframeSecret only scrubs the secret from the string used for
// logging (see loggableCommand). It does NOT remove the secret from the
// actual process argv (r.cmd.Args) that gets exec'd, which remains visible to
// local users via /proc/<pid>/cmdline, `ps -ef`, or equivalent process-listing
// mechanisms on other OSes. This only protects log output, not the process
// table; a complete fix would require passing the secret via an environment
// variable or another non-argv channel instead of a CLI flag.
func (r *Runner) loggableCommand() string {
	return omitOpenframeSecret(r.cmd.Path, r.cmd.Args)
}

func omitOpenframeSecret(path string, args []string) string {
	parts := []string{path}
	for i := 1; i < len(args); i++ {
		switch {
		case args[i] == openframeSecretFlag:
			i++
		case strings.HasPrefix(args[i], openframeSecretFlag+"="):
		default:
			parts = append(parts, args[i])
		}
	}
	return strings.Join(parts, " ")
}
