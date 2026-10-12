package httpsig

import (
	"context"
	"crypto/elliptic"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/fleetdm/fleet/v4/ee/pkg/hostidentity/types"
	"github.com/fleetdm/fleet/v4/pkg/fleethttpsig"
	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/remitly-oss/httpsig-go"
)

type HTTPSig struct {
	ds     fleet.Datastore
	logger *slog.Logger
}

type KeySpecer struct {
	hostIdentityCert types.HostIdentityCertificate
	keySpec          httpsig.KeySpec
}

func (k KeySpecer) KeySpec() (httpsig.KeySpec, error) {
	return k.keySpec, nil
}

// _ ensures that KeySpecer implements the httpsig.KeySpecer interface.
var _ httpsig.KeySpecer = KeySpecer{}

func NewHTTPSig(ds fleet.Datastore, logger *slog.Logger) *HTTPSig {
	return &HTTPSig{
		ds:     ds,
		logger: logger,
	}
}

// _ ensures that HTTPSig implements the httpsig.KeyFetcher interface.
var _ httpsig.KeyFetcher = (*HTTPSig)(nil)

func (h *HTTPSig) Verifier() (*httpsig.Verifier, error) {
	return fleethttpsig.Verifier(h)
}

func (h *HTTPSig) FetchByKeyID(ctx context.Context, _ http.Header, keyID string) (httpsig.KeySpecer, error) {
	keyIDInt, err := strconv.ParseUint(keyID, 16, 64)
	if err != nil {
		err = ctxerr.Wrap(ctx, err, "invalid hex key ID")
		h.logger.InfoContext(ctx, "FetchByKeyID error", "err", err)
		return nil, err
	}
	identityCert, err := h.ds.GetHostIdentityCertBySerialNumber(ctx, keyIDInt)
	switch {
	case fleet.IsNotFound(err):
		return nil, ctxerr.Errorf(ctx, "certificate not found with keyID: %d", keyIDInt)
	case err != nil:
		err = ctxerr.Wrap(ctx, err, "loading certificate")
		h.logger.ErrorContext(ctx, "FetchByKeyID error", "err", err)
		return nil, err
	}
	publicKey, err := identityCert.UnmarshalPublicKey()
	if err != nil {
		err = ctxerr.Wrap(ctx, err, "unmarshaling public key")
		h.logger.ErrorContext(ctx, "FetchByKeyID error", "err", err)
		return nil, err
	}

	var algo httpsig.Algorithm
	switch publicKey.Curve {
	case elliptic.P256():
		algo = httpsig.Algo_ECDSA_P256_SHA256
	case elliptic.P384():
		algo = httpsig.Algo_ECDSA_P384_SHA384
	default:
		err = ctxerr.Errorf(ctx, "unsupported elliptic curve: %s", publicKey.Curve.Params().Name)
		h.logger.InfoContext(ctx, "FetchByKeyID error", "err", err)
		return nil, err
	}

	return &KeySpecer{
		hostIdentityCert: *identityCert,
		keySpec: httpsig.KeySpec{
			KeyID:  keyID,
			Algo:   algo,
			PubKey: publicKey,
		},
	}, nil
}

func (h *HTTPSig) Fetch(ctx context.Context, _ http.Header, _ httpsig.MetadataProvider) (httpsig.KeySpecer, error) {
	return nil, ctxerr.New(ctx, "not implemented")
}

// VerifyHostIdentity checks that host identity certificate matches the node key and host ID.
// Host identity cert is used for TPM-backed HTTP message signatures.
// If the host has one, then all agent traffic should have HTTP message signatures unless specified otherwise.
// The host identity certificate must match the host's node key.
func VerifyHostIdentity(ctx context.Context, ds fleet.Datastore, host *fleet.Host) error {
	hostIdentityCert, ok := FromContext(ctx)
	if !ok {
		return ctxerr.New(ctx, "authentication error: missing host identity certificate")
	}
	if host.OsqueryHostID == nil || *host.OsqueryHostID != hostIdentityCert.CommonName {
		return ctxerr.New(ctx, "authentication error: http message signature does not match node key")
	}
	if hostIdentityCert.HostID == nil {
		return ctxerr.Errorf(ctx, "authentication error: found host identity certificate without host ID. "+
			"This should not happen since host ID for a certificate should be set at enrollment. identifier/CN: %s host ID: %d",
			hostIdentityCert.CommonName, host.ID)
	}
	if *hostIdentityCert.HostID != host.ID {
		return ctxerr.New(ctx, "authentication error: http message signature does not match host ID")
	}
	return nil
}
