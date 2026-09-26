// OPENFRAME(osquery-host-id): unit test for the API exposure of
// osquery_host_id — openframe/docs/api-expose-osquery-host-id.md
//
// Upstream tags this field json:"-"; the fork exposes it because OpenFrame
// correlates devices by their osquery host id. A merge that takes upstream's
// struct tag would drop the field from every host API response with no
// compile error — this test turns that into a red build.
package fleet

import (
	"encoding/json"
	"testing"

	"github.com/fleetdm/fleet/v4/server/ptr"
	"github.com/stretchr/testify/require"
)

func TestHostJSONExposesOsqueryHostID(t *testing.T) {
	h := Host{OsqueryHostID: ptr.String("host-uuid-1")}

	b, err := json.Marshal(h)
	require.NoError(t, err)
	require.Contains(t, string(b), `"osquery_host_id":"host-uuid-1"`)

	var back struct {
		OsqueryHostID *string `json:"osquery_host_id"`
	}
	require.NoError(t, json.Unmarshal(b, &back))
	require.NotNil(t, back.OsqueryHostID)
	require.Equal(t, "host-uuid-1", *back.OsqueryHostID)
}
