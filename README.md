# NinerLog API

RESTful backend for [NinerLog](https://ninerlog.com) — a free, open-source, EASA/FAA compliant digital pilot logbook with multi-license tracking, currency evaluation, and PDF export.

## Tech Stack

- **Go** / **Gin** / **PostgreSQL**
- **JWT** (access + refresh tokens) with **TOTP 2FA**
- **lib/pq** (Postgres driver) / **golang-migrate** (schema migrations)
- **oapi-codegen** (OpenAPI server types) / **kin-openapi** (spec parsing)
- **fpdf** (PDF logbook export)

## Prerequisites

- Go
- PostgreSQL
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

## Environment Variables

See `.env.example` for a complete list of configuration options including database connection, JWT secrets, CORS settings, SMTP configuration, and TLS settings.

## Docker

```bash
# Build and run with Docker Compose
docker compose up -d

# Or build the image directly
docker build -t ninerlog-api .
```

## Documentation

- [Running Tests](docs/RUNNING_TESTS.md) — Unit, integration, and E2E testing guide
- [API Specification](https://github.com/fjaeckel/ninerlog-project/blob/main/api-spec/openapi.yaml) — OpenAPI 3.1 spec (source of truth)
- [OpenAPI Quick Reference](https://github.com/fjaeckel/ninerlog-project/blob/main/OPENAPI_QUICKREF.md) — Code generation workflow
- [Architecture](https://github.com/fjaeckel/ninerlog-project/blob/main/docs/architecture/README.md) — System design decisions
- [Database Schema](https://github.com/fjaeckel/ninerlog-project/blob/main/docs/database/schema.md) — PostgreSQL schema documentation

## Related Repositories

| Repository | Description |
|---|---|
| [ninerlog-project](https://github.com/fjaeckel/ninerlog-project) | Project planning, documentation, OpenAPI spec |
| [ninerlog-frontend](https://github.com/fjaeckel/ninerlog-frontend) | React/TypeScript PWA frontend |
| [ninerlog-website](https://github.com/fjaeckel/ninerlog-website) | Marketing website |

## Contributing

See [CONTRIBUTING.md](https://github.com/fjaeckel/ninerlog-project/blob/main/CONTRIBUTING.md) for development guidelines.

## Security

To report a vulnerability, see [SECURITY.md](SECURITY.md).
