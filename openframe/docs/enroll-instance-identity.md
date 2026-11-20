# Enrollment by instance identifier keeps the device's host

**Slug:** `OPENFRAME(instance-identity)` · **Files:**
[`server/datastore/mysql/hosts.go`](../../server/datastore/mysql/hosts.go) (`matchHostDuringEnrollment`, `EnrollOrbit`),
[`server/datastore/mysql/openframe.go`](../../server/datastore/mysql/openframe.go) (`adoptsUUIDHost`),
[`server/fleet/openframe.go`](../../server/fleet/openframe.go) (`IsOpenframeModeContext`)

In OpenFrame mode, an agent that enrolls with an instance identifier (`--host-identifier=instance`)
takes over the host its device already has from enrolling by hardware UUID, instead of getting a
new one. A second device that shares the same hardware UUID gets a host of its own.

## Why the fork carries this

By default orbit and osquery enroll with the hardware UUID. Two devices with the same UUID (cloned
VMs, boards shipped with a placeholder SMBIOS UUID) then share one host: each enrollment overwrites
the other's node key, the other device re-enrolls, and every enrollment refetches the host's whole
inventory. A few such pairs re-enroll hundreds of times an hour and turn into a constant stream of
`host_software` changes.

Upstream's answer is `--host-identifier=instance`: osquery's random per-install `instance_id`
becomes `osquery_host_id`, so each device is its own host. But upstream matches an instance
identifier only by `osquery_host_id`, so switching an existing fleet to it gives every device a new
host on its next enrollment and leaves the old one, with its history and OpenFrame's tool
connection, behind.

## What changed

`matchHostDuringEnrollment` gets one more candidate, ranked after every upstream match (priority 4),
when OpenFrame mode is on, the enrolling agent sends an identifier that differs from its hardware
UUID, and the platform is not Android:

```sql
SELECT ... FROM hosts
WHERE uuid = ?
  AND (osquery_host_id IS NULL OR osquery_host_id = '' OR osquery_host_id = uuid)
  AND platform <> 'android' [AND team_id = ?]
ORDER BY last_enrolled_at DESC, id DESC LIMIT 1
```

That is a host enrolled by hardware UUID that no instance has claimed yet. When it is the match,
the enrollment first locks the row (`lockUnclaimedUUIDHost`, `SELECT ... FOR UPDATE`) and checks it
is still unclaimed; if a device sharing the UUID claimed it in the meantime, the enrollment goes on
as if nothing matched and creates a new host. Then:

- orbit enrollment updates the host as upstream does, then sets `osquery_host_id` to the instance
  identifier;
- osquery enrollment needs nothing more: upstream already writes `osquery_host_id` on re-enroll.

From then on the host matches the instance identifier by `osquery_host_id` (priority 1). The next
device that brings the same UUID with a different instance finds no unclaimed host and gets a new
one. The team fence of `OPENFRAME(mysql-multitenancy)` applies to the candidate too.

With OpenFrame mode off, enrollment is unchanged. Tests switch the mode with
`fleet.NewOpenframeModeContext`, since the MySQL test harness runs in parallel and
`FLEET_OPENFRAME_MODE` is process-wide.

## Rollout

1. This Fleet image first. Without it, the agent change below creates a new host per device.
2. Then the agent: orbit runs with `--host-identifier=instance` (set in OpenFrame's agent
   configuration). Orbit only sends the identifier when it enrolls, so devices with a valid node key keep their
   host untouched until their next enrollment; devices that share a UUID re-enroll constantly and
   split on their next attempt.
3. OpenFrame already maps instance-mode devices by `osquery_host_id`. The device of a former pair
   that got the new host stays mapped to the old host id until OpenFrame resolves it again.

## Tests

[`server/datastore/mysql/hosts_instance_identity_openframe_test.go`](../../server/datastore/mysql/hosts_instance_identity_openframe_test.go)
(MySQL, `MYSQL_TEST=1`): orbit and osquery enrollment take over the uuid-mode host; a second
device with the same UUID gets its own host; re-enrolling with the same instance keeps the adopted
host; another tenant's host is not taken over; two devices sharing a UUID that enroll at once get one
host each; without OpenFrame mode an instance identifier gets
a new host, as upstream.
[`server/fleet/openframe_mode_context_test.go`](../../server/fleet/openframe_mode_context_test.go):
OpenFrame mode from the env var and from the context.

## Rebase notes

Marked blocks in `hosts.go`: the extra `UNION` candidate at the end of
`matchHostDuringEnrollment` (and `AdoptsUUIDHost` in `enrolledHostInfo`), the row lock right after
the match in `EnrollOrbit` and `EnrollOsquery`, and the `osquery_host_id` update after the
existing-host `UPDATE` in `EnrollOrbit`. If upstream reworks the
match query, keep the candidate last in priority and keep the team filter on it.

## Upstream

Upstream could take the same fallback for any deployment moving to `--host-identifier=instance`;
the OpenFrame-mode gate would then go.
