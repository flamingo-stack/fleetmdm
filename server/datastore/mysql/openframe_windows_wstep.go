// >>> OPENFRAME(windows-wstep): one Windows MDM WSTEP pair per tenant, generated with its first enrollment and kept in mdm_config_assets — openframe/docs/mysql-multitenancy-feature.md

package mysql

import (
	"context"
	"errors"
	"fmt"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/jmoiron/sqlx"
)

func openframeWindowsWSTEPAssetNames(teamID uint) (cert, key fleet.MDMAssetName) {
	return fleet.MDMAssetName(fmt.Sprintf("openframe_windows_wstep_cert/%d", teamID)),
		fleet.MDMAssetName(fmt.Sprintf("openframe_windows_wstep_key/%d", teamID))
}

const openframeWindowsMDMTeamDataExistsQuery = `
SELECT
  EXISTS (
    SELECT 1 FROM mdm_windows_enrollments e
    JOIN hosts h ON h.uuid = e.host_uuid
    WHERE h.team_id = ?
  ) OR
  EXISTS (
    SELECT 1 FROM host_disk_encryption_keys k
    JOIN hosts h ON h.id = k.host_id
    WHERE h.platform = 'windows' AND h.team_id = ?
  )`

// OpenframeLoadWindowsWSTEP returns the tenant's WSTEP pair, generating and storing it when the
// tenant has none yet. Concurrent first calls agree on one pair: the losing insert is a duplicate
// and every caller reads the stored pair back.
func (ds *Datastore) OpenframeLoadWindowsWSTEP(
	ctx context.Context,
	teamID uint,
	newPair func() (certPEM, keyPEM []byte, err error),
) (certPEM, keyPEM []byte, err error) {
	if teamID == 0 {
		return nil, nil, ctxerr.New(ctx, "a Windows MDM WSTEP pair belongs to a tenant team")
	}
	if ds.serverPrivateKey == "" {
		return nil, nil, ctxerr.New(ctx, "the server private key is required to store the Windows MDM WSTEP pair")
	}

	certPEM, keyPEM, err = ds.OpenframeGetWindowsWSTEP(ctx, teamID)
	if err == nil || !fleet.IsNotFound(err) {
		return certPEM, keyPEM, err
	}

	var exists bool
	if err := sqlx.GetContext(ctx, ds.writer(ctx), &exists, openframeWindowsMDMTeamDataExistsQuery, teamID, teamID); err != nil {
		return nil, nil, ctxerr.Wrap(ctx, err, "checking the tenant's Windows MDM data")
	}
	if exists {
		return nil, nil, ctxerr.Errorf(ctx, "team %d has no Windows MDM WSTEP pair stored, but its Windows devices are already enrolled or "+
			"have escrowed BitLocker keys; a new pair would break them", teamID)
	}

	certPEM, keyPEM, err = newPair()
	if err != nil {
		return nil, nil, ctxerr.Wrap(ctx, err, "generating Windows MDM WSTEP pair")
	}
	certName, keyName := openframeWindowsWSTEPAssetNames(teamID)
	err = ds.InsertMDMConfigAssets(ctx, []fleet.MDMConfigAsset{
		{Name: certName, Value: certPEM},
		{Name: keyName, Value: keyPEM},
	}, nil)
	if err != nil && !IsDuplicate(err) {
		return nil, nil, ctxerr.Wrap(ctx, err, "storing Windows MDM WSTEP pair")
	}

	certPEM, keyPEM, err = ds.OpenframeGetWindowsWSTEP(ctx, teamID)
	if fleet.IsNotFound(err) {
		return nil, nil, ctxerr.New(ctx, "Windows MDM WSTEP pair missing after storing it")
	}
	return certPEM, keyPEM, err
}

// OpenframeGetWindowsWSTEP returns the tenant's stored WSTEP pair, or a NotFound error.
func (ds *Datastore) OpenframeGetWindowsWSTEP(ctx context.Context, teamID uint) (certPEM, keyPEM []byte, err error) {
	certName, keyName := openframeWindowsWSTEPAssetNames(teamID)
	assets, err := ds.GetAllMDMConfigAssetsByName(ctx, []fleet.MDMAssetName{certName, keyName}, ds.writer(ctx))
	switch {
	case fleet.IsNotFound(err):
		return nil, nil, ctxerr.Wrap(ctx, notFound("WindowsWSTEPPair").WithID(teamID))
	case errors.Is(err, ErrPartialResult):
		return nil, nil, ctxerr.Errorf(ctx, "only one half of team %d's Windows MDM WSTEP pair is stored", teamID)
	case err != nil:
		return nil, nil, ctxerr.Wrap(ctx, err, "reading Windows MDM WSTEP pair")
	}
	return assets[certName].Value, assets[keyName].Value, nil
}

// <<< OPENFRAME(windows-wstep)
