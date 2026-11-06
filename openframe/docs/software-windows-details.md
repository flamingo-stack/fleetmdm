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

The fork stores them per software row and returns them, together with the vendor Fleet already stores, on:

- `GET /api/latest/fleet/software/titles` and `/software/titles/{id}`: on each `versions[]` entry;
- `GET /api/latest/fleet/hosts/{id}/software`: on each `installed_versions[]` entry.

Every field is omitted when it is empty.

## How

- Table `software_windows_details (software_id PK -> software.id ON DELETE CASCADE, ...)`, migration
  `migrations/openframe/20261105000001_AddSoftwareWindowsDetailsTable.go`. `schema.sql` carries it too, because
  upstream queries join it and test databases are built from that file.
- Detail query `software_windows_details` (`server/service/osquery_utils/queries_openframe_software_windows_details.go`)
  runs on Windows with software inventory on, for rows with an ARP key or a package family name. It is a separate
  query, so an osquery without the columns fails only this query and the regular `software_windows` ingestion is
  untouched.
- Its ingest rebuilds each row the way `software_windows` builds it (`SoftwareFromOsqueryRow` with source
  `programs`, `MutateSoftwareOnIngestion`, `shouldRemoveSoftware`) and finds the software row by its raw checksum.
  `Datastore.OpenframeUpsertSoftwareWindowsDetails` writes them in batches of 500; a checksum with no software row
  yet is skipped and filled on a later report.
- The checksum is not changed, so no existing software row is re-created and no title or host link moves.

## Traps

- Any change to how `directIngestSoftware` builds a `programs` row (new field in the checksum, a new mutation) must
  be mirrored in `directIngestSoftwareWindowsDetails`, or the details stop finding their rows. The ingest test
  compares the two checksums.
- A software row is shared by every team. Two hosts that report the same name, version, vendor and upgrade code with
  different details overwrite each other; the last report wins.
- Writing details does not touch `host_software`, so a CDC consumer of `host_software` sees them only with the host's
  next software change.
- The reads go through the existing fenced title and host software queries; the join adds no rows of other teams.
- `make dump-test-schema` drops the table from `schema.sql`. Put it back.
