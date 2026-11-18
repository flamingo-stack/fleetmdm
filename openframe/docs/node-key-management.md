# Orbit Node Key Management

> Relocated from `docs/node-key-management.md` so that all fork documentation
> lives under `openframe/docs/`. Complements
> [agent-openframe-mode.md](agent-openframe-mode.md) — the node key is the Fleet
> enrollment secret; the OpenFrame bearer token is a separate, gateway-level
> credential.

## Overview

Orbit authenticates with the Fleet server using a **node key**, a secret token
obtained during enrollment and kept in `secret-orbit-node-key.txt`
(`constant.OrbitNodeKeyFileName`). `getNodeKeyOrEnroll` in
[`client/orbit_client.go`](../../client/orbit_client.go) reads the file on every
authenticated request and enrolls (`POST /api/fleet/orbit/enroll`) when the file
is missing or empty. A new key is written atomically: a temp file in the same
directory, renamed over the old one, so an existing key is never truncated or
deleted before its replacement exists.

The fork's earlier in-memory key cache and `.stale` rename fallback
(`fix: enrollment key`, bcb124c0a6) are no longer in the code; the atomic write
above (upstream #47652) covers the interrupted-write case they handled.

## Re-enrollment (401 handling)

Upstream behaviour (#47652): a 401 on an authenticated request does not drop the
key.

1. The first 401 starts a streak (`unauthenticatedSince`).
2. A 401 that arrives once the streak is older than
   `unauthenticatedReenrollGracePeriod` (45 s) arms a re-enroll
   (`forceReenroll`). With a host identity certificate, arming also deletes the
   certificate and restarts orbit.
3. The next authenticated request sees the armed re-enroll, enrolls and replaces
   the key.
4. A successful request or enrollment clears the streak.

### Fork: the streak survives orbit restarts

Upstream keeps the streak in memory only. A host whose orbit lives shorter than
the grace period (osqueryd exits about 10 s after start and orbit exits with it)
started a new streak in every process, so re-enroll never armed and the host
stayed offline with a rejected key (CU-17tkuw63f94).

The fork saves the streak next to the node key, in
`secret-orbit-node-key.txt.unauthenticated`, as `<RFC 3339 start>` or
`<RFC 3339 start> armed` (`client/orbit_reenroll_openframe.go`, hooks marked
`OPENFRAME(orbit-reenroll-persist)` in `client/orbit_client.go`):

- read once per process, on the first streak check;
- written when a streak starts and when re-enroll arms;
- deleted when the streak is cleared (a successful request or enrollment).

On a crash-looping host this gives:

| Orbit run | What happens |
|-----------|--------------|
| 1 | First 401 starts the streak and saves its start. |
| 2 (at least 45 s later) | The first 401 is past the grace period: re-enroll arms and is saved as `armed`. |
| 3 | `getNodeKeyOrEnroll` sees the saved arming and enrolls before the first request. |

Run 2 re-enrolls itself if it makes another authenticated request before it
exits. A start time in the future or unreadable content is ignored. A 401 at the
end of one run and another at the start of a run hours later count as one
streak, so the second 401 arms re-enroll at once. Deleting the Orbit directory
(the OpenFrame client's Fleet reinstall) deletes the file with the key.

## Diagnostic logs

- `authenticated request got 401, invalidating node key` (`path`, `has_auth_token`)
- `orbit received 401 unauthenticated, retrying with existing node key before re-enrolling` (`after` = streak age, counted across restarts)
- `orbit node key repeatedly rejected with 401, will re-enroll on next request`
- `orbit node key was rejected by the server, re-enrolling` / `orbit node key file is empty, enrolling` / `orbit node key file not found, enrolling`
- `read orbit 401 streak file` / `write orbit 401 streak file` / `remove orbit 401 streak file`: the streak file could not be used; orbit goes on with the in-memory streak.

## Files changed

| File | Purpose |
|------|---------|
| `client/orbit_client.go` | Hooks that load, save and remove the saved streak (`OPENFRAME(orbit-reenroll-persist)`) |
| `client/orbit_reenroll_openframe.go` | Streak file read, write and delete |
| `client/orbit_reenroll_openframe_test.go` | Restart, clear-on-success and bad-content tests |
