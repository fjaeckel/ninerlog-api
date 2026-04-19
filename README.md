# NinerLog API

RESTful backend for NinerLog — an EASA/FAA compliant digital pilot logbook with multi-license tracking, currency evaluation, and PDF export.

## Tech Stack

- **Go 1.25** / **Gin** / **PostgreSQL 18**
- **JWT** (access + refresh tokens) with **TOTP 2FA**
- **lib/pq** (Postgres driver) / **golang-migrate** (schema migrations)
- **oapi-codegen** (OpenAPI server types) / **kin-openapi** (spec parsing)
- **fpdf** (PDF logbook export)

## Prerequisites

- Go 1.25+
- PostgreSQL 18
- Docker & Docker Compose (recommended)

## Quick Start

```bash
# Start PostgreSQL
make docker-up

# Run migrations & start server
make run

# Run unit tests
make test

# Run integration tests (spins up test DB via Docker)
make test-integration
```

## Make Targets

| Target | Description |
|---|---|
| `make run` | Start the API server |
| `make build` | Build binary to `bin/ninerlog-api` |
| `make generate` | Generate Go types from OpenAPI spec |
| `make test` | Unit tests with coverage |
| `make test-short` | Unit tests (skip slow) |
| `make test-integration` | Integration tests (Docker test DB) |
| `make test-e2e` | End-to-end tests |
| `make test-all` | All tests |
| `make coverage` | Generate HTML coverage report |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code |
| `make migrate-up` | Apply all pending migrations |
| `make migrate-down` | Roll back last migration |
| `make migrate-create NAME=...` | Create a new migration |
| `make docker-up` / `docker-down` | Manage Docker containers |

## Project Structure

```
cmd/api/main.go              # Entry point (config, DI, graceful shutdown)
internal/
├── api/
│   ├── generated/           # OpenAPI codegen output (do not edit)
│   ├── handlers/            # HTTP request handlers
│   └── middleware/          # CORS, request logging
├── airports/                # OurAirports database loader
├── config/                  # Environment configuration
├── models/                  # Domain models
├── repository/
│   ├── interfaces.go        # Repository contracts
│   └── postgres/            # PostgreSQL implementations
├── service/                 # Business logic
│   ├── currency/            # EASA/FAA currency evaluators
│   └── flightcalc/          # Flight time & solar calculations
└── testutil/                # Test helpers & fixtures
pkg/
├── email/                   # SMTP email sender
├── hash/                    # Password hashing (bcrypt)
├── jwt/                     # JWT token management
└── solar/                   # Solar position calculations (night time)
db/migrations/               # SQL migrations (25 total)
test/e2e/                    # End-to-end tests
```

## API Endpoints

| Group | Endpoints |
|---|---|
| **Auth** | Register, login, refresh, password reset, 2FA setup/verify |
| **User** | Profile read/update |
| **Licenses** | CRUD, set default |
| **Class Ratings** | CRUD (per license) |
| **Flights** | CRUD with crew, block times, instrument tracking |
| **Aircraft** | CRUD |
| **Credentials** | CRUD (medicals, certificates) |
| **Currency** | Evaluate recency per license (EASA/FAA) |
| **Notifications** | List, mark read |
| **Contacts** | CRUD (crew/people) |
| **Reports** | Statistics, totals by time period |
| **Maps** | Airport search & lookup |
| **Import** | CSV flight log import |
| **Export** | Logbook PDF export |

All endpoints are under `/api/v1`. Health check at `GET /health`.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `postgresql://ninerlog:changeme@localhost:5432/ninerlog?sslmode=disable` | PostgreSQL connection |
| `PORT` | `3000` | Server port |
| `JWT_SECRET` | — | Access token signing secret |
| `REFRESH_SECRET` | — | Refresh token signing secret |
| `CORS_ORIGIN` | `http://localhost:5173` | Comma-separated allowed origins |
| `MIGRATIONS_PATH` | `db/migrations` | Path to migration files |
| `SMTP_HOST` | — | SMTP server (for password reset emails) |
| `SMTP_PORT` | `587` | SMTP port |
| `SMTP_USERNAME` | — | SMTP username |
| `SMTP_PASSWORD` | — | SMTP password |
| `SMTP_FROM` | `noreply@ninerlog.app` | Sender address |

## Docker

Multi-stage build: `golang:1.25-alpine` (build) → `alpine:3.19` (runtime). Runs as non-root user. Health check via `wget` on `/health`.

```bash
# Build and run with Docker Compose (from workspace root)
docker compose -f docker-compose.dev.yml up -d
```

See [DOCKER.md](../DOCKER.md) for full deployment guide.

## Documentation

- [Running Tests](docs/RUNNING_TESTS.md) — How to run unit, integration, and E2E tests
- [Testing Guide](docs/TESTING.md) — Test architecture and writing tests
- [OpenAPI Compliance](docs/OPENAPI_COMPLIANCE.md) — API spec compliance details

## Related Repositories

- [ninerlog-project](../ninerlog-project) — OpenAPI spec & project planning
- [ninerlog-frontend](../ninerlog-frontend) — React/TypeScript web frontend
