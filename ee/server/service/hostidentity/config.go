package hostidentity

import (
	"context"

	"github.com/fleetdm/fleet/v4/pkg/certificate"
	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mdm/scep/depot"
)

func initAssets(ctx context.Context, ds fleet.Datastore) error {
	// Check if we have existing certs and keys
	expectedAssets := []fleet.MDMAssetName{
		fleet.MDMAssetHostIdentityCACert,
		fleet.MDMAssetHostIdentityCAKey,
	}
	savedAssets, err := ds.GetAllMDMConfigAssetsByName(ctx, expectedAssets, nil)
	if err != nil {
		// allow not found errors as it means we're generating the assets for the first time.
		if !fleet.IsNotFound(err) {
			return ctxerr.Wrap(ctx, err, "loading existing host identity assets from the database")
		}
	}

	if len(savedAssets) != len(expectedAssets) {
		// Then we should create them
		caCert := depot.NewCACert(
			depot.WithYears(10),
			depot.WithCommonName("Fleet Host Identity CA"),
			// Signal that the CA is local to the deployment and not necessarily managed by Fleet or another external vendor
			depot.WithOrganization("Local Certificate Authority"),
			depot.WithCountry("US"),
		)
		scepCert, scepKey, err := depot.NewCACertKey(caCert)
		if err != nil {
			return ctxerr.Wrap(ctx, err, "generating host identity SCEP cert and key")
		}

		// Store our config assets encrypted
		var assets []fleet.MDMConfigAsset
		for k, v := range map[fleet.MDMAssetName][]byte{
			fleet.MDMAssetHostIdentityCACert: certificate.EncodeCertPEM(scepCert),
			fleet.MDMAssetHostIdentityCAKey:  certificate.EncodePrivateKeyPEM(scepKey),
		} {
			assets = append(assets, fleet.MDMConfigAsset{
				Name:  k,
				Value: v,
			})
		}

		if err := ds.InsertMDMConfigAssets(ctx, assets, nil); err != nil {
			return ctxerr.Wrap(ctx, err, "inserting host identity SCEP assets")
		}
	}
	return nil
}
