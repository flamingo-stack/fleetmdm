package osquery

import "strings"

const (
	openframeSecretFlag     = "--openframe-secret"
	openframeSecretRedacted = "***"
)

func (r *Runner) loggableCommand() string {
	return redactOpenframeSecret(r.cmd.Path, r.cmd.Args)
}

func redactOpenframeSecret(path string, args []string) string {
	parts := []string{path}
	if len(args) > 1 {
		parts = append(parts, args[1:]...)
	}
	for i := 1; i < len(parts); i++ {
		switch {
		case parts[i] == openframeSecretFlag && i+1 < len(parts):
			parts[i+1] = openframeSecretRedacted
			i++
		case strings.HasPrefix(parts[i], openframeSecretFlag+"="):
			parts[i] = openframeSecretFlag + "=" + openframeSecretRedacted
		}
	}
	return strings.Join(parts, " ")
}
