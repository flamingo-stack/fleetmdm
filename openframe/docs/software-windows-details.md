# Software Windows details

## What

The OpenFrame osquery build adds columns to the Windows `programs` table that upstream Fleet does not keep. A
winget-style matcher needs them to correlate an installed program with a package the way winget does:

| Column | Meaning |
|---|---|
| `arp_key` | name of the program's Uninstall registry subkey (`Google Chrome`, `Git_is1`, an MSI GUID); winget's ProductCode |
| `package_family_name` | MSIX package family name; winget correlates MSIX packages by it |
| `system_component` | the Uninstall entry is hidden from Programs and Features; winget skips such entries |
| `installer_type` | msi, exe, portable or msix |
| `install_scope` | machine or user |
| `install_arch` | 64 or 32, the registry view of a machine scope entry |
| `winget_id` | winget package id, only on `winget_packages` rows (see [winget-packages.md](winget-packages.md)) |

The fork stores them per host and returns them on `GET /api/latest/fleet/hosts/{id}/software`, on each
`installed_versions[]` entry, together with the vendor Fleet already stores. The title endpoints
(`/software/titles`, `/software/titles/{id}`) return the vendor on each `versions[]` entry and no per-install detail: a
`software` row is shared by every host and tenant, while an install's key, scope and installer differ per host (the
same Chrome version sits under `Google Chrome` from the EXE and under an MSI GUID from the MSI).

Every field is omitted when it is empty.

## How

- Table `host_software_windows_details (host_id, software_id PK; software_id -> software.id ON DELETE CASCADE, ...)`,
  shaped like `host_software_installed_paths`, migration
  `migrations/openframe/20261105000001_AddHostSoftwareWindowsDetailsTable.go`; `winget_id` added by
  `20261111000001_AddHostSoftwareWindowsDetailsWingetId.go`. `schema.sql` carries it too, because
  upstream queries join it and test databases are built from that file. `hostRefs` lists it, so deleting a host
  deletes its rows.
- Detail query `software_windows_details` (`server/service/osquery_utils/queries_openframe_software_windows_details.go`)
  runs on Windows with software inventory on, for rows with an ARP key or a package family name. It is a separate
  query, so an osquery without the columns fails only this query and the regular `software_windows` ingestion is
  untouched.
- The software override queries that rewrite `software_windows` rows (`windows_acrobat_dc` adds " DC" to the name)
  rewrite the details rows the same way (`applyOverrideToWindowsDetails` in `server/service/osquery.go`); rows an
  override appends carry no key and are dropped; winget rows keep their place through it.
- Its ingest rebuilds each row the way `software_windows` builds it (`SoftwareFromOsqueryRow` with the row's source,
  `programs` when it has none, `MutateSoftwareOnIngestion`, `shouldRemoveSoftware`) and finds the software row by its raw checksum.
  `Datastore.OpenframeReplaceHostSoftwareWindowsDetails` replaces the host's rows as a whole in one transaction:
  upserts in batches of 500, then deletes the rows the host no longer reports. A checksum with no software row yet is
  skipped and filled on a later report.
- The checksum is not changed, so no existing software row is re-created and no title or host link moves.

## Traps

- Any change to how a `programs` row reaches `directIngestSoftware` (a new override query, a new field in the
  checksum, a new mutation) must be mirrored for the details rows, or they stop finding their software rows. The
  ingest test compares the two checksums; the service test checks the Acrobat rename reaches both.
- The rows are per host on purpose. Do not move them onto `software`: that row is shared by every host and tenant,
  and the details would flip between hosts whose installs differ.
- Writing details does not touch `host_software`, so a CDC consumer of `host_software` sees them only with the host's
  next software change.
- The reads go through the existing fenced title and host software queries; the join adds no rows of other teams.
- `make dump-test-schema` drops the table from `schema.sql`. Put it back.
