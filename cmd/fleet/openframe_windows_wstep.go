// >>> OPENFRAME(windows-wstep): each tenant's Windows MDM WSTEP pair is generated with its first enrollment and kept in mdm_config_assets — openframe/docs/mysql-multitenancy-feature.md

package main

import (
	"errors"

	"github.com/fleetdm/fleet/v4/server/config"
)

// openframeCheckWindowsWSTEP refuses a shared-mode boot with Windows MDM on that would not keep
// the tenants' WSTEP pairs: no server private key to encrypt them with, or an instance pair
// configured through the usual settings that the tenants' devices would never use.
func openframeCheckWindowsWSTEP(cfg *config.FleetConfig) error {
	if cfg.MDM.IsMicrosoftWSTEPSet() {
		return errors.New("FLEET_OPENFRAME_INSTANCE_WINDOWS_MDM_ENABLED gives every tenant its own WSTEP pair in the database; unset the configured WSTEP certificate and key")
	}
	if cfg.Server.PrivateKey == "" {
		return errors.New("FLEET_OPENFRAME_INSTANCE_WINDOWS_MDM_ENABLED needs the server private key: the tenants' WSTEP pairs are stored encrypted with it")
	}
	return nil
}

// <<< OPENFRAME(windows-wstep)
