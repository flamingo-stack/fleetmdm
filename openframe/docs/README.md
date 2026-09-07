# OpenFrame Fork Documentation

This directory documents **every change this fork makes on top of upstream
[fleetdm/fleet](https://github.com/fleetdm/fleet)**. The fork (`flamingo-stack/fleetmdm`)
adapts Fleet to run as the MDM/osquery tool inside OpenFrame's multi-tenant MSP
platform.

If you add a fork-specific change, add or update a doc here so this index stays a
complete map of the divergence from upstream.

> **See [fork-file-manifest.md](fork-file-manifest.md)** for the path-level
> inventory of every file/directory the fork creates, modifies, and deletes vs.
> upstream Fleet.

## Maintaining the fork (syncing from upstream)

When merging `upstream/main`, follow the
**[upstream-sync-conflict-resolution.md](upstream-sync-conflict-resolution.md)**
runbook — it covers ours/theirs orientation, per-hotspot resolution recipes, the
*semantic-conflict watchlist* (breaks that produce no git conflict), and the
mandatory post-merge steps. Fork edits in shared files are tagged with
`// OPENFRAME(<slug>)` markers; after resolving, run `make openframe-verify`
([openframe/scripts/verify.sh](../scripts/verify.sh)). The repo-root
[`CLAUDE.md`](../../CLAUDE.md) points Claude Code at this protocol automatically.

## The master switch

Most server-side behavior is gated behind a single environment variable:

- **`FLEET_OPENFRAME_MODE=1`** — read by `fleet.IsOpenframeMode()` in
  [`server/fleet/openframe.go`](../../server/fleet/openframe.go). When unset, the
  server behaves like stock Fleet.

The agent has its own switch, `--openframe-mode` / `ORBIT_OPENFRAME_MODE`.

## Server-side features

| Doc | Change |
|-----|--------|
| [architecture-host-assignments.md](architecture-host-assignments.md) | Direct host → policy/query targeting (`policy_hosts` / `query_hosts` join tables), gated by `FLEET_OPENFRAME_MODE`. Design & internals. |
| [api-host-assignments.md](api-host-assignments.md) | REST API for the above (add/remove/replace/list hosts). |
| [managed-policies.md](managed-policies.md) | `policies.openframe_managed` — platform-owned policies omitted from the policy list/count endpoints (and from GitOps deletion) while still running on hosts and reporting results. |
| [managed-queries.md](managed-queries.md) | `queries.openframe_managed` — the queries twin: platform-owned queries omitted from the query listing and its count. |
| [api-expose-osquery-host-id.md](api-expose-osquery-host-id.md) | Exposes `osquery_host_id` in the host JSON so the OpenFrame control plane can match agents. |
| [query-results-ttl-cleanup.md](query-results-ttl-cleanup.md) | Time-based cleanup of `query_results` (keeps the Debezium CDC pipeline alive without unbounded growth). Gated by OpenFrame mode **and** a positive TTL. |
| [redis-key-prefix.md](redis-key-prefix.md) | Per-tenant Redis key/channel prefix (`FLEET_REDIS_KEY_PREFIX`) so tenants can share one Redis. |

## Agent-side (orbit / fleetd)

| Doc | Change |
|-----|--------|
| [agent-openframe-mode.md](agent-openframe-mode.md) | OpenFrame agent mode: gateway URL prefix, encrypted bearer-token pipeline (extract / decrypt / refresh), custom osqueryd, `orbit uuid` command. |
| [node-key-management.md](node-key-management.md) | Node-key enrollment caching, 401 re-enrollment, Windows file-lock resilience. |
| [agent-json-content-type.md](agent-json-content-type.md) | Orbit sets `Content-Type: application/json` on requests with a body (upstream sets none). Without it a WAF cannot JSON-parse the body — Cloud Armor flagged 100% of `/orbit/config` polls as SQLi. Unconditional, not gated on OpenFrame mode. |
| [agent-options.md](agent-options.md) | Keeps Fleet agent options (`distributed_interval: 10` et al.): skips upstream's setup-time starter-library apply in OpenFrame mode (it nulls the options setup just wrote) and seeds shared-mode tenants with the full new-install defaults. |
| [agent-inventory-waf-shape.md](agent-inventory-waf-shape.md) | The `certificates_darwin`/`certificates_windows` detail queries hex-encode their distinguished-name columns; the ingest decodes them. Raw X.509 DNs are `/`+`=` dense and trip CRS 942431/942432 on every inventory write. Server-side only — no agent upgrade. |

## Database migrations

| Doc | Change |
|-----|--------|
| [migrations.md](migrations.md) | Separate goose client for OpenFrame migrations (`migration_status_openframe`), **and** the in-place rewrite of ~475 upstream migrations to be idempotent. |

## Infrastructure

| Doc | Change |
|-----|--------|
| [helm-chart.md](helm-chart.md) | Fork-owned Helm chart: externalized config (ConfigMaps/Secrets), OpenFrame mode, Redis prefix wiring, restructured migration job. |
| [ci-cd-release-pipeline.md](ci-cd-release-pipeline.md) | Release pipeline, macOS/Windows code signing, GHCR publishing, automated upstream sync. |

## Developer setup

| Doc | Change |
|-----|--------|
| [local-setup.md](local-setup.md) | Build and run Fleet locally in OpenFrame mode. |
| [test_host_assignments.md](test_host_assignments.md) | The `openframe/scripts/test_host_assignments.sh` end-to-end test. |

## Map of fork code surface

New OpenFrame-only code lives in:

- [`server/service/openframe/`](../../server/service/openframe/) — agent token-auth pipeline
- [`server/datastore/mysql/migrations/openframe/`](../../server/datastore/mysql/migrations/openframe/) — OpenFrame migrations
- [`server/datastore/redis/keyprefix.go`](../../server/datastore/redis/keyprefix.go) — Redis prefix wrapper
- [`server/fleet/openframe.go`](../../server/fleet/openframe.go) — the `IsOpenframeMode()` gate
- [`openframe/`](../) — these docs and `scripts/`

Existing Fleet files are also modified (host assignment service/datastore code,
`orbit/cmd/orbit/orbit.go`, `server/service/orbit_client.go`, `cmd/fleet/serve.go`,
`server/config/config.go`, the Helm chart, and the upstream migrations). Each
doc's **Files changed** and **Rebase notes** sections list the specifics.
