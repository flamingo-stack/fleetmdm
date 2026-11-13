# Fleet MySQL multi-tenancy (OpenFrame)

Single-document reference for the OpenFrame shared-database multi-tenancy feature in this fork.
It lets **one Fleet server + one shared MySQL serve many tenants**, where **tenant = Fleet team
(`team_id`)**, with row-level isolation enforced at the datastore. Everything is behind a master
flag; **with the flag off, Fleet behaves exactly as the fork did before this feature** (schema
change aside — see [Backward compatibility](#backward-compatibility)).

## The master flag

`FLEET_OPENFRAME_MULTI_TENANCY_ENABLED` (maps the platform property
`openframe.fleet.multi-tenancy.enabled`). Parsed once, cached. Three states:

| State | Meaning |
|---|---|
| **off** (default) | Pre-feature fork behavior. Every fence is inert; a stray `FLEET_OPENFRAME_TEAM_ID` does **not** pin the process. |
| **on + process pin** (`FLEET_OPENFRAME_TENANT_UUID`, or `FLEET_OPENFRAME_TEAM_ID`) | **Pinned mode** — one Fleet per tenant on the shared DB; the whole process is scoped to one team. |
| **on + no pin** | **Shared mode** — one Fleet per cluster; **every request is pinned individually** (X-Tenant-Id header / host team / enroll-secret team), fail-closed. |

`ValidateOpenframeMultitenancy()` (called in `cmd/fleet/serve.go`) refuses to boot only on a
`FLEET_OPENFRAME_TEAM_ID` that is set-but-unparsable (a typo must not silently change the mode).
Key API in `server/fleet/openframe.go`: `IsOpenframeMultitenancy()`, `IsOpenframeSharedMode()`,
`OpenframeTeamID(ctx)`, `NewOpenframeTeamContext(ctx, teamID)`, `SetOpenframeTeamID(teamID)`.

## Identity: the team pin and the UUID→team bridge

`OpenframeTeamID(ctx)` is the single source of the current tenant team, in precedence order:
1. **ctx value** — per-request pin (shared-mode middleware, agent pins, tests);
2. **process pin** — team resolved from `FLEET_OPENFRAME_TENANT_UUID` at startup;
3. **env fallback** — `FLEET_OPENFRAME_TEAM_ID` (inert unless the flag is on).
Returns `ok=false` when none yields a non-zero team; callers must then assume no tenant scope
(fences no-op).

**`EnsureOpenframeTeamID(ctx, tenantUUID)`** (`server/datastore/mysql/openframe.go`) bridges the
platform's UUID tenant identity to Fleet's integer `team_id`: it resolves-or-creates the team keyed
by `teams.openframe_tenant_uuid` (unique; race-safe via insert-lose-reselect). A **newly created
team is seeded, in the same transaction, with one random team-scoped enroll secret** (same default
as EE team creation) — without it a fresh tenant could never enroll an agent (the pinned
`GET /spec/enroll_secret` would be empty). The resolve path never touches an existing team's
secrets. In pinned mode `serve.go` calls this at startup and `SetOpenframeTeamID`s the result.

## Schema migrations (`server/datastore/mysql/migrations/openframe/`)

Applied by `fleet prepare db`. Idempotent (`columnExists`/`indexExists` guards).

| Migration | Change | Why |
|---|---|---|
| `20260629000001_AddTeamsOpenframeTenantUUID` | `teams.openframe_tenant_uuid CHAR(36)` + unique key | the UUID→team bridge |
| `20260626000001_ScopeHostIdentityUniqueToTeam` | `hosts` virtual col `openframe_team_key = IFNULL(team_id,0)` + `UNIQUE(osquery_host_id, openframe_team_key)`, drop global `UNIQUE(osquery_host_id)` | host identity unique **per team** — the same device can exist in two tenants |
| `20260620000001_ScopeLabelUniqueNameToTeam` | same generated-column pattern for `labels.name` | label names unique per team; built-ins stay global |
| `20260722000001_AddTeamIdToCdcTables` | nullable `team_id` on `activity_past`, `activity_host_past`, `query_results`, `policy_membership` (no index, no FK) | the Debezium CDC tables must be self-describing on a shared DB — see "CDC team stamping" below |

The `IFNULL(team_id,0)` sentinel collapses all NULL-team rows onto key `0` (team ids start at 1), so
**flag-off / pre-backfill the uniqueness is bit-for-bit the old global uniqueness**, and the
`labels` ODKU upsert (`ApplyLabelSpecs`) keeps working. `fleet.Label` carries an ignored
`OpenframeTeamKey` field so `SELECT l.*` scans don't break.

## Datastore fences (the tenant boundary)

The pattern everywhere: `if teamID, ok := fleet.OpenframeTeamID(ctx); ok { … AND team_id = ? … }`
— a **no-op when unpinned**. On the shared DB this is the real isolation (the Fleet team was only a
role-authz grouping upstream; here it is a hard boundary regardless of token role). Coverage:

- **hosts** — `ListHosts`/`CountHosts` (in `applyHostFilters`), by-id `Host`/`HostLite`, bulk delete
  (`filterHostIDsByTeam`), enrollment matcher (`matchHostDuringEnrollment`), `AddHostsToTeam`, and
  the minor getters `HostLiteByIdentifier`/`HostLiteByID`, `ListHostsLiteByIDs`, `HostIDsByIdentifier`.
  Deliberately **unfenced**: `HostByUUID` (pre-auth iDevice identity lookup — no pin yet).
- **policies** — list/count/by-id (`Policy`, `PolicyLite`, `PoliciesByID`), create→pinned, save
  (verify-on-primary), delete (filter foreign ids); service-layer `DeleteGlobalPolicies` and
  `modifyPolicy` (the global modify endpoint) treat an own-pinned-team policy as global — creation
  re-homes "global" policies to the pinned team, so the tenant UI's global endpoints must accept
  them back.
- **queries** — list/by-id/name, create→pinned, save-verify, delete, `ApplyQueries` re-home.
- **enroll_secrets** (`app_configs.go`) — `GetEnrollSecrets`/`ApplyEnrollSecrets` force `teamID =
  pinned`; `VerifyEnrollSecret` only accepts a secret whose `team_id = pinned` (agent boundary).
- **live-query targets** (`targets.go`) — `HostIDsInTargets`/`CountHostsInTargets` scoped.
- **live-query campaigns** (`campaigns.go`) — `DistributedQueryCampaign`/
  `DistributedQueryCampaignTargetIDs` fenced via the campaign's query team (`EXISTS` against
  `queries.team_id`; pinned creation re-homes ad-hoc query rows, so every campaign's query carries
  its tenant's team). Upstream's only guard is `campaign.UserID`, which is useless on the shared
  Fleet where every tenant operates as the same Admin user.
- **host-assignments** (`policy_hosts`/`query_hosts`) — parent verified in team + foreign host ids
  dropped (pre-existing `OPENFRAME(host-assignments)` feature, extended here).
- **teams** — `TeamLite`/`ListTeams` read fence.
- **app_config** cache (`cached_mysql.go`) — cache key includes the pinned team so one tenant's
  config is never served to another; unpinned keeps the constant key.
- **app_config** reads (`app_configs.go`) — tenant rows at `id = team id`; `id = 1` is the
  *instance* row, read by every unpinned path (crons, workers, boot). Unpinned multitenant reads
  select `WHERE id = 1` instead of upstream's bare `LIMIT 1`, which returns an arbitrary row. 
  A missing row falls back to `OpenframeDefaultAppConfig()` for any multitenant read (nothing else 
  seeds `id = 1`; a zero-value config would disable software inventory and historical data instance-wide).
  Non-multitenant keeps the upstream statement and fallback byte-identical. Migration
  `20260831000001` seeds/repairs the row and reserves team id 1.
- **MDM enabled flags** (`app_configs.go`) — `mdm.enabled_and_configured` and
  `mdm.android_enabled_and_configured` describe the instance (one APNs certificate, one Android
  enterprise), so in shared mode they live on the instance row: a pinned read takes them from
  `id = 1`. Only the flows that change a flag write it there, one flag per save, with `JSON_SET`:
  the APNs upload and delete (`server/service/mdm.go`) and `SetAndroidEnabledAndConfigured` mark
  their save with `fleet.NewOpenframeInstanceMDMFlagContext`. Any other pinned save leaves the
  instance row alone, so a stale copy of the config cannot turn a flag back. An APNs upload by one
  tenant turns MDM on for every tenant and for the unpinned enrollment routes, with no restart.
  The tenant row keeps its own stale copy, which is never read. A pinned request that needs the
  instance row itself marks its context with `fleet.OpenframeInstanceConfigContext` (shared mode
  only): `fleet.OpenframeAppConfigTeamID` then reports it unpinned to the app-config row selection
  and to the app-config cache key, and to nothing else. The APNs CSR (org name), the Android
  signup (enabled flag and callback URL) and the Android enterprise check
  (`verifyEnterpriseExistsWithGoogle`, which the signup runs before its 409 check) use it. The
  proxy finds the enterprise by the URL it was created with, so a check with a tenant row's URL
  would report the instance's enterprise as deleted and wipe it for every tenant; with no instance
  URL at all the check fails instead of treating the enterprise as deleted. The marker is for reads only: `SaveAppConfig`
  (and so `NewAppConfig`) refuses a marked context, so a tenant request can never write the
  instance row through it, and the cache never stores a tenant's object under the instance key. `mdm.apple_bm_enabled_and_configured`
  stays on the tenant row. A process-pinned Fleet keeps all flags on its tenant row.
- **software titles + vulnerabilities** (`software_titles.go`, `vulnerabilities.go`) —
  `ListSoftwareTitles`, `SoftwareTitleByID`, `ListVulnerabilities`, `CountVulnerabilities`,
  `Vulnerability`, `SoftwareByCVE`, `OSVersionsByCVE` replace the caller's team scope with the
  pinned team (`openframeTeamScope`), so titles, versions, host counts and CVEs come from the
  per-team rows of `software_titles_host_counts` / `software_host_counts` /
  `vulnerability_host_counts` only. Applied in the datastore, not via the `team_id` param, because
  the service gates `team_id` on titles behind a Premium license. The OpenFrame api reads these
  server-side for Software Management, so the gateway allowlist does not protect them.
  `GET /vulnerabilities`: a pinned empty page reports `counts_updated_at` as the instance's last
  host-count recalculation (`VulnerabilityHostCountsUpdatedAt`, zero if never) instead of `now()`.
  `SoftwareByID` stays unfenced on purpose — the Office vulnerability analyzers call it.
- **host list software filters** (`server/service/hosts.go`) — with `software_version_id` /
  `software_id` / `software_title_id`, a pinned request resolves the `software` / `software_title`
  block within its team and skips the unscoped name lookups (`SoftwareLiteByID`,
  `SoftwareTitleNameForHostFilter`), so another tenant's software is never described.

## Per-request pinning

- **Control-plane / UI** — `service.WithOpenframeTenant` (`server/service/openframe_middleware.go`),
  wired in `serve.go` after `apiendpoints.Validate`. Shared mode only (returns `next` unchanged
  otherwise). Pins each `/api/**` request from the gateway-injected trusted `X-Tenant-Id`; a
  non-exempt request without it is **401 (fail closed)**. Exempt (tenant comes from host/secret):
  paths containing `/osquery/`, `/fleet/orbit/`, `/fleet/device/`, `/api/mdm/`, `/fleet/ota_enrollment`,
  plus four enrollment routes matched by method and exact path (`openframeTenantExemptRoute`):
  `GET /api/{v}/fleet/enrollment_profiles/ota` (enroll secret),
  `GET /api/{v}/fleet/android_enterprise/connect/{token}` (signup token),
  `GET /api/{v}/fleet/android_enterprise/enrollment_token` (enroll secret) and
  `POST /api/{v}/fleet/android_enterprise/pubsub` (Pub/Sub token). They run unpinned, read the
  instance config row (org name, MDM URL, the Android enabled flag) and put the device into the
  team of the enroll secret it enrolled with.
- **Agent plane** — `openframePinHostTeam` pins from the authenticated `host.team_id` in
  `authenticatedHost`/`authenticatedOrbitHost`/`authenticatedDevice` (`endpoint_middleware.go`) and
  the osquery header pre-auth paths (`osquery_header_auth.go`); fail-closed on a team-less host.
- **Enrollment** — after `VerifyEnrollSecret`, `osquery.go`/`orbit.go` pin from `secret.TeamID`
  (reject if the secret has no team). The host row is then created carrying that `team_id`.
  Android enrollment (`server/mdm/android/service/pubsub.go`) takes the team from the secret
  embedded in the Google enrollment token. Upstream then restores the device's last team, found by
  its `enterprise_specific_id`; with one Android enterprise per instance that id is the same in
  every tenant, so a wiped device re-enrolled by another tenant would land in the first tenant's
  team. In shared mode the secret's team wins (`openframeAndroidEnrollTeam`). When the device's
  host row still exists in another tenant, shared mode deletes that host and enrolls the device as
  a new host (`openframeAndroidReenrollAsNewHost`), so the previous tenant's host history does not
  move with it; the `android_devices` row is kept and reused, as upstream does after a host
  deletion. The previous tenant loses the host as soon as the device enrolls elsewhere.
  The deletion is logged with the host id and both team ids, and it uses
  `fleet.NewOpenframeHostTenantChangeContext`, so `deleteHosts` also drops the host's
  `host_mdm_idp_accounts` row, which is keyed by the device UUID and would otherwise show the
  previous tenant's end user on the new host. A deletion without that marker keeps the upstream
  behaviour.
  Apple OTA enrollment (`IngestMDMAppleDeviceFromOTAEnrollment`) skips the insert when any host
  already has the device's serial, and the Authenticate check-in then matches that host, so an
  iPhone enrolled through another tenant's link would stay in the first tenant, and upstream would
  also rewrite that host's UDID and IdP link. Neither OTA nor the check-in proves possession of the
  device (see "Any tenant can obtain a device identity" below), so the fork does not move Apple
  hosts between tenants: in shared mode an OTA request is refused with 409 and logged while a host
  of another team, on any platform, has the request's serial or its UDID as `uuid` (the upstream
  ingest matches on the serial alone and would otherwise rewrite that host's `uuid`), and nothing
  is written. Once the previous
  tenant deletes the host, the upstream path enrolls the device into the new tenant.
- **Live-query results websocket** (`endpoint_campaigns.go`) — the sockjs handler rebuilds its
  context from `context.Background()`, discarding the middleware-pinned upgrade-request context,
  so it **re-pins from `session.Request().Context()`** (read once — polling transports mutate the
  session request). Fail closed: in shared mode an unpinned session is rejected. Without the
  re-pin the whole campaign stream ran unfenced and the `live_query` activity was stamped
  `team_id NULL`.

Agents send **no tenant header** — tenant identity flows in via the enroll secret and thereafter via
the host record (node key → host → team). This is by design and stronger than a header.

## Migration serialization on a shared DB

`fleet prepare db` wraps the migration sequence in a MySQL named lock
`GET_LOCK('openframe_fleet_migrations', 900s)` (`AcquireOpenframeMigrationLock` in
`server/datastore/mysql/openframe.go`, called from `cmd/fleet/prepare.go`) — **flag-gated**. With N
clusters' migration Jobs pointed at one DB, the first wins and migrates; the rest block, then find
the schema already applied and no-op. Session-scoped (auto-released if the Job dies). Flag-off runs
are untouched.

## CDC team stamping (Debezium on the shared DB)

The OpenFrame Debezium pipeline captures `activity_past`, `activity_host_past`, `query_results`
and `policy_membership`. On a shared DB one connector serves every tenant, and its SMTs are
stateless — a CDC record must therefore carry its own tenant discriminator. The fork stamps the
`team_id` column (added by `20260722000001`) at write time:

| Table | Stamp source | Where |
|---|---|---|
| `query_results` | request team pin (`fleet.OpenframeTeamID(ctx)` — the agent plane is always pinned in shared mode) | `OverwriteQueryResultRows` |
| `policy_membership` (sync) | request team pin | `RecordPolicyQueryExecutions` |
| `policy_membership` (async collector) | the row's own host, `(SELECT team_id FROM hosts WHERE id = ?)` — the cron context is never pinned | `AsyncBatchInsertPolicyMembership` |
| `activity_past` | request team pin — the only tenant signal for host-less (user/team-level) activities | `server/activity/internal/mysql/new_activity.go` |
| `activity_host_past` | the row's own host via subselect (host activities can come from unpinned crons; the host is authoritative anyway) | same |

Unpinned writes (flag off, or background crons writing host-less activities) keep the original
statement **byte-identical** and leave `team_id` NULL; the downstream consumer drops NULL-team
events (fail closed). The `teams` table (with the `openframe_tenant_uuid` bridge) is also added
to the connector's capture list platform-side, so consumers can resolve `team_id` → tenant UUID
without touching Fleet's API. Platform side of this pipeline: shared connector registration in
`openframe-saas-shared`, and shared-plane consumption in that repo's `openframe-saas-stream`
(the MeshCentral pattern — per-event tenant resolution, gated by
`openframe.fleet.multi-tenancy.enabled`).

## Helm / config wiring (`charts/fleet/`)

`values.yaml` adds `fleet.openframe.multiTenancy` (`enabled: false` default; `tenantUuid` /
`existingConfigMap`+`tenantUuidKey` / `teamId`). `deployment.yaml` injects
`FLEET_OPENFRAME_MULTI_TENANCY_ENABLED` (+ optional `FLEET_OPENFRAME_TENANT_UUID` /
`FLEET_OPENFRAME_TEAM_ID`); `job-migration.yaml` gets the flag too (so the `GET_LOCK` guard engages).

Shared mode also reads two optional envs at boot (`cmd/fleet/serve.go`, set through the chart's
`environments` map): `FLEET_OPENFRAME_INSTANCE_SERVER_URL`, the instance's public MDM address
(an http(s) URL with a valid host and port and no query, fragment, credentials or whitespace; a
trailing slash is dropped and the scheme and host are lowercased),
and `FLEET_OPENFRAME_INSTANCE_ORG_NAME`. Each set value is written to the instance row
(`server_settings.server_url`, `org_info.org_name`) in the same boot-time save that recomputes the
Apple MDM flags; an unset value leaves the row as it is, and a malformed URL stops the boot.
Every cluster on the shared database writes the same row at boot, so the values must be identical
on all of them; different values would flip the row on each restart. Set the URL before the
Android enterprise is created and do not change it afterwards: the proxy finds the enterprise by
the URL it was created with, and the `cleanup_android_enterprise` cron, checking with a new URL,
would treat it as deleted and wipe it for every tenant. Changing the URL therefore means deleting
the enterprise and signing up again (and re-enrolling Android and Apple devices, whose profiles
carry the old address). The
Android enterprise signup reads the URL from the instance row, so in shared mode it is refused
(400) while that row has no server URL with a host; before a version with this change reaches an
environment, check the instance row's `server_settings.server_url` there or set the variable.

## Backward compatibility

Flag off ⇒ pre-feature fork behavior everywhere **except the schema**: the three migrations run
unconditionally at `prepare db`, but are semantics-preserving — all rows are `team_id = NULL` →
generated key `0` → the original global uniqueness, byte-for-byte. Verified locally (fresh
branch-native DB): `prepare db` applies all three, the server boots healthy flag-off,
flag-on-pinned (team auto-created + secret seeded, `team_id=1`), and flag-on-shared. The upstream
`TestLabels`/`TestHosts` suites pass on the migrated schema with no OpenFrame env.

## Tests

MySQL-backed (`MYSQL_TEST=1`), all in `*_openframe_test.go`: enrollment isolation, host-identity
per-team, host by-id/list/identifier fences, policy/query CRUD + by-id + GitOps, enroll-secret
fence, host-assignment fence, live-query target fence, teams read fence, app-config isolation,
`EnsureOpenframeTeamID` (incl. secret seeding), delete-global-policies pin, migration pipeline,
software-title / vulnerability / CVE-detail fences, instance-wide MDM flags through `AppConfig` and
`SaveAppConfig` in shared mode, Android re-enrollment after a host deletion and the IdP link on a
tenant change, Apple OTA refused while another tenant holds the device. Shared mode is switched
on per test through an unexported context key (`openframeSharedModeCtxKey`, in the datastore and the
Android service packages), because `fleet.IsOpenframeSharedMode` is read once per process and the
MySQL tests run in parallel. Android service tests with the mock datastore cover the cross-tenant
re-enrollment.
Flag-parsing / mode-precedence unit tests in `server/fleet/openframe_test.go`. Middleware tests in
`server/service/openframe_middleware_test.go`. Harness: `make openframe-verify` (add `MYSQL_TEST=1`
+ Docker for the deep tier).

## Known deferred (latent, non-applicable to OpenFrame today)

- **Users/sessions, software versions (`/software/versions`), os_versions and activities
  read-views** — unfenced; only matters if the Fleet UI is exposed beyond the gateway allowlist and
  the OpenFrame api does not read them. Not today.
- **Custom-label creation while unpinned** → `team_id NULL` → distributed globally. OpenFrame seeds
  only built-ins and targets via host-assignments; pinned label writes are team-scoped.
- **Legacy 2017 "user packs" scheduled-query-stats join** — resolves by global pack name; packs are
  unused. New-format stats are team-scoped.
- **`IsEnrollSecretAvailable`** — intentionally unfenced (cross-team uniqueness check).
- **"Instance" means every Fleet on the shared database** — one instance row, one APNs
  certificate, one Android enterprise and one MDM URL for every cluster on that MySQL (seven on
  prod). The MDM URL on the instance row must be an address that serves all of them, not one
  cluster's domain.
- **One Android enterprise per instance** — `android_enterprises` has no `team_id`. A second
  tenant's signup is refused (409) because the instance flag is already set, but
  `DELETE /android_enterprise` from any tenant removes the instance's enterprise.
- **One APNs certificate and SCEP CA per instance** — `mdm_config_assets` has no `team_id`; a
  later APNs upload or delete changes it for every tenant. Treat APNs, ABM and the Android
  enterprise as operator actions, not tenant settings. ABM tokens are instance-wide too, but
  `apple_bm_enabled_and_configured` stays on each tenant row: only the tenant that added the token
  ever sees it, and the instance row (read by unpinned work such as the DEP sync) gets it only when
  a Fleet restart recomputes it. The OTA enrollment profile, the SCEP URL, the
  Android signup callback and Pub/Sub push URLs and the APNs CSR all take the server URL or org
  name from the instance row; set them with `FLEET_OPENFRAME_INSTANCE_SERVER_URL` and
  `FLEET_OPENFRAME_INSTANCE_ORG_NAME` (see "Helm / config wiring"). A non-empty
  `mdm.apple_server_url` on the instance row still takes precedence for Apple MDM.
- **Any tenant can obtain a device identity for the shared SCEP CA** — the SCEP challenge is
  instance-wide and is embedded in every tenant's OTA profile, and nanomdm accepts an Authenticate
  with any new certificate (`nano_cert_auth_associations` keeps the real device's certificate as
  well). Whoever holds one tenant's enrollment link can present another tenant's device serial and
  UDID at OTA and check-in, and a forged check-in resets that device's MDM enrollment. This is why
  an Apple device is never moved between tenants by enrollment (see "Enrollment"); per-tenant SCEP
  challenges or CAs would be needed to close the gap itself.
- **The OTA 409 reveals that a device is enrolled elsewhere** — whoever holds a tenant's
  enrollment link learns that a serial or UDID belongs to some other tenant (not which one).
- **Leftover table from an earlier revision** — an earlier revision of this change added
  `openframe_apple_enrollment_intents` (fork migration `20261111000002`). It was never merged; a
  database that ran that branch keeps the empty table and its migration record, which nothing reads.
  Do not reuse version `20261111000002` for another fork migration: such a database would skip it.

## Deployment / cutover

Roll out in order: (1) deploy the fenced code (inert until pinned); (2) run migrations
(`prepare db`); (3) **backfill each tenant's existing rows' `team_id` before flipping its flag** —
un-backfilled rows fail closed (hide data, never leak); (4) enable the flag per tenant. Greenfield
(empty shared DB, agents re-enroll) skips the backfill at the cost of host history.
