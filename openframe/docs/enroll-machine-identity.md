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

## Switch

The server decides, so no agent argument changes and no agent reinstall is needed (the client
passes new run arguments only on a reinstall, which deletes orbit's directory and node key):

| `FLEET_OPENFRAME_MACHINE_IDENTITY` | Capabilities | Agents |
|---|---|---|
| unset / `false` | `openframe_identifier_move` | move back to the identifier `--host-identifier` asks for (hardware UUID by default) |
| `true` | `openframe_identifier_move`, `openframe_machine_identity` | move to the machine id |

An older server advertises neither, and agents keep the identifier they have. An unparsable value
refuses to boot.

## Agent

`orbit/cmd/orbit/enroll_identifier_openframe.go`, at every start in OpenFrame mode:

- previous identity: the identifier recorded for `orbit uuid` (`osquery-identifier.txt`);
- the capabilities come from `HEAD /api/fleet/orbit/ping`; unreachable or older server: keep the
  previous identity (a new install follows `--host-identifier`);
- wanted identity: the machine id with osquery `--host_identifier=specified
  --specified_identifier=<id>` when the server asks for it, otherwise `--host-identifier`. Without a
  machine id the agent keeps the identity it has;
- an enrolled agent that changes identity moves with `MoveToIdentifier`
  (`client/orbit_identifier_openframe.go`): an orbit enrollment with the new identifier and
  `previous_orbit_node_key` set to the key on disk. A failed move keeps the previous identity until
  the next start;
- after a move the new identifier is recorded for `orbit uuid` only once the move succeeded, so
  OpenFrame's hourly host lookup (`FleetMdmAgentIdTransformer`) finds the host by `osquery_host_id`.

The new orbit reaches devices with a normal agent version update, which keeps orbit's directory and
node key. osquery keeps its own node key, so a moved agent does not re-enroll osquery.

## Server

`server/datastore/mysql/openframe.go`, hooks in `EnrollOrbit`:

| Case | Result |
|---|---|
| `previous_orbit_node_key` is the orbit node key of a host with the same hardware UUID (in the pinned team), the new identifier is free | that host takes the new identifier; no refetch, no takeover log |
| the key belongs to no such host (another machine of a pair took the host over) | upstream: the host the identifier matches, or a new one |
| the new identifier already belongs to another host | upstream match to that host |
| an orbit enrollment matches a host another agent enrolled | WARN `openframe: orbit enrollment takes over a host enrolled by another agent`; refused while the host enrolled less than `FLEET_OPENFRAME_ORBIT_ENROLL_COOLDOWN` ago |

MDM identifies a device by its hardware UUID (Apple UDID, `mdm_windows_enrollments.host_uuid`):

- Windows (`LinkWindowsHostMDMEnrollment`, the EUA link in `EnrollOrbit`): the enrollment is not
  linked to a host whose hardware UUID another host of the same team shares
  (`OpenframeRefuseSharedHardwareUUID`). Hosts of other teams never count.
- Apple (`MDMResetEnrollment` finds the host by UDID across teams, as upstream): refused only when
  every host with that UDID is in one team. A UDID shared across teams is left to upstream.

Both log WARN `openframe: MDM enrollment refused, several hosts of one team share the hardware UUID`.

## Configuration

| Where | Key | Value |
|---|---|---|
| Fleet | `FLEET_OPENFRAME_MACHINE_IDENTITY` | `true` to move agents to the machine id |
| Fleet | `FLEET_OPENFRAME_ORBIT_ENROLL_COOLDOWN` | Go duration, e.g. `10m`; unset means no cooldown. Unparsable refuses to boot |
| Fleet | `FLEET_OSQUERY_ENROLL_COOLDOWN` | upstream's cooldown for osquery, set to the same value |

## Rollout

1. This Fleet image (switch off: agents keep the hardware UUID).
2. Agents with this orbit, through the usual agent version update.
3. `FLEET_OPENFRAME_MACHINE_IDENTITY=true`, per environment: each agent moves its host once at its
   next start; the second machine of a pair gets its own host.

## Rollback

- `FLEET_OPENFRAME_MACHINE_IDENTITY` off: each agent moves its host back to the hardware UUID the
  same way. Pairs end up on one host again and loop as before.
- Do not roll the agent back to an orbit without this change before that: at its next enrollment an
  old agent enrolls by hardware UUID, finds no host with it and creates a new one.
- Rolling the Fleet image back stops the moves: agents keep the identity they have.

## Limits

- An agent reinstalled before its host moved (orbit's directory and node key gone) has no proof and
  enrolls as a new host; the old one stays offline. After the move a reinstall keeps the host: the
  agent enrolls with the same machine id.
- Machines cloned after openframe-client was installed share the machine id; Fleet cannot tell them
  apart. The cooldown holds them back and the takeover log raises the alert; openframe-client gives
  the clone a new machine id.
- Two machines sharing a hardware UUID within one team cannot use Fleet MDM.

## Tests

- `server/datastore/mysql/hosts_machine_identity_openframe_test.go` (`MYSQL_TEST=1`)
- `orbit/cmd/orbit/enroll_identifier_openframe_test.go`
- `client/orbit_identifier_openframe_test.go`
- `server/service/osquery_utils/queries_openframe_mdm_link_test.go`
- `server/fleet/openframe_test.go`: the switch, the cooldown, the capabilities
