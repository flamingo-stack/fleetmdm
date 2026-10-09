# Development Environment Setup

This guide covers IDE recommendations, editor extensions, and tooling configuration for an efficient FleetMDM development experience.

## Recommended IDEs

### VS Code (Recommended)

[Visual Studio Code](https://code.visualstudio.com/) works well for both Go backend and React/TypeScript frontend development in a single workspace.

**Required extensions:**

| Extension | ID | Purpose |
|---|---|---|
| Go | `golang.go` | Go language support, IntelliSense, debugging |
| ESLint | `dbaeumer.vscode-eslint` | JavaScript/TypeScript lint feedback |
| Prettier | `esbenp.prettier-vscode` | Auto-format JS/TS on save |
| YAML | `redhat.vscode-yaml` | YAML syntax and schema validation |
| Docker | `ms-azuretools.vscode-docker` | Docker Compose file support |
| GitLens | `eamodio.gitlens` | Enhanced Git blame and history |

**Optional but useful:**

| Extension | ID | Purpose |
|---|---|---|
| Thunder Client | `rangav.vscode-thunder-client` | API testing inside VS Code |
| TODO Highlight | `wayou.vscode-todo-highlight` | Highlight TODO/FIXME comments |
| DotENV | `mikestead.dotenv` | Syntax highlighting for `.env` files |

**Recommended `.vscode/settings.json`:**

```json
{
  "editor.formatOnSave": true,
  "editor.defaultFormatter": "esbenp.prettier-vscode",
  "[go]": {
    "editor.defaultFormatter": "golang.go"
  },
  "go.formatTool": "goimports",
  "go.lintTool": "golangci-lint",
  "eslint.workingDirectories": ["."],
  "typescript.tsdk": "node_modules/typescript/lib"
}
```

### GoLand

[GoLand](https://www.jetbrains.com/go/) by JetBrains is an excellent choice for Go-heavy backend work. It includes built-in support for Go modules, database connections, and Docker Compose integration.

### Vim / Neovim

For terminal-based workflows, install:

- `gopls` — Go language server: `go install golang.org/x/tools/gopls@latest`
- `typescript-language-server` — TypeScript LSP: `npm install -g typescript-language-server`
- A Vim plugin manager (lazy.nvim or Packer) with nvim-lspconfig

---

## Go Development Tooling

Install these Go tools after setting up the Go SDK:

```bash
# Code formatter (replaces gofmt, adds missing imports)
go install golang.org/x/tools/cmd/goimports@latest

# Static analysis
go install honnef.co/go/tools/cmd/staticcheck@latest

# Race condition detector (used at test runtime via -race flag)
# Built into the Go toolchain — no install needed

# Database migration tool (used via go run ./server/goose/...)
# Already vendored in the repository
```

Verify your Go installation:

```bash
go version
# go version go1.22.x linux/amd64

go env GOPATH
# /home/youruser/go
```

Ensure `$GOPATH/bin` is in your `$PATH`:

```bash
# Add to ~/.bashrc or ~/.zshrc
export PATH="$PATH:$(go env GOPATH)/bin"
```

---

## Node.js / npm Setup

The frontend requires Node.js 18 LTS or later. Use [nvm](https://github.com/nvm-sh/nvm) to manage Node versions:

```bash
# Install nvm
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash

# Install and use Node 18
nvm install 18
nvm use 18
node --version
# v18.x.x
```

After cloning the repository, install all frontend dependencies:

```bash
npm install
```

---

## Environment Variables

Create a local `.env` file (do not commit it) or export these variables in your shell profile for development:

```bash
# MySQL
export FLEET_MYSQL_ADDRESS=localhost:3306
export FLEET_MYSQL_DATABASE=fleet
export FLEET_MYSQL_USERNAME=fleet
export FLEET_MYSQL_PASSWORD=insecure

# Redis
export FLEET_REDIS_ADDRESS=localhost:6379

# Server
export FLEET_SERVER_TLS=false
export FLEET_LOGGING_DEBUG=true
```

> The Fleet server also accepts a YAML config file via `--config`. See the [Local Development guide](local-development.md) for details.

---

## Docker Compose Services

The `docker-compose.yml` in the repository root provides all backing services for local development:

```bash
# Start MySQL and Redis
docker compose up -d mysql redis

# Start MySQL test databases (needed for integration tests)
docker compose up -d mysql_test mysql_replica_test

# View logs for a service
docker compose logs -f mysql

# Stop all services
docker compose down
```

The default MySQL credentials for local development:

| Setting | Value |
|---|---|
| Host | `localhost:3306` |
| Database | `fleet` |
| Username | `fleet` |
| Password | `insecure` |

---

## TypeScript Configuration

The project's TypeScript configuration is in `tsconfig.json` at the repository root and extends `@tsconfig/recommended`. Key settings:

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "strict": true,
    "jsx": "react"
  }
}
```

Type-checking runs automatically during the Webpack build. You can run it standalone:

```bash
npx tsc --noEmit
```

---

## ESLint and Prettier

The project uses ESLint with the Airbnb style guide and Prettier for formatting.

```bash
# Run lint checks
npx eslint frontend/

# Auto-fix fixable issues
npx eslint --fix frontend/

# Format all frontend files
npx prettier --write "frontend/**/*.{ts,tsx,js,jsx}"
```

The ESLint config is in `.eslintrc.js` at the repository root. Prettier config is in `package.json` under the `"prettier"` key.

---

## Storybook

Storybook is configured for visual development of React components:

```bash
# Start Storybook dev server (port 6006)
npx storybook dev

# Build static Storybook
npx storybook build
```

Component stories live alongside their components in `frontend/components/**/*.stories.tsx` and `frontend/pages/**/*.stories.tsx`.
