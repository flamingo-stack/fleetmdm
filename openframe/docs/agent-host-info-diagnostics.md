# Agent: host-info startup error names its cause

**Slug:** `OPENFRAME(agent-host-info-diagnostics)` · **Files:**
[`orbit/cmd/orbit/orbit.go`](../../orbit/cmd/orbit/orbit.go) (`getHostInfo`),
[`orbit/cmd/orbit/host_info_openframe.go`](../../orbit/cmd/orbit/host_info_openframe.go)

When orbit's startup host-info query returns the wrong number of rows, the error now
carries osqueryd's stderr, and on Windows with zero rows from a query that ran cleanly
it names WMI as the cause.
Unconditional (not gated on `--openframe-mode`): it only changes the text of an error
that already ends the run.

## Why the fork carries this

At startup orbit shells out to `osqueryd -S` and joins `system_info`, `os_version` and
`osquery_info`. It needs exactly one row. On a Windows host with broken WMI the join is
empty and upstream reports only:

```
run orbit failed error="get UUID: invalid number of rows from system info query: 0"
```

osqueryd exits 0 in that case, and upstream logs its stderr only on a non-zero exit, so
whatever osquery said about the failure is dropped. The line says the result was empty
but not why, and diagnosing it took a live look at the machine.

On Windows only `os_version` can make that join empty: `system_info` falls back to `-1`
values when its WMI queries fail and `osquery_info` always has one row, while
`os_version` returns no row when its `Win32_OperatingSystem` query fails or comes back
empty. So zero rows on Windows means osquery got no OS data from WMI, and the error
says so even when osqueryd itself printed nothing.

## What changed

The `len(info) != 1` branch of `getHostInfo` returns `hostInfoRowsError(...)` instead of
the bare row count. The message keeps upstream's text as its prefix, so existing log
searches still match:

```
get UUID: invalid number of rows from system info query: 0; on Windows this means osquery got no OS data from WMI (Win32_OperatingSystem); osqueryd stderr: W1005 12:00:00.000000 4321 os_version.cpp:33] enum osquery::WmiError[0] (WmiRequest creation failed to connect to server: 0x800706ba)
```

- The WMI clause is added only for zero rows on Windows when osqueryd exited 0. A query
  that fails (SQL error, a table that throws) also prints an empty result, but exits
  non-zero, and then the cause is whatever the stderr clause says.
- The stderr clause is added only when osqueryd printed something. It is flattened to
  one line and capped at 2048 bytes, because the supervisor restarts orbit every few
  seconds on this failure and the line is logged again each time.

The Windows error code inside the stderr comes from the companion change in the osquery
fork (WMI request errors carry the `HRESULT`, and an empty `os_version` warns). With an
older osqueryd the stderr clause may be absent or carry no code; the WMI clause is
there regardless.

## Tests

[`orbit/cmd/orbit/host_info_openframe_test.go`](../../orbit/cmd/orbit/host_info_openframe_test.go):
the message per platform and row count, the truncation, and `getHostInfo` end to end
against a fake `osqueryd` that prints `[]` and a warning and exits 0.

## Rebase notes

One marked block in `getHostInfo`, replacing upstream's `fmt.Errorf` on the row-count
check. If upstream rewrites that check, keep its condition and return
`hostInfoRowsError` from it.

## Upstream

No OpenFrame-specific behaviour; a candidate to send upstream, which would let the fork
drop the marker.
