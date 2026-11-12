package service

import (
	"context"

	"github.com/fleetdm/fleet/v4/server/fleet"
)

type openframeSharedModeCtxKey struct{}

func openframeSharedMode(ctx context.Context) bool {
	if shared, ok := ctx.Value(openframeSharedModeCtxKey{}).(bool); ok {
		return shared
	}
	return fleet.IsOpenframeSharedMode()
}

func openframeAndroidEnrollTeam(secretTeamID, priorTeamID *uint, priorFound, shared bool) *uint {
	if priorFound && !shared {
		return priorTeamID
	}
	return secretTeamID
}

func openframeAndroidReenrollAsNewHost(hostTeamID, secretTeamID *uint, shared bool) bool {
	if !shared {
		return false
	}
	if hostTeamID == nil || secretTeamID == nil {
		return hostTeamID != secretTeamID
	}
	return *hostTeamID != *secretTeamID
}

func openframeInstanceConfigContext(ctx context.Context) context.Context {
	if openframeSharedMode(ctx) {
		return fleet.NewOpenframeInstanceConfigContext(ctx)
	}
	return ctx
}
