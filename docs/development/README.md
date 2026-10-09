# Development Documentation

Welcome to the FleetMDM development guide. This section covers everything you need to contribute to, extend, or run the `flamingo-stack/fleetmdm` codebase.

## Contents

| Document | Description |
|---|---|
| [Environment Setup](setup/environment.md) | IDE recommendations, editor plugins, and dev tooling |
| [Local Development](setup/local-development.md) | Clone, build, run, hot reload, and debug locally |
| [Architecture Overview](architecture/README.md) | High-level design, component map, and data flow diagrams |
| [Security Guidelines](security/README.md) | Auth patterns, secrets management, input validation, and security testing |
| [Testing](testing/README.md) | Test structure, running tests, writing new tests, and coverage |
| [Contributing Guidelines](contributing/guidelines.md) | Code style, branch naming, commit format, and PR process |

## Technology Stack

### Backend (Go)

| Package / Framework | Version | Role |
|---|---|---|
| Go | 1.22+ | Primary server language |
| Cobra | — | CLI commands (`fleet serve`, `fleetctl`) |
| Viper | — | Config file and environment variable loading |
| sqlx | — | MySQL query builder and row scanning |
| goose | — | Database migrations |
| NanoMDM | embedded | Apple MDM protocol implementation |
| go-kit | — | Service-layer middleware and logging |

### Frontend (TypeScript / React)

| Package | Version | Role |
|---|---|---|
| React | 18.3.1 | UI component library |
| TypeScript | 6.x | Type-safe JavaScript |
| Webpack | 5 | Module bundler |
| react-query | 3.39.3 | Server-state data fetching |
| react-router | 3.2.6 | Client-side routing |
| recharts | 3.8.1 | Dashboard data visualizations |
| Storybook | 8.x | Component development and documentation |
| Jest | 29.x | Unit and integration testing |
| MSW (Mock Service Worker) | 2.x | API mocking in tests |

### Infrastructure

| Service | Purpose |
|---|---|
| MySQL 8.0 | Primary datastore |
| Redis 6+ | Caching, live query pub/sub |
| S3 / GCS | Software installer and file carve storage |
| Docker Compose | Local development service orchestration |
| Kafka + Pinot | OpenFrame streaming analytics (optional) |

## Repository Layout

```text
fleetmdm/
├── cmd/
│   ├── fleet/          # Fleet server binary entry points
│   └── fleetctl/       # Operator CLI
├── ee/                 # Enterprise Edition features
│   ├── fleetd-chrome/  # ChromeOS extension (TypeScript)
│   └── maintained-apps/ # Software catalog ingestion
├── frontend/           # React/TypeScript web console
│   ├── components/     # Shared UI components
│   ├── pages/          # Page-level route components
│   ├── services/       # API service layer
│   └── interfaces/     # TypeScript type definitions
├── orbit/              # fleetd orbit agent
├── server/
│   ├── datastore/      # MySQL, Redis, S3 data layer
│   ├── fleet/          # Core domain types and interfaces
│   ├── mdm/            # Apple, Microsoft, Android MDM stacks
│   ├── service/        # HTTP handlers and business logic
│   └── vulnerabilities/ # CVE scanning engines
├── openframe/          # OpenFrame integration scripts
├── docker-compose.yml  # Local dev services
└── package.json        # Frontend dependencies
```

## Quick Navigation

- **Starting fresh?** → [Local Development](setup/local-development.md)
- **Want to understand the codebase?** → [Architecture](architecture/README.md)
- **Filing a bug or feature?** → Join the [OpenMSP Slack](https://www.openmsp.ai/)
- **Ready to contribute?** → [Contributing Guidelines](contributing/guidelines.md)
