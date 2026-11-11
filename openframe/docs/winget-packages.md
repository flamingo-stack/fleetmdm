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
- The winget package id is not stored: a Fleet title is keyed by name and source, and the titles API does not return
  `extension_id`.
- A user scope package is listed only while its owner's registry hive is loaded, like user scope rows of `programs`.

## Files

`server/service/osquery_utils/queries_openframe_winget_packages.go`, `server/service/osquery.go`
(`preProcessSoftwareResults`), `server/service/osquery_utils/queries.go` (`GetDetailQueries`),
`server/vulnerabilities/nvd/cpe_openframe_winget_packages.go`, `server/vulnerabilities/nvd/cpe.go`.
