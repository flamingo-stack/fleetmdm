# Agent JSON `Content-Type` header

**Slug:** `OPENFRAME(agent-json-content-type)` · **File:** [`client/orbit_client.go`](../../client/orbit_client.go)

Orbit sets `Content-Type: application/json` on every request that carries a body.
Three lines, unconditional (not gated on `--openframe-mode`), because it is correct
for any Fleet deployment sitting behind a WAF — not just OpenFrame's.

## Why the fork carries this

Upstream `OrbitClient.requestWithExternal()` marshals `params` to JSON and hands the
bytes to `http.NewRequestWithContext`, but never sets `Content-Type`. Go's `net/http`
adds **no default request `Content-Type`** (the `http.DetectContentType` sniffing you
may be thinking of is server-side, for responses). So orbit ships a JSON body with no
content type at all.

That is invisible to Fleet itself — `makeDecoder` in
[`server/service/endpoint_utils.go`](../../server/service/endpoint_utils.go) just
`json.Decode`s the body and never inspects the request content type — so upstream has
no reason to notice. It is very visible to a WAF.

OpenFrame terminates agent traffic on a GCP **Cloud Armor** policy (`client-policy`,
`advanced_options_config { json_parsing = "STANDARD" }`). Cloud Armor keys JSON body
parsing **off the `Content-Type` header**. With no header, parsing silently does not
engage and the entire raw body is evaluated as one opaque field, so the JSON
punctuation itself becomes the attack surface:

```
matchedFieldType:  ARG_NAMES
matchedFieldName:  {"orbit_node_key":"QKafSDrSZRqNP6JYc3bH2T33B2gODS+0"}   ← whole body = one field
matchedOffset: 1   matchedLength: 18                                        ← exactly `"orbit_node_key":"`
preconfiguredExprIds: [owasp-crs-v042200-id942340-sqli]
```

CRS **942340** ("SQL authentication bypass 3/3") matches the `"key":"` sequence. That is
content-independent — it does not depend on the node key at all — so it fired on
**100%** of `POST /api/fleet/orbit/config` polls, which is **94.5%** of every WAF match
on the agent policy. It blocked promoting the Cloud Armor WAF band out of `preview`:
enforcing would have `deny(502)`'d every config poll for every agent in the fleet.

Measured in `shared-j62b` (dev), 2026-08-07.

## Reproduction

Same body, four content types, against the dev LB:

| `Content-Type` sent | Cloud Armor result |
|---|---|
| `application/json` | body parsed → **no match** |
| `application/json; charset=utf-8` | body parsed → **no match** |
| *(absent — upstream behaviour)* | raw `ARG_NAMES` → **942340** |
| `text/plain` | raw `ARG_NAMES` → **942340** |

Note `json_custom_config` on the Armor side **cannot** substitute for this fix: it only
adds content-type *strings* to match against, and the unfixed request has no header to
match. The header has to come from the agent.

## Scope

All three agent→Fleet request builders had the defect; all three are fixed:

| File | Notes |
|---|---|
| [`client/orbit_client.go`](../../client/orbit_client.go) | `requestWithExternal()`. The live one — 100% of the observed false positives. |
| [`client/device_client.go`](../../client/device_client.go) | `requestAttempt()`. Fleet Desktop, latent — zero `/api/fleet/device/` traffic in dev. |
| [`orbit/cmd/fetch_cert/main.go`](../../orbit/cmd/fetch_cert/main.go) | `requestCert()`. One-shot cert CLI. |

Both client fixes are guarded on `len(bodyBytes) > 0`, so bodyless requests
(`HEAD /api/fleet/orbit/ping`, `CheckToken`) stay header-free. Orbit's `external` branch
(`DownloadSoftwareInstallerFromURL`) is a bodyless `GET` to a third-party URL and is untouched.

In `fetch_cert` the header is set **before** `signer.Sign(req)`. `content-type` is not a
covered field today — [`pkg/fleethttpsig`](../../pkg/fleethttpsig/fleethttpsig.go) covers
`@method`, `@authority`, `@path`, `@query`, `content-digest` — but signing the final header
set stays correct if that list ever widens.

Regression tests:
[`client/orbit_client_content_type_test.go`](../../client/orbit_client_content_type_test.go),
covering both the header on body-carrying requests and its absence on bodyless ones.

Adding that test surfaced a second fork bug, fixed here too: `.gitignore` carried an
unanchored `orbit**` / `osquery**` alongside the anchored `/orbit-*` / `/osquery-*`. Unanchored
patterns match a basename at any depth, so they silently ignored `orbit/`, `client/orbit_*.go`
and any new `orbit_*_test.go` — 63 tracked files matched, surviving only because ignores do not
apply to already-tracked files. The anchored pair (plus `/build`) covers the real artifacts, so
the unanchored lines were dropped.

## Upstream

This is a plain bug fix with no OpenFrame-specific behaviour; it is a good candidate to
send upstream, which would let the fork drop the marker entirely.

See also [agent-openframe-mode.md](agent-openframe-mode.md) — the adjacent marker in the
same function.
