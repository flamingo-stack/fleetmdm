package osquery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactOpenframeSecret(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "separate value",
			args: []string{"osqueryd", "--openframe-mode", "true", "--openframe-secret", "k3y", "--openframe-token-path", "/p"},
			want: "/bin/osqueryd --openframe-mode true --openframe-secret *** --openframe-token-path /p",
		},
		{
			name: "equals value",
			args: []string{"osqueryd", "--openframe-secret=k3y", "--verbose"},
			want: "/bin/osqueryd --openframe-secret=*** --verbose",
		},
		{
			name: "flag without value",
			args: []string{"osqueryd", "--openframe-secret"},
			want: "/bin/osqueryd --openframe-secret",
		},
		{
			name: "no secret",
			args: []string{"osqueryd", "--pidfile=/p", "--verbose"},
			want: "/bin/osqueryd --pidfile=/p --verbose",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, redactOpenframeSecret("/bin/osqueryd", tt.args))
		})
	}
}

func TestLoggableCommandMasksSecretAndKeepsArgs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "osqueryd")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	r, err := NewRunner(path, WithFlags([]string{"--openframe-mode", "true", "--openframe-secret", "k3y"}))
	require.NoError(t, err)

	assert.NotContains(t, r.loggableCommand(), "k3y")
	assert.Contains(t, r.loggableCommand(), "--openframe-secret ***")
	assert.Equal(t, []string{"--openframe-mode", "true", "--openframe-secret", "k3y"}, r.cmd.Args[len(r.cmd.Args)-4:])
}
