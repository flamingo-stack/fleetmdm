package service

import (
	"context"
	"testing"

	"github.com/fleetdm/fleet/v4/server/contexts/viewer"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mock"
	"github.com/fleetdm/fleet/v4/server/ptr"
	"github.com/stretchr/testify/require"
)

// TestOpenframeDeleteGlobalPoliciesPinnedTeam verifies the OPENFRAME(mysql-multitenancy) adjustment
// in DeleteGlobalPolicies: under a per-request tenant pin the tenant's own policies carry the pinned
// team id (creation re-homes them), so the upstream "belongs to a team → Forbidden" check must let
// own-team policies through. Foreign-team policies still reject, and unpinned behavior is unchanged.
func TestOpenframeDeleteGlobalPoliciesPinnedTeam(t *testing.T) {
	const pinnedTeam = uint(7)

	newSvc := func(policyTeamID *uint) (fleet.Service, context.Context) {
		ds := new(mock.Store)
		ds.PoliciesByIDFunc = func(ctx context.Context, ids []uint) (map[uint]*fleet.Policy, error) {
			policies := make(map[uint]*fleet.Policy, len(ids))
			for _, id := range ids {
				policies[id] = &fleet.Policy{PolicyData: fleet.PolicyData{ID: id, TeamID: policyTeamID}}
			}
			return policies, nil
		}
		// Empty deleted set keeps the post-delete activity loop out of scope for this test.
		ds.DeleteGlobalPoliciesFunc = func(ctx context.Context, ids []uint) ([]uint, error) {
			return nil, nil
		}
		ds.AppConfigFunc = func(ctx context.Context) (*fleet.AppConfig, error) {
			return &fleet.AppConfig{}, nil
		}
		svc, ctx := newTestService(t, ds, nil, nil)
		ctx = viewer.NewContext(ctx, viewer.Viewer{User: &fleet.User{GlobalRole: ptr.String(fleet.RoleAdmin)}})
		return svc, ctx
	}

	t.Run("pinned: own-team policy deletes without Forbidden", func(t *testing.T) {
		svc, ctx := newSvc(ptr.Uint(pinnedTeam))
		ctx = fleet.NewOpenframeTeamContext(ctx, pinnedTeam)

		_, err := svc.DeleteGlobalPolicies(ctx, []uint{1})
		require.NoError(t, err)
	})

	t.Run("pinned: foreign-team policy still Forbidden", func(t *testing.T) {
		svc, ctx := newSvc(ptr.Uint(pinnedTeam + 1))
		ctx = fleet.NewOpenframeTeamContext(ctx, pinnedTeam)

		_, err := svc.DeleteGlobalPolicies(ctx, []uint{1})
		require.Error(t, err, "foreign-team policy must remain Forbidden")
	})

	t.Run("unpinned: team policy Forbidden (upstream behavior unchanged)", func(t *testing.T) {
		svc, ctx := newSvc(ptr.Uint(pinnedTeam))

		_, err := svc.DeleteGlobalPolicies(ctx, []uint{1})
		require.Error(t, err, "unpinned team-policy delete must keep upstream's Forbidden")
	})

	t.Run("unpinned: global (nil-team) policy deletes fine", func(t *testing.T) {
		svc, ctx := newSvc(nil)

		_, err := svc.DeleteGlobalPolicies(ctx, []uint{1})
		require.NoError(t, err)
	})
}

// TestOpenframeModifyGlobalPolicyPinnedTeam verifies the OPENFRAME(mysql-multitenancy) adjustment
// in modifyPolicy: the tenant UI edits policies through the global endpoint, but creation re-homed
// them to the pinned team, so under a pin the upstream "policy does not belong to team/global"
// check must let own-team policies through. Foreign-team policies still reject, and unpinned
// behavior is unchanged.
func TestOpenframeModifyGlobalPolicyPinnedTeam(t *testing.T) {
	const pinnedTeam = uint(7)

	newSvc := func(policyTeamID *uint) (fleet.Service, context.Context) {
		ds := new(mock.Store)
		ds.PolicyFunc = func(ctx context.Context, id uint) (*fleet.Policy, error) {
			return &fleet.Policy{PolicyData: fleet.PolicyData{ID: id, TeamID: policyTeamID, Query: "SELECT 1"}}, nil
		}
		ds.SavePolicyFunc = func(ctx context.Context, p *fleet.Policy, shouldRemoveAllPolicyMemberships bool, removePolicyStats bool) error {
			return nil
		}
		ds.AppConfigFunc = func(ctx context.Context) (*fleet.AppConfig, error) {
			return &fleet.AppConfig{}, nil
		}
		svc, ctx := newTestService(t, ds, nil, nil)
		ctx = viewer.NewContext(ctx, viewer.Viewer{User: &fleet.User{GlobalRole: ptr.String(fleet.RoleAdmin)}})
		return svc, ctx
	}

	payload := fleet.ModifyPolicyPayload{Name: ptr.String("renamed")}

	t.Run("pinned: own-team policy modifies through the global endpoint", func(t *testing.T) {
		svc, ctx := newSvc(ptr.Uint(pinnedTeam))
		ctx = fleet.NewOpenframeTeamContext(ctx, pinnedTeam)

		policy, err := svc.ModifyGlobalPolicy(ctx, 1, payload)
		require.NoError(t, err)
		require.Equal(t, "renamed", policy.Name)
	})

	t.Run("pinned: foreign-team policy still rejects", func(t *testing.T) {
		svc, ctx := newSvc(ptr.Uint(pinnedTeam + 1))
		ctx = fleet.NewOpenframeTeamContext(ctx, pinnedTeam)

		_, err := svc.ModifyGlobalPolicy(ctx, 1, payload)
		require.Error(t, err, "foreign-team policy must keep the bad-request reject")
	})

	t.Run("unpinned: team policy rejects (upstream behavior unchanged)", func(t *testing.T) {
		svc, ctx := newSvc(ptr.Uint(pinnedTeam))

		_, err := svc.ModifyGlobalPolicy(ctx, 1, payload)
		require.Error(t, err, "unpinned team-policy modify must keep upstream's reject")
	})

	t.Run("unpinned: global (nil-team) policy modifies fine", func(t *testing.T) {
		svc, ctx := newSvc(nil)

		policy, err := svc.ModifyGlobalPolicy(ctx, 1, payload)
		require.NoError(t, err)
		require.Equal(t, "renamed", policy.Name)
	})
}
