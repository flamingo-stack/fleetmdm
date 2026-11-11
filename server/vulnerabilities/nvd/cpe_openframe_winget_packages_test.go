// OPENFRAME(winget-packages)
package nvd

import (
	"context"
	"errors"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mock"
	"github.com/stretchr/testify/require"
)

var errStopTranslate = errors.New("stop")

func TestTranslateSoftwareToCPE_skipsWingetPackages(t *testing.T) {
	ds := new(mock.Store)
	var excluded [][]string
	ds.AllSoftwareIteratorFunc = func(ctx context.Context, opts fleet.SoftwareIterQueryOptions) (fleet.SoftwareIterator, error) {
		excluded = append(excluded, opts.ExcludedSources)
		return nil, errStopTranslate
	}

	_ = TranslateSoftwareToCPE(context.Background(), ds, t.TempDir(), nil)

	require.NotEmpty(t, excluded)
	require.Contains(t, excluded[0], "winget_packages")
	require.Contains(t, excluded[0], "ios_apps")
}
