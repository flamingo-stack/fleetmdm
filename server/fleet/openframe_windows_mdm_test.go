// OPENFRAME(windows-mdm-tenant)
package fleet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOpenframeWindowsServerURL(t *testing.T) {
	for raw, want := range map[string]string{
		"https://acme.openframe.ai/mdm":    "https://acme.openframe.ai/mdm",
		" https://Acme.OpenFrame.ai/mdm/ ": "https://acme.openframe.ai/mdm",
		"https://acme.openframe.ai:8443":   "https://acme.openframe.ai:8443",
	} {
		got, err := ParseOpenframeWindowsServerURL(raw)
		require.NoError(t, err, raw)
		require.Equal(t, want, got, raw)
	}
	for _, raw := range []string{
		"",
		"http://acme.openframe.ai/mdm",
		"acme.openframe.ai/mdm",
		"https://user:pw@acme.openframe.ai/mdm",
		"https://acme.openframe.ai/mdm?x=1",
		"https://acme.openframe.ai/mdm#x",
		"https://acme openframe.ai/mdm",
		"https:///mdm",
		"https://-acme.openframe.ai/mdm",
	} {
		_, err := ParseOpenframeWindowsServerURL(raw)
		require.Error(t, err, raw)
	}
}
