# Certificate inventory wire shape

**Slug:** `OPENFRAME(waf-inventory-shape)` · **File:** [`server/service/osquery_utils/queries.go`](../../server/service/osquery_utils/queries.go)

The `certificates_darwin` and `certificates_windows` detail queries hex-encode their three
distinguished-name columns (`common_name`, `subject`, `issuer`), aliased with a `_hex` suffix.
`decodeCertificateDNColumns` decodes them at ingest, before anything else reads the row.

## Why

An X.509 distinguished name looks like `/C=US/ST=California/O=Acme, Inc./CN=…`. The CRS
`942431`/`942432` signatures are *restricted special-character counters* — they fire when an
argument value exceeds 6 (resp. 2) characters from a punctuation class that includes `/` and `=`.
Every DN exceeds that, so every certificate row in every inventory write matched.

Measured in `shared-j62b` (dev), 24 h to 2026-08-07: **453** Cloud Armor preview DENYs on
`/tools/agent/fleetmdm-server/api/v1/osquery/*`, all `client-policy`, all UA `osquery/5.9.1`.
`certificates_darwin.*.subject` / `.common_name` was **237 of them (52.3 %)** — the single largest
source, and the only large one that is fixable in code:

| Source | Events | % |
|---|---:|--:|
| `certificates_darwin` DN columns | 237 | 52.3 % |
| `data.N.hostIdentifier` | 65 | 14.3 % |
| `scheduled_query_stats.*.query` | 59 | 13.0 % |
| `software_windows.*.version` | 49 | 10.8 % |
| `orbit_info.*.last_recorded_error` | 21 | 4.6 % |
| `host_details.*`, `fleet_distributed_query_*` | 22 | 4.9 % |

Sample matched values: `=com.apple.kerbe`, `=US/ST=Californi`, `local (`.

Hex specifically, not base64: `[0-9A-F]` contains no punctuation at all. Base64 std (`+/=`) and
base64url (`-_`) both land back inside the restricted class.

## Rollout

Detail-query SQL is generated server-side and handed to the agent in the `distributed/read`
response (`detailQueriesForHost`, [`server/service/osquery.go`](../../server/service/osquery.go)).
Both halves of this change live in the server binary, so **the census clears on server deploy —
there is no agent upgrade wait**, unlike [agent-json-content-type.md](agent-json-content-type.md).

During the upgrade window a host may post results computed from the previous query. Those rows
carry the plain columns and no `_hex` key, so `decodeCertificateDNColumns` skips the hex step and
runs only the `\xHH` unescape — identical to pre-change behaviour. Nothing is dropped.

## Not covered

This removes 52 % of the agent-ingest false positives, not all of them. Hardware UUIDs, third-party
version strings and live query results are arbitrary by nature; the only way to change their shape
would be to re-encode the osquery↔Fleet wire format on both ends. `scheduled_query_stats` has a
supported off switch instead — `FLEET_APP_ENABLE_SCHEDULED_QUERY_STATS=false` — so it is not
patched here.

Enforcing `942431`/`942432` on the agent path therefore still needs a path-scoped rule band in
Cloud Armor. See `openframe-saas-tf` `openframe-saas/*/services/03-shared/armor.tf`.

## Verifying

```bash
gcloud logging read \
  'resource.type="http_load_balancer" AND
   jsonPayload.previewSecurityPolicy.outcome="DENY" AND
   httpRequest.requestUrl:"/api/v1/osquery/"' \
  --project=shared-j62b --format=json --limit=5000
```

Group by `previewSecurityPolicy.matchedFieldName`; the `certificates_darwin.*` rows should reach
zero within one detail-query refetch interval of the deploy.

## Before shipping

Run the modified `SELECT` under `osqueryi` on a real Mac and a real Windows host and confirm
`hex(subject)` round-trips through `hex.DecodeString` + `DecodeHexEscapes` to the same string the
previous query produced. Whether osquery's `\xHH` escaping of non-ASCII sits inside the value that
`hex()` sees decides the decode order, and that is worth confirming against a live host rather than
a fixture. `TestDirectIngestHostCertificatesHexEncodedDN` covers the Go side.
