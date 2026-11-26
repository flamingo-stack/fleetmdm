# Agent: verify the server certificate in openframe mode

**Slug:** `OPENFRAME(agent-verify-server-tls)` · **Files:**
[`orbit/cmd/orbit/orbit.go`](../../orbit/cmd/orbit/orbit.go) (`orbitAction`),
[`orbit/cmd/orbit/tls_openframe.go`](../../orbit/cmd/orbit/tls_openframe.go),
[`orbit/cmd/orbit/openframe_certs.pem`](../../orbit/cmd/orbit/openframe_certs.pem)

In openframe mode orbit verifies the server's TLS certificate even when its launch
arguments still carry `--insecure`, and it gives osqueryd a CA bundle so osqueryd can
verify the server too. Upstream mode is unchanged.

## Why the fork carries this

The OpenFrame client starts orbit with the launch arguments it received when the tool
was installed. Until openframe-oss-lib dropped it, those arguments included
`--insecure`, which sets `InsecureSkipVerify` on every orbit client and also allows
plain HTTP. The arguments are stored on the device and only a forced reinstall
rewrites them, so removing the flag on the backend does not reach installed devices.
A new orbit binary does: the client updates it like any tool version.

osqueryd has its own TLS stack. Upstream gives it a CA file either through the
installer (`certs.pem` from `orbit/pkg/packaging`) or through `--fleet-certificate`.
OpenFrame installs orbit without the installer, so orbit writes the bundle itself.

## What changed

- `dropOpenframeInsecure` runs in `orbitAction` before the first read of `insecure`.
  In openframe mode, when `--insecure` or `ORBIT_INSECURE` is set, it logs a warning
  and sets the flag to false, so every later `c.Bool("insecure")` sees false. With
  `ORBIT_OPENFRAME_ALLOW_INSECURE=true` in orbit's environment the flag is kept, for
  development only.
- In the openframe branch of the osqueryd setup orbit passes
  `--tls_server_certs <path>`: the `--fleet-certificate` file when set, otherwise
  `certs.pem`, which it writes from the embedded bundle to the root dir on every start.
  A failed write is logged as a warning and orbit still passes the path, so a bundle
  written by an earlier start keeps working and orbit itself keeps running.
- `openframe_certs.pem` is a byte copy of `orbit/pkg/packaging/certs.pem`. Importing
  `orbit/pkg/packaging` would pull the installer builders into the agent (+6.4 MB on
  Windows). `TestOpenframeOsqueryCertsMatchPackaging` fails when the two differ, so an
  upstream sync that refreshes the packaging bundle shows up as a failing test: copy
  the file over.

## Self-signed servers (local k3d, self-hosted)

A server with a certificate from a private CA needs `--fleet-certificate <CA file>`
in the launch arguments (openframe-oss-lib `openframe.management.agent-extra-run-args`).
It covers both orbit and osqueryd. `ORBIT_OPENFRAME_ALLOW_INSECURE` affects orbit only,
not osqueryd.

## Tests

`orbit/cmd/orbit/tls_openframe_test.go`: the flag and environment combinations of
`dropOpenframeInsecure`, the certificate path choice and file write, the bundle copy
check, and `TestOrbitActionUsesOpenframeTLSHelpers`, which parses `orbit.go` and fails
when `orbitAction` reads `--insecure` before `dropOpenframeInsecure` runs or stops
passing the bundle path to osqueryd.
