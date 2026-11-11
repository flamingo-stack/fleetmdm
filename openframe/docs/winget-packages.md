# Winget packages

## What

The OpenFrame osquery build (0.0.14+) has a Windows table `winget_packages`: the packages winget itself installed that
are still installed, read from winget's tracking catalog in each user profile.

Fleet collects it as an extra software query, `software_winget_packages`, with source `winget_packages`. Its rows join
the Windows software rows before ingestion, like `go_binaries` and the other extra queries.

## Rules

- The query runs only where osquery has the table (`discoveryTable`), so older agents skip it and keep reporting the
  rest of their software.
- No CPE is made for `winget_packages` rows: each has a `programs` twin for the same install, which carries the CVEs
  with the right target platform. A CPE for the winget row would duplicate them and, having no target platform, would
  also match CVEs written for other operating systems.
- The winget package id (table column `id`) has no place on a Fleet software row. The query returns it as `winget_id`,
  which software ingestion ignores, and `appendWingetRowsToWindowsDetails` copies each winget row into the
  `software_windows_details` results before the winget rows join `software_windows`. The details ingest keys the row by
  the checksum of its `winget_packages` software row and stores `winget_id` in `host_software_windows_details`
  (migration `20261111000001`); `GET /hosts/{id}/software` returns it on `installed_versions[]`. The copy happens only
  when both queries ran: the details are replaced per host as a whole, so a failed details query must not leave only
  the winget rows.
- A user scope package is listed only while its owner's registry hive is loaded, like user scope rows of `programs`.

## Files

`server/service/osquery_utils/queries_openframe_winget_packages.go`, `server/service/osquery.go`
(`preProcessSoftwareResults`), `server/service/osquery_utils/queries.go` (`GetDetailQueries`),
`server/vulnerabilities/nvd/cpe_openframe_winget_packages.go`, `server/vulnerabilities/nvd/cpe.go`,
`server/service/osquery_openframe_winget_details.go`, `server/service/osquery_utils/queries_openframe_software_windows_details.go`,
`server/datastore/mysql/{openframe_host_software_windows_details,software}.go`, `server/fleet/openframe_software_windows_details.go`,
`migrations/openframe/20261111000001_AddHostSoftwareWindowsDetailsWingetId.go`, `schema.sql`.
