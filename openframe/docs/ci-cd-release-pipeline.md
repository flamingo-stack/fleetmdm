# CI/CD: Release, Code-Signing & Upstream Sync

## Overview

The fork replaces upstream Fleet's release machinery with its own pipeline that
builds and **code-signs** the agent binaries, publishes multi-arch server images
and the Helm chart to GitHub Container Registry (GHCR), and keeps the fork current
via an automated upstream-sync workflow.

These workflows are **net-new** (added by the fork, not present upstream):

| File | Role |
|------|------|
| [`.github/workflows/release.yml`](../../.github/workflows/release.yml) | Main release pipeline |
| [`.github/workflows/test.yml`](../../.github/workflows/test.yml) | PR validation (build + sign, no publish) |
| [`.github/workflows/changes.yaml`](../../.github/workflows/changes.yaml) | Reusable path-filter (server / client / helm) |
| [`.github/workflows/sync-upstream.yml`](../../.github/workflows/sync-upstream.yml) | Scheduled merge from `fleetdm/fleet` |
| [`.github/steps/sign-macos-package/action.yml`](../../.github/steps/sign-macos-package/action.yml) | macOS sign + notarize |
| [`.github/steps/sign-windows-package/action.yml`](../../.github/steps/sign-windows-package/action.yml) | Windows sign (Azure Trusted Signing) |

`.goreleaser.yml` / `.goreleaser-snapshot.yml` are **modified** to template the
image destination from environment variables. `Dockerfile` and `Makefile` are
essentially upstream (no fork-specific build targets).

> Source commits: `Release workflow` (6abfe9c9), `Feature/release` (4885c073),
> `Update Agent CI/CD for Fleet` (60b838b1), `Update macOS signing pipeline`
> (8b21105e), `Update signing step` (fd7d2451), `Add windows signing` (0dab01e9),
> `CI to build Fleet` (01a6bb8a), `Add action - Sync from upstream fleet`
> (584511513a), `Update GitHub Actions workflow for sync upstream` (fe662428).

## Release pipeline (`release.yml`)

### Triggers

```yaml
on:
  push:
    branches: [main]
    paths-ignore: ["**/*.md"]
  workflow_dispatch:
    inputs:
      version:   # e.g. v1.2.3
```

- **Push to `main`** → tags images `latest`, publishes a GitHub **pre-release**.
- **Manual dispatch with `version`** → tags images with that semver, publishes a
  normal release.

Shared env: `REGISTRY: ghcr.io`, `BINARY_NAME: fleet`. Permissions include
`contents: write`, `packages: write`, `attestations: write`, `id-token: write`.

### Jobs

| Job | Lines | What it does |
|-----|-------|--------------|
| `changes` | 37 | Calls `changes.yaml` to decide which of server/client/helm actually changed. |
| `build_client` | 45 | Builds agent binaries: **macOS universal** (arm64 + amd64 merged with `lipo`) and **Windows amd64**; then runs the platform signing step. |
| `build` | 138 | Runs `goreleaser release --clean` to build the Linux server and push multi-arch Docker images. |
| `build_helm` | 201 | Packages and pushes the Helm chart as an OCI artifact via `appany/helm-oci-chart-releaser@v0.5.0`. |
| `release` | 227 | Aggregates signed client artifacts and creates the GitHub release with `softprops/action-gh-release@v2`. |

### Server image build (goreleaser)

`.goreleaser.yml` templates the image destination entirely from env, which is what
lets the same config build under any fork/registry:

```yaml
image_templates:
  - "{{ .Env.REGISTRY }}/{{ .Env.GITHUB_REPOSITORY }}/fleet:{{ .Env.DOCKER_TAG }}-amd64"
  - "{{ .Env.REGISTRY }}/{{ .Env.GITHUB_REPOSITORY }}/fleet:{{ .Env.DOCKER_TAG }}-arm64"
# + docker_manifests combining the two into a single multi-arch tag
```

`.goreleaser-snapshot.yml` is the no-push variant used for local/dev and PR builds.

## Code signing

### macOS — `sign-macos-package`

Signs and notarizes the universal binary using Apple Developer ID:

1. Base64-decode the `.p12` certificate into a throwaway keychain
   (`security create-keychain` / `import`).
2. `codesign` the binary with the Developer ID identity (hardened runtime).
3. Submit to Apple's notary service with `xcrun notarytool` and verify.

**Inputs / secrets:** `apple_certificate_p12`, `apple_certificate_password`,
`apple_developer_id`, `apple_id_username`, `apple_id_password`, `apple_team_id`.

### Windows — `sign-windows-package`

Uses **Azure Trusted Signing** via `azure/trusted-signing-action@v0.5.0` (no local
certificate material — signing is a cloud service authenticated with an Azure AD
service principal).

**Inputs / secrets:** `azure_tenant_id`, `azure_client_id`, `azure_client_secret`,
`signing_endpoint`, `code_signing_account_name`, `certificate_profile_name`.

> Linux server binaries are not signed; trust comes from the signed container
> images / GHCR provenance.

## Publish targets

| Artifact | Destination |
|----------|-------------|
| Server image (multi-arch) | `ghcr.io/<owner>/<repo>/fleet:<tag>` (`<tag>` = `latest` or the dispatch semver) |
| Helm chart (OCI) | `ghcr.io/<owner>/<repo>/helm-charts/fleet` |
| Signed client binaries | GitHub Release assets (`fleet-macos-universal.tar.gz`, Windows `.exe`) |

For this fork `<owner>/<repo>` is `flamingo-stack/fleetmdm`.

## Upstream sync (`sync-upstream.yml`)

Keeps the fork current with `fleetdm/fleet` without manual merging.

```yaml
on:
  schedule:
    - cron: "0 4 * * 1"   # Mondays 04:00 UTC
  workflow_dispatch:
```

Flow:

1. **Skip if a sync PR is already open** for the `sync/upstream-main` branch
   (avoids duplicate PRs — added in fe662428).
2. Configure git as `flamingo-sync-bot` and fetch `upstream` (`fleetdm/fleet`).
3. Merge `upstream/main` into the sync branch. **Conflicts are not auto-resolved**
   — the run skips/defers and a human resolves them on the next attempt.
4. On a clean merge, push `sync/upstream-main` and open a PR against the fork's
   `main`.

Conflict resolution is deliberately manual because the fork's invasive changes
(orbit OpenFrame mode, migration idempotency, the Helm chart) routinely collide
with upstream. See the per-feature **Rebase notes** in the other docs for what to
re-apply.

## Required secrets (summary)

| Purpose | Secrets |
|---------|---------|
| macOS signing/notarization | `APPLE_CERTIFICATE_P12`, `APPLE_CERTIFICATE_PASSWORD`, `APPLE_DEVELOPER_ID`, `APPLE_ID_USERNAME`, `APPLE_ID_PASSWORD`, `APPLE_TEAM_ID` |
| Windows signing | `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_SIGNING_ENDPOINT`, `AZURE_CODE_SIGNING_ACCOUNT_NAME`, `AZURE_CERTIFICATE_PROFILE_NAME` |
| Registry / releases | `GITHUB_TOKEN` (built-in; `packages: write`) |

> Secret **names** above reflect how the workflows pass values into the signing
> actions; confirm exact repository secret names in
> Settings → Secrets before relying on them.

## Files changed

| File | Status | Purpose |
|------|--------|---------|
| `.github/workflows/release.yml` | new | Build + sign + publish pipeline |
| `.github/workflows/test.yml` | new | PR build/sign validation |
| `.github/workflows/changes.yaml` | new | Reusable path filter |
| `.github/workflows/sync-upstream.yml` | new | Scheduled upstream merge |
| `.github/steps/sign-macos-package/action.yml` | new | macOS sign + notarize |
| `.github/steps/sign-windows-package/action.yml` | new | Windows Azure Trusted Signing |
| `.goreleaser.yml`, `.goreleaser-snapshot.yml` | modified | Env-templated image destinations |
