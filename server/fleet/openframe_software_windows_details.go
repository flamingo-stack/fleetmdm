// OPENFRAME(software-windows-details): openframe/docs/software-windows-details.md
package fleet

// SoftwareWindowsDetails is what the OpenFrame osquery programs table reports about a Windows program beyond the
// columns Fleet keeps. A field is nil when the host's osquery does not report it.
type SoftwareWindowsDetails struct {
	// ArpKey is the name of the program's Uninstall registry subkey.
	ArpKey *string `db:"arp_key" json:"arp_key,omitempty"`
	// PackageFamilyName identifies an MSIX package across versions and architectures.
	PackageFamilyName *string `db:"package_family_name" json:"package_family_name,omitempty"`
	// SystemComponent is set when the Uninstall entry is hidden from Programs and Features.
	SystemComponent *bool `db:"system_component" json:"system_component,omitempty"`
	// InstallerType is msi, exe, portable or msix.
	InstallerType *string `db:"installer_type" json:"installer_type,omitempty"`
	// InstallScope is machine or user.
	InstallScope *string `db:"install_scope" json:"install_scope,omitempty"`
	// InstallArch is 64 or 32 for a machine scope entry.
	InstallArch *string `db:"install_arch" json:"install_arch,omitempty"`
}
