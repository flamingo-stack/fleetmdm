# OpenFrame-Managed Policies

## Overview

An **OpenFrame-managed policy** is a normal Fleet policy carrying `policies.openframe_managed = 1`.
It is omitted from the policy **list** and **count** endpoints — the set the main UI renders and the
set GitOps reconciles — while it keeps running on hosts and keeps recording results exactly like any
other policy.

The use case is platform-owned checks: OpenFrame needs its own compliance policies on every tenant
without them cluttering the tenant operator's Policies page.

The column is named `openframe_managed` rather than something generic like `hidden` or `internal` so
that it can never collide semantically with a field upstream Fleet may add later — the same
reasoning as `teams.openframe_tenant_uuid`.

The queries twin of this feature is [managed-queries.md](managed-queries.md).

This is fork-only behavior, but it is **not** gated on `FLEET_OPENFRAME_MODE`: that flag gates
behavior such as host assignments, while the column is created by the OpenFrame migration pipeline,
which `prepare db` runs unconditionally. The filter is therefore always on — and inert until
something actually sets the flag.

## What the flag does — and what it deliberately does not

| Surface | Managed policy |
|---|---|
| `GET /policies`, `GET /teams/{id}/policies` (incl. inherited + `merge_inherited`) | **omitted** |
| `GET /policies/count`, `GET /teams/{id}/policies/count` | **not counted** |
| GitOps deletion pass (`fleetctl gitops`) | **invisible → never deleted** |
| `GET /policies/{id}` (single policy) | returned in full |
| `PATCH /policies/{id}` | accepted — including `openframe_managed: false` |
| `POST /policies/delete` | accepted |
| `POST /spec/policies` with a matching name | **silently overwrites the policy** |
| Host details → `policies` array | returned |
| Host failing-policies count / Issues column | counted |
| Fleet Desktop ("My device") | counted |
| Activity feed (`created_policy` / `edited_policy`) | recorded |
| Automations (webhooks, Jira/Zendesk, install software, run script, calendar) | fire normally |
| osquery agent config on the host | delivered (the query text is readable on the endpoint) |

**This flag is decluttering, not access control.** It filters listings only; write paths and by-id
reads never consult it. Verified against a running server: a user holding admin or maintainer can

1. confirm a managed policy's name — `POST /policies` with that name returns
   `Policy "<name>" already exists`;
2. read it in full by id (ids are sequential and trivially enumerated);
3. unmanage and rewrite it — `PATCH /policies/{id} {"openframe_managed": false, "query": "..."}`;
4. delete it — `POST /policies/delete {"ids":[N]}`;
5. **overwrite it silently** — `POST /spec/policies` matches on `(team, name)` via
   `INSERT ... ON DUPLICATE KEY UPDATE`, so the query/description/platform are replaced while
   `openframe_managed` stays `1`. The operator ends up owning a policy they cannot see, and the
   platform ends up running a query it did not write. This one fires by accident on a mere name
   collision, no malice required.

If any of that matters, the fix is guards on the write paths (`modifyPolicy`, both delete paths,
`ApplyPolicySpecs`) returning 404 for managed policies, plus 404 on by-id reads. Full concealment
from a tenant admin is not reachable while they hold write access to the same name space — the
uniqueness key `(team_id, name)` always leaks existence — unless platform policy names get a
reserved prefix.

### GitOps interaction

`fleetctl gitops` deletes team policies that are not in the YAML. It builds that "existing" set from
`GetPolicies`, i.e. the list endpoint, so managed policies are invisible to it and survive a GitOps
apply untouched. The apply half is the hazard described above.

## API

Create (both `POST /policies` and `POST /teams/{id}/policies`):

```json
{ "name": "openframe: disk encryption", "query": "SELECT 1 ...", "openframe_managed": true }
```

Modify takes `"openframe_managed": true|false`; omitting the field leaves the flag as-is. Every
policy payload returns `"openframe_managed"` so the platform can tell them apart.

There is deliberately **no `include_managed` query parameter** on the standard endpoints: the
listing fails closed, and a user-supplied flag cannot widen it. The platform reads managed policies
by id, or through a dedicated OpenFrame endpoint.

## Implementation

### Schema

`server/datastore/mysql/migrations/openframe/20260818000001_AddPoliciesOpenframeManagedColumn.go`
adds `policies.openframe_managed TINYINT(1) NOT NULL DEFAULT 0`. It ALTERs an upstream table from
the OpenFrame pipeline — the same pattern as `teams.openframe_tenant_uuid` — and it is on the
[semantic-conflict watchlist](upstream-sync-conflict-resolution.md).

### Where it lives

The column is in `policyCols` like any other column, and `schema.sql` carries it too. That last part
is the point worth remembering: `cmd/fleet/prepare.go` runs `MigrateOpenframe` **unconditionally**,
so every real deployment has the column no matter what `FLEET_OPENFRAME_MODE` says. `schema.sql` is
only used to build test databases, so it must reflect that same reality; without the column there,
the test harness would diverge from production and every policy test touching `policyCols` would
fail on `Unknown column`.

The flag is written by the same `INSERT`/`UPDATE` statements as every other policy field
(`newGlobalPolicy`, `newTeamPolicy`, `savePolicy`) — no separate write. `ApplyPolicySpecs` does not
name the column, so a spec apply leaves it untouched.

The only helper is a constant in `server/datastore/mysql/policies.go`, under
`OPENFRAME(managed-policies)` markers:

```go
const openframeManagedExclusion = ` AND p.openframe_managed = 0`
```

appended to the five listing/count queries (`listPoliciesDB`, `getInheritedPoliciesForTeam`,
`ListMergedTeamPolicies`, `CountPolicies`, `CountMergedTeamPolicies`). Every one of them aliases the
table as `p`, which is why the alias is baked into the constant rather than passed in.

SEMANTIC-CONFLICT WATCHLIST: `make dump-test-schema` regenerates `schema.sql` from the upstream
`tables/` migrations only and would drop the `openframe_managed` line. Re-add it after any such
regeneration — see [upstream-sync-conflict-resolution.md](upstream-sync-conflict-resolution.md).

### Touched files

| File | Change |
|------|--------|
| `migrations/openframe/20260818000001_AddPoliciesOpenframeManagedColumn.go` | the column |
| `server/datastore/mysql/schema.sql` | the column, so test databases match production |
| `server/fleet/policies.go` | `OpenframeManaged` on `PolicyData`, `PolicyPayload`, `NewTeamPolicyPayload`, `ModifyPolicyPayload` |
| `server/fleet/api_policies.go` | `OpenframeManaged` on `GlobalPolicyRequest` |
| `server/service/global_policies.go` | maps the flag into the create payload |
| `server/service/team_policies.go` | maps the flag on team create and on modify |
| `server/datastore/mysql/policies.go` | `policyCols`, the two `INSERT`s, the `UPDATE`, and the exclusion |
| `server/datastore/mysql/policies_openframe_managed_test.go` | MySQL coverage |

## Host assignment interaction (open item)

In OpenFrame mode `PolicyQueriesForHost` requires a `policy_hosts` row for every policy
([architecture-host-assignments.md](architecture-host-assignments.md) claims a "no rows → all hosts"
fallback that the code does not implement). A managed policy is subject to the same rule: without
assignments it reaches no host, and hosts enrolling later are not backfilled.

Making the flag bypass that requirement is a one-line change in both `policyQueriesForHostStmt` and
`ListPoliciesForHost`:

```sql
AND (p.openframe_managed = 1 OR EXISTS (
        SELECT 1 FROM policy_hosts ph WHERE ph.policy_id = p.id AND ph.host_id = ?
))
```

It is **not** part of this change — it alters what agents execute, so it wants its own review.
