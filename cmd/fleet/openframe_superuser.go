// >>> OPENFRAME(mysql-multitenancy): shared-mode research superuser provisioned at startup — openframe/docs/mysql-multitenancy-feature.md#superuser-shared-mode-only

package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
)

const (
	openframeSuperuserName          = "OpenFrame research"
	openframeSuperuserRetryInterval = time.Minute
)

// openframeSuperuserStore is the slice of the datastore the provisioning needs.
type openframeSuperuserStore interface {
	HasUsers(ctx context.Context) (bool, error)
	UserByEmail(ctx context.Context, email string) (*fleet.User, error)
	NewUser(ctx context.Context, user *fleet.User) (*fleet.User, error)
}

// provisionOpenframeSuperuser keeps trying until the account exists, then returns. Run in its own
// goroutine: Fleet must not wait for it, and on a fresh database it has to wait for Fleet setup.
func provisionOpenframeSuperuser(ctx context.Context, ds openframeSuperuserStore, account fleet.OpenframeSuperuserAccount,
	saltKeySize, bcryptCost int, retryInterval time.Duration, logger *slog.Logger,
) {
	for !tryProvisionOpenframeSuperuser(ctx, ds, account, saltKeySize, bcryptCost, logger) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryInterval):
		}
	}
}

// tryProvisionOpenframeSuperuser reports whether the account exists after this attempt. Fleet
// treats a database with any user as set up (SetupRequired is !HasUsers), so creating the account
// before the first admin would make the setup that creates admin@openframe.local fail. It waits.
func tryProvisionOpenframeSuperuser(ctx context.Context, ds openframeSuperuserStore, account fleet.OpenframeSuperuserAccount,
	saltKeySize, bcryptCost int, logger *slog.Logger,
) bool {
	setUp, err := ds.HasUsers(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "openframe superuser: checking Fleet setup", "err", err)
		return false
	}
	if !setUp {
		logger.InfoContext(ctx, "openframe superuser: waiting for Fleet setup", "email", account.Email)
		return false
	}
	err = ensureOpenframeSuperuser(ctx, ds, account, saltKeySize, bcryptCost, logger)
	if err != nil {
		logger.ErrorContext(ctx, "openframe superuser: provisioning failed, will retry", "email", account.Email, "err", err)
		return false
	}
	return true
}

// ensureOpenframeSuperuser creates the research account with its global role if it does not exist
// yet. An existing account is left as it is, password and role included, so a change made in Fleet
// is never reset by a restart.
func ensureOpenframeSuperuser(ctx context.Context, ds openframeSuperuserStore, account fleet.OpenframeSuperuserAccount,
	saltKeySize, bcryptCost int, logger *slog.Logger,
) error {
	_, err := ds.UserByEmail(ctx, account.Email)
	if err == nil {
		logger.InfoContext(ctx, "openframe superuser already exists", "email", account.Email)
		return nil
	}
	if !fleet.IsNotFound(err) {
		return ctxerr.Wrap(ctx, err, "look up openframe superuser")
	}
	if account.Password == "" {
		return errors.New("FLEET_OPENFRAME_SUPERUSER_PASSWORD is required to create the superuser")
	}

	user, err := buildOpenframeSuperuser(account, saltKeySize, bcryptCost)
	if err != nil {
		return ctxerr.Wrap(ctx, err, "build openframe superuser")
	}
	_, err = ds.NewUser(ctx, user)
	if isAlreadyExists(err) {
		// Another replica created it between the lookup and the insert.
		return nil
	}
	if err != nil {
		return ctxerr.Wrap(ctx, err, "create openframe superuser")
	}
	logger.InfoContext(ctx, "openframe superuser created", "email", account.Email, "role", account.Role)
	return nil
}

func buildOpenframeSuperuser(account fleet.OpenframeSuperuserAccount, saltKeySize, bcryptCost int) (*fleet.User, error) {
	user := &fleet.User{
		Name:       openframeSuperuserName,
		Email:      account.Email,
		GlobalRole: new(account.Role),
	}
	if err := user.SetPassword(account.Password, saltKeySize, bcryptCost); err != nil {
		return nil, err
	}
	return user, nil
}

func isAlreadyExists(err error) bool {
	var exists fleet.AlreadyExistsError
	return errors.As(err, &exists) && exists.IsExists()
}

// <<< OPENFRAME(mysql-multitenancy)
