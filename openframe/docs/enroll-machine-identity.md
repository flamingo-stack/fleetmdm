# Enrollment by OpenFrame machine id

Marker: `OPENFRAME(enroll-machine-identity)`. Active only with `--openframe-mode` on the agent and
`FLEET_OPENFRAME_MODE=1` on the server; otherwise enrollment is upstream's.

## Problem

Fleet identifies a host by its hardware UUID. Machines that share one (cloned VMs, boards with a
vendor-default SMBIOS UUID) share a host: each enrollment overwrites the other machine's node key,
the other machine re-enrolls seconds later, and every enrollment refetches the whole inventory. On
OpenFrame's shared Fleet one such pair loads the shared MySQL and floods `host_software` CDC.

## Model

One Fleet host per OpenFrame machine. The agent enrolls with the OpenFrame `machine_id` (the file
openframe-client writes, read by `OpenFrameMachineIdProvider`) as osquery's `specified` identifier,
so `osquery_host_id` is the machine id and Fleet matches the host the upstream way (priority 1 in
`matchHostDuringEnrollment`).

## Agent

`--host-identifier=openframe` (only with `--openframe-mode`), in
`orbit/cmd/orbit/enroll_identifier_openframe.go`:

- wanted identity: the machine id, osquery `--host_identifier=specified --specified_identifier=<id>`;
  without a machine id the agent keeps the identity it has;
- previous identity: the identifier recorded for `orbit uuid` (`osquery-identifier.txt`);
- when they differ the agent asks the server (`HEAD /api/fleet/orbit/ping`) for the
  `openframe_identifier_move` capability. Without it, or when the move fails, it stays on the
  previous identity and tries again at its next start;
- an enrolled agent moves with `MoveToIdentifier` (`client/orbit_identifier_openframe.go`): an orbit
  enrollment with the new identifier and `previous_orbit_node_key` set to the key on disk;
- after a move the new identifier is recorded for `orbit uuid` only once the move succeeded, so
  OpenFrame's hourly host lookup (`FleetMdmAgentIdTransformer`) finds the host by `osquery_host_id`.

osquery keeps its own node key, so a moved agent does not re-enroll osquery.

## Server

`server/datastore/mysql/openframe.go`, hooks in `EnrollOrbit`:

| Case | Result |
|---|---|
| `previous_orbit_node_key` is the orbit node key of a host with the same hardware UUID (in the pinned team), the new identifier is free | that host takes the new identifier; no refetch, no takeover log |
| the key belongs to no such host (another machine of a pair took the host over) | upstream: the host the identifier matches, or a new one |
| the new identifier already belongs to another host | upstream match to that host |
| an orbit enrollment matches a host another agent enrolled | WARN `openframe: orbit enrollment takes over a host enrolled by another agent`; refused while the host enrolled less than `FLEET_OPENFRAME_ORBIT_ENROLL_COOLDOWN` ago |

MDM identifies a device by its hardware UUID (Apple UDID, `mdm_windows_enrollments.host_uuid`). When
several hosts of one team share it, `MDMResetEnrollment` and `UpdateMDMWindowsEnrollmentsHostUUID`
refuse the enrollment and log WARN `openframe: MDM enrollment refused, several hosts of one team share
the hardware UUID` instead of picking one host silently. Hosts of different teams are left to upstream.

## Configuration

| Where | Key | Value |
|---|---|---|
| Fleet | `FLEET_OPENFRAME_ORBIT_ENROLL_COOLDOWN` | Go duration, e.g. `10m`; unset means no cooldown. Unparsable refuses to boot |
| Fleet | `FLEET_OSQUERY_ENROLL_COOLDOWN` | upstream's cooldown for osquery, set to the same value |
| Agent | `ORBIT_HOST_IDENTIFIER` | `openframe` |

## Rollout

1. Fleet image with this change (advertises `openframe_identifier_move`).
2. Agent release with this change.
3. `ORBIT_HOST_IDENTIFIER=openframe` in the fleetmdm-agent configuration: dev, then tenant by
   tenant. Each agent moves its host once at its next start; the second machine of a pair gets its
   own host.

## Rollback

- Set `ORBIT_HOST_IDENTIFIER=uuid`: each agent moves its host back to the hardware UUID the same way.
  Pairs end up on one host again and loop as before.
- Do not roll the agent binary back before that: at its next enrollment an old agent enrolls by
  hardware UUID, finds no host with it and creates a new one.
- Rolling the Fleet image back first stops the moves: agents keep the identity they have.

## Limits

- Machines cloned after openframe-client was installed share the machine id; Fleet cannot tell them
  apart. The cooldown holds them back and the takeover log raises the alert; openframe-client has
  to give the clone a new machine id.
- An agent that lost its node key (reinstalled with its root directory wiped) before it ever moved
  has no proof and enrolls as a new host; the old one stays offline.
- Two machines sharing a hardware UUID cannot both use Fleet MDM.

## Tests

- `server/datastore/mysql/hosts_machine_identity_openframe_test.go` (`MYSQL_TEST=1`)
- `orbit/cmd/orbit/enroll_identifier_openframe_test.go`
- `client/orbit_identifier_openframe_test.go`
- `server/fleet/openframe_test.go`: cooldown parsing, the capability
