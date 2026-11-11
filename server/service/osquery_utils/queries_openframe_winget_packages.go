// OPENFRAME(winget-packages): reports the packages winget installed, from the OpenFrame osquery winget_packages table
package osquery_utils

// Kept apart from software_windows: an osquery without the winget_packages table skips only this query. The winget id
// is not reported: a Fleet title is keyed by name and source and its API does not return extension_id.
var softwareWingetPackages = DetailQuery{
	Query: `
SELECT
  name AS name,
  version AS version,
  '' AS extension_id,
  '' AS extension_for,
  'winget_packages' AS source,
  '' AS vendor,
  '' AS installed_path
FROM winget_packages`,
	Platforms: []string{"windows"},
	Discovery: discoveryTable("winget_packages"),
	// Has no IngestFunc, DirectIngestFunc or DirectTaskIngestFunc because
	// the results of this query are appended to the results of the other software queries.
}
