// >>> OPENFRAME(windows-wstep): Windows MDM WSTEP pair generated once and kept in mdm_config_assets — openframe/docs/mysql-multitenancy-feature.md

package mysql

import (
	"context"
	"errors"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/jmoiron/sqlx"
)

const (
	OpenframeMDMAssetWindowsWSTEPCert fleet.MDMAssetName = "openframe_windows_wstep_cert"
	OpenframeMDMAssetWindowsWSTEPKey  fleet.MDMAssetName = "openframe_windows_wstep_key"
)

const openframeWindowsMDMDataExistsQuery = `
SELECT
  EXISTS (SELECT 1 FROM mdm_windows_enrollments) OR
  EXISTS (
    SELECT 1 FROM host_disk_encryption_keys k
    JOIN hosts h ON h.id = k.host_id
    WHERE h.platform = 'windows'
  )`

func (ds *Datastore) OpenframeLoadWindowsWSTEP(
	ctx context.Context,
	newPair func() (certPEM, keyPEM []byte, err error),
) (certPEM, keyPEM []byte, err error) {
	if ds.serverPrivateKey == "" {
		return nil, nil, ctxerr.New(ctx, "the server private key is required to store the Windows MDM WSTEP pair")
	}

	certPEM, keyPEM, found, err := ds.openframeReadWindowsWSTEP(ctx)
	if err != nil || found {
		return certPEM, keyPEM, err
	}

	var exists bool
	if err := sqlx.GetContext(ctx, ds.writer(ctx), &exists, openframeWindowsMDMDataExistsQuery); err != nil {
		return nil, nil, ctxerr.Wrap(ctx, err, "checking for Windows MDM data")
	}
	if exists {
		return nil, nil, ctxerr.New(ctx, "no Windows MDM WSTEP pair is stored, but Windows devices are already enrolled or have escrowed BitLocker keys; "+
			"a new pair would break them")
	}

	certPEM, keyPEM, err = newPair()
	if err != nil {
		return nil, nil, ctxerr.Wrap(ctx, err, "generating Windows MDM WSTEP pair")
	}
	err = ds.InsertMDMConfigAssets(ctx, []fleet.MDMConfigAsset{
		{Name: OpenframeMDMAssetWindowsWSTEPCert, Value: certPEM},
		{Name: OpenframeMDMAssetWindowsWSTEPKey, Value: keyPEM},
	}, nil)
	if err != nil && !IsDuplicate(err) {
		return nil, nil, ctxerr.Wrap(ctx, err, "storing Windows MDM WSTEP pair")
	}

	certPEM, keyPEM, found, err = ds.openframeReadWindowsWSTEP(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, ctxerr.New(ctx, "Windows MDM WSTEP pair missing after storing it")
	}
	return certPEM, keyPEM, nil
}

func (ds *Datastore) openframeReadWindowsWSTEP(ctx context.Context) (certPEM, keyPEM []byte, found bool, err error) {
	assets, err := ds.GetAllMDMConfigAssetsByName(ctx, []fleet.MDMAssetName{
		OpenframeMDMAssetWindowsWSTEPCert,
		OpenframeMDMAssetWindowsWSTEPKey,
	}, ds.writer(ctx))
	switch {
	case fleet.IsNotFound(err):
		return nil, nil, false, nil
	case errors.Is(err, ErrPartialResult):
		return nil, nil, false, ctxerr.New(ctx, "only one half of the Windows MDM WSTEP pair is stored")
	case err != nil:
		return nil, nil, false, ctxerr.Wrap(ctx, err, "reading Windows MDM WSTEP pair")
	}
	return assets[OpenframeMDMAssetWindowsWSTEPCert].Value, assets[OpenframeMDMAssetWindowsWSTEPKey].Value, true, nil
}

// <<< OPENFRAME(windows-wstep)
