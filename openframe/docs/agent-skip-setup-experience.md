# Agent: no Premium-only setup-experience calls

**Slug:** `OPENFRAME(agent-skip-setup-experience)` · **Files:**
[`orbit/cmd/orbit/orbit.go`](../../orbit/cmd/orbit/orbit.go),
[`orbit/pkg/setup_experience/setup_experience.go`](../../orbit/pkg/setup_experience/setup_experience.go)

Orbit's *setup experience* (the post-install "My Device" onboarding that tracks
software installs and scripts) talks to two Fleet endpoints that exist only under a
Fleet Premium license:

| Endpoint | Free build answer |
|---|---|
| `POST /api/fleet/orbit/setup_experience/init` | `402 Requires Fleet Premium license` (`SetupExperienceInit` in [`server/service/orbit.go`](../../server/service/orbit.go)) |
| `POST /api/fleet/orbit/setup_experience/status` | `402`, unconditionally (`GetOrbitSetupExperienceStatus`, same file) |

The OpenFrame platform Fleet is the free build, and OpenFrame never uses the setup
experience (no Fleet Desktop, no device token, no Apple/Windows MDM onboarding through
Fleet — see [agent-openframe-mode.md](agent-openframe-mode.md)). So every one of these
calls is a paid-tier API call that can only ever fail. The fork removes them.

## Why the fork carries this

Upstream `processSetupExperience` (Windows/Linux, `orbit.go`) runs at startup when the
root dir has no `setup_experience.json`:

1. calls `init`; a `402` is tolerated (`ErrMissingLicense` → treated as *not enabled*);
2. writes `setup_experience.json` with `enabled=false` and no `time_finished`;
3. **registers the `LinuxSetupExperiencer` config receiver anyway**.

That receiver's `Run` only checks `time_finished`, never `enabled`, so on every config
cycle (every 30 s) it calls `status`, gets `402`, and returns the error — which never
sets `time_finished`. The loop ends only when the orbit process restarts: the file
then exists with `enabled=false` and the `!exp.Enabled` branch registers nothing.
Upstream has the same code as of 2026-09-07; it is invisible on a Premium server.

**Prod incident, 2026-09-05** (`CU-86ake48vx`):
the fleet-wide Fleet reinstall wiped every orbit root dir, so ~90% of the online
Windows/Linux agents in 168 tenants armed the poller at once — a flat **~50 req/s of
`402`** against the platform Fleet (equal to the `orbit/config` rate), plus one
`ERR running config receivers error="missing or invalid license" tool_id=fleetmdm-agent`
line in every client log per cycle. It did not heal on its own; only an orbit restart
per machine cleared it. The pre-incident baseline was the same mechanism on every
fresh install (0.2–0.7 req/s).

## What changed

Four marked edits, all under one slug:

| Where | Edit | Gated on |
|---|---|---|
| `orbit.go` preflight (`runSetupExperience`) | `--openframe-mode` implies `--disable-setup-experience`: neither `init` nor `status` is ever called | openframe mode |
| `orbit.go` darwin receiver block | the macOS `SetupExperiencer` is not registered | openframe mode |
| `orbit.go` `processSetupExperience` | when `init` said *not enabled* (including the `402` case) return before registering the Linux poller — the same outcome the restart path already produces | unconditional |
| `setup_experience.go` `LinuxSetupExperiencer.Run` | a file with `enabled=false` is never polled | unconditional |

The first two are the OpenFrame behaviour (no paid calls at all). The last two are a
plain upstream bug fix and keep the fork's orbit correct even when run without
`--openframe-mode` against a free Fleet.

The agent still writes `setup_experience.json` on the non-openframe path exactly as
upstream does; in openframe mode the file is simply never created. Nothing else reads it.

## Scope

- Only the setup-experience endpoints are affected. The other Premium-only orbit
  endpoint, `software_install/package`, is reached only when the server queues a
  software install for the host, which the free build never does for OpenFrame.
- macOS agents never ran the Windows/Linux preflight; the darwin receiver only acts on a
  `run_setup_experience` notification the free server sends solely to hosts in Apple
  MDM setup assistant. Skipping it also keeps that receiver from starting the device-token
  rotation (`trw.StartRotation()`) that [agent-openframe-mode.md](agent-openframe-mode.md)
  deliberately leaves off.
- Existing agents keep polling until their next orbit restart; the fix takes effect on
  each machine once it runs a build carrying it. A repeat `reinstall=true` on an old
  build re-arms the storm.

## Tests

- [`orbit/cmd/orbit/setup_experience_openframe_test.go`](../../orbit/cmd/orbit/setup_experience_openframe_test.go)
  drives `processSetupExperience` against a fake Fleet: `init` → `402`, `init` → `{enabled:false}`
  and `init` → `{enabled:true}`. Only the enabled case may register a receiver, and a
  config cycle after the disabled cases must produce zero `status` calls. The test
  fails on the unfixed code.
- [`orbit/pkg/setup_experience/setup_experience_openframe_test.go`](../../orbit/pkg/setup_experience/setup_experience_openframe_test.go)
  checks `LinuxSetupExperiencer.Run` polls an enabled file and skips a disabled one.

## Rebase notes

Upstream may restructure `processSetupExperience` or move the `runSetupExperience`
gate. After a sync confirm: (1) `--openframe-mode` still short-circuits the preflight,
(2) the darwin `NewSetupExperiencer` registration is still inside `if !c.Bool("openframe-mode")`,
(3) no `RegisterConfigReceiver` follows a not-enabled `init` result. The two tests
above catch (3). See the *Upstream ungates an agent feature* row of
[upstream-sync-conflict-resolution.md](upstream-sync-conflict-resolution.md).

## Upstream

The `processSetupExperience` / `Run` half is a plain bug fix (a disabled setup
experience must not poll) and is a candidate to send upstream; the openframe-mode gate
is fork-only.
