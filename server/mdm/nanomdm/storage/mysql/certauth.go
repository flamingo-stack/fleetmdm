package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fleetdm/fleet/v4/server/mdm/nanomdm/mdm"
)

// Executes SQL statements that return a single COUNT(*) of rows.
func (s *MySQLStorage) queryRowContextRowExists(ctx context.Context, query string, args ...interface{}) (bool, error) {
	var ct int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&ct)
	if err != nil {
		return false, fmt.Errorf("querying row exists: %w", err)
	}
	return ct > 0, nil
}

func (s *MySQLStorage) EnrollmentHasCertHash(r *mdm.Request, _ string) (bool, error) {
	return s.queryRowContextRowExists(
		r.Context,
		`SELECT COUNT(*) FROM nano_cert_auth_associations WHERE id = ?;`,
		r.ID,
	)
}

func (s *MySQLStorage) HasCertHash(r *mdm.Request, hash string) (bool, error) {
	return s.queryRowContextRowExists(
		r.Context,
		`SELECT COUNT(*) FROM nano_cert_auth_associations WHERE sha256 = ?;`,
		strings.ToLower(hash),
	)
}

func (s *MySQLStorage) IsCertHashAssociated(r *mdm.Request, hash string) (bool, error) {
	return s.queryRowContextRowExists(
		r.Context,
		`SELECT COUNT(*) FROM nano_cert_auth_associations WHERE id = ? AND sha256 = ?;`,
		r.ID, strings.ToLower(hash),
	)
}

func (s *MySQLStorage) AssociateCertHash(r *mdm.Request, hash string, certNotValidAfter time.Time) error {
	_, err := s.db.ExecContext(
		r.Context, `
INSERT INTO nano_cert_auth_associations (id, sha256, cert_not_valid_after) VALUES (?, ?, ?)
ON DUPLICATE KEY
UPDATE
	sha256 = VALUES(sha256),
	cert_not_valid_after = VALUES(cert_not_valid_after)`,
		r.ID,
		strings.ToLower(hash),
		certNotValidAfter,
	)
	if err != nil {
		return fmt.Errorf("associating cert hash: %w", err)
	}
	return nil
}

func (s *MySQLStorage) EnrollmentFromHash(ctx context.Context, hash string) (string, error) {
	var id string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id FROM cert_auth_associations WHERE sha256 = ? LIMIT 1;`,
		hash,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("querying enrollment from hash: %w", err)
	}
	return id, nil
}
