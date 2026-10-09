package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/activity/api"
	"github.com/fleetdm/fleet/v4/server/activity/internal/testutils"
	"github.com/fleetdm/fleet/v4/server/activity/internal/types"
	// >>> OPENFRAME(mysql-multitenancy)
	"github.com/fleetdm/fleet/v4/server/datastore/mysql/migrations/openframe"
	"github.com/fleetdm/fleet/v4/server/fleet"
	// <<< OPENFRAME(mysql-multitenancy)
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewActivity(t *testing.T) {
	tdb := testutils.SetupTestDB(t, "activity_new")
	ds := NewDatastore(tdb.Conns(), tdb.Logger)
	env := &testEnv{TestDB: tdb, ds: ds}

	cases := []struct {
		name string
		fn   func(t *testing.T, env *testEnv)
	}{
		{"WebhookContextKeyRequired", testNewActivityWebhookContextKeyRequired},
		{"BasicWithUser", testNewActivityBasicWithUser},
		{"NilUser", testNewActivityNilUser},
		{"AutomationActivity", testNewActivityAutomation},
		{"HostAssociation", testNewActivityHostAssociation},
		{"HostOnly", testNewActivityHostOnly},
		{"DeletedUser", testNewActivityDeletedUser},
		// >>> OPENFRAME(mysql-multitenancy)
		{"OpenframeTeamStamping", testNewActivityOpenframeTeamStamping},
		// <<< OPENFRAME(mysql-multitenancy)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer env.TruncateTables(t)
			c.fn(t, env)
		})
	}
}

// dummyActivity is a minimal ActivityDetails implementation for testing.
type dummyActivity struct {
	name    string
	details map[string]any
}

func (d dummyActivity) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.details)
}

func (d dummyActivity) ActivityName() string {
	return d.name
}

// automatableActivity is a test activity that satisfies types.AutomatableActivity.
type automatableActivity struct {
	dummyActivity
}

func (a automatableActivity) WasFromAutomation() bool {
	return true
}

// hostActivity is a test activity that satisfies types.ActivityHosts.
type hostActivity struct {
	dummyActivity
	hostIDs []uint
}

func (h hostActivity) HostIDs() []uint {
	return h.hostIDs
}

// hostOnlyActivity is a test activity that satisfies types.ActivityHostOnly.
type hostOnlyActivity struct {
	dummyActivity
}

func (h hostOnlyActivity) HostOnly() bool {
	return true
}

// webhookCtx returns a context with the webhook key set, as required by NewActivity.
func webhookCtx(t *testing.T) context.Context {
	return context.WithValue(t.Context(), types.ActivityWebhookContextKey, true)
}

func testNewActivityWebhookContextKeyRequired(t *testing.T, env *testEnv) {
	ctx := t.Context()
	userID := env.InsertUser(t, "test", "test@example.com")
	user := &api.User{ID: userID, Name: "test", Email: "test@example.com"}
	activity := dummyActivity{name: "test", details: map[string]any{"key": "val"}}
	detailsJSON, err := json.Marshal(activity)
	require.NoError(t, err)

	// No webhook context key set; should fail
	assert.Error(t, env.ds.NewActivity(ctx, user, activity, detailsJSON, time.Now()))

	// Wrong context value type; should fail
	badCtx := context.WithValue(ctx, types.ActivityWebhookContextKey, "wrong")
	assert.Error(t, env.ds.NewActivity(badCtx, user, activity, detailsJSON, time.Now()))

	// Correct context key; should succeed
	assert.NoError(t, env.ds.NewActivity(webhookCtx(t), user, activity, detailsJSON, time.Now()))
}

func testNewActivityBasicWithUser(t *testing.T, env *testEnv) {
	ctx := webhookCtx(t)
	userID := env.InsertUser(t, "fullname", "email@example.com")
	user := &api.User{ID: userID, Name: "fullname", Email: "email@example.com"}

	details := map[string]any{"detail": 1, "sometext": "aaa"}
	detailsJSON, err := json.Marshal(details)
	require.NoError(t, err)

	require.NoError(t, env.ds.NewActivity(ctx, user, dummyActivity{name: "test_one", details: details}, detailsJSON, time.Now()))
	require.NoError(t, env.ds.NewActivity(ctx, user, dummyActivity{name: "test_two", details: map[string]any{"detail": 2}}, mustJSON(t, map[string]any{"detail": 2}), time.Now()))

	// Verify via listing (explicit ascending order for deterministic results)
	activities, _, err := env.ds.ListActivities(t.Context(), listOpts(withPerPage(1), withOrder("id", api.OrderAscending)))
	require.NoError(t, err)
	require.Len(t, activities, 1)
	assert.Equal(t, "fullname", *activities[0].ActorFullName)
	assert.Equal(t, "email@example.com", *activities[0].ActorEmail)
	assert.Equal(t, "test_one", activities[0].Type)

	// Second page
	activities, _, err = env.ds.ListActivities(t.Context(), listOpts(withPerPage(1), withPage(1), withOrder("id", api.OrderAscending)))
	require.NoError(t, err)
	require.Len(t, activities, 1)
	assert.Equal(t, "test_two", activities[0].Type)

	// All results
	activities, _, err = env.ds.ListActivities(t.Context(), listOpts(withOrder("id", api.OrderAscending)))
	require.NoError(t, err)
	assert.Len(t, activities, 2)
}

func testNewActivityNilUser(t *testing.T, env *testEnv) {
	ctx := webhookCtx(t)
	details := map[string]any{"detail": 1}
	detailsJSON := mustJSON(t, details)

	require.NoError(t, env.ds.NewActivity(ctx, nil, dummyActivity{name: "system_task", details: details}, detailsJSON, time.Now()))

	activities, _, err := env.ds.ListActivities(t.Context(), listOpts())
	require.NoError(t, err)
	require.Len(t, activities, 1)
	assert.Nil(t, activities[0].ActorID)
	assert.Nil(t, activities[0].ActorFullName)
	// user_email defaults to empty string (NOT NULL DEFAULT '') when no user is provided
	require.NotNil(t, activities[0].ActorEmail)
	assert.Empty(t, *activities[0].ActorEmail)
	assert.Equal(t, "system_task", activities[0].Type)
}

func testNewActivityAutomation(t *testing.T, env *testEnv) {
	ctx := webhookCtx(t)
	activity := automatableActivity{
		dummyActivity: dummyActivity{name: "auto_task", details: map[string]any{"automated": true}},
	}
	detailsJSON := mustJSON(t, activity.details)

	require.NoError(t, env.ds.NewActivity(ctx, nil, activity, detailsJSON, time.Now()))

	activities, _, err := env.ds.ListActivities(t.Context(), listOpts())
	require.NoError(t, err)
	require.Len(t, activities, 1)
	assert.Nil(t, activities[0].ActorID)
	require.NotNil(t, activities[0].ActorFullName)
	assert.Equal(t, types.ActivityAutomationAuthor, *activities[0].ActorFullName)
	assert.True(t, activities[0].FleetInitiated)
}

func testNewActivityHostAssociation(t *testing.T, env *testEnv) {
	ctx := webhookCtx(t)
	userID := env.InsertUser(t, "testuser", "test@example.com")
	user := &api.User{ID: userID, Name: "testuser", Email: "test@example.com"}
	hostID := env.InsertHost(t, "h1.local", nil)

	activity := hostActivity{
		dummyActivity: dummyActivity{name: "ran_script", details: map[string]any{"host_id": float64(hostID)}},
		hostIDs:       []uint{hostID},
	}
	detailsJSON := mustJSON(t, activity.details)

	require.NoError(t, env.ds.NewActivity(ctx, user, activity, detailsJSON, time.Now()))

	// Verify the activity is linked to the host via activity_host_past
	acts, _, err := env.ds.ListHostPastActivities(t.Context(), hostID, listOpts())
	require.NoError(t, err)
	require.Len(t, acts, 1)
	assert.Equal(t, "ran_script", acts[0].Type)
	require.NotNil(t, acts[0].ActorFullName)
	assert.Equal(t, "testuser", *acts[0].ActorFullName)
	require.NotNil(t, acts[0].ActorEmail)
	assert.Equal(t, "test@example.com", *acts[0].ActorEmail)
}

func testNewActivityHostOnly(t *testing.T, env *testEnv) {
	ctx := webhookCtx(t)
	userID := env.InsertUser(t, "testuser", "test@example.com")
	user := &api.User{ID: userID, Name: "testuser", Email: "test@example.com"}

	// Create a regular activity and a host-only activity
	regularDetails := mustJSON(t, map[string]any{"regular": true})
	require.NoError(t, env.ds.NewActivity(ctx, user, dummyActivity{name: "regular", details: map[string]any{"regular": true}}, regularDetails, time.Now()))

	hostOnlyDetails := mustJSON(t, map[string]any{"host_only": true})
	require.NoError(t, env.ds.NewActivity(ctx, user, hostOnlyActivity{
		dummyActivity: dummyActivity{name: "host_scoped", details: map[string]any{"host_only": true}},
	}, hostOnlyDetails, time.Now()))

	// ListActivities excludes host-only activities
	activities, _, err := env.ds.ListActivities(t.Context(), listOpts())
	require.NoError(t, err)
	require.Len(t, activities, 1)
	assert.Equal(t, "regular", activities[0].Type)
}

func testNewActivityDeletedUser(t *testing.T, env *testEnv) {
	ctx := webhookCtx(t)
	// User with Deleted=true should have their name/email preserved but user_id set to NULL
	user := &api.User{ID: 42, Name: "deleted_user", Email: "deleted@example.com", Deleted: true}
	details := mustJSON(t, map[string]any{"detail": 1})

	require.NoError(t, env.ds.NewActivity(ctx, user, dummyActivity{name: "post_delete", details: map[string]any{"detail": 1}}, details, time.Now()))

	activities, _, err := env.ds.ListActivities(t.Context(), listOpts())
	require.NoError(t, err)
	require.Len(t, activities, 1)
	// user_id should be NULL (deleted user), but name is preserved
	assert.Nil(t, activities[0].ActorID)
	require.NotNil(t, activities[0].ActorFullName)
	assert.Equal(t, "deleted_user", *activities[0].ActorFullName)
	require.NotNil(t, activities[0].ActorEmail)
	assert.Equal(t, "deleted@example.com", *activities[0].ActorEmail)
}

// mustJSON marshals v and fails the test on error.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// >>> OPENFRAME(mysql-multitenancy): CDC team_id stamping — openframe/docs/mysql-multitenancy-feature.md
// testNewActivityOpenframeTeamStamping verifies that with the multitenancy flag on:
//   - a team-pinned context stamps activity_past.team_id (the only tenant signal for host-less
//     activities);
//   - activity_host_past rows are stamped with their host's own team via the scalar subselect
//     (host activities may be written from unpinned cron contexts).
//
// The test schema comes from schema.sql, which does not include the OpenFrame migration
// pipeline, so the CDC migration is applied directly first.
func testNewActivityOpenframeTeamStamping(t *testing.T, env *testEnv) {
	// No t.Setenv (the harness runs parallel): the host-activity stamping gate is
	// (multitenancy env || ctx pin), and the pinned context below engages it.
	ctx := webhookCtx(t)

	// Apply the openframe CDC migration (idempotent) — adds team_id to the captured tables.
	tx, err := env.DB.Begin()
	require.NoError(t, err)
	require.NoError(t, openframe.Up_20260722000001(tx))
	require.NoError(t, tx.Commit())

	// A real team row (hosts.team_id has an FK to teams).
	res, err := env.DB.ExecContext(t.Context(), `INSERT INTO teams (name) VALUES ('cdc-team')`)
	require.NoError(t, err)
	teamID64, err := res.LastInsertId()
	require.NoError(t, err)
	hostTeamID := uint(teamID64) //nolint:gosec // test value from LastInsertId
	hostID := env.InsertHost(t, "cdc.local", &hostTeamID)

	userID := env.InsertUser(t, "cdcuser", "cdc@example.com")
	user := &api.User{ID: userID, Name: "cdcuser", Email: "cdc@example.com"}

	// Pin the request to a different team than the host's to tell the two stamps apart.
	const pinnedTeam = uint(77)
	pinnedCtx := fleet.NewOpenframeTeamContext(ctx, pinnedTeam)

	activity := hostActivity{
		dummyActivity: dummyActivity{name: "ran_script", details: map[string]any{"host_id": float64(hostID)}},
		hostIDs:       []uint{hostID},
	}
	require.NoError(t, env.ds.NewActivity(pinnedCtx, user, activity, mustJSON(t, activity.details), time.Now()))

	// activity_past carries the request pin.
	var actTeam sql.NullInt64
	var actID uint
	require.NoError(t, env.DB.QueryRowContext(t.Context(),
		`SELECT id, team_id FROM activity_past WHERE activity_type = 'ran_script' AND user_id = ? ORDER BY id DESC LIMIT 1`,
		userID).Scan(&actID, &actTeam))
	require.True(t, actTeam.Valid)
	require.EqualValues(t, pinnedTeam, actTeam.Int64)

	// activity_host_past carries the HOST's team (subselect), not the pin.
	var hostActTeam sql.NullInt64
	require.NoError(t, env.DB.QueryRowContext(t.Context(),
		`SELECT team_id FROM activity_host_past WHERE activity_id = ? AND host_id = ?`, actID, hostID).Scan(&hostActTeam))
	require.True(t, hostActTeam.Valid)
	require.EqualValues(t, hostTeamID, hostActTeam.Int64)
}

// <<< OPENFRAME(mysql-multitenancy)
