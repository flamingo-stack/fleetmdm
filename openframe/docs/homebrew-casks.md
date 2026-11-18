# Homebrew casks

## What

Fleet's macOS software query leaves out a cask that has an app: the app is already an `apps` row, and that row has the
bundle identifier the vulnerability matching needs. So a program brew installed as a cask looks like a manual install,
and OpenFrame cannot tell that brew manages it.

Fleet collects these casks as an extra software query, `software_homebrew_casks`, with source `homebrew_casks`. Its
rows join the macOS software rows before ingestion, like `go_binaries` and the other extra queries.

## Rules

- The condition is the reverse of the one `software_macos` puts on casks, so a cask is reported by exactly one of the
  two: with an app here, without one (a font, a command line tool) as `homebrew_packages`, as before.
- The row's name is the cask token and its version is the cask version; `installed_path` is the cask's folder in the
  Caskroom.
- The version is the one brew recorded at install or upgrade. An app that updates itself (a cask with
  `auto_updates`) moves on without brew, so the installed version is the one on the `apps` row.
- No CPE is made for `homebrew_casks` rows: each has an `apps` twin for the same install, which carries the CVEs. A CPE
  for the cask row would count them twice.
- The upstream query is not changed.

## Files

`server/service/osquery_utils/queries_openframe_homebrew_casks.go`, `server/service/osquery.go`
(`preProcessSoftwareResults`), `server/service/osquery_utils/queries.go` (`GetDetailQueries`),
`server/vulnerabilities/nvd/cpe_openframe_winget_packages.go`.
