# Fleet Helm Chart (OpenFrame fork)

## Overview

The fork maintains its own build of the Fleet Helm chart under
[`charts/fleet/`](../../charts/fleet/) and publishes it as an OCI artifact from the
fork's release pipeline (see [ci-cd-release-pipeline.md](ci-cd-release-pipeline.md)).
The chart is adapted for OpenFrame's multi-tenant, GitOps-driven deployment model:
configuration is externalized into ConfigMaps/Secrets, the migration job is
restructured, OpenFrame mode and the per-tenant Redis prefix are wired in.

- Chart name: `fleet`, version `v6.8.4`, appVersion `v4.81.2`
  ([Chart.yaml](../../charts/fleet/Chart.yaml)).
- Subcharts: Bitnami MySQL `9.12.5` (`mysql.enabled`), Bitnami Redis `18.1.6`
  (`redis.enabled`).

> Source commits: `Migrate helm chart to our own Build` (1387fa6f), `Update
> charts` (9ce084f6), `Refactor Fleet Helm chart` (b318417d), `Fix mysql.enabled
> at Pre Sync` (8223125e), `Enable helm hooks for fleet-migration job` (f3e636d5),
> `Remove Chart Pre Hooks` (ba488747), `Remove chart ttlSecondsAfterFinished`
> (7178785e), `per-tenant Redis key prefix ... change in configmap` (719eab41).

## OpenFrame mode flag

[`values.yaml`](../../charts/fleet/values.yaml) exposes `fleet.setup.openframeMode`
(default `"0"`), rendered into the deployment as the server-side feature flag
([deployment.yaml](../../charts/fleet/templates/deployment.yaml)):

```yaml
- name: FLEET_OPENFRAME_MODE
  value: {{ .Values.fleet.setup.openframeMode | quote }}
```

Set it to `"1"` for tenant deployments. This is the switch behind host
assignments and the query-results TTL cleanup
(see [architecture-host-assignments.md](architecture-host-assignments.md),
[query-results-ttl-cleanup.md](query-results-ttl-cleanup.md)).

## MySQL-multitenancy feature envs (`mysql-multitenancy`)

`values.yaml` exposes a `fleet.openframe.multiTenancy` block — the chart-side wiring of the
platform property `openframe.fleet.multi-tenancy.enabled` (see
[process-team-pin.md](process-team-pin.md) for the flag/mode semantics):

```yaml
fleet:
  openframe:
    multiTenancy:
      enabled: false          # → FLEET_OPENFRAME_MULTI_TENANCY_ENABLED (deployment + migration Job)
      tenantUuid: ""          # pinned mode: static FLEET_OPENFRAME_TENANT_UUID
      existingConfigMap: ""   # pinned mode: read the UUID from a ConfigMap instead (wins over tenantUuid)
      tenantUuidKey: ""       # key within existingConfigMap (default FLEET_OPENFRAME_TENANT_UUID)
      teamId: ""              # escape hatch: direct FLEET_OPENFRAME_TEAM_ID pin (prefer tenantUuid)
```

Rendered env vars ([deployment.yaml](../../charts/fleet/templates/deployment.yaml)):
`FLEET_OPENFRAME_MULTI_TENANCY_ENABLED` is always emitted (`"false"` by default — pre-feature
fork behavior). `FLEET_OPENFRAME_TENANT_UUID` is **always read via `configMapKeyRef`** — there is
no inline value or branching in the Deployment (same pattern as the DB/cache config) — from one of:
- `existingConfigMap` set → the operator's own ConfigMap;
- `existingConfigMap` unset → the chart-managed **`fleet-openframe-tenant`** ConfigMap that
  [configmap.yaml](../../charts/fleet/templates/configmap.yaml) creates, holding `tenantUuid`.

In shared per-request mode (`enabled: true`, no `tenantUuid`) and flag-off, the value is empty and
Fleet treats `""` as unset — so no pin. The **flag is also emitted into
[job-migration.yaml](../../charts/fleet/templates/job-migration.yaml)** so `fleet prepare db`
takes the `GET_LOCK` serialization on a shared MySQL.

Downstream (openframe-saas-tenant) pinned-mode wiring can reuse the existing per-namespace
`tenant` ConfigMap, whose `TENANT_ID` key already holds the tenant UUID (it is the same key the
Redis prefix reads):

```yaml
fleetmdm:
  fleet:
    openframe:
      multiTenancy:
        enabled: true
        existingConfigMap: "tenant"
        tenantUuidKey: "TENANT_ID"
```

## Externalized configuration (ConfigMaps + Secrets)

Where upstream takes Redis/MySQL connection details as plain Helm values, the fork
adds [`templates/configmap.yaml`](../../charts/fleet/templates/configmap.yaml) and
[`templates/secret.yaml`](../../charts/fleet/templates/secret.yaml) and lets an
operator point the chart at **externally managed** ConfigMaps/Secrets. This is the
GitOps-friendly pattern: the tenant's control plane owns the ConfigMaps, the chart
just references them.

| Concern | values.yaml block | `existingConfigMap` / `existingSecret` override | Generated object (when no override) |
|---------|-------------------|--------------------------------------------------|-------------------------------------|
| Database | `database.*` | `database.existingConfigMap`, `database.existingSecret` | `fleet-database` ConfigMap (host/port/db/user) + Secret (password) |

`database.existingSecret` covers **both** MySQL secrets: the password (`database.passwordKey`) is read
from it as an env var, and — when `database.tls.enabled` — the same Secret is mounted at
`/secrets/mysql`, where `database.tls.caCertKey` names the server CA file. It therefore takes
precedence over the legacy `database.secretName`, which still applies when `existingSecret` is unset.
The mount is whole-Secret (no `items:` projection), so every key in it surfaces as a file in the Fleet
container; keep unrelated material out of that Secret if that matters to you.
| Cache (Redis) | `cache.*` | `cache.existingConfigMap` | `fleet-cache` ConfigMap (address, key prefix) |
| Tenant UUID (multi-tenancy) | `fleet.openframe.multiTenancy.*` | `fleet.openframe.multiTenancy.existingConfigMap` | `fleet-openframe-tenant` ConfigMap (`FLEET_OPENFRAME_TENANT_UUID` = `tenantUuid`, empty in shared mode) |
| Admin setup | `fleet.setup.*` | `fleet.setup.adminPassword.existingSecret` | `fleet-setup` Secret (`FLEET_SETUP_ADMIN_PASSWORD`) |

Keys within the referenced ConfigMap are themselves configurable
(`database.hostKey`, `database.portKey`, `cache.addressKey`, …), so the chart can
adapt to whatever key names the tenant control plane already uses.

## Per-tenant Redis key prefix

`cache.keyPrefixKey` wires the [Redis key prefix](redis-key-prefix.md) into both
the main deployment and the vuln-processing cron. When non-empty, the chart
renders:

```yaml
{{- if .Values.cache.keyPrefixKey }}
- name: FLEET_REDIS_KEY_PREFIX
  valueFrom:
    configMapKeyRef:
      name: {{ default "fleet-cache" .Values.cache.existingConfigMap }}
      key: {{ .Values.cache.keyPrefixKey }}
{{- end }}
```

It reads from the cache ConfigMap, so the prefix (typically the tenant ID) is
managed alongside the Redis address. See
[redis-key-prefix.md](redis-key-prefix.md) for what the prefix does in the server.

## Migration job

[`templates/job-migration.yaml`](../../charts/fleet/templates/job-migration.yaml)
runs `/usr/bin/fleet prepare db --no-prompt` (which executes all three migration
pipelines, including the OpenFrame one — see [migrations.md](migrations.md)).

Current behavior:

- **Gated by `fleet.autoApplySQLMigrations`** (default `true`). When false, no
  migration Job is rendered (the control plane applies migrations itself).
- **`waitForMysql` init container** (`fleet.waitForMysql.enabled`, image
  `fleet.waitForMysql.image`) — a `nc`-based wait loop that blocks until the MySQL
  host/port (read from the database ConfigMap) is reachable, so the job does not
  race the database.
- **No Helm hooks** and **no `ttlSecondsAfterFinished`** — the job runs as an
  ordinary Job and idempotent migrations make re-runs safe.

### Why the migration job evolved (commit history)

The hook strategy was deliberately walked back:

1. `Enable helm hooks for fleet-migration job` (f3e636d5) added
   `pre-install,pre-upgrade` hooks (with `hook-weight` / `hook-delete-policy`),
   gated on external MySQL (`not .Values.mysql.enabled`).
2. `Fix mysql.enabled at Pre Sync` (8223125e) corrected that gating.
3. `Remove Chart Pre Hooks` (ba488747) **removed the hooks entirely** in favor of
   the `waitForMysql` init container — hooks ran outside the normal release
   ordering and were harder to reason about than an explicit readiness wait.
4. `Remove chart ttlSecondsAfterFinished` (7178785e) dropped the Job's
   `ttlSecondsAfterFinished: 100` so cluster-level TTL/GC policy governs cleanup.

This walk-back depends on the [migration idempotency](migrations.md) work: because
re-running `prepare db` is a no-op, the job no longer needs hook-managed
exactly-once semantics.

### Making it an Argo CD PreSync hook

Argo CD hooks are not Helm hooks — an operator opts in with
`fleet.migrationJobAnnotations: {argocd.argoproj.io/hook: PreSync}`. The trap is that
PreSync runs *before* the Sync phase, so every Sync-phase resource is unreachable
from the hook. The Job hardcodes `serviceAccountName: fleet` and resolves the
database ConfigMap and Secret at pod-create time, so all three must be pulled into
the same phase or the sync deadlocks: the Job cannot start, so the Sync phase never
runs, so the resources it waits on are never created.

| Dependency | Knob | Rendered by |
|------------|------|-------------|
| ServiceAccount `fleet` | `serviceAccountAnnotations` | [sa.yaml](../../charts/fleet/templates/sa.yaml) |
| ConfigMap `fleet-database` (host/port/db/user) | `database.configMapAnnotations` | [configmap.yaml](../../charts/fleet/templates/configmap.yaml) |
| Secret `fleet-database` (password) | `database.secretAnnotations` | [secret.yaml](../../charts/fleet/templates/secret.yaml) |

When `database.existingConfigMap` / `database.existingSecret` point at externally
managed objects, the operator annotates those instead — the knobs above only reach
the chart-managed ones.

Give the three dependencies `argocd.argoproj.io/hook-delete-policy: HookFailed`, not
the `BeforeHookCreation` default: the Deployment reads the same ConfigMap and Secret,
and BeforeHookCreation deletes and recreates them on every sync. Put the Job a wave
behind them (`argocd.argoproj.io/sync-wave: "1"`) so ordering does not rely on Argo's
intra-wave kind ordering.

## Other fork additions

| Feature | values.yaml | Notes |
|---------|-------------|-------|
| Deployment annotations | `deploymentAnnotations` | Arbitrary annotations merged onto the Fleet Deployment metadata (e.g. for ArgoCD / reloader). |
| Additional CA certs | `fleet.additionalCAs.*` | Init-container injection of CA bundles from named ConfigMaps/Secrets, for private PKI. |
| Dedicated vuln processing | `vulnProcessing.dedicated`, `vulnProcessing.schedule` | When `true`, runs vulnerability processing as a separate CronJob ([vulnprocessing/cronjob.yaml](../../charts/fleet/templates/vulnprocessing/cronjob.yaml)) and disables it in the main deployment. |
| Vuln feed persistence | `vulnProcessing.persistence.*`, `vulnProcessing.staggerSchedule`, `vulnProcessing.staggerKey` | See [below](#vulnerability-feed-persistence-vuln-persistence). |
| Probe split | `fleet.probes.*` | See [below](#probes). |

## Probes

All three probes keep upstream's endpoint, `/healthz`, which checks MySQL and Redis
(`healthCheckers` in [cmd/fleet/serve.go](../../cmd/fleet/serve.go)). What the fork adds
is timings, which upstream leaves unset, and a retry on the first Redis dial.

| Probe | Budget | `fleet.probes.*` |
|-------|--------|------------------|
| `startupProbe` | 6 min | `25 × 15s` |
| `livenessProbe` | 2 min | `9 × 15s` |
| `readinessProbe` | 1 min | `5 × 15s` |

Without timings every probe runs on the Kubernetes defaults, which kill a container 30s
after the first failure. That is far too short for Fleet: it waits for MySQL on its own
for about 105s (15 attempts sleeping 0,1,…,14s, see
[common.go](../../server/platform/mysql/common.go) and `defaultMaxAttempts` in
[config.go](../../server/datastore/mysql/config.go)) and does not listen on `listenPort`
while it waits.

That is what the `startupProbe` is for. Liveness and readiness do not run until it
succeeds, so the boot gets its own budget and the other two can stay short.

The three budgets are ordered by how expensive the action is. Readiness is reversible and
so the shortest; liveness throws away a warm process; startup supervises a boot that is
legitimately slow. The minute between readiness and liveness is deliberate: it is the
window where a pod takes no traffic but can still recover on its own.

### Why `cache.connectRetryAttempts` is set

A probe budget only means something while the process is alive. Upstream defaults
`redis.connect_retry_attempts` to 0 ([config.go](../../server/config/config.go)), a single
dial, so a boot during a Redis outage ended in `initFatal` within seconds and the pod
crashlooped for the length of the outage with no probe ever consulted.

The fork sets it to 20, which turns that dial into an exponential backoff (`backoff/v4`
defaults, capped at 60s per attempt). The process now stays up retrying and the 6 min
startup budget becomes the real ceiling.

One caveat: the retry only covers errors Go reports as temporary or as timeouts, while a
plain `connection refused` is `backoff.Permanent` and still exits at once
([redis.go](../../server/datastore/redis/redis.go)).

### Notes

`timeoutSeconds` is 10s everywhere, up from the 1s default, because `/healthz` does real
MySQL and Redis round trips. It caps one attempt and sits inside the period rather than
adding to it, so keep it under `periodSeconds`.

The first probe runs at t=0, so the Nth failure lands at `(N - 1) × periodSeconds`. A
Cloud SQL HA failover takes about 60s and therefore passes without a restart and without
leaving the Service ([Cloud SQL HA](https://cloud.google.com/sql/docs/mysql/high-availability));
on upstream's 30s it would not have.

If the endpoint ever needs to change rather than the timings, `health.Handler`
([health.go](../../server/health/health.go)) takes a `?check=<name>` filter, and `/version`
([version.go](../../server/version/version.go)) is a static struct with no dependencies at
all.

## Vulnerability feed persistence (`vuln-persistence`)

Fleet downloads ~800MB of vulnerability feeds (NVD, CPE, OSV, OVAL, MSRC, …) into
`FLEET_VULNERABILITIES_DATABASES_PATH` (`/tmp/vuln`). Upstream mounts `/tmp` as an
`emptyDir`, so every pod replacement re-downloads the full set, even though Fleet's
sync code is delta-aware (per-file mtime / SHA-256 / `.meta` comparisons that skip
unchanged artifacts when the directory survives a restart).

The fork adds first-class persistence for the **dedicated cron** mode:

```yaml
vulnProcessing:
  dedicated: true        # required — persistence only wires into the CronJob
  staggerSchedule: true  # hourly at a minute derived from staggerKey
  staggerKey: ""         # hash input; empty = the release namespace
  persistence:
    enabled: true
    size: 5Gi
    # storageClassName: ""   # cluster default
    # existingClaim: ""      # reuse a pre-created PVC
```

What this renders:

- [vulnprocessing/pvc.yaml](../../charts/fleet/templates/vulnprocessing/pvc.yaml)
  — a `ReadWriteOnce` PVC `fleet-vulnprocessing` (skipped when `existingClaim` is
  set). RWO is safe because the CronJob runs with `concurrencyPolicy: Forbid`, so
  at most one pod attaches it at a time.
- [vulnprocessing/bind-job.yaml](../../charts/fleet/templates/vulnprocessing/bind-job.yaml)
  — a one-shot Job (`persistence.bindJob.enabled`, default on) that mounts the
  claim at install, using `bindJob.image` (default `busybox:1.36`, like
  `fleet.waitForMysql` — any image that can exit 0 works). With a
  `WaitForFirstConsumer` storage class the claim
  otherwise stays Pending until the first cron tick — up to an hour during which
  an Argo CD sync operation sits in `Running` on
  "waiting for healthy state of /PersistentVolumeClaim/fleet-vulnprocessing"
  (`ignore-healthcheck` only fixes aggregated app health, not operation gating —
  argo-cd issue #22940). Argo CD users should also set
  `bindJob.annotations: {argocd.argoproj.io/sync-options: Replace=true,Force=true}`
  so later image-tag changes don't hit the Job-spec-immutable apply error. The
  Job must stay a plain Sync-phase resource — PostSync would deadlock on the very
  PVC health it unblocks, PreSync runs before the claim exists.
- The CronJob mounts the PVC at `/tmp/vuln` (nested inside the `/tmp` emptyDir)
  and gets a pod-level `securityContext` from `fleet.podSecurityContext`, whose
  `fsGroup` (+ `fsGroupChangePolicy: OnRootMismatch`) is what lets it write —
  without it uid 3333 cannot write a fresh root-owned PVC.
- With `staggerSchedule: true`, `vulnProcessing.schedule` is ignored and the cron
  fires hourly at `adler32sum(staggerKey) mod 60`, so releases sharing a feed
  mirror — or a database — don't all fire at `:00`. `staggerKey` defaults to the
  release namespace, which only spreads anything when the releases actually sit
  in differently named namespaces. One release per cluster in a fixed namespace
  (`platform`) derives the identical minute everywhere, so set `staggerKey` to a
  per-cluster value; OpenFrame passes `platform.id`.

Combined effect: server pods never download feeds (redeploys are free), and each
hourly job only fetches deltas (EPSS/CISA and, when Amazon Linux hosts exist,
goval-dictionary sqlite files are always re-fetched — they are the small part).
Do **not** enable `dedicated` without `persistence` on a busy schedule: each job
pod would start with an empty `/tmp` and re-download the full set every run.

Caveat: `cpe.sqlite` is versionless on disk (schema lives in the `cpe_2` table
name) and the sync skips it when its mtime is "today", so after a Fleet upgrade a
persisted copy can lag by up to a day. Deleting the PVC (the data is a disposable
cache) forces a full re-sync.

Server-side guard (`server/vulnerabilities/nvd/cpe.go`): when a feed sync fails
but the scan continues, the CPE translation phase opens the missing
`cpe.sqlite` with the sqlite driver, which creates an **empty 0-byte file**. On
an emptyDir that artifact dies with the pod, but on a persisted volume its
fresh mtime satisfies both freshness gates in `DownloadCPEDBFromGithub` and the
real download is never retried — vuln processing wedges until the next
`fleetdm/nvd` release (~24h) while the Job reports Success. The fork adds a
`stat.Size() == 0 → treat as absent` case ahead of the mtime gate, so the next
run after a transient failure (rate limit, GitHub outage) re-downloads
immediately. The write path is temp-file + atomic rename, so replacing the
empty file is safe. A non-empty corrupt file would still pass, but that cannot
be produced by this code path — only by disk corruption.

## Operator quick reference

Tenant-style install (external DB + shared Redis + OpenFrame mode):

```bash
helm upgrade --install fleet oci://ghcr.io/flamingo-stack/fleetmdm/helm-charts/fleet \
  --set fleet.setup.openframeMode="1" \
  --set database.existingConfigMap=fleet-db-config \
  --set database.existingSecret=fleet-db-secret \
  --set cache.existingConfigMap=fleet-cache \
  --set cache.keyPrefixKey=FLEET_REDIS_KEY_PREFIX \
  --set fleet.autoApplySQLMigrations=true \
  --set fleet.waitForMysql.enabled=true
```

> The OCI chart coordinates above match the release pipeline in
> [ci-cd-release-pipeline.md](ci-cd-release-pipeline.md); confirm the exact
> registry path for your environment.

## Files changed

| File | Purpose |
|------|---------|
| `charts/fleet/values.yaml` | OpenFrame mode, externalized DB/cache/setup config, `cache.keyPrefixKey`, `cache.connectRetryAttempts`, `waitForMysql`, `probes`, `additionalCAs`, `vulnProcessing`, `deploymentAnnotations` |
| `charts/fleet/templates/configmap.yaml` | **New** — generated DB/cache ConfigMaps |
| `charts/fleet/templates/secret.yaml` | **New** — generated DB password / admin-setup Secrets |
| `charts/fleet/templates/deployment.yaml` | `FLEET_OPENFRAME_MODE`, `FLEET_OPENFRAME_MULTI_TENANCY_ENABLED` / `FLEET_OPENFRAME_TENANT_UUID` / `FLEET_OPENFRAME_TEAM_ID`, `FLEET_REDIS_KEY_PREFIX`, ConfigMap/Secret refs, annotations, CA init container, probe split |
| `charts/fleet/templates/job-migration.yaml` | `waitForMysql` init container, hook removal, TTL removal |
| `charts/fleet/templates/vulnprocessing/cronjob.yaml` | Dedicated vuln-processing cron + `FLEET_REDIS_KEY_PREFIX`, feed-cache PVC mount, fsGroup, schedule stagger (moved from `templates/cron-vulnprocessing.yaml`) |
| `charts/fleet/templates/vulnprocessing/pvc.yaml` | **New** — PVC persisting the vulnerability feed cache across cron runs |
| `charts/fleet/templates/vulnprocessing/bind-job.yaml` | **New** — one-shot Job binding the WFFC feed PVC at install |
| `charts/fleet/Chart.yaml` | Fork chart version, MySQL/Redis subchart pins |

## Rebase notes

The chart is largely fork-owned, so upstream chart changes rarely apply cleanly —
treat the fork chart as the source of truth and cherry-pick upstream improvements
deliberately. The `charts/fleet/README.md` is still upstream-oriented and does not
yet describe the externalized-config / multi-tenant patterns above.
