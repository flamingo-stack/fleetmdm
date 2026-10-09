<div align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://shdrojejslhgnojzkzak.supabase.co/storage/v1/object/public/public/doc-orchestrator/logos/1771371901777-lc3cse-logo-openframe-full-dark-bg.png">
    <source media="(prefers-color-scheme: light)" srcset="https://shdrojejslhgnojzkzak.supabase.co/storage/v1/object/public/public/doc-orchestrator/logos/1771372526604-k3y1w-logo-openframe-full-light-bg.png">
    <img alt="OpenFrame" src="https://shdrojejslhgnojzkzak.supabase.co/storage/v1/object/public/public/doc-orchestrator/logos/1771372526604-k3y1w-logo-openframe-full-light-bg.png" width="400">
  </picture>
</div>

# Contributing to FleetMDM

Thank you for your interest in contributing to FleetMDM! This document describes our community norms, code style conventions, branch naming strategy, commit format, and pull request process.

---

## Community First

All community discussion, questions, bug reports, and feature requests happen in the **OpenMSP Slack** — not GitHub Issues or Discussions.

- **Join:** [openmsp.ai](https://www.openmsp.ai/)
- **Direct invite:** [join.slack.com/t/openmsp/...](https://join.slack.com/t/openmsp/shared_invite/zt-36bl7mx0h-3~U2nFH6nqHqoTPXMaHEHA)
- **Repository:** [github.com/flamingo-stack/fleetmdm](https://github.com/flamingo-stack/fleetmdm)

Before starting a significant contribution, discuss it in the Slack community to align on approach and avoid duplicate effort.

---

## Development Setup

Before contributing, make sure your local environment is ready. See the full guides:

- **[Prerequisites](./docs/getting-started/prerequisites.md)** — Required tools and versions
- **[Local Development](./docs/development/setup/local-development.md)** — Clone, build, run, and hot-reload
- **[Environment Setup](./docs/development/setup/environment.md)** — IDE recommendations and tooling

### Quick Setup

```bash
# Clone the repository
git clone https://github.com/flamingo-stack/fleetmdm.git
cd fleetmdm

# Start backing services
docker compose up -d mysql redis

# Install frontend dependencies
npm install

# Build the web console
npm run build

# Run database migrations
go run ./cmd/fleet/... prepare db \
  --mysql_address=localhost:3306 \
  --mysql_database=fleet \
  --mysql_username=fleet \
  --mysql_password=insecure

# Start the server
go run ./cmd/fleet/... serve \
  --dev --dev_license \
  --mysql_address=localhost:3306 \
  --mysql_database=fleet \
  --mysql_username=fleet \
  --mysql_password=insecure \
  --redis_address=localhost:6379 \
  --server_tls=false \
  --logging_debug
```

---

## Code Style and Conventions

### Go Style

Follow standard Go conventions enforced by `goimports` and `staticcheck`:

- Run `goimports` (not `gofmt`) to format and manage imports
- Keep functions small and focused on a single responsibility
- Use meaningful variable names — avoid abbreviations except for well-known ones (`ctx`, `err`, `ds`, `svc`)
- Return errors rather than panicking in library code
- Add comments to all exported functions and types

```go
// CORRECT — exported function with a doc comment
// CreateHost creates a new host record and returns it.
func (svc *Service) CreateHost(ctx context.Context, req fleet.CreateHostRequest) (*fleet.Host, error) {
    // implementation
}
```

**Import grouping** (enforced by `goimports`):

```go
import (
    // 1. Standard library
    "context"
    "fmt"

    // 2. Third-party packages
    "github.com/jmoiron/sqlx"
    "github.com/stretchr/testify/require"

    // 3. Internal packages
    "github.com/fleetdm/fleet/v4/server/fleet"
    "github.com/fleetdm/fleet/v4/server/authz"
)
```

### TypeScript / React Style

Follow the Airbnb style guide as configured in `.eslintrc.js`:

- Use functional components with hooks — no class components
- Use TypeScript interfaces for all props and API response types
- Keep components focused — extract sub-components when a component grows large
- Co-locate styles, tests, and stories with the component file
- Use `react-query` for all server-state data fetching — no manual `useEffect` + `fetch`
- Run `npx prettier --write` before committing

```typescript
// CORRECT — typed props interface, functional component
interface MyComponentProps {
  title: string;
  onSubmit: () => void;
  isLoading?: boolean;
}

const MyComponent = ({
  title,
  onSubmit,
  isLoading = false,
}: MyComponentProps): JSX.Element => {
  // implementation
};

export default MyComponent;
```

### YAML / Config Files

- Use 2-space indentation
- Add comments explaining non-obvious configuration values
- Never store credentials or secrets in config files committed to the repo

---

## Branch Naming

Use the following prefixes for branch names:

| Prefix | Use Case | Example |
|---|---|---|
| `feat/` | New features | `feat/android-mdm-profiles` |
| `fix/` | Bug fixes | `fix/policy-host-count-off-by-one` |
| `chore/` | Maintenance, dependencies, refactoring | `chore/upgrade-react-18-3` |
| `docs/` | Documentation changes | `docs/add-openframe-auth-guide` |
| `test/` | Test additions or improvements | `test/add-integration-tests-scim` |
| `openframe/` | OpenFrame-specific integration work | `openframe/tenant-isolation-fix` |

**Rules:**

- Use lowercase and hyphens — no underscores or spaces
- Keep names concise but descriptive (3–6 words)
- Branch from `main` for all new work

```bash
# Good branch names
git checkout -b feat/vulnerability-dashboard-filtering
git checkout -b fix/enroll-secret-unicode-support
git checkout -b chore/update-webpack-5-config

# Bad branch names
git checkout -b my_fix         # underscores
git checkout -b FEATURE        # uppercase
git checkout -b fix            # too vague
```

---

## Commit Message Format

Use the [Conventional Commits](https://www.conventionalcommits.org/) format:

```text
<type>(<scope>): <short description>

<optional body>

<optional footer>
```

**Types:**

| Type | Use For |
|---|---|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation changes |
| `test` | Adding or updating tests |
| `chore` | Maintenance (deps, build, tooling) |
| `refactor` | Code restructuring with no behavior change |
| `perf` | Performance improvements |
| `ci` | CI/CD pipeline changes |

**Examples:**

```text
feat(mdm): add Android Enterprise profile reconciliation

Adds a background cron job that reconciles Android ONC profiles
against enrolled devices, removing stale profiles and applying
new ones within 5 minutes.

Related: OpenMSP Slack #android-mdm thread
```

```text
fix(vulnerabilities): correct CVSS score sorting for NVD results

The NVD vulnerability list was sorting by string comparison instead
of numeric comparison, causing CVSS 9.9 to sort before CVSS 10.0.
```

```text
chore(deps): upgrade react-query from 3.38 to 3.39.3
```

**Rules:**

- Subject line ≤ 72 characters
- Use imperative mood: "add feature" not "added feature"
- Reference relevant Slack thread or PR context in the body
- Separate subject from body with a blank line

---

## Pull Request Process

### Before Opening a PR

1. **Sync your branch** with `main`:

   ```bash
   git fetch origin
   git rebase origin/main
   ```

2. **Run all tests** and ensure they pass:

   ```bash
   go test ./...
   npm test
   ```

3. **Run linters**:

   ```bash
   staticcheck ./...
   npx eslint frontend/ --max-warnings=0
   ```

4. **Build the frontend** to catch TypeScript errors:

   ```bash
   npm run build
   ```

### PR Title

Follow the same Conventional Commits format as commit messages:

```text
feat(policies): add label-scoped policy targets
fix(frontend): correct team dropdown ordering on hosts page
docs(contributing): add branch naming guidelines
```

### PR Description Template

```markdown
## Summary

Brief description of what this PR changes and why.

## Changes

- List key changes made
- Include any important implementation notes

## Testing

Describe how you tested this:
- Unit tests added: yes/no
- Integration tests: yes/no
- Manual testing steps performed

## Screenshots (if UI change)

Add before/after screenshots for UI changes.
```

### Review Process

- Every PR requires at least **one approving review** from a maintainer
- All CI checks must pass before merge
- Address review comments or explain why you disagree
- Squash merge is preferred to keep `main` history clean

### Review Checklist

Reviewers verify:

- [ ] Code follows style conventions
- [ ] New features have adequate test coverage
- [ ] No secrets or credentials in the code
- [ ] Database migrations are reversible and non-destructive
- [ ] API changes are backwards compatible or clearly documented
- [ ] Authorization checks (`authz.Authorize()`) present on new endpoints
- [ ] UI changes are accessible (semantic HTML, ARIA attributes)
- [ ] Performance impact considered for database queries

---

## Testing

See the full [Testing Guide](./docs/development/testing/README.md) for details. The short version:

```bash
# Go unit tests
go test ./...

# Go unit tests with race detector
go test -race ./server/...

# Frontend tests
npm test

# Both at once
go test ./... && npm test
```

### Integration Tests

Integration tests require additional Docker Compose services:

```bash
docker compose up -d mysql_test mysql_replica_test redis

MYSQL_TEST_DATABASE=fleet \
MYSQL_TEST_PORT=3307 \
  go test ./server/service/integrationtest/... -v
```

---

## Database Migrations

All schema changes go through the goose migration system:

```bash
# Create a new migration
go run ./server/goose/cmd/goose/... create "add_my_new_table" sql \
  --dir server/datastore/mysql/migrations/tables
```

Migration rules:

1. **Never modify existing migrations** — they may have already run in production
2. **Always add a down migration** to allow rollback
3. **Non-destructive by default** — prefer adding columns/tables over altering/dropping
4. **Test on a fresh schema** before submitting

OpenFrame-specific migrations go in `server/datastore/mysql/migrations/openframe/`.

---

## Dependency Management

### Go Dependencies

```bash
# Add a new dependency
go get github.com/some/package@v1.2.3

# Tidy up go.mod and go.sum
go mod tidy

# Verify all dependencies
go mod verify
```

### Frontend Dependencies

```bash
# Add a production dependency
npm install some-package@1.2.3

# Add a dev dependency
npm install --save-dev some-package@1.2.3
```

Check with the team in Slack before adding new dependencies — prefer extending existing packages.

---

## Security Guidelines

See the full [Security Guide](./docs/development/security/README.md). Key rules:

1. **Never commit secrets** to the repository
2. **Never log secrets** — audit all `log.*` calls for sensitive data
3. All new endpoints must call `authz.Authorize()` before accessing data
4. All database queries must use parameterized statements via `sqlx` — never string concatenation
5. User-controlled content rendered as HTML must go through DOMPurify

---

## CI Pipeline

The following checks run automatically on every pull request and must all pass before merge:

1. Go build verification
2. Go unit tests (`go test ./...`)
3. Go race condition detector (`go test -race ./...`)
4. Frontend build (`npm run build`)
5. Frontend unit tests (`npm test`)
6. TypeScript type checking (`npx tsc --noEmit`)
7. ESLint (`npx eslint frontend/`)

---

<div align="center">
  Built with 💛 by the <a href="https://www.flamingo.run/about"><b>Flamingo</b></a> team
</div>
