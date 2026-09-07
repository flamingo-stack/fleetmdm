// OPENFRAME(agent-skip-setup-experience): LinuxSetupExperiencer must not poll a disabled setup experience — openframe/docs/agent-skip-setup-experience.md
package setupexperience

import (
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/require"
)

type countingOrbitClient struct {
	calls int
}

func (c *countingOrbitClient) GetSetupExperienceStatus(bool) (*fleet.SetupExperienceStatusPayload, error) {
	c.calls++
	return &fleet.SetupExperienceStatusPayload{}, nil
}

func TestLinuxSetupExperiencerSkipsDisabled(t *testing.T) {
	for _, tc := range []struct {
		name      string
		enabled   bool
		wantCalls int
	}{
		{"disabled: no status poll", false, 0},
		{"enabled: polls and finishes", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rootDir := t.TempDir()
			require.NoError(t, WriteSetupExperienceStatusFile(rootDir, &SetupExperienceInfo{
				TimeInitiated: time.Now(),
				Enabled:       tc.enabled,
			}))

			client := &countingOrbitClient{}
			require.NoError(t, NewLinuxSetupExperiencer(client, rootDir).Run(&fleet.OrbitConfig{}))
			require.Equal(t, tc.wantCalls, client.calls)

			info, err := ReadSetupExperienceStatusFile(rootDir)
			require.NoError(t, err)
			require.Equal(t, tc.enabled, info.TimeFinished != nil)
		})
	}
}
