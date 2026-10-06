// Package s3test holds test helpers for spinning up scratch S3 stores
// (software installers, bootstrap packages, software title icons) backed by a
// local MinIO/S3 endpoint. It imports the "testing" package and is therefore
// only ever imported from test code; importing it from production code would
// pull "testing" into the resulting binary.
package s3test

import (
	"context"
	"os"
	"testing"

	"github.com/fleetdm/fleet/v4/server/config"
	"github.com/fleetdm/fleet/v4/server/datastore/s3"
	"github.com/stretchr/testify/require"
)

const (
	accessKeyID     = "locals3"
	secretAccessKey = "locals3"
)

var testEndpoint = getTestEndpoint()

func getTestEndpoint() string {
	if port := os.Getenv("FLEET_S3_PORT"); port != "" {
		return "http://localhost:" + port
	}
	return "http://localhost:9000"
}

// SetupSoftwareInstallerStore returns a *s3.SoftwareInstallerStore backed by
// the local test bucket and registers cleanup to drop the bucket when the
// test finishes.
func SetupSoftwareInstallerStore(tb testing.TB, bucket, prefix string) *s3.SoftwareInstallerStore {
	return setupStore(tb, bucket, prefix, s3.NewSoftwareInstallerStore)
}

// SetupBootstrapPackageStore returns a *s3.BootstrapPackageStore backed by
// the local test bucket and registers cleanup to drop the bucket when the
// test finishes.
func SetupBootstrapPackageStore(tb testing.TB, bucket, prefix string) *s3.BootstrapPackageStore {
	return setupStore(tb, bucket, prefix, s3.NewBootstrapPackageStore)
}

// SetupSoftwareTitleIconStore returns a *s3.SoftwareTitleIconStore backed by
// the local test bucket and registers cleanup to drop the bucket when the
// test finishes.
//
// NOTE: s3.NewSoftwareTitleIconStore (and s3.NewOrgLogoStore) internally
// reuse the SoftwareInstallers* config fields rather than having dedicated
// SoftwareTitleIcon/OrgLogo fields, so setupStore only needs to populate the
// SoftwareInstallers* fields below for this helper (and for a future
// OrgLogo test helper) to work.
func SetupSoftwareTitleIconStore(tb testing.TB, bucket, prefix string) *s3.SoftwareTitleIconStore {
	return setupStore(tb, bucket, prefix, s3.NewSoftwareTitleIconStore)
}

// testStore is the small surface s3test needs from a store, satisfied via
// method promotion by SoftwareInstallerStore, BootstrapPackageStore, and
// SoftwareTitleIconStore (each embeds *commonFileStore, which embeds
// *s3store).
type testStore interface {
	CreateTestBucket(ctx context.Context, name string) error
	CleanupTestBucket(ctx context.Context) error
}

func setupStore[T testStore](tb testing.TB, bucket, prefix string, newFn func(config.S3Config) (T, error)) T {
	checkEnv(tb)

	store, err := newFn(config.S3Config{
		// NOTE: these SoftwareInstallers* fields are also relied upon by
		// s3.NewSoftwareTitleIconStore and s3.NewOrgLogoStore, which reuse
		// them instead of having dedicated SoftwareTitleIcon/OrgLogo config
		// fields. Do not remove or rename without checking those stores.
		SoftwareInstallersBucket:           bucket,
		SoftwareInstallersPrefix:           prefix,
		SoftwareInstallersRegion:           "localhost",
		SoftwareInstallersEndpointURL:      testEndpoint,
		SoftwareInstallersAccessKeyID:      accessKeyID,
		SoftwareInstallersSecretAccessKey:  secretAccessKey,
		SoftwareInstallersForceS3PathStyle: true,
		SoftwareInstallersDisableSSL:       true,

		CarvesBucket:           bucket,
		CarvesPrefix:           prefix,
		CarvesRegion:           "localhost",
		CarvesEndpointURL:      testEndpoint,
		CarvesAccessKeyID:      accessKeyID,
		CarvesSecretAccessKey:  secretAccessKey,
		CarvesForceS3PathStyle: true,
		CarvesDisableSSL:       true,
	})
	require.NoError(tb, err)

	err = store.CreateTestBucket(context.Background(), bucket)
	require.NoError(tb, err)

	tb.Cleanup(func() {
		if err := store.CleanupTestBucket(context.Background()); err != nil {
			tb.Errorf("cleanup s3 bucket %q: %v", bucket, err)
		}
	})

	return store
}

func checkEnv(tb testing.TB) {
	if _, ok := os.LookupEnv("S3_STORAGE_TEST"); !ok {
		tb.Skip("set S3_STORAGE_TEST environment variable to run S3-based tests")
	}
}
