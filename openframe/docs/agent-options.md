# Agent options on OpenFrame fleets

## What this covers

Fleet's *agent options* (`app_config_json` → `agent_options`) drive osquery behavior on every
enrolled device — `distributed_interval` (live-query/policy poll, upstream default **10s**),
`logger_tls_period`, `distributed_tls_max_attempts`. Without them, osquery falls back to its
compiled-in defaults (`distributed_interval` **60s**), making live queries and refetch feel slow.

`POST /api/v1/setup` writes the upstream new-install defaults. Two things used to remove them:

1. **Upstream's GitOps starter library**: after setup, upstream auto-applies its starter
   templates (`serve.go` → `service.ApplyStarterLibrary` → `fleetctl new` + `fleetctl gitops`).
   The template declares no `agent_options`, and the declarative apply nulls them — observed as
   an `edited_agent_options {"global": true}` activity ~1s after setup. The apply is best-effort
   (needs the setup request's server URL reachable from the pod), so some environments were
   wiped and others not.
2. **Shared-mode tenant seeding**: `OpenframeDefaultAppConfig()` used to strip agent options,
   fearing served endpoint options would override orbit's gateway-prefixed CLI flags.

## Fork behavior

- `OPENFRAME(setup-no-starter-library)` (`cmd/fleet/serve.go`): under
  `FLEET_OPENFRAME_MULTI_TENANCY_ENABLED` the starter library is skipped entirely (same pattern
  as upstream's Primo skip), so a shared Fleet keeps what setup wrote and gets no starter
  labels/scripts. Deployments without multitenancy keep upstream behavior untouched.
- `OpenframeDefaultAppConfig()` (`server/fleet/openframe.go`) seeds shared-mode tenants (and
  the missing-row read fallback, and the instance-row migration) with the upstream defaults,
  agent options included but **already trimmed** of endpoint/plugin keys — so no seeded or
  fallback config can carry an endpoint override even if read by a server predating the
  serve-time trim.
Why trimmed rather than dropped: serving `logger_tls_endpoint` broke log delivery on openframe
agents, which is why PR #135 nulled the whole block; nulling threw away the behavioral
options with it. Trimming keeps the behavioral options while making the unsafe keys
unreachable.

Coverage under multitenancy: agent config reads are team-pinned, so a host is served its
tenant's `app_config_json` row — written by `EnsureOpenframeTeamID` (trimmed) or, if the row is
missing, by the read fallback (trimmed). `POST /setup` writes upstream's untrimmed defaults to
the unpinned instance row (`id = 1`), which unpinned readers (crons, boot) use and agents never
see. Team-level agent options, if an operator ever sets them, are served as written.
