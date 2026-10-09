# Testing Overview

FleetMDM has a comprehensive test suite covering Go backend unit tests, integration tests, React component tests, and end-to-end Storybook stories. This guide explains the test structure, how to run tests, and how to write new ones.

## Test Structure

```text
fleetmdm/
├── server/
│   ├── service/
│   │   ├── *_test.go           # Unit tests alongside source files
│   │   └── integrationtest/    # Full API integration test suites
│   ├── datastore/
│   │   └── mysql/
│   │       └── *_test.go       # MySQL datastore tests (require running DB)
│   └── mdm/
│       └── */
│           └── *_test.go       # MDM subsystem tests
├── frontend/
│   ├── components/
│   │   └── **/*.tests.tsx      # React component unit tests (Jest)
│   ├── pages/
│   │   └── **/*.tests.tsx      # Page-level component tests
│   └── test/
│       ├── handlers/           # MSW (Mock Service Worker) API handlers
│       ├── test-utils.tsx      # Shared test utilities and render helpers
│       └── mock-server.ts      # MSW server configuration
├── orbit/
│   └── **/*_test.go            # Agent package tests
└── ee/
    └── fleetd-chrome/
        └── src/**/*.test.ts    # Chrome extension tests
```

## Running Tests

### Go Backend Tests

#### Unit Tests (no external services required)

```bash
# Run all unit tests
go test ./...

# Run tests for a specific package
go test ./server/service/...

# Run tests with verbose output
go test -v ./server/service/...

# Run a specific test by name
go test -run TestPolicies ./server/service/...

# Run with race condition detector
go test -race ./server/...
```

#### Integration Tests (require MySQL and Redis)

Start the test databases first:

```bash
docker compose up -d mysql_test mysql_replica_test redis
```

Then run integration tests:

```bash
# Server service integration tests
MYSQL_TEST_DATABASE=fleet \
MYSQL_TEST_PORT=3307 \
  go test ./server/service/integrationtest/... -v

# MySQL datastore integration tests
MYSQL_TEST_DATABASE=fleet \
MYSQL_TEST_PORT=3307 \
  go test ./server/datastore/mysql/... -v

# EE integration tests
MYSQL_TEST_DATABASE=fleet \
MYSQL_TEST_PORT=3307 \
  go test ./ee/server/integrationtest/... -v
```

#### Running Tests with Coverage

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out
```

### Frontend Tests

#### Unit and Integration Tests (Jest)

```bash
# Run all frontend tests
npm test

# Run tests in watch mode (re-runs on file changes)
npm test -- --watch

# Run a specific test file
npm test -- frontend/components/Modal/Modal.tests.tsx

# Run tests matching a pattern
npm test -- --testNamePattern="renders correctly"

# Generate coverage report
npm test -- --coverage
```

#### Running Storybook Test Runner

```bash
# Start Storybook (port 6006) first
npx storybook dev &

# Run Storybook test runner
npx storybook test
```

### All Tests at Once

```bash
# Backend + frontend (in order)
go test ./... && npm test
```

---

## Test Configuration

### Go Test Configuration

Test utilities live in `server/test/` and `server/platform/mysql/testing_utils/`:

```go
import (
    "github.com/fleetdm/fleet/v4/server/test"
    mysqltest "github.com/fleetdm/fleet/v4/server/platform/mysql/testing_utils"
)

// Create a test datastore (connects to test MySQL)
ds := mysqltest.CreateMySQLDS(t)

// Create a test user
user := test.NewUser(t, ds, "admin@example.com", fleet.RoleGlobalAdmin, false)
```

### Frontend Test Configuration

The Jest configuration is in `frontend/test/jest.config.js`. Key settings:

- Test environment: `jest-environment-jsdom` with `jest-fixed-jsdom`
- MSW is configured in `frontend/test/mock-server.ts` to intercept API calls
- CSS and file imports are mocked via `identity-obj-proxy`

The `test-utils.tsx` file provides a custom `render()` wrapper that includes all React context providers needed by components:

```typescript
import { renderWithSetup } from "test/test-utils";

// Renders a component with all required providers
renderWithSetup(<MyComponent />);
```

---

## Writing New Tests

### Writing Go Unit Tests

Place test files next to the source they test, using the `_test.go` suffix:

```go
// server/service/my_feature_test.go
package service

import (
    "testing"
    "github.com/stretchr/testify/require"
    "github.com/fleetdm/fleet/v4/server/fleet"
)

func TestMyFeature(t *testing.T) {
    t.Run("creates entity successfully", func(t *testing.T) {
        svc, ctx := newTestService(t)
        result, err := svc.CreateMyEntity(ctx, fleet.MyEntityPayload{
            Name: "test entity",
        })
        require.NoError(t, err)
        require.Equal(t, "test entity", result.Name)
    })

    t.Run("returns error for empty name", func(t *testing.T) {
        svc, ctx := newTestService(t)
        _, err := svc.CreateMyEntity(ctx, fleet.MyEntityPayload{})
        require.Error(t, err)
    })
}
```

### Writing Go Integration Tests

Integration tests in `server/service/integrationtest/` use a full HTTP test server:

```go
func (s *IntegrationTestSuite) TestMyEndpoint() {
    t := s.T()

    // Create test data
    team := s.createTeam("test-team")

    // Make HTTP request
    resp := s.Do("GET", "/api/v1/fleet/my-endpoint", nil, http.StatusOK)
    var result fleet.MyEndpointResponse
    s.DoJSON("GET", "/api/v1/fleet/my-endpoint", nil, http.StatusOK, &result)
    require.NotNil(t, result.Data)
}
```

### Writing React Component Tests

```typescript
// frontend/components/MyComponent/MyComponent.tests.tsx
import React from "react";
import { screen } from "@testing-library/react";
import { renderWithSetup } from "test/test-utils";
import MyComponent from "./MyComponent";

describe("MyComponent", () => {
  it("renders the component title", () => {
    renderWithSetup(<MyComponent title="Hello Fleet" />);
    expect(screen.getByText("Hello Fleet")).toBeInTheDocument();
  });

  it("calls onSubmit when the button is clicked", async () => {
    const onSubmit = jest.fn();
    const { user } = renderWithSetup(
      <MyComponent title="Test" onSubmit={onSubmit} />
    );
    await user.click(screen.getByRole("button", { name: /submit/i }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });
});
```

### Mocking API Calls in Frontend Tests

Add MSW handlers in `frontend/test/handlers/`:

```typescript
// frontend/test/handlers/my-feature-handlers.ts
import { rest } from "msw";

export const myFeatureHandlers = [
  rest.get("/api/v1/fleet/my-feature", (req, res, ctx) => {
    return res(
      ctx.json({
        my_feature: { id: 1, name: "test feature" },
      })
    );
  }),
];
```

Then register the handler in your test:

```typescript
import { server } from "test/mock-server";
import { myFeatureHandlers } from "test/handlers/my-feature-handlers";

beforeEach(() => server.use(...myFeatureHandlers));
```

---

## Coverage Requirements

While there is no hard code coverage gate enforced in CI, the following guidelines apply:

| Layer | Expected Coverage |
|---|---|
| Business logic (`server/service/`) | High — all happy paths and error branches |
| Datastore (`server/datastore/mysql/`) | High — all query paths, including edge cases |
| HTTP handlers | Medium — integration tests cover most paths |
| React components | Medium — all interactive behaviors and state changes |
| Utility functions | High — pure functions should be fully unit tested |

Focus testing on:
1. **Business logic** — all branches of service methods
2. **Error handling** — invalid inputs, network failures, permission denials
3. **Security-sensitive paths** — auth checks, input sanitization, privilege boundaries

---

## Continuous Integration

Tests run automatically on every pull request. The CI pipeline runs:

1. Go build verification
2. Go unit tests (`go test ./...`)
3. Go race condition detector (`go test -race ./...`)
4. Frontend build (`npm run build`)
5. Frontend unit tests (`npm test`)
6. TypeScript type checking (`npx tsc --noEmit`)
7. ESLint (`npx eslint frontend/`)

All checks must pass before a PR can be merged.
