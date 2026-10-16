package licensing

import (
	"crypto/ecdsa"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/net/context"
)

const (
	expectedAlgorithm = "ES256"
	expectedIssuer    = "Fleet Device Management Inc."
)

//go:embed pubkey.pem
var pubKeyPEM []byte

// loadPublicKey loads the public key from pubkey.pem.
func loadPublicKey(ctx context.Context) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(pubKeyPEM)
	if block == nil {
		return nil, ctxerr.New(ctx, "no key block found in pem")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "failed to parse ecdsa key")
	}

	if pub, ok := pub.(*ecdsa.PublicKey); ok {
		return pub, nil
	}
	return nil, ctxerr.Errorf(ctx, "%T is not *ecdsa.PublicKey", pub)
}

// LoadLicense loads and validates the license key.
func LoadLicense(licenseKey string) (*fleet.LicenseInfo, error) {
	ctx := context.Background()

	// No license key
	if licenseKey == "" {
		return &fleet.LicenseInfo{Tier: fleet.TierFree}, nil
	}

	parsedToken, err := jwt.ParseWithClaims(
		licenseKey,
		&licenseClaims{},
		// Always use the same public key
		func(*jwt.Token) (interface{}, error) {
			return loadPublicKey(ctx)
		},
	)
	if err != nil {
		v, _ := err.(*jwt.ValidationError)

		// if the ONLY error is that it's expired, then we ignore it
		if v == nil || v.Errors != jwt.ValidationErrorExpired {
			return nil, ctxerr.Wrap(ctx, err, "parse license")
		}
		parsedToken.Valid = true
	}

	license, err := validate(ctx, parsedToken)
	if err != nil {
		return nil, ctxerr.Wrap(ctx, err, "validate license")
	}

	// for backwards compatibility we'll convert basic tier to premium
	license.ForceUpgrade()

	return license, nil
}

type licenseClaims struct {
	// jwt.StandardClaims includes validation for iat, nbf, and exp.
	jwt.StandardClaims
	Tier                  string `json:"tier"`
	Devices               int    `json:"devices"`
	Note                  string `json:"note"`
	AllowDisableTelemetry bool   `json:"notel"`
}

func validate(ctx context.Context, token *jwt.Token) (*fleet.LicenseInfo, error) {
	// token.IssuedAt, token.ExpiresAt, token.NotBefore already validated by JWT
	// library.
	if !token.Valid {
		// ParseWithClaims should have errored already, but double-check here
		return nil, ctxerr.New(ctx, "token invalid")
	}

	if token.Method.Alg() != expectedAlgorithm {
		return nil, ctxerr.Errorf(ctx, "unexpected algorithm %s", token.Method.Alg())
	}

	var claims *licenseClaims
	claims, ok := token.Claims.(*licenseClaims)
	if !ok || claims == nil {
		return nil, ctxerr.Errorf(ctx, "unexpected claims type %T", token.Claims)
	}

	if claims.Devices == 0 {
		return nil, ctxerr.New(ctx, "missing devices")
	}

	if claims.Tier == "" {
		return nil, ctxerr.New(ctx, "missing tier")
	}

	if claims.ExpiresAt == 0 {
		return nil, ctxerr.New(ctx, "missing exp")
	}

	if claims.Issuer != expectedIssuer {
		return nil, ctxerr.Errorf(ctx, "unexpected issuer %s", claims.Issuer)
	}

	return &fleet.LicenseInfo{
		Tier:                  claims.Tier,
		Organization:          claims.Subject,
		DeviceCount:           claims.Devices,
		Expiration:            time.Unix(claims.ExpiresAt, 0),
		Note:                  claims.Note,
		AllowDisableTelemetry: claims.AllowDisableTelemetry,
	}, nil

}
