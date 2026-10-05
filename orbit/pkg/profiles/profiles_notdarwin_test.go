//go:build !darwin

package profiles

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetFleetdConfig(t *testing.T) {
	config, err := GetFleetdConfig()
	require.ErrorIs(t, err, ErrNotImplemented)
	require.Nil(t, config)
}

func TestIsEnrolledInMDM(t *testing.T) {
	enrolled, serverURL, err := IsEnrolledInMDM()
	require.ErrorIs(t, err, ErrNotImplemented)
	require.False(t, enrolled)
	require.Empty(t, serverURL)
}

func TestCheckAssignedEnrollmentProfile(t *testing.T) {
	err := CheckAssignedEnrollmentProfile("https://test.example.com")
	require.ErrorIs(t, err, ErrNotImplemented)
}

