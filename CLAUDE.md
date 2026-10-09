# CLAUDE.md

This repository is **`flamingo-stack/fleetmdm`** — a fork of
[`fleetdm/fleet`](https://github.com/fleetdm/fleet) adapted to run as the MDM /
osquery tool inside the **OpenFrame** multi-tenant MSP platform.

## Fork documentation lives in `openframe/docs/`

Start at [openframe/docs/README.md](openframe/docs/README.md) — it indexes every
change this fork makes on top of upstream (host assignments, per-tenant Redis key
prefix, query-results TTL cleanup, agent OpenFrame mode, migration idempotency,
the Helm chart, and the CI/release pipeline), plus
[openframe/docs/fork-file-manifest.md](openframe/docs/fork-file-manifest.md)
listing every created/modified/deleted path.

## Fork edits are marked in the code

Every fork edit inside a shared upstream file is wrapped in sentinel comments:

```go
// >>> OPENFRAME(<slug>): <why> — openframe/docs/<doc>.md
...fork lines...
// <<< OPENFRAME(<slug>)
```

Find them all: `grep -rn "OPENFRAME(" --include='*.go' --include='*.yaml' --include='*.tpl' .`
Net-new fork-only code lives under `openframe/`, `server/service/openframe/`,
`server/datastore/mysql/migrations/openframe/`, `server/datastore/redis/keyprefix.go`,
`server/fleet/openframe.go`, `server/datastore/mysql/openframe.go`, and
`server/service/openframe_middleware.go`.

## Syncing from upstream (the important workflow)

When merging `upstream/main`, follow
**[openframe/docs/upstream-sync-conflict-resolution.md](openframe/docs/upstream-sync-conflict-resolution.md)**.
Key rules:

- **ours = fork** (`HEAD`), **theirs = upstream** in a `git merge upstream/main`.
- **Never drop fork logic to resolve a conflict.** Keep both sides — take
  upstream's change *and* re-apply the `OPENFRAME`-marked fork edit.
- Some breaks have **no git conflict** (new non-idempotent migrations, a Redis
  pool refactor that un-wires the key prefix, mocks going stale). The runbook's
  *semantic-conflict watchlist* lists them — check it every sync.
- After resolving, **verify**: `make openframe-verify` (add `MYSQL_TEST=1` with
  Docker for the deeper tier). Regenerate mocks (`make generate-mock`) if any
  datastore/service interface changed.

## Build / test quick reference

```bash
go build -tags full,fts5,netgo -o build/fleet ./cmd/fleet   # server
go build -o build/orbit ./orbit/cmd/orbit                   # agent
make openframe-verify                                       # fork-feature sanity check
```

Upstream Fleet's own conventions still apply for everything not OpenFrame-specific.
