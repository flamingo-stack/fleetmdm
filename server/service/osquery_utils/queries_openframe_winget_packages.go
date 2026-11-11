// OPENFRAME(winget-packages): reports the packages winget installed, from the OpenFrame osquery winget_packages table
package osquery_utils

// Kept apart from software_windows: an osquery without the winget_packages table skips only this query. winget_id is
// not a software column: Fleet ignores it on the software row and the windows details keep it.
var softwareWingetPackages = DetailQuery{
	Query: `
SELECT
  name AS name,
  version AS version,
  '' AS extension_id,
  '' AS extension_for,
  'winget_packages' AS source,
  '' AS vendor,
  '' AS installed_path,
  id AS winget_id
FROM winget_packages`,
	Platforms: []string{"windows"},
	Discovery: discoveryTable("winget_packages"),
	// Has no IngestFunc, DirectIngestFunc or DirectTaskIngestFunc because
	// the results of this query are appended to the results of the other software queries.
}
