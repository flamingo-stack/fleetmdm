package fleet

import "context"

// >>> OPENFRAME(acme-write-service): fork-specific ACME write-service interface
// ACMEWriteService is the subset of the ACME service module service
// used by the legacy service layer for write operations.
type ACMEWriteService interface {
	// NewACMEEnrollment creates a new enrollment in the acme_enrollments table with the specified
	// host_uuid and returns a new path_identifier for the created row.
	NewACMEEnrollment(ctx context.Context, hostIdentifier string) (string, error)
}
// <<< OPENFRAME(acme-write-service)

