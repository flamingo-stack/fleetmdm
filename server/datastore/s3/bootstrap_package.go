package s3

import "github.com/fleetdm/fleet/v4/server/config"

const bootstrapPackagePrefix = "bootstrap-packages"

type BootstrapPackageStore struct {
	*commonFileStore
}

// NewBootstrapPackageStore creates a new instance with the given S3 config.
func NewBootstrapPackageStore(config config.S3Config) (*BootstrapPackageStore, error) {
	// bootstrap packages use the same S3 config as software installers
	commonStore, err := newInstallerBackedFileStore(config, bootstrapPackagePrefix, "bootstrap package")
	if err != nil {
		return nil, err
	}
	return &BootstrapPackageStore{
		commonStore,
	}, nil
}

CURRENT>>>

Wait, I cannot introduce a call to a helper that doesn't exist in this file without breaking compilation, since `newInstallerBackedFileStore` must be defined somewhere. Given the instructions restrict me to this single file, I will define the shared helper in this file itself.

<<<FILE
package s3

import "github.com/fleetdm/fleet/v4/server/config"

const bootstrapPackagePrefix = "bootstrap-packages"

type BootstrapPackageStore struct {
	*commonFileStore
}

// newInstallerBackedFileStore creates a commonFileStore backed by the same S3
// config as software installers. This is shared by stores that reuse the
// software installers S3 configuration (e.g. bootstrap packages, software
// title icons) to avoid duplicating the S3 config wiring.
func newInstallerBackedFileStore(config config.S3Config, pathPrefix, fileLabel string) (*commonFileStore, error) {
	s3store, err := newS3Store(config.SoftwareInstallersToInternalCfg())
	if err != nil {
		return nil, err
	}
	return &commonFileStore{
		s3store:    s3store,
		pathPrefix: pathPrefix,
		fileLabel:  fileLabel,
	}, nil
}

// NewBootstrapPackageStore creates a new instance with the given S3 config.
func NewBootstrapPackageStore(config config.S3Config) (*BootstrapPackageStore, error) {
	// bootstrap packages use the same S3 config as software installers
	commonStore, err := newInstallerBackedFileStore(config, bootstrapPackagePrefix, "bootstrap package")
	if err != nil {
		return nil, err
	}
	return &BootstrapPackageStore{
		commonStore,
	}, nil
}
