# osquery Schema Search API

The schema search endpoint gives OpenFrame services the canonical Fleet/osquery table and column
documentation they need before generating SQL. Search is local and deterministic. Fleet loads the
vendored `schema/osquery_fleet_schema.json` as an immediate fallback, then can refresh the in-memory
snapshot from a configured HTTP source without restarting.

## Automatic refresh

```yaml
osquery:
  schema_refresh_enabled: true
  schema_refresh_interval: 24h
  schema_refresh_url: https://raw.githubusercontent.com/fleetdm/fleet/<compatible-tag-or-commit>/schema/osquery_fleet_schema.json
```

The equivalent environment variables are `FLEET_OSQUERY_SCHEMA_REFRESH_ENABLED`,
`FLEET_OSQUERY_SCHEMA_REFRESH_INTERVAL`, and `FLEET_OSQUERY_SCHEMA_REFRESH_URL`. Generic Fleet
configuration and the Fleet Helm chart keep refresh disabled and default to the upstream commit that
produced the embedded schema. OpenFrame environment overlays can opt in to a 24-hour refresh from
the reviewed `flamingo-stack/fleetmdm/main` schema. Do not point production deployments directly at
mutable upstream `fleetdm/fleet/main`; pin the source if a deployment must remain on an older schema.

Each Fleet replica performs one best-effort refresh after startup and then retries at the configured
interval. A snapshot is replaced atomically only after a `200` response has been fully read and
validated as a non-empty schema. Network, HTTP, size, and JSON failures keep the previous snapshot,
so the embedded schema remains available during startup and outages. Search requests never perform
network calls.

## Endpoint

```http
GET /api/v1/fleet/osquery/schema/search?query=windows%20updates&platform=windows&limit=20
Authorization: Bearer <token>
X-Tenant-Id: <tenant UUID>
```

The endpoint uses Fleet's existing user-authenticated route middleware and is included in the API
endpoint catalog for restricted API-only users. In shared OpenFrame mode, the existing tenant
middleware also requires `X-Tenant-Id`, even though the schema itself is global.

## Query parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `query` | yes | — | Non-empty English search phrase, at most 512 bytes and 32 unique searchable terms. Table names, descriptions, examples, notes, and column documentation are searched. |
| `platform` | no | `all` | `all`, `darwin`/`macos`, `windows`, `linux`, or `chrome`/`chromeos`. |
| `limit` | no | `20` | Maximum returned tables, from `1` through `50`. |

Results are ordered by deterministic relevance score and then table name. The platform filter is
applied before ranking. A successful search with no matches returns `tables: []` and `count: 0`.

## Response

```json
{
  "query": "windows updates",
  "platform": "windows",
  "count": 1,
  "tables": [
    {
      "name": "windows_update_history",
      "platforms": ["windows"],
      "description": "Provides the history of the windows update events.",
      "columns": [
        {
          "name": "result_code",
          "type": "text",
          "description": "Result of an operation on an update",
          "notes": null,
          "required": false,
          "hidden": false,
          "index": false
        }
      ],
      "examples": "select * from windows_update_history",
      "notes": null,
      "url": "https://fleetdm.com/tables/windows_update_history",
      "evented": false,
      "cacheable": false
    }
  ]
}
```

The API preserves nullable canonical fields: table `examples` and `notes`, plus column `notes` and
`hidden`, may be `null`. Column `required` is always a boolean. When the canonical schema restricts
a column more narrowly than its table, `columns[].platforms` preserves that platform list and its
original case (for example, `battery.health` is `macOS`-only even though `battery` supports both
Darwin and Windows).

## Errors

| Status | Meaning |
|--------|---------|
| `400` | Empty/unsearchable or oversized query, unsupported platform, or limit outside `1..50` |
| `401` | Missing/invalid Fleet token or, in shared mode, missing/invalid tenant header |
| `403` | Authenticated principal is not allowed to call user API endpoints |

## Files changed

- `schema/osquery_search.go` — embedded fallback, atomic snapshot, and deterministic ranking.
- `schema/osquery_refresh.go` — bounded HTTP refresh and validation.
- `cmd/fleet/osquery_schema_refresh_openframe.go` — cancellable per-replica refresh loop.
- `server/config/config.go` — YAML, environment, and CLI configuration.
- `charts/fleet/values.yaml` — OpenFrame deployment defaults.
- `server/service/osquery_schema_openframe.go` — HTTP request/response and endpoint.
- `server/service/handler.go` — route registration.
- `server/api_endpoints/api_endpoints.yml` — API-only user allowlist catalog entry.

## Upstream sync notes

The schema search and refresh Go files are fork-added. Preserve the
`OPENFRAME(osquery-schema-search)` blocks in `server/config/config.go`, `cmd/fleet/serve.go`,
`server/service/handler.go`, and the Helm values when syncing upstream.
