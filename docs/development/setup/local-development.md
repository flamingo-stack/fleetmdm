# Local Development Guide

This guide walks through cloning FleetMDM, running it locally with hot-reload support, and configuring your debug setup.

## Clone and Initial Setup

```bash
# Clone the repository
git clone https://github.com/flamingo-stack/fleetmdm.git
cd fleetmdm

# Install frontend dependencies
npm install
```

---

## Running the Backend (Go Server)

### 1. Start Backing Services

```bash
docker compose up -d mysql redis
```

Wait a few seconds for MySQL to be ready, then:

### 2. Run Database Migrations

```bash
go run ./cmd/fleet/... prepare db \
  --mysql_address=localhost:3306 \
  --mysql_database=fleet \
  --mysql_username=fleet \
  --mysql_password=insecure
```

### 3. Start the Fleet Server

```bash
go run ./cmd/fleet/... serve \
  --dev \
  --dev_license \
  --mysql_address=localhost:3306 \
  --mysql_database=fleet \
  --mysql_username=fleet \
  --mysql_password=insecure \
  --redis_address=localhost:6379 \
  --server_tls=false \
  --logging_debug
```

The server listens on **http://localhost:8080**.

### Using a Config File

Create `fleet.yml`:

```yaml
mysql:
  address: localhost:3306
  database: fleet
  username: fleet
  password: insecure

redis:
  address: localhost:6379

server:
  tls: false

logging:
  debug: true
```

Then start with:

```bash
go run ./cmd/fleet/... serve --config fleet.yml --dev --dev_license
```

---

## Running the Frontend (React)

### Production Build (served by Go server)

The Go server serves the bundled frontend assets from the `frontend/` build output. Build once:

```bash
npm run build
```

### Watch Mode (Hot Reload)

For frontend development, run Webpack in watch mode alongside the Go server:

```bash
# Terminal 1: Go server
go run ./cmd/fleet/... serve --config fleet.yml --dev --dev_license

# Terminal 2: Frontend watcher
npm run watch
```

With `--dev` mode enabled, the Fleet server automatically serves the latest compiled assets on each page reload.

> **Note:** The Fleet server in `--dev` mode disables asset caching and reads directly from the build output directory, so changes take effect on the next browser refresh after Webpack finishes.

---

## Building Binaries

Build the Fleet server and CLI as standalone binaries:

```bash
# Fleet server
go build -o fleet ./cmd/fleet/...

# fleetctl CLI
go build -o fleetctl ./cmd/fleetctl/...

# Verify
./fleet version
./fleetctl version
```

---

## fleetctl CLI

The `fleetctl` CLI is the operator tool for managing Fleet via the API or GitOps:

```bash
# Build
go build -o fleetctl ./cmd/fleetctl/...

# Configure to talk to your local Fleet server
./fleetctl config set --address http://localhost:8080

# Login (creates a session token)
./fleetctl login

# Example: list all hosts
./fleetctl get hosts

# Apply GitOps configuration
./fleetctl gitops --config ./my-fleet-config.yml
```

---

## Debug Configuration

### VS Code (Go)

Create `.vscode/launch.json`:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Fleet Server (debug)",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/fleet",
      "args": [
        "serve",
        "--config", "${workspaceFolder}/fleet.yml",
        "--dev",
        "--dev_license",
        "--logging_debug"
      ],
      "env": {
        "FLEET_MYSQL_PASSWORD": "insecure"
      }
    }
  ]
}
```

Set breakpoints in any Go file and press **F5** to launch the debugger.

### Delve (terminal)

```bash
# Install Delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug the Fleet server
dlv debug ./cmd/fleet/... -- serve --config fleet.yml --dev
```

### Frontend Debugging

The React application is built with source maps in development mode. Open your browser's Developer Tools (F12) and use the Sources panel to set breakpoints in the original TypeScript source files.

---

## Environment Variables for Development

Set these in your shell or in a `.env` file:

```bash
# Core services
FLEET_MYSQL_ADDRESS=localhost:3306
FLEET_MYSQL_DATABASE=fleet
FLEET_MYSQL_USERNAME=fleet
FLEET_MYSQL_PASSWORD=insecure
FLEET_REDIS_ADDRESS=localhost:6379

# Development flags
FLEET_SERVER_TLS=false
FLEET_LOGGING_DEBUG=true
```

> Do not commit `.env` files or credentials to the repository.

---

## Running Integration Tests Locally

Integration tests require additional Docker Compose services:

```bash
# Start test databases
docker compose up -d mysql_test mysql_replica_test

# Run Go integration tests (example: server service tests)
MYSQL_TEST_DATABASE=fleet MYSQL_TEST_PORT=3307 \
  go test ./server/service/... -v -run TestIntegration
```

See the [Testing guide](../testing/README.md) for full details.

---

## Verifying Your Local Setup

After completing setup, verify everything is working:

```bash
# Fleet server health check
curl http://localhost:8080/healthz
# Expected: OK

# Fleet server version
curl http://localhost:8080/api/v1/fleet/version
# Expected: JSON with version info

# MySQL connection
docker compose exec mysql mysql -u fleet -pinsecure fleet -e "SHOW TABLES;" | head -20
```

---

## Common Issues

| Problem | Solution |
|---|---|
| Port 8080 already in use | Kill the conflicting process or change the Fleet server port with `--server_address=:8081` |
| MySQL connection refused | Run `docker compose up -d mysql` and wait ~10 seconds for startup |
| `migrate: no migrations to apply` | The database is already up to date — this is not an error |
| Frontend changes not appearing | Run `npm run build` or ensure `npm run watch` is still running |
| `go: module not found` | Run `go mod download` to fetch dependencies |
