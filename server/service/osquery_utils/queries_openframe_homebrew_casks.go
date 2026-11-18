// OPENFRAME(homebrew-casks): reports the casks brew installed that have an app, which software_macos leaves out
package osquery_utils

// The condition is the reverse of the one software_macos puts on casks, so a cask is in exactly one of the two. The
// rows get their own source: the app is already an apps row, and a second homebrew_packages row would get a CPE.
// Homebrew puts a cask's app at <cask>/<version>/<App>.app, so only that level is listed: a recursive pattern follows
// symlinks into whatever a cask links to, and the walk delayed the live queries sent in the same batch past Fleet's
// 25 second window. The second condition is on rtrim(path), not path, so osquery does not expand it as a pattern of
// its own; folders come back with a trailing slash.
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
AND EXISTS (SELECT 1 FROM file WHERE file.path LIKE homebrew_packages.path || '/%/%' AND rtrim(file.path, '/') LIKE '%.app')`,
	Platforms: []string{"darwin"},
	// Has no IngestFunc, DirectIngestFunc or DirectTaskIngestFunc because
	// the results of this query are appended to the results of the other software queries.
}
