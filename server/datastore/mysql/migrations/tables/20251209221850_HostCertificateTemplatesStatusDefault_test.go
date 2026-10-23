package tables

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUp_20251209221850(t *testing.T) {
	db := applyUpToPrev(t)

	// Insert a row before the migration to verify existing rows are unaffected.
	insertStmt := `INSERT INTO host_certificate_templates (
		host_id, certificate_template_id, status, fleet_challenge
	) VALUES (?, ?, ?, ?)`
	res, err := db.Exec(insertStmt, 1, 1, "pending", "abcdefabcdefabcdefabcdefabcdefab")
	require.NoError(t, err)
	existingID, err := res.LastInsertId()
	require.NoError(t, err)

	applyNext(t, db)

	// Existing row should be unaffected.
	var status string
	var fleetChallenge sql.NullString
	err = db.QueryRow(
		`SELECT status, fleet_challenge FROM host_certificate_templates WHERE id = ?`, existingID,
	).Scan(&status, &fleetChallenge)
	require.NoError(t, err)
	assert.Equal(t, "pending", status)
	assert.True(t, fleetChallenge.Valid)
	assert.Equal(t, "abcdefabcdefabcdefabcdefabcdefab", fleetChallenge.String)

	// fleet_challenge should now accept NULL.
	res, err = db.Exec(
		`INSERT INTO host_certificate_templates (host_id, certificate_template_id, status, fleet_challenge) VALUES (?, ?, ?, ?)`,
		2, 1, "pending", nil,
	)
	require.NoError(t, err)
	nullChallengeID, err := res.LastInsertId()
	require.NoError(t, err)

	err = db.QueryRow(
		`SELECT fleet_challenge FROM host_certificate_templates WHERE id = ?`, nullChallengeID,
	).Scan(&fleetChallenge)
	require.NoError(t, err)
	assert.False(t, fleetChallenge.Valid)

	// New inserts without an explicit status should default to 'pending'.
	res, err = db.Exec(
		`INSERT INTO host_certificate_templates (host_id, certificate_template_id, fleet_challenge) VALUES (?, ?, ?)`,
		3, 1, nil,
	)
	require.NoError(t, err)
	defaultStatusID, err := res.LastInsertId()
	require.NoError(t, err)

	err = db.QueryRow(
		`SELECT status FROM host_certificate_templates WHERE id = ?`, defaultStatusID,
	).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "pending", status)
}
