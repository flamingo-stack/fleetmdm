// OPENFRAME(homebrew-casks): reports the casks brew installed that have an app, which software_macos leaves out
package osquery_utils

// The condition is the reverse of the one software_macos puts on casks, so a cask is in exactly one of the two. The
// rows get their own source: the app is already an apps row, and a second homebrew_packages row would get a CPE.
var softwareHomebrewCasks = DetailQuery{
	Query: `
SELECT
  name AS name,
  version AS version,
  '' AS bundle_identifier,
  '' AS extension_id,
  '' AS extension_for,
  'homebrew_casks' AS source,
  '' AS vendor,
  0 AS last_opened_at,
  path AS installed_path
FROM homebrew_packages
WHERE type = 'cask'
AND EXISTS (SELECT 1 FROM file WHERE file.path LIKE CONCAT(homebrew_packages.path, '/%%') AND file.path LIKE '%.app%' LIMIT 1)`,
	Platforms: []string{"darwin"},
	// Has no IngestFunc, DirectIngestFunc or DirectTaskIngestFunc because
	// the results of this query are appended to the results of the other software queries.
}
