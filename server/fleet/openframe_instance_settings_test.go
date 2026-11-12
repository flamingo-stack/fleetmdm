package fleet

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOpenframeInstanceSettings(t *testing.T) {
	valid := []struct {
		name, url, org string
		want           OpenframeInstanceSettings
	}{
		{"both unset", "", "", OpenframeInstanceSettings{}},
		{"trailing slash and spaces trimmed", " https://mdm.example.com/ ", " Flamingo ", OpenframeInstanceSettings{ServerURL: "https://mdm.example.com", OrgName: "Flamingo"}},
		{"path prefix kept", "https://example.com/fleet", "", OpenframeInstanceSettings{ServerURL: "https://example.com/fleet"}},
		{"org name alone", "", "Flamingo", OpenframeInstanceSettings{OrgName: "Flamingo"}},
	}
	for _, c := range valid {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseOpenframeInstanceSettings(c.url, c.org)
			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}

	for _, raw := range []string{"mdm.example.com", "ftp://mdm.example.com", "https://", "https://mdm.example.com?x=1", "https://mdm.example.com?", "https://mdm.example.com#a", "https://mdm.example.com#", "https://mdm.example.com/?", "https://u:p@mdm.example.com", "https://mdm example.com"} {
		t.Run("rejects "+raw, func(t *testing.T) {
			_, err := parseOpenframeInstanceSettings(raw, "")
			require.ErrorContains(t, err, "FLEET_OPENFRAME_INSTANCE_SERVER_URL")
		})
	}
}

func TestOpenframeInstanceSettingsApplyTo(t *testing.T) {
	cfg := &AppConfig{}
	cfg.ServerSettings.ServerURL = "https://old.example.com"
	cfg.OrgInfo.OrgName = "Old"

	OpenframeInstanceSettings{}.ApplyTo(cfg)
	require.Equal(t, "https://old.example.com", cfg.ServerSettings.ServerURL, "unset values leave the row alone")
	require.Equal(t, "Old", cfg.OrgInfo.OrgName)

	OpenframeInstanceSettings{ServerURL: "https://mdm.example.com", OrgName: "Flamingo"}.ApplyTo(cfg)
	require.Equal(t, "https://mdm.example.com", cfg.ServerSettings.ServerURL)
	require.Equal(t, "Flamingo", cfg.OrgInfo.OrgName)
}

func TestOpenframeAppConfigTeamID(t *testing.T) {
	pinned := NewOpenframeTeamContext(context.Background(), 5)

	teamID, ok := OpenframeAppConfigTeamID(pinned)
	require.True(t, ok)
	require.Equal(t, uint(5), teamID)

	_, ok = OpenframeAppConfigTeamID(NewOpenframeInstanceConfigContext(pinned))
	require.False(t, ok, "the instance config marker selects the instance row")

	require.False(t, OpenframeInstanceConfigFromContext(pinned))
}
