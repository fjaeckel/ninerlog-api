# Running Tests

This guide covers how to run the NinerLog API test suites.

## Prerequisites

**Local development:**
- Go 1.26.7+
- Docker & Docker Compose (for integration and E2E tests)

**Docker only (no local Go needed):**
- Docker & Docker Compose

---

## Running Tests via Docker

You can run all tests without installing Go locally. The API Dockerfile builds a static Go binary, and the test infrastructure runs entirely in containers.

### Full E2E Suite in Docker

Runs the API, Postgres, and MailPit in containers, then executes E2E tests:

```bash
make test-e2e-full
# or
bash scripts/run-e2e-tests.sh
```

This spins up `docker-compose.e2e.yaml` which includes:
- **postgres-e2e** — Ephemeral Postgres on port 5434 (tmpfs, no persistence)
- **mailpit** — SMTP test server (web UI at http://localhost:8025)
- **api** — Built from Dockerfile, connected to both services, port 3333

The script waits for health checks, runs `go test -tags=e2e ./test/e2e/...`, and tears everything down.

> **Tip:** Use `bash scripts/run-e2e-tests.sh -k` to keep the Docker environment running after tests finish. This lets you re-run tests quickly without waiting for containers to start again.

> **Tip:** Filter tests with `bash scripts/run-e2e-tests.sh TestEASA` to run only matching tests.

### All Tests via Script

```bash
make test-all
# or
bash scripts/run-all-tests.sh
```

This runs the full pipeline:
1. Starts Postgres test container
2. Runs migrations
3. Runs unit tests
4. Runs integration tests
5. Runs E2E tests
6. Tears down containers

### Workspace-Level (Both Repos)

From the workspace root:

```bash
bash run-all-tests.sh
```

Runs both API and frontend test suites in sequence.

### Docker Compose Files

| File | Purpose |
|---|---|
| `docker-compose.test.yaml` | Lightweight Postgres for unit/integration tests (port 5433) |
| `docker-compose.e2e.yaml` | Full stack — Postgres + MailPit + API for E2E (port 3333) |
| `docker-compose.yml` | Local dev stack (Postgres + API) |

---

## Running Tests Locally

### Unit Tests

Unit tests run in-process with mocked dependencies. No database required.

```bash
# Run all unit tests with coverage
make test

# Or directly with go test
go test -v -short ./...

# Run tests for a specific package
go test -v -short ./internal/service/...
go test -v -short ./pkg/jwt/...

# Run a specific test by name
go test -v -short -run TestHashPassword ./pkg/hash/...
```

## Integration Tests

Integration tests run against a real PostgreSQL database in Docker.

```bash
# Run integration tests (starts/stops test DB automatically)
make test-integration
```

This command will:
1. Start a PostgreSQL container on port 5433
2. Run database migrations
3. Execute repository-layer tests with a real database
4. Tear down the container

### Manual Database Setup

If you prefer to manage the test database yourself:

```bash
# Start the test database
docker compose -f docker-compose.test.yaml up -d

# Wait for it to be ready, then seed the schema
sleep 3
./scripts/seed-test-db.sh

# Run integration tests with env vars
export TEST_DB_HOST=localhost TEST_DB_PORT=5433 \
       TEST_DB_USER=testuser TEST_DB_PASSWORD=testpass \
       TEST_DB_NAME=ninerlog_test
go test -v -tags=integration ./internal/repository/postgres/...

# Clean up
docker compose -f docker-compose.test.yaml down
```

### How the schema is seeded

`scripts/seed-test-db.sh` applies `db/migrations/*.up.sql` in order and then records the
resulting version in golang-migrate's `schema_migrations` table, so an API process pointed
at the same database starts cleanly instead of trying to replay migration 1.

It replaces the hand-maintained `db/migrations/test_init.sql`, which had to be updated by
hand for every migration and had fallen behind the real schema. Adding a migration now
requires no test-harness change.

To seed a database outside Docker, override the psql invocation:

```bash
PSQL_CMD="psql -h 127.0.0.1 -p 5433 -U testuser -d ninerlog_test" ./scripts/seed-test-db.sh
```

> Integration tests carry a `//go:build integration` tag, so `-tags=integration` is
> required — without it the tier compiles and runs nothing.

## Multi-Replica WebAuthn Check

Passkey ceremonies span two requests (`options` then `verify`). Ceremony state lives
in Postgres so those requests may land on different instances — this check proves it,
and is the acceptance test for that design.

```bash
make verify-multi-replica     # needs a Postgres reachable on :5433
```

It starts two API instances against one database and asserts that a ceremony begun on
one is consumable on the other, that consumption is single-use *across* instances, that
a ceremony survives the death of the process that started it, that an expired handle is
refused while its row still exists, that a handle cannot be redeemed by another account,
and that no raw handle reaches the logs.

Ceremonies are finished with a deliberately invalid credential payload — verifying a real
attestation needs a real authenticator — so the signal is *which* error comes back:
`Invalid ... response` means the session was found and consumed, `session expired` means
it was refused. See [AUTHENTICATION.md](./AUTHENTICATION.md#passkeys-webauthn).

## End-to-End Tests

E2E tests validate complete API flows (register, login, CRUD operations, etc.).

```bash
# Run E2E tests (starts/stops test DB automatically)
make test-e2e
```

### Full E2E Suite (Docker Compose)

Runs the API and database together in Docker for a fully isolated test:

```bash
make test-e2e-full
# or
bash scripts/run-e2e-tests.sh
```

## Running All Tests

Run unit, integration, and E2E tests in sequence:

```bash
make test-all
# or
bash scripts/run-all-tests.sh
```

## Coverage Report

Generate an HTML coverage report:

```bash
make coverage
open coverage.html
```

## Linting

```bash
make lint
```

Requires [golangci-lint](https://golangci-lint.run/welcome/install/).

## Quick Reference

| Command | What it does |
|---|---|
| `make test` | Unit tests with coverage |
| `make test-short` | Unit tests (alias for `make test`) |
| `make test-integration` | Integration tests (Docker DB) |
| `make test-e2e` | E2E tests (Docker DB) |
| `make test-e2e-full` | Full E2E suite via Docker Compose |
| `make test-all` | All tests in sequence |
| `make coverage` | HTML coverage report |
| `make lint` | golangci-lint |
| `bash scripts/run-e2e-tests.sh` | Full E2E in Docker (API + DB + MailPit) |
| `bash scripts/run-e2e-tests.sh -k` | E2E in Docker, keep env running |
| `bash scripts/run-all-tests.sh` | All tests (Docker pipeline) |

## Environment Variables

Integration and E2E tests use these variables (set automatically by Makefile targets):

| Variable | Value | Description |
|---|---|---|
| `TEST_DB_HOST` | `localhost` | Test database host |
| `TEST_DB_PORT` | `5433` | Test database port (avoids conflict with dev) |
| `TEST_DB_USER` | `testuser` | Test database user |
| `TEST_DB_PASSWORD` | `testpass` | Test database password |
| `TEST_DB_NAME` | `ninerlog_test` | Test database name |

## Troubleshooting

**Database connection errors:**
- Ensure Docker is running: `docker info`
- Check if port 5433 is available: `lsof -i :5433`
- Wait a few seconds after starting the container

**Migration errors:**
- Check that `psql` client is available in the Docker container
- `scripts/seed-test-db.sh` stops on the first failing migration and names the file
- Seeding an already-seeded database will fail on duplicate objects; drop and recreate it first

**Test timeouts:**
- Integration tests: ~10–15 seconds
- E2E tests: ~20–30 seconds
- Database startup: ~2–3 seconds

## Further Reading

- [docs/TESTING.md](TESTING.md) — Comprehensive testing guide (architecture, writing tests)
- [docs/TEST_IMPLEMENTATION.md](TEST_IMPLEMENTATION.md) — Test infrastructure details
