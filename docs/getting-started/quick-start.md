# Quick Start

Get FleetMDM running locally in under 10 minutes. This guide walks you through cloning the repository, starting the backing services, building the server, and opening the web console.

## TL;DR — 5-Minute Setup

```bash
# 1. Clone the repository
git clone https://github.com/flamingo-stack/fleetmdm.git
cd fleetmdm

# 2. Start MySQL and Redis via Docker Compose
docker compose up -d mysql redis

# 3. Install frontend dependencies
npm install

# 4. Build the frontend assets
npm run build

# 5. Prepare the database (run migrations)
go run ./cmd/fleet/... prepare db \
  --mysql_address=localhost:3306 \
  --mysql_database=fleet \
  --mysql_username=fleet \
  --mysql_password=insecure

# 6. Start the Fleet server (development mode)
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

Then open your browser at **http://localhost:8080** and complete the initial setup wizard.

---

## Step-by-Step Walkthrough

### Step 1 — Clone the Repository

```bash
git clone https://github.com/flamingo-stack/fleetmdm.git
cd fleetmdm
```

### Step 2 — Start Backing Services

The `docker-compose.yml` at the root of the repository includes MySQL 8.0 and Redis:

```bash
docker compose up -d mysql redis
```

Verify they are running:

```bash
docker compose ps
```

You should see `mysql` and `redis` services in the `running` state.

> **Apple Silicon:** See the [Prerequisites](prerequisites.md) page for the correct image overrides.

### Step 3 — Install Frontend Dependencies

```bash
npm install
```

This installs React 18, TypeScript, Webpack 5, Jest, Storybook, and all other frontend dependencies listed in `package.json`.

### Step 4 — Build the Web Console

```bash
npm run build
```

This compiles the React/TypeScript source in `frontend/` and outputs bundled assets that the Fleet server serves.

### Step 5 — Prepare the Database

Run database migrations to initialize the Fleet schema:

```bash
go run ./cmd/fleet/... prepare db \
  --mysql_address=localhost:3306 \
  --mysql_database=fleet \
  --mysql_username=fleet \
  --mysql_password=insecure
```

Expected output ends with a line like:

```text
Applied N migrations in Xs
```

### Step 6 — Start the Fleet Server

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

| Flag | Purpose |
|---|---|
| `--dev` | Enables development mode (relaxed defaults, live reload assets) |
| `--dev_license` | Enables premium features without a real license key |
| `--server_tls=false` | Disables HTTPS for local development |
| `--logging_debug` | Verbose debug logging to stdout |

### Step 7 — Open the Web Console

Navigate to [http://localhost:8080](http://localhost:8080) in your browser.

On first run you will see the **Fleet Setup Wizard**. Create your first admin account by filling in:

- Organization name
- Full name
- Email address
- Password (refer to your environment configuration)

After completing setup, you land on the **Fleet Dashboard** — your new device management console.

---

## Alternative: Use a Config File

Instead of passing flags on every run, create a YAML config file:

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

Then run:

```bash
go run ./cmd/fleet/... serve --config fleet.yml --dev --dev_license
```

---

## Enroll Your First Host

Once the server is running, install the `fleetd` agent on a test machine. The quickest way to generate an installer is with `fleetctl`:

```bash
go run ./cmd/fleetctl/... package \
  --type=pkg \
  --fleet-url=http://localhost:8080 \
  --enroll-secret=<your-enroll-secret>
```

Copy the enroll secret from **Settings → Enroll Secrets** in the web console, then distribute the generated package.

---

## What You Have Running

After completing these steps:

- The **Fleet Server** is listening on port `8080`
- **MySQL** stores all inventory, policy, and job data
- **Redis** handles caching and live query pub/sub
- The **React web console** is served from the Fleet server

## Next Steps

After your quick start is working, you may want to:

- Review the [First Steps](first-steps.md) guide to explore key platform features
- Set up your development environment with IDE plugins and hot reload
- Enroll additional hosts using generated installers
- Create your first compliance policy in the Fleet web console
