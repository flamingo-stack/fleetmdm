# Upstream Sync — Conflict-Resolution Runbook

This is the procedure for merging upstream `fleetdm/fleet` into this fork and
resolving the conflicts. It is written to be followed by a human **or by Claude
Code**. Read it before touching a conflicted merge.

## Mechanics & orientation

- The fork tracks Fleet **~v4.81.2** and is thousands of commits behind
  `upstream/main`. It does **not** clean-merge — historically it squash-rebases.
- The CI workflow `.github/workflows/sync-upstream.yml` runs `git merge --no-ff
  upstream/main` weekly and **aborts on any conflict** (it skips and retries next
  week). So real resolution happens **locally**:

  ```bash
  git remote get-url upstream || git remote add upstream https://github.com/fleetdm/fleet.git
  git fetch upstream
  git checkout -b sync/upstream-main
  git merge --no-ff upstream/main      # resolve conflicts, then continue
  ```

- **Orientation:** in this merge, **ours = the fork** (`HEAD` / `sync/upstream-main`),
  **theirs = upstream**. In a conflict hunk, `<<<<<<< HEAD` is fork code and
  `>>>>>>> upstream/main` is upstream code.

## Cardinal rule

**Never drop fork logic to make a conflict go away.** The default resolution is
to **keep both sides**: take upstream's change *and* re-apply the fork's edit on
top. Fork edits inside shared files are wrapped in `// OPENFRAME(<slug>)` markers
(see below) — if a conflict removes one, you have lost fork behavior.

## How to find what the fork changed (and why)

1. **In-code markers.** Every fork edit in a shared upstream file is wrapped:

   ```go
   // >>> OPENFRAME(redis-key-prefix): namespace keys per tenant — openframe/docs/redis-key-prefix.md
   ...fork lines...
   // <<< OPENFRAME(redis-key-prefix)
   ```

   List them: `grep -rn "OPENFRAME(" --include='*.go' --include='*.yaml' --include='*.tpl' .`
   Slugs: `host-assignments`, `redis-key-prefix`, `query-results-ttl`,
   `osquery-host-id`, `agent-openframe-mode`, `helm`, plus `migrations-idempotency`
   (the upstream migrations carry `// Idempotent migration.`).

2. **The manifest.** [fork-file-manifest.md](fork-file-manifest.md) lists every
   created/modified/deleted path.

3. **The feature docs.** Each has a *Rebase notes* / *Upstream merge notes* section:
   [architecture-host-assignments.md](architecture-host-assignments.md),
   [redis-key-prefix.md](redis-key-prefix.md),
   [query-results-ttl-cleanup.md](query-results-ttl-cleanup.md),
   [agent-openframe-mode.md](agent-openframe-mode.md),
   [migrations.md](migrations.md), [api-expose-osquery-host-id.md](api-expose-osquery-host-id.md),
   [helm-chart.md](helm-chart.md).

## Per-hotspot resolution recipes

| File(s) | Fork content (keep it) | On conflict |
|---------|------------------------|-------------|
| `cmd/fleet/serve.go` | `query-results-ttl` cron registration block; `redis-key-prefix` `KeyPrefix:` pool wiring | Re-place the cron `StartCronSchedule` block after upstream's other schedule registrations; keep `KeyPrefix:` in the pool config literal |
| `server/config/config.go` | `KeyPrefix` (RedisConfig) + `QueryResultsTTL`/`QueryResultsCleanupInterval` (ServerConfig) fields, their `addConfig*`/`getConfig*` calls | Re-add the fields and the matching register/read lines if upstream restructures the structs |
| `server/datastore/mysql/policies.go`, `queries.go` | `if fleet.IsOpenframeMode()` hooks inside upstream query builders + the fork `Add/Remove/Replace/List*Hosts` funcs + `loadHostsFor*` | Keep the fork funcs verbatim; re-insert each `IsOpenframeMode()` hook into the (possibly rewritten) upstream query builder |
| `server/datastore/mysql/hosts.go` | `policy_hosts` EXISTS filter under `IsOpenframeMode()` | Re-insert the `AND EXISTS (SELECT 1 FROM policy_hosts …)` clause into the host-policies query |
| `server/datastore/mysql/mysql.go`, `cmd/fleet/prepare.go` | `MigrateOpenframe` method + its call (and the removed early-return in `prepare.go`) | Keep `MigrateOpenframe` running after `MigrateTables`/`MigrateData`; ensure no early `return` skips it |
| `server/fleet/datastore.go`, `service.go` | 8 host-assignment methods + `MigrateOpenframe` + `CleanupExpiredQueryResults` on the interfaces | Re-add the method signatures; then **regenerate mocks** (below) |
| `server/fleet/hosts.go` | `OsqueryHostID` json tag is `json:"osquery_host_id"` (upstream = `json:"-"`) | Keep the fork tag |
| `server/service/handler.go` | routes `POST/DELETE/PUT/GET …/{id}/hosts` | Re-register the 4+4 routes |
| `server/datastore/redis/redis.go` | pool `keyPrefix` fields, `KeyPrefix()` accessors, `normalizeKeyPrefix`, `newPrefixedConn`/`unwrapConn` around `redisc` | **Critical** — see watchlist; verify every pool `Get()` returns `newPrefixedConn(...)` |
| `orbit/cmd/orbit/orbit.go` | 4 `openframe-*` flags, custom osqueryd path, token-refresher startup, `uuid` cmd, osquery flag passthrough, `NewOrbitClient(..., openFrameMode, authManager)` args | Keep all; re-thread the two extra `NewOrbitClient` args at both call sites |
| `server/service/orbit_client.go` | `openFrameMode`/`authManager` fields, bearer-header block, `/tools/agent/fleetmdm-server` url prefix, `NewOrbitClient` signature | Keep the two trailing constructor params and the header/prefix logic |
| `charts/fleet/*` | externalized config, `FLEET_OPENFRAME_MODE`, `FLEET_REDIS_KEY_PREFIX`, migration job, waitForMysql, probe split, additionalCAs | The chart is fork-owned — prefer ours, cherry-pick upstream chart improvements deliberately |
| `go.mod` / `go.sum` | fork adds `github.com/robfig/cron/v3` (token refresher) | Keep the require line on conflict; run `go mod tidy` after |

## Semantic-conflict watchlist (no git conflict — the dangerous ones)

These break fork behavior **without** producing a merge conflict. Check each after every sync:

| Risk | Why it's invisible | Detection |
|------|--------------------|-----------|
| **New upstream migration not idempotent** | A brand-new file in `migrations/tables|data/` is not a conflict, but the fork's invariant is that all migrations are idempotent. | `git diff --name-only --diff-filter=A upstream/main...HEAD -- 'server/datastore/mysql/migrations/tables/*' 'server/datastore/mysql/migrations/data/*'`, then patch each to `CREATE TABLE IF NOT EXISTS` / `INSERT IGNORE` / `DROP TABLE IF EXISTS` and add `// Idempotent migration.` |
| **`schema.sql` lost the OpenFrame tables** | `make dump-test-schema` regenerates `schema.sql` from upstream migrations only — it never contains `policy_hosts`/`query_hosts`/`migration_status_openframe`. This is expected. | Don't "fix" it. Fork datastore tests must call `ds.MigrateOpenframe(ctx)` first (see `migrations_openframe_test.go`). |
| **Redis pool refactor un-wires the prefix** | If upstream rewrites pool construction, `newPrefixedConn` may stop wrapping `Get()` — keys silently go unprefixed → **cross-tenant data leak**. The key-prefix unit test still passes (it tests the wrapper, not the wiring). | After any `redis.go` conflict: confirm `standalonePool.Get()` and `clusterPool.Get()` both return `newPrefixedConn(...)`, and `ConfigureDoer`/`EachNode` unwrap/re-wrap. |
| **Query-planner change bypasses the host filter** | If upstream rewrites the policy/query SQL builders, the `IsOpenframeMode()` EXISTS filter may end up on a dead code path. | Re-read `PolicyQueriesForHost` / `ListScheduledQueriesForAgents` and confirm the `policy_hosts`/`query_hosts` filter is still applied. |
| **Orbit/osquery invocation refactor** | Upstream may change how osquery is launched, dropping the `--openframe-*` flags or the custom binary path. | Re-read the `osquery.NewRunner` options and the `openframe-mode` osqueryd-path branch in `orbit.go`. |
| **Upstream ungates an agent feature OpenFrame relies on being off** | Upstream moves a feature out of a flag guard so it runs unconditionally (e.g. the Fleet Desktop device-token check `trw.StartRotation()` was moved out of `if --fleet-desktop` to "keep the identifier valid for refetch-host/device-auth"). OpenFrame agents run **without** those flags, so the feature silently turns on and hits paths that aren't gatewayed → a fleet-wide 401 storm on `/api/.../fleet/device/{token}/ping`. No git conflict; the fork never edited that line. | After an `orbit.go` sync, confirm the device-token check + `trw.StartRotation()` are still wrapped in `if !c.Bool("openframe-mode")` (`OPENFRAME(agent-openframe-mode)`). More generally, watch for upstream removing flag guards around device-token / MDM / Fleet Desktop features. |
| **Setup endpoint side effects return** | Upstream attaches behavior to `POST /setup` that runs with admin rights right after our programmatic setup (the GitOps starter library nulled `agent_options` this way — fork PR #54 era). No conflict: the hook lives in upstream-only wiring. | After a sync, confirm `cmd/fleet/serve.go` still routes openframe mode to the no-op starter-library callback (`OPENFRAME(setup-no-starter-library)`), and skim `endpoint_setup.go` for new post-setup applies. |
| **Mocks out of sync** | `datastore_mock.go` / `service_mock.go` are generated; an interface change makes them stale (compile error if lucky, wrong behavior if not). | `make generate-mock` after any change to `server/fleet/datastore.go` or `service.go`. |

## Mandatory post-merge steps

1. **Idempotency sweep** — make every newly added/changed upstream migration idempotent (watchlist row 1).
2. **Regenerate mocks** if `server/fleet/datastore.go` or `service.go` changed: `make generate-mock`.
3. **`go mod tidy`** if `go.mod` conflicted.
4. **Verify**:
   ```bash
   make openframe-verify                 # build + vet + markers + key-prefix tests (no Docker)
   MYSQL_TEST=1 make openframe-verify     # + MySQL-backed fork tests (needs Docker)
   ```
   See [openframe/scripts/verify.sh](../scripts/verify.sh).
5. (Optional, deepest) run the live E2E against a dev server:
   `FLEET_URL=http://localhost:8080 bash openframe/scripts/test_host_assignments.sh`
   (see [test_host_assignments.md](test_host_assignments.md) and [local-setup.md](local-setup.md)).

## Notes

- **`git blame` is NOT reliable for fork-vs-upstream here.** The squash re-sync
  commit (`01a6bb8a63`, "CI to build Fleet") re-imported whole upstream trees, so
  blame attributes thousands of *upstream* lines to a fork committer. To decide
  whether code is fork or upstream, compare content:
  `git show upstream/main:<file>` (or `git show v4.81.2:<file>`) — if the symbol
  exists there, it's upstream and must **not** be marked. The `OPENFRAME` markers +
  `make openframe-verify`'s coverage check are the source of truth, not blame.
- `.coderabbit.yaml` enforces a review rule that host-scoped SQL keeps its `WHERE`
  filter — directly relevant when re-applying the `IsOpenframeMode()` host filters.
- Optional: `git config rerere.enabled true` locally so repeated resolutions are
  remembered across sync attempts (modest help given the squash-rebase history).
- Test coverage gaps worth filling over time (not blockers): datastore CRUD tests
  for `policy_hosts`/`query_hosts` and a `CleanupExpiredQueryResults` test — both
  require `MYSQL_TEST=1` + Docker and a `ds.MigrateOpenframe(ctx)` call first.
