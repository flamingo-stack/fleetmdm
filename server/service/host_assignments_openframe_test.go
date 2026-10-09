// OPENFRAME(host-assignments): unit tests for the fork-only host-assignment
// service methods — openframe/docs/architecture-host-assignments.md
//
// The 8 svc methods (add/remove/replace/list × policy/query hosts) previously
// had only a manual curl script; this pins down the openframe-mode gate, the
// authorization order, host-existence validation (incl. de-duplication), and
// delegation to the datastore, so an upstream sync cannot silently drop any of
// them. mock.Store only — no external deps.
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/fleetdm/fleet/v4/server/contexts/viewer"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mock"
	"github.com/stretchr/testify/require"
)

// newHostAssignmentTestSvc builds a service around a mock store pre-wired with
// a global policy 1 and a global query 1, plus hosts 1..3. The returned
// contexts carry the test license plus an admin/observer viewer.
func newHostAssignmentTestSvc(t *testing.T) (svc fleet.Service, ds *mock.Store, adminCtx, observerCtx context.Context) {
	t.Helper()
	ds = new(mock.Store)
	ds.PolicyFunc = func(ctx context.Context, id uint) (*fleet.Policy, error) {
		return &fleet.Policy{PolicyData: fleet.PolicyData{ID: id}}, nil
	}
	ds.QueryFunc = func(ctx context.Context, id uint) (*fleet.Query, error) {
		return &fleet.Query{ID: id}, nil
	}
	ds.ListHostsLiteByIDsFunc = func(ctx context.Context, ids []uint) ([]*fleet.Host, error) {
		var hosts []*fleet.Host
		for _, id := range ids {
			if id <= 3 {
				hosts = append(hosts, &fleet.Host{ID: id})
			}
		}
		return hosts, nil
	}
	svc, baseCtx := newTestService(t, ds, nil, nil)
	adminCtx = viewer.NewContext(baseCtx, viewer.Viewer{User: &fleet.User{ID: 1, GlobalRole: new(fleet.RoleAdmin)}})
	observerCtx = viewer.NewContext(baseCtx, viewer.Viewer{User: &fleet.User{ID: 2, GlobalRole: new(fleet.RoleObserver)}})
	return svc, ds, adminCtx, observerCtx
}

// TestOpenframeHostAssignmentsRequireOpenframeMode: every method must reject
// with BadRequest when FLEET_OPENFRAME_MODE is unset, before touching authz or
// the datastore.
func TestOpenframeHostAssignmentsRequireOpenframeMode(t *testing.T) {
	t.Setenv("FLEET_OPENFRAME_MODE", "") // hermetic baseline: don't depend on the ambient shell env
	svc, ds, ctx, _ := newHostAssignmentTestSvc(t)

	calls := map[string]func() error{
		"AddPolicyHosts":     func() error { _, err := svc.AddPolicyHosts(ctx, 1, []uint{1}); return err },
		"RemovePolicyHosts":  func() error { _, err := svc.RemovePolicyHosts(ctx, 1, []uint{1}); return err },
		"ReplacePolicyHosts": func() error { return svc.ReplacePolicyHosts(ctx, 1, []uint{1}) },
		"ListPolicyHosts":    func() error { _, _, err := svc.ListPolicyHosts(ctx, 1, fleet.ListOptions{}); return err },
		"AddQueryHosts":      func() error { _, err := svc.AddQueryHosts(ctx, 1, []uint{1}); return err },
		"RemoveQueryHosts":   func() error { _, err := svc.RemoveQueryHosts(ctx, 1, []uint{1}); return err },
		"ReplaceQueryHosts":  func() error { return svc.ReplaceQueryHosts(ctx, 1, []uint{1}) },
		"ListQueryHosts":     func() error { _, _, err := svc.ListQueryHosts(ctx, 1, fleet.ListOptions{}); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			require.Error(t, err)
			var br *fleet.BadRequestError
			require.ErrorAs(t, err, &br)
		})
	}
	require.False(t, ds.PolicyFuncInvoked, "the mode gate must fire before any datastore access")
	require.False(t, ds.QueryFuncInvoked, "the mode gate must fire before any datastore access")
}

func TestOpenframePolicyHostAssignments(t *testing.T) {
	t.Setenv("FLEET_OPENFRAME_MODE", "1")

	t.Run("add delegates to the datastore", func(t *testing.T) {
		svc, ds, adminCtx, _ := newHostAssignmentTestSvc(t)
		var gotPolicyID uint
		var gotHostIDs []uint
		ds.AddPolicyHostsFunc = func(ctx context.Context, policyID uint, hostIDs []uint) (uint, error) {
			gotPolicyID, gotHostIDs = policyID, hostIDs
			return uint(len(hostIDs)), nil
		}
		n, err := svc.AddPolicyHosts(adminCtx, 1, []uint{1, 2})
		require.NoError(t, err)
		require.Equal(t, uint(2), n)
		require.True(t, ds.AddPolicyHostsFuncInvoked)
		require.Equal(t, uint(1), gotPolicyID)
		require.Equal(t, []uint{1, 2}, gotHostIDs)
	})

	t.Run("add rejects nonexistent hosts", func(t *testing.T) {
		svc, ds, adminCtx, _ := newHostAssignmentTestSvc(t)
		ds.AddPolicyHostsFunc = func(ctx context.Context, policyID uint, hostIDs []uint) (uint, error) {
			return uint(len(hostIDs)), nil
		}
		_, err := svc.AddPolicyHosts(adminCtx, 1, []uint{1, 99})
		require.Error(t, err, "host 99 does not exist")
		require.False(t, ds.AddPolicyHostsFuncInvoked, "validation must run before the write")
	})

	t.Run("add deduplicates host ids before validating", func(t *testing.T) {
		svc, ds, adminCtx, _ := newHostAssignmentTestSvc(t)
		var validatedIDs []uint
		ds.ListHostsLiteByIDsFunc = func(ctx context.Context, ids []uint) ([]*fleet.Host, error) {
			validatedIDs = ids
			hosts := make([]*fleet.Host, len(ids))
			for i, id := range ids {
				hosts[i] = &fleet.Host{ID: id}
			}
			return hosts, nil
		}
		ds.AddPolicyHostsFunc = func(ctx context.Context, policyID uint, hostIDs []uint) (uint, error) {
			return uint(len(hostIDs)), nil
		}
		_, err := svc.AddPolicyHosts(adminCtx, 1, []uint{2, 2, 3, 2})
		require.NoError(t, err)
		require.Equal(t, []uint{2, 3}, validatedIDs,
			"duplicate ids must collapse or the existence count check would spuriously fail")
	})

	t.Run("observer cannot write assignments", func(t *testing.T) {
		svc, ds, _, observerCtx := newHostAssignmentTestSvc(t)
		_, err := svc.AddPolicyHosts(observerCtx, 1, []uint{1})
		require.Error(t, err)
		_, err = svc.RemovePolicyHosts(observerCtx, 1, []uint{1})
		require.Error(t, err)
		require.Error(t, svc.ReplacePolicyHosts(observerCtx, 1, []uint{1}))
		require.False(t, ds.AddPolicyHostsFuncInvoked)
		require.False(t, ds.RemovePolicyHostsFuncInvoked)
		require.False(t, ds.ReplacePolicyHostsFuncInvoked)
	})

	t.Run("observer can list assignments", func(t *testing.T) {
		svc, ds, _, observerCtx := newHostAssignmentTestSvc(t)
		ds.ListPolicyHostsFunc = func(ctx context.Context, policyID uint, opts fleet.ListOptions) ([]fleet.HostIdent, *fleet.PaginationMetadata, error) {
			return []fleet.HostIdent{{HostID: 1}}, nil, nil
		}
		hosts, _, err := svc.ListPolicyHosts(observerCtx, 1, fleet.ListOptions{})
		require.NoError(t, err)
		require.Len(t, hosts, 1)
	})

	t.Run("remove delegates without host validation", func(t *testing.T) {
		// Removing an already-deleted host must keep working, so Remove does
		// not require the hosts to still exist.
		svc, ds, adminCtx, _ := newHostAssignmentTestSvc(t)
		ds.RemovePolicyHostsFunc = func(ctx context.Context, policyID uint, hostIDs []uint) (uint, error) {
			return uint(len(hostIDs)), nil
		}
		n, err := svc.RemovePolicyHosts(adminCtx, 1, []uint{99})
		require.NoError(t, err)
		require.Equal(t, uint(1), n)
		require.False(t, ds.ListHostsLiteByIDsFuncInvoked)
	})

	t.Run("replace validates then delegates", func(t *testing.T) {
		svc, ds, adminCtx, _ := newHostAssignmentTestSvc(t)
		var gotHostIDs []uint
		ds.ReplacePolicyHostsFunc = func(ctx context.Context, policyID uint, hostIDs []uint) error {
			gotHostIDs = hostIDs
			return nil
		}
		require.NoError(t, svc.ReplacePolicyHosts(adminCtx, 1, []uint{1, 3}))
		require.Equal(t, []uint{1, 3}, gotHostIDs)

		require.Error(t, svc.ReplacePolicyHosts(adminCtx, 1, []uint{99}))
	})

	t.Run("missing policy surfaces the datastore error", func(t *testing.T) {
		svc, ds, adminCtx, _ := newHostAssignmentTestSvc(t)
		ds.PolicyFunc = func(ctx context.Context, id uint) (*fleet.Policy, error) {
			return nil, errors.New("policy not found")
		}
		_, err := svc.AddPolicyHosts(adminCtx, 42, []uint{1})
		require.Error(t, err)
		require.False(t, ds.AddPolicyHostsFuncInvoked)
	})
}

func TestOpenframeQueryHostAssignments(t *testing.T) {
	t.Setenv("FLEET_OPENFRAME_MODE", "1")

	t.Run("add validates then delegates", func(t *testing.T) {
		svc, ds, adminCtx, _ := newHostAssignmentTestSvc(t)
		var gotQueryID uint
		var gotHostIDs []uint
		ds.AddQueryHostsFunc = func(ctx context.Context, queryID uint, hostIDs []uint) (uint, error) {
			gotQueryID, gotHostIDs = queryID, hostIDs
			return uint(len(hostIDs)), nil
		}
		n, err := svc.AddQueryHosts(adminCtx, 1, []uint{2, 3})
		require.NoError(t, err)
		require.Equal(t, uint(2), n)
		require.Equal(t, uint(1), gotQueryID)
		require.Equal(t, []uint{2, 3}, gotHostIDs)

		_, err = svc.AddQueryHosts(adminCtx, 1, []uint{99})
		require.Error(t, err, "host 99 does not exist")
	})

	t.Run("observer cannot write assignments", func(t *testing.T) {
		svc, ds, _, observerCtx := newHostAssignmentTestSvc(t)
		_, err := svc.AddQueryHosts(observerCtx, 1, []uint{1})
		require.Error(t, err)
		_, err = svc.RemoveQueryHosts(observerCtx, 1, []uint{1})
		require.Error(t, err)
		require.Error(t, svc.ReplaceQueryHosts(observerCtx, 1, []uint{1}))
		require.False(t, ds.AddQueryHostsFuncInvoked)
		require.False(t, ds.RemoveQueryHostsFuncInvoked)
		require.False(t, ds.ReplaceQueryHostsFuncInvoked)
	})

	t.Run("remove and replace delegate", func(t *testing.T) {
		svc, ds, adminCtx, _ := newHostAssignmentTestSvc(t)
		ds.RemoveQueryHostsFunc = func(ctx context.Context, queryID uint, hostIDs []uint) (uint, error) {
			return uint(len(hostIDs)), nil
		}
		ds.ReplaceQueryHostsFunc = func(ctx context.Context, queryID uint, hostIDs []uint) error {
			return nil
		}
		n, err := svc.RemoveQueryHosts(adminCtx, 1, []uint{1, 2})
		require.NoError(t, err)
		require.Equal(t, uint(2), n)
		require.NoError(t, svc.ReplaceQueryHosts(adminCtx, 1, []uint{1}))
		require.True(t, ds.RemoveQueryHostsFuncInvoked)
		require.True(t, ds.ReplaceQueryHostsFuncInvoked)
	})

	t.Run("list delegates and returns metadata", func(t *testing.T) {
		svc, ds, adminCtx, _ := newHostAssignmentTestSvc(t)
		meta := &fleet.PaginationMetadata{HasNextResults: true}
		ds.ListQueryHostsFunc = func(ctx context.Context, queryID uint, opts fleet.ListOptions) ([]fleet.HostIdent, *fleet.PaginationMetadata, error) {
			return []fleet.HostIdent{{HostID: 1}, {HostID: 2}}, meta, nil
		}
		hosts, gotMeta, err := svc.ListQueryHosts(adminCtx, 1, fleet.ListOptions{})
		require.NoError(t, err)
		require.Len(t, hosts, 2)
		require.Equal(t, meta, gotMeta)
	})

	t.Run("missing query surfaces the datastore error", func(t *testing.T) {
		svc, ds, adminCtx, _ := newHostAssignmentTestSvc(t)
		ds.QueryFunc = func(ctx context.Context, id uint) (*fleet.Query, error) {
			return nil, errors.New("query not found")
		}
		_, _, err := svc.ListQueryHosts(adminCtx, 42, fleet.ListOptions{})
		require.Error(t, err)
		require.False(t, ds.ListQueryHostsFuncInvoked)
	})
}

// TestVerifyHostsToAssociate covers the shared host-existence validator used
// by the add/replace paths.
func TestVerifyHostsToAssociate(t *testing.T) {
	t.Run("empty list skips the datastore", func(t *testing.T) {
		ds := new(mock.Store)
		require.NoError(t, verifyHostsToAssociate(t.Context(), ds, nil))
		require.False(t, ds.ListHostsLiteByIDsFuncInvoked)
	})

	t.Run("all hosts exist", func(t *testing.T) {
		ds := new(mock.Store)
		ds.ListHostsLiteByIDsFunc = func(ctx context.Context, ids []uint) ([]*fleet.Host, error) {
			hosts := make([]*fleet.Host, len(ids))
			for i, id := range ids {
				hosts[i] = &fleet.Host{ID: id}
			}
			return hosts, nil
		}
		require.NoError(t, verifyHostsToAssociate(t.Context(), ds, []uint{1, 2}))
	})

	t.Run("a missing host fails", func(t *testing.T) {
		ds := new(mock.Store)
		ds.ListHostsLiteByIDsFunc = func(ctx context.Context, ids []uint) ([]*fleet.Host, error) {
			return nil, nil
		}
		require.Error(t, verifyHostsToAssociate(t.Context(), ds, []uint{1}))
	})

	t.Run("datastore error is propagated", func(t *testing.T) {
		ds := new(mock.Store)
		ds.ListHostsLiteByIDsFunc = func(ctx context.Context, ids []uint) ([]*fleet.Host, error) {
			return nil, errors.New("boom")
		}
		err := verifyHostsToAssociate(t.Context(), ds, []uint{1})
		require.Error(t, err)
		require.Contains(t, err.Error(), "boom")
	})
}
