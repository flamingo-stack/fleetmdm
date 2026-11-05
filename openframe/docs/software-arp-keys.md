# Software ARP keys

## What

Windows lists every installed program under an Uninstall registry key. The name of that subkey (the ARP key) is
what winget calls the program's ProductCode: `Google Chrome`, `Git_is1`, `{23170F69-40C1-2702-2409-000001000000}`.
winget correlates an installed program with a package of its index by this key first, and by the normalized name
and publisher only after it. Upstream Fleet does not keep the key: `programs.identifying_number` holds it only when
it is a GUID.

The fork collects the key from the `programs.arp_key` column of the OpenFrame osquery build and returns it, together
with the vendor Fleet already stores, on:

- `GET /api/latest/fleet/software/titles` and `/software/titles/{id}`: `versions[].arp_key`, `versions[].vendor`;
- `GET /api/latest/fleet/hosts/{id}/software`: `installed_versions[].arp_key`, `installed_versions[].vendor`.

Both fields are omitted when empty.

## How

- Table `software_arp_keys (software_id PK -> software.id ON DELETE CASCADE, arp_key)`, migration
  `migrations/openframe/20261105000001_AddSoftwareArpKeysTable.go`. `schema.sql` carries it too, because upstream
  queries join it and test databases are built from that file.
- Detail query `software_windows_arp_keys` (`server/service/osquery_utils/queries_openframe_software_arp_keys.go`)
  runs on Windows with software inventory on. It is a separate query, so an osquery without the column fails only
  this query and the regular `software_windows` ingestion is untouched.
- Its ingest rebuilds each row the way `software_windows` builds it (`SoftwareFromOsqueryRow` with source
  `programs`, `MutateSoftwareOnIngestion`, `shouldRemoveSoftware`) and finds the software row by its raw checksum.
  `Datastore.OpenframeUpsertSoftwareArpKeys` writes them in batches of 500; a checksum with no software row yet is
  skipped and filled on a later report.
- The checksum is not changed, so no existing software row is re-created and no title or host link moves.

## Traps

- Any change to how `directIngestSoftware` builds a `programs` row (new field in the checksum, a new mutation) must
  be mirrored in `directIngestSoftwareArpKeys`, or the keys stop finding their rows. The ingest test compares the
  two checksums.
- A software row is shared by every team. Two hosts that report the same name, version, vendor and upgrade code
  under different ARP keys overwrite each other's key; the last report wins.
- The reads go through the existing fenced title and host software queries; the join adds no rows of other teams.
- `make dump-test-schema` drops the table from `schema.sql`. Put it back.
