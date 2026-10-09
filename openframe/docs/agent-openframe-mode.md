# Agent OpenFrame Mode (orbit / fleetd)

## Overview

In an OpenFrame deployment the fleetd agent (`orbit`) does not talk to a public
Fleet server directly. Instead it runs in **OpenFrame mode**, where it:

1. Routes every Fleet request through the OpenFrame gateway under the path
   prefix `/tools/agent/fleetmdm-server`.
2. Authenticates each request with a short-lived **bearer token** that is read
   from an **encrypted token file** on disk and **refreshed every 5 seconds**.
3. Runs a **pre-installed, OpenFrame-aware `osqueryd`** binary (orbit's built-in
   auto-update path is disabled) and forwards the OpenFrame flags to it.
4. Exposes an `orbit uuid` helper command that returns the host hardware UUID.

This mode is the agent-side counterpart to the server-side `FLEET_OPENFRAME_MODE`
flag (see [architecture-host-assignments.md](architecture-host-assignments.md)).
It is gated entirely behind the `--openframe-mode` orbit flag; when the flag is
off, orbit behaves exactly like upstream fleetd.

> Source commits: `Openframe mode` (995eb59e), `Add token path to osquery run
> command` (5bb97bb0), `UUID openframe mode` (bb2e5d7c), `Create no auth manager
> for non openframe mode` (71dce177), `OpenFrame logs cleanup` (cf3b07be),
> `chore: add enrollment logs` (bb39445e).

## Configuration flags

All flags live on the `orbit` command in
[`orbit/cmd/orbit/orbit.go`](../../orbit/cmd/orbit/orbit.go).

| Flag | Env var | Type | Description |
|------|---------|------|-------------|
| `--openframe-mode` | `ORBIT_OPENFRAME_MODE` | bool | Master switch. Enables every behavior below. |
| `--openframe-secret` | `ORBIT_OPENFRAME_SECRET` | string | AES key used to decrypt the token file (and passed to osquery). |
| `--openframe-osquery-path` | `ORBIT_OPENFRAME_OSQUERY_PATH` | string | Absolute path to the OpenFrame `osqueryd` binary. **Required** when `--openframe-mode` is set. |
| `--openframe-token-path` | `ORBIT_OPENFRAME_TOKEN_PATH` | string | Path to the encrypted bearer-token file that the refresher polls. |

When `--openframe-mode` is enabled, orbit runs with auto-updates **disabled** and
uses the binary at `--openframe-osquery-path`. If that flag is empty, or the
binary does not exist, orbit logs a fatal error and exits
([orbit.go](../../orbit/cmd/orbit/orbit.go) — `running with auto updates disabled`
branch).

## Token authentication pipeline

The pipeline lives in
[`server/service/openframe/`](../../server/service/openframe/) and is composed of
four small, single-responsibility types:

```
                 ┌──────────────────────────────────────────────┐
   token file ──▶│ OpenframeTokenExtractor                      │
  (encrypted,    │  • os.ReadFile(tokenFilePath)                │
   on disk)      │  • delegates decryption ─────────┐           │
                 └──────────────────────────────────┼───────────┘
                                                    ▼
                 ┌──────────────────────────────────────────────┐
                 │ OpenframeEncryptionService                   │
                 │  • base64-decode                             │
                 │  • AES-GCM open (key = --openframe-secret)   │
                 │  • returns plaintext token                   │
                 └──────────────────────────────────────────────┘
                                                    │
   every 5s      ┌──────────────────────────────────▼───────────┐
  ┌─────────────▶│ OpenframeTokenRefresher (robfig/cron)        │
  │              │  • ExtractToken()                            │
  │              │  • if changed → authManager.UpdateToken()    │
  │              └──────────────────────────────────┬───────────┘
  │                                                 ▼
  │              ┌──────────────────────────────────────────────┐
  └──────────────│ OpenFrameAuthorizationManager (RWMutex)      │
                 │  • GetToken() / UpdateToken()                │
                 └──────────────────────────────────┬───────────┘
                                                    ▼
                 ┌──────────────────────────────────────────────┐
                 │ OrbitClient.requestWithExternal              │
                 │  • Authorization: Bearer <token>             │
                 └──────────────────────────────────────────────┘
```

### Components

| Type | File | Responsibility |
|------|------|----------------|
| `OpenframeEncryptionService` | [openframe-encryption-service.go](../../server/service/openframe/openframe-encryption-service.go) | AES-GCM decryption. Base64-decodes the input, splits the GCM nonce from the ciphertext, and calls `gcm.Open`. The cipher key is the `--openframe-secret` value, so its length must be a valid AES key size (16/24/32 bytes). |
| `OpenframeTokenExtractor` | [openframe-token-extractor.go](../../server/service/openframe/openframe-token-extractor.go) | Reads the encrypted token file and runs it through the encryption service. Returns the plaintext token. |
| `OpenFrameAuthorizationManager` | [openframe_authorization_manager.go](../../server/service/openframe/openframe_authorization_manager.go) | Thread-safe (`sync.RWMutex`) holder of the current token. `NewOpenFrameAuthorizationManagerWithToken` seeds it with the initial value. |
| `OpenframeTokenRefresher` | [openframe_token_refresher.go](../../server/service/openframe/openframe_token_refresher.go) | A `robfig/cron` job scheduled at `*/5 * * * * *` (every 5 seconds). Extracts the token; if it differs from the current one it calls `UpdateToken`. Empty/extract errors are logged but do not crash the agent. |

### Error-log throttling

Reading and decrypting the token file happens every 5 seconds, so a persistent
failure (missing file, wrong key) would flood the logs. Each component throttles
repeated errors using the shared constant
`openframeTokenRefreshErrorLogInterval = 100` — only every 100th consecutive
failure is logged, and the counter resets on the first success.

## Startup wiring

In [`orbit/cmd/orbit/orbit.go`](../../orbit/cmd/orbit/orbit.go), when
`--openframe-mode` is set:

```go
var authManager *openframe.OpenFrameAuthorizationManager
if c.Bool("openframe-mode") {
    encryptionService := openframe.NewOpenframeEncryptionService(c.String("openframe-secret"))
    tokenExtractor    := openframe.NewOpenframeTokenExtractor(encryptionService, c.String("openframe-token-path"))

    openframeToken, err := tokenExtractor.ExtractToken()
    if err != nil {
        // fatal — the agent cannot authenticate without an initial token
        return fmt.Errorf("failed to extract OpenFrame token: %w", err)
    }
    authManager = openframe.NewOpenFrameAuthorizationManagerWithToken(openframeToken)

    tokenRefresher := openframe.NewOpenframeTokenRefresher(tokenExtractor, authManager)
    _ = tokenRefresher.Start()

    // registered as the "openframe token refresher" subsystem so it is
    // stopped gracefully on shutdown via tokenRefresher.Stop()
    addSubsystem(&g, "openframe token refresher", ...)
}
```

The extracted initial token is required: if the first `ExtractToken()` fails the
agent exits. After that, the refresher keeps the in-memory token current and the
agent tolerates transient file/decrypt errors.

## OrbitClient integration

[`server/service/orbit_client.go`](../../server/service/orbit_client.go) takes two
extra constructor arguments — `openFrameMode bool` and
`authManager *openframe.OpenFrameAuthorizationManager`:

- **URL prefix.** When `openFrameMode` is true the client is built with the base
  path `/tools/agent/fleetmdm-server`, so all orbit traffic is routed through the
  OpenFrame gateway's tools proxy. In non-OpenFrame mode there is no prefix.
- **Bearer header.** In `requestWithExternal`, when `openFrameMode` is true the
  client reads `authManager.GetToken()` and, if non-empty, adds
  `Authorization: Bearer <token>` to every request. An empty token logs a debug
  line and sends no header (the gateway then rejects the request, and the next
  refresh cycle will repopulate the token).
- **Machine-id header.** Also in `requestWithExternal`, `OpenFrameMachineIdProvider`
  reads the shared OpenFrame `machine_id` file (written by openframe-client) and,
  if non-empty, adds `x-machine-id: <id>` to every request for gateway/firewall
  machine identification. A missing or empty file sends no header.

The node-key enrollment behavior is unchanged by OpenFrame mode and is documented
separately in [node-key-management.md](node-key-management.md).

## Forwarding flags to osquery

When OpenFrame mode is on, orbit appends OpenFrame flags to the osquery runner
([orbit.go](../../orbit/cmd/orbit/orbit.go), `NewRunner` options):

```go
options = append(options, osquery.WithFlags([]string{"--openframe-mode", "true"}))
options = append(options, osquery.WithFlags([]string{"--openframe-secret", c.String("openframe-secret")}))
options = append(options, osquery.WithFlags([]string{"--openframe-token-path", c.String("openframe-token-path")}))
```

> **Note:** these flags are only understood by the **OpenFrame-aware `osqueryd`
> build** referenced by `--openframe-osquery-path`. A stock upstream `osqueryd`
> does not recognize them. This is why OpenFrame mode requires a custom osquery
> binary and disables orbit's auto-update path.

A small helper, `Runner.GetCommand()` in
[`orbit/pkg/osquery/osquery.go`](../../orbit/pkg/osquery/osquery.go), returns the
fully assembled osqueryd command string (added for logging/diagnostics).

## `orbit uuid` subcommand

`UUID openframe mode` (bb2e5d7c) adds a standalone `orbit uuid` command that
prints the host's hardware UUID. It runs the OpenFrame `osqueryd` against a
temporary database and executes:

```sql
SELECT uuid FROM system_info
```

| Flag | Env var | Description |
|------|---------|-------------|
| `--json` | — | Emit `{"uuid":"..."}` instead of a bare string. |
| `--openframe-mode` | `ORBIT_OPENFRAME_MODE` | Required to select OpenFrame behavior. |
| `--openframe-osquery-path` | `ORBIT_OPENFRAME_OSQUERY_PATH` | Path to the `osqueryd` used for the one-shot query. |

The OpenFrame control plane uses this hardware UUID as the stable host identity,
which pairs with the server-side change that exposes `osquery_host_id` in the
host API (see [api-expose-osquery-host-id.md](api-expose-osquery-host-id.md)).

## Files changed

| File | Purpose |
|------|---------|
| `orbit/cmd/orbit/orbit.go` | OpenFrame CLI flags, custom osqueryd path, token-refresher startup + subsystem, `orbit uuid` command, osquery flag forwarding |
| `orbit/pkg/osquery/osquery.go` | `Runner.GetCommand()` helper |
| `server/service/orbit_client.go` | `openFrameMode` / `authManager` fields, `/tools/agent/fleetmdm-server` URL prefix, bearer-header injection |
| `server/service/openframe/openframe-encryption-service.go` | AES-GCM decryption |
| `server/service/openframe/openframe-token-extractor.go` | Read + decrypt token file |
| `server/service/openframe/openframe_authorization_manager.go` | Thread-safe token holder |
| `server/service/openframe/openframe_machine_id_provider.go` | Cached reader of the shared OpenFrame `machine_id` file |
| `server/service/openframe/openframe_token_refresher.go` | 5-second cron token refresh |
| `server/service/base_client.go` | Additional enrollment/debug logging |

## Deployment

The agent is packaged and installed by the OpenFrame client service (outside this
repo). A minimal OpenFrame-mode invocation looks like:

```bash
orbit \
  --openframe-mode \
  --openframe-secret "$OPENFRAME_AES_KEY" \
  --openframe-osquery-path /opt/openframe/osqueryd \
  --openframe-token-path /var/lib/openframe/token.enc \
  --fleet-url https://<tenant>.openframe.ai
```

The encrypted token file and AES secret are provisioned by the OpenFrame agent
onboarding flow; orbit only consumes them.

## Rebase notes

`orbit/cmd/orbit/orbit.go` and `server/service/orbit_client.go` are heavily
modified relative to upstream and are the most conflict-prone files on rebase:

- `NewOrbitClient` has **two extra trailing parameters** (`openFrameMode`,
  `authManager`). If upstream changes the signature, re-add them and update both
  call sites in `orbit.go`.
- The bearer-header block and the URL-prefix branch in `orbit_client.go` are
  self-contained — re-apply them after resolving surrounding upstream changes.
- The `server/service/openframe/` package is net-new and rarely conflicts.
