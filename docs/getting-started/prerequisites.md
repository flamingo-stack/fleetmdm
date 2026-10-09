# Prerequisites

Before running FleetMDM locally or in production you need a set of tools, services, and access credentials in place. This page lists everything required and how to verify each one.

## Required Software

| Tool | Minimum Version | Purpose |
|---|---|---|
| [Go](https://go.dev/dl/) | 1.22+ | Compile and run the Fleet server and CLI |
| [Node.js](https://nodejs.org/) | 18 LTS | Build the React web console (`frontend/`) |
| npm | 9+ | Install frontend JavaScript dependencies |
| [Docker](https://docs.docker.com/get-docker/) | 20.10+ | Run MySQL, Redis, and supporting services locally |
| [Docker Compose](https://docs.docker.com/compose/) | 2.x | Orchestrate local development service stack |
| [Git](https://git-scm.com/) | 2.30+ | Clone and manage the repository |
| [TypeScript](https://www.typescriptlang.org/) | 6.x | Included via npm devDependencies; no global install needed |

> TypeScript 6.x and Webpack 5 are listed as devDependencies in `package.json` — no global install is required beyond Node.js and npm.

## System Requirements

| Resource | Development | Production (minimum) |
|---|---|---|
| CPU | 2 cores | 4 cores |
| RAM | 4 GB | 8 GB |
| Disk | 10 GB | 50 GB (plus agent data) |
| OS | macOS, Linux, Windows (WSL2) | Linux (Ubuntu 22.04+ recommended) |

## Required Services

| Service | Version | Purpose |
|---|---|---|
| MySQL | 8.0+ | Primary datastore for all Fleet entities |
| Redis | 6.x+ | Caching, live query pub/sub, host status |
| S3-compatible storage | — | Software installer storage, file carves |

All three services are provided by the included `docker-compose.yml` for local development. In production they are typically managed cloud services (RDS, ElastiCache, S3/GCS).

## Optional Services

| Service | Purpose |
|---|---|
| OpenFrame Gateway | Multitenancy routing and tenant-aware auth tokens |
| Kafka + Pinot | OpenFrame streaming analytics |
| SigNoz (OTEL) | Distributed tracing and metrics |
| Apple Push Notification (APNs) cert | Required for Apple MDM |
| Android Enterprise account | Required for Android MDM |

## Account and Access Requirements

| Access | Required For |
|---|---|
| GitHub account with repo access | Cloning `flamingo-stack/fleetmdm` |
| OpenFrame Gateway credentials | Production multitenancy deployments |
| Apple Developer / Business Manager account | Apple MDM (DEP, APNs) |
| Entra / Azure AD tenant | Windows Entra MDM enrollment |
| Google Play EMM account | Android Enterprise MDM |
| AWS / GCP credentials | S3-compatible installer storage (production) |

## Environment Variables

The Fleet server is configured via environment variables prefixed with `FLEET_`. The following are required for a minimal local setup:

| Variable | Example | Description |
|---|---|---|
| `FLEET_MYSQL_ADDRESS` | `localhost:3306` | MySQL host and port |
| `FLEET_MYSQL_DATABASE` | `fleet` | MySQL database name |
| `FLEET_MYSQL_USERNAME` | `fleet` | MySQL username |
| `FLEET_MYSQL_PASSWORD` | `insecure` | MySQL password |
| `FLEET_REDIS_ADDRESS` | `localhost:6379` | Redis host and port |
| `FLEET_SERVER_TLS` | `false` | Whether to enable TLS (use `false` for local dev) |
| `FLEET_SERVER_PRIVATE_KEY` | 32-byte string | Required for encrypted storage features |

For a full list of configuration options, see the `server/config/config.go` source file.

> **Security note:** Never commit real credentials to source control. Use environment files (`.env`) or a secrets manager in production.

## Verification Commands

Run these commands to confirm your toolchain is ready:

```bash
# Verify Go
go version
# Expected: go version go1.22.x ...

# Verify Node.js
node --version
# Expected: v18.x.x or higher

# Verify npm
npm --version
# Expected: 9.x.x or higher

# Verify Docker
docker --version
# Expected: Docker version 20.x.x, build ...

# Verify Docker Compose
docker compose version
# Expected: Docker Compose version v2.x.x

# Verify Git
git --version
# Expected: git version 2.x.x
```

> If Docker Compose is available only as the legacy `docker-compose` binary (not `docker compose`), install the v2 plugin: [docs.docker.com/compose/install/](https://docs.docker.com/compose/install/).

## macOS Apple Silicon Notes

The `docker-compose.yml` supports Apple Silicon. Set these environment variables before starting services:

```bash
export FLEET_MYSQL_IMAGE=arm64v8/mysql:oracle
export FLEET_MYSQL_PLATFORM=linux/arm64/v8
docker compose up -d mysql redis
```

## Windows Notes

Development on Windows is supported via WSL2 (Windows Subsystem for Linux 2). Install WSL2, then follow the Linux instructions inside the WSL environment. Docker Desktop for Windows with WSL2 backend is recommended.
