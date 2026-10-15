package osquery

import "strings"

const openframeSecretFlag = "--openframe-secret"

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
