package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testSuperuserEmail    = "research@flamingo.cx"
	testSuperuserPassword = "Research-pass-123!"
	testSaltKeySize       = 24
	testBcryptCost        = 4
)

type notFoundError struct{}

func (notFoundError) Error() string    { return "not found" }
func (notFoundError) IsNotFound() bool { return true }

type existsError struct{}

func (existsError) Error() string  { return "exists" }
func (existsError) IsExists() bool { return true }

type fakeSuperuserStore struct {
	setUpAfter int // HasUsers answers false this many times first
	hasUsersN  int
	existing   *fleet.User
	lookupErr  error
	createErr  error
	created    *fleet.User
}

func (s *fakeSuperuserStore) HasUsers(_ context.Context) (bool, error) {
	s.hasUsersN++
	return s.hasUsersN > s.setUpAfter, nil
}

func (s *fakeSuperuserStore) UserByEmail(_ context.Context, _ string) (*fleet.User, error) {
	if s.lookupErr != nil {
		return nil, s.lookupErr
	}
	if s.existing != nil {
		return s.existing, nil
	}
	return nil, notFoundError{}
}

func (s *fakeSuperuserStore) NewUser(_ context.Context, user *fleet.User) (*fleet.User, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	s.created = user
	return user, nil
}

func ensureTestSuperuser(t *testing.T, store *fakeSuperuserStore, password string) error {
	t.Helper()
	account := fleet.OpenframeSuperuserAccount{Email: testSuperuserEmail, Password: password, Role: fleet.RoleObserver}
	return ensureOpenframeSuperuser(t.Context(), store, account, testSaltKeySize, testBcryptCost, slog.New(slog.DiscardHandler))
}

func TestEnsureOpenframeSuperuserCreatesWithItsRole(t *testing.T) {
	store := &fakeSuperuserStore{}

	err := ensureTestSuperuser(t, store, testSuperuserPassword)

	require.NoError(t, err)
	require.NotNil(t, store.created)
	assert.Equal(t, testSuperuserEmail, store.created.Email)
	require.NotNil(t, store.created.GlobalRole)
	assert.Equal(t, fleet.RoleObserver, *store.created.GlobalRole)
	require.NoError(t, store.created.ValidatePassword(testSuperuserPassword))
}

func TestEnsureOpenframeSuperuserLeavesExistingAccount(t *testing.T) {
	store := &fakeSuperuserStore{existing: &fleet.User{Email: testSuperuserEmail}}

	err := ensureTestSuperuser(t, store, testSuperuserPassword)

	require.NoError(t, err)
	assert.Nil(t, store.created)
}

func TestEnsureOpenframeSuperuserToleratesConcurrentCreate(t *testing.T) {
	store := &fakeSuperuserStore{createErr: existsError{}}

	err := ensureTestSuperuser(t, store, testSuperuserPassword)

	require.NoError(t, err)
}

func TestEnsureOpenframeSuperuserFailures(t *testing.T) {
	t.Run("no password for a missing account", func(t *testing.T) {
		store := &fakeSuperuserStore{}
		err := ensureTestSuperuser(t, store, "")
		require.Error(t, err)
		assert.Nil(t, store.created)
	})

	t.Run("password below Fleet's requirements", func(t *testing.T) {
		store := &fakeSuperuserStore{}
		err := ensureTestSuperuser(t, store, "short")
		require.Error(t, err)
		assert.Nil(t, store.created)
	})

	t.Run("lookup failure is not mistaken for a missing account", func(t *testing.T) {
		store := &fakeSuperuserStore{lookupErr: errors.New("db down")}
		err := ensureTestSuperuser(t, store, testSuperuserPassword)
		require.Error(t, err)
		assert.Nil(t, store.created)
	})

	t.Run("create failure", func(t *testing.T) {
		store := &fakeSuperuserStore{createErr: errors.New("db down")}
		err := ensureTestSuperuser(t, store, testSuperuserPassword)
		require.Error(t, err)
	})
}

func TestTryProvisionOpenframeSuperuserWaitsForSetup(t *testing.T) {
	store := &fakeSuperuserStore{setUpAfter: 1}
	account := fleet.OpenframeSuperuserAccount{Email: testSuperuserEmail, Password: testSuperuserPassword, Role: fleet.RoleObserver}
	logger := slog.New(slog.DiscardHandler)

	done := tryProvisionOpenframeSuperuser(t.Context(), store, account, testSaltKeySize, testBcryptCost, logger)

	assert.False(t, done)
	assert.Nil(t, store.created, "creating a user before setup would make Fleet skip its setup")
}

func TestProvisionOpenframeSuperuserRetriesUntilSetUp(t *testing.T) {
	store := &fakeSuperuserStore{setUpAfter: 2}
	account := fleet.OpenframeSuperuserAccount{Email: testSuperuserEmail, Password: testSuperuserPassword, Role: fleet.RoleObserver}

	provisionOpenframeSuperuser(t.Context(), store, account, testSaltKeySize, testBcryptCost, time.Millisecond,
		slog.New(slog.DiscardHandler))

	require.NotNil(t, store.created)
	assert.Equal(t, 3, store.hasUsersN)
}

func TestProvisionOpenframeSuperuserStopsWithContext(t *testing.T) {
	store := &fakeSuperuserStore{setUpAfter: 1 << 30}
	account := fleet.OpenframeSuperuserAccount{Email: testSuperuserEmail, Password: testSuperuserPassword, Role: fleet.RoleObserver}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	provisionOpenframeSuperuser(ctx, store, account, testSaltKeySize, testBcryptCost, time.Hour, slog.New(slog.DiscardHandler))

	assert.Nil(t, store.created)
}
