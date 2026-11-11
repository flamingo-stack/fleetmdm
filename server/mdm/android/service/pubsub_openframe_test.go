package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenframeAndroidEnrollTeam(t *testing.T) {
	tenantA, tenantB := new(uint(5)), new(uint(6))

	cases := []struct {
		name       string
		secret     *uint
		prior      *uint
		priorFound bool
		shared     bool
		want       *uint
	}{
		{"shared mode keeps the secret's tenant when the device was another tenant's", tenantB, tenantA, true, true, tenantB},
		{"shared mode keeps the secret's tenant for a device seen in the same tenant", tenantB, tenantB, true, true, tenantB},
		{"shared mode keeps the secret's tenant for a new device", tenantB, nil, false, true, tenantB},
		{"upstream restores the device's last team", tenantB, tenantA, true, false, tenantA},
		{"upstream restores a last team of no team", tenantB, nil, true, false, nil},
		{"upstream uses the secret's team for a new device", tenantB, nil, false, false, tenantB},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, openframeAndroidEnrollTeam(c.secret, c.prior, c.priorFound, c.shared))
		})
	}
}
