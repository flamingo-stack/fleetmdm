package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type streakTestServer struct {
	*httptest.Server
	mu           sync.Mutex
	rejectAuthed bool
	enrollCalls  int
}

func newStreakTestServer(t *testing.T) *streakTestServer {
	t.Helper()
	s := &streakTestServer{rejectAuthed: true}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/orbit/enroll") {
			s.enrollCalls++
			writeEnrollResponse(t, w, "new-key")
			return
		}
		if s.rejectAuthed {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		assert.NoError(t, json.NewEncoder(w).Encode(fleet.OrbitConfig{}))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *streakTestServer) setRejectAuthed(reject bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejectAuthed = reject
}

func (s *streakTestServer) enrolls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enrollCalls
}

func requestConfig(oc *OrbitClient) error {
	return oc.authenticatedRequest("POST", "/api/fleet/orbit/config", &fleet.OrbitGetConfigRequest{}, &fleet.OrbitConfig{})
}

func TestUnauthenticatedStreakSurvivesRestart(t *testing.T) {
	setReenrollGracePeriod(t, 50*time.Millisecond)
	_, nodeKeyPath := newNodeKeyFile(t, "existing-key")
	streakPath := nodeKeyPath + ".unauthenticated"
	srv := newStreakTestServer(t)

	require.ErrorIs(t, requestConfig(newReenrollTestClient(t, srv.URL, nodeKeyPath)), ErrUnauthenticated)
	require.FileExists(t, streakPath, "the first 401 should save the streak start")

	time.Sleep(60 * time.Millisecond)

	second := newReenrollTestClient(t, srv.URL, nodeKeyPath)
	require.ErrorIs(t, requestConfig(second), ErrUnauthenticated)
	require.True(t, second.reenrollForced(), "a restarted orbit should continue the saved streak past the grace period")
	require.Zero(t, srv.enrolls())
	requireNodeKey(t, nodeKeyPath, "existing-key")

	third := newReenrollTestClient(t, srv.URL, nodeKeyPath)
	require.True(t, third.reenrollForced(), "a re-enroll armed by an earlier run should apply after another restart")
	require.Zero(t, srv.enrolls())
}

func TestUnauthenticatedStreakArmedEarlierEnrollsFirst(t *testing.T) {
	setReenrollGracePeriod(t, time.Hour)
	_, nodeKeyPath := newNodeKeyFile(t, "existing-key")
	streakPath := nodeKeyPath + ".unauthenticated"
	require.NoError(t, os.WriteFile(streakPath, []byte(time.Now().Add(-2*time.Hour).UTC().Format(time.RFC3339Nano)+" armed"), 0o600))
	srv := newStreakTestServer(t)
	srv.setRejectAuthed(false)

	oc := newReenrollTestClient(t, srv.URL, nodeKeyPath)
	require.NoError(t, requestConfig(oc))
	require.Equal(t, 1, srv.enrolls(), "a re-enroll armed by an earlier run should happen before the first request")
	requireNodeKey(t, nodeKeyPath, "new-key")
	require.False(t, oc.reenrollForced())
	require.NoFileExists(t, streakPath)
}

func TestUnauthenticatedStreakClearedBySuccess(t *testing.T) {
	setReenrollGracePeriod(t, time.Hour)
	_, nodeKeyPath := newNodeKeyFile(t, "existing-key")
	streakPath := nodeKeyPath + ".unauthenticated"
	srv := newStreakTestServer(t)

	require.ErrorIs(t, requestConfig(newReenrollTestClient(t, srv.URL, nodeKeyPath)), ErrUnauthenticated)
	require.FileExists(t, streakPath)

	srv.setRejectAuthed(false)

	restarted := newReenrollTestClient(t, srv.URL, nodeKeyPath)
	require.NoError(t, requestConfig(restarted))
	require.NoFileExists(t, streakPath)
	require.Zero(t, srv.enrolls())
	requireNodeKey(t, nodeKeyPath, "existing-key")
}

func TestLoadUnauthenticatedStreakIgnoresBadContent(t *testing.T) {
	now := time.Now()
	cases := map[string]string{
		"empty":        "",
		"not a time":   "yesterday armed",
		"future start": now.Add(time.Hour).UTC().Format(time.RFC3339Nano) + " armed",
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			_, nodeKeyPath := newNodeKeyFile(t, "existing-key")
			require.NoError(t, os.WriteFile(nodeKeyPath+".unauthenticated", []byte(contents), 0o600))
			oc := &OrbitClient{nodeKeyFilePath: nodeKeyPath}

			oc.reenrollMu.Lock()
			oc.loadUnauthenticatedStreakLocked(now)
			oc.reenrollMu.Unlock()

			require.True(t, oc.unauthenticatedSince.IsZero())
			require.False(t, oc.forceReenroll)
		})
	}
}
