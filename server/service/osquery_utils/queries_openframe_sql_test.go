// OPENFRAME(waf-inventory-shape): guards the query side of the DN hex-encoding
// — openframe/docs/agent-inventory-waf-shape.md
//
// The ingest side (decodeCertificateDNColumns) is covered in queries_test.go;
// this pins the SQL itself. An upstream sync that takes upstream's certificate
// detail queries would drop the hex() wrapping with no git conflict, and every
// inventory write would start tripping the edge WAF's SQLi rules again.
package osquery_utils

import (
	"testing"

	"github.com/fleetdm/fleet/v4/server/config"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/require"
)

func TestOpenframeCertificateQueriesHexEncodeDN(t *testing.T) {
	queries := GetDetailQueries(t.Context(), config.FleetConfig{}, nil, &fleet.Features{}, Integrations{}, nil)

	for _, name := range []string{"certificates_darwin", "certificates_windows"} {
		q, ok := queries[name]
		require.True(t, ok, "detail query %s is missing", name)

		for _, col := range []string{
			"hex(common_name) AS common_name_hex",
			"hex(subject) AS subject_hex",
			"hex(issuer) AS issuer_hex",
		} {
			require.Contains(t, q.Query, col, "%s must hex-encode its DN columns for the WAF", name)
		}
		// The raw columns must not leak through alongside the hex ones.
		require.NotRegexp(t, `(?m)^\s*subject\s*,`, q.Query, "%s selects a raw DN column", name)
	}
}
