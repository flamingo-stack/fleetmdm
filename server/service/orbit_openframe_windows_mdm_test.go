package service

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mock"
	"github.com/stretchr/testify/require"
)

func TestOpenframeWindowsMDMServerURL(t *testing.T) {
	ds := new(mock.Store)
	ds.AppConfigFunc = func(ctx context.Context) (*fleet.AppConfig, error) {
		cfg := &fleet.AppConfig{}
		if fleet.OpenframeInstanceConfigFromContext(ctx) {
			cfg.ServerSettings.ServerURL = "https://mdm.example.com/mdm"
		} else {
			cfg.ServerSettings.ServerURL = "https://tenant.example.com"
		}
		return cfg, nil
	}
	tenantCfg := &fleet.AppConfig{}
	tenantCfg.ServerSettings.ServerURL = "https://tenant.example.com"
	ctx := fleet.NewOpenframeTeamContext(t.Context(), 7)

	got, err := openframeWindowsMDMServerURL(ctx, ds, tenantCfg, true)
	require.NoError(t, err)
	require.Equal(t, "https://mdm.example.com/mdm", got, "shared mode uses the instance row's public MDM address")

	ds.AppConfigFuncInvoked = false
	got, err = openframeWindowsMDMServerURL(ctx, ds, tenantCfg, false)
	require.NoError(t, err)
	require.Equal(t, "https://tenant.example.com", got, "outside shared mode the config already loaded is used")
	require.False(t, ds.AppConfigFuncInvoked)
}
