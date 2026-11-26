// >>> OPENFRAME(mysql-multitenancy): the Windows MDM discovery URL sent to orbit comes from the instance row — openframe/docs/mysql-multitenancy-feature.md

package service

import (
	"context"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
)

func openframeWindowsMDMServerURL(ctx context.Context, ds fleet.Datastore, appConfig *fleet.AppConfig, shared bool) (string, error) {
	if !shared {
		return appConfig.ServerSettings.ServerURL, nil
	}
	instance, err := ds.AppConfig(fleet.NewOpenframeInstanceConfigContext(ctx))
	if err != nil {
		return "", ctxerr.Wrap(ctx, err, "reading the instance config for the Windows MDM discovery URL")
	}
	return instance.ServerSettings.ServerURL, nil
}

// <<< OPENFRAME(mysql-multitenancy)
