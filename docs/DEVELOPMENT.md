# Development Guide

How to set up, build, test, and contribute to NinerLog API. For coding standards in
depth see [CONTRIBUTING.md](../CONTRIBUTING.md); this document focuses on the day-to-day
developer workflow and how it ties into the architecture.

## Prerequisites

- **Go** — the toolchain version pinned in `go.mod` (`go` directive).
- **PostgreSQL** — provided via Docker for local development.
- **Docker & Docker Compose** — for the database and the e2e/perf stacks.
- **golangci-lint** — for linting.
- **k6** (optional) — for performance tests.

## First-time setup

```bash
cp .env.example .env       # adjust values as needed
go mod download
make docker-up             # start PostgreSQL (docker-compose.yml)
make run                   # runs migrations automatically, then starts the server
```

Migrations are applied **automatically at startup** (`m.Up()` in `cmd/api/main.go`), so
`make run` brings the schema up to date. The server listens on `PORT` (default 3000);
check `GET /health`.

## Common commands (Makefile)

| Command | What it does |
| --- | --- |
| `make run` | Run the API (applies migrations, starts server) |
| `make build` | Build the binary |
| `make generate` | Regenerate OpenAPI server types (`scripts/generate-server-types.sh`) |
| `make fmt` | `go fmt ./...` |
| `make lint` | `golangci-lint run` |
| `make test` | Unit tests (`-short`) |
| `make test-integration` | Integration tests (spins up a test DB via Docker) |
| `make test-e2e` / `make test-e2e-full` | End-to-end tests (full Docker stack) |
| `make test-all` | Unit + integration + e2e |
| `make coverage` | HTML coverage report |
| `make bench` | Go benchmarks |
| `make test-perf` / `make test-perf-seed` | k6 performance tests / seed data |
| `make profile` / `profile-pprof` / `profile-explain` | Profiling (pprof + `EXPLAIN ANALYZE`) |
| `make migrate-up` / `migrate-down` | Apply / roll back migrations manually |
| `make migrate-create NAME=...` | Scaffold a new migration pair |
| `make docker-up` / `docker-down` / `docker-logs` | Manage the local Docker stack |

> The Makefile also exposes a `sqlc-generate` target, but the repository currently has no
> `sqlc.yaml`/`db/queries`; repositories in `internal/repository/postgres` are written by
> hand using parameterized SQL. Treat `sqlc-generate` as inactive unless that tooling is
> introduced.

## The OpenAPI-first workflow

API behaviour is defined by `api-spec/openapi.yaml` **first**, then generated, then
implemented. The full loop:

1. Edit `api-spec/openapi.yaml`.
2. `make generate` — regenerates `internal/api/generated/*` via `oapi-codegen`.
3. Implement/adjust the handler method on `APIHandler` and the backing service.
4. Regenerate the **frontend** client too:
   `cd ../ninerlog-frontend && bash scripts/generate-api-client.sh`.
5. Add/adjust tests; run the suites below.
6. Update the relevant docs (see [Documentation](#documentation)).

Never hand-edit `internal/api/generated/`. See [API.md](./API.md).

## Testing

Testing is mandatory — all code must be covered by unit, integration, and (where relevant)
e2e tests. See [RUNNING_TESTS.md](./RUNNING_TESTS.md) for environment details and
troubleshooting.

- **Unit** (`make test`) — pure logic with mocked repositories. Use table-driven tests
  with `t.Run(...)` and `testify` assertions.
- **Integration** (`make test-integration`) — services/repositories against a real
  PostgreSQL test instance (`docker-compose.test.yaml`).
- **End-to-end** (`make test-e2e` / `-full`) — the full stack via
  `docker-compose.e2e.yaml` (API + Postgres + MailPit for email + S3/SFTP/WebDAV for
  backup tests); Go tests in `test/e2e/`.
- **Performance** (`make test-perf`) — k6 scenarios in `test/performance/`. See
  [PERFORMANCE.md](./PERFORMANCE.md).

### Pre-commit / pre-push checklist

Before committing or pushing:

1. `make fmt`
2. `make lint`
3. `make test` (unit) — must be green
4. `make test-e2e` (e2e) — must be green

Do not push with failing tests. If you discover a regression, **document it as a GitHub
issue** rather than working around it in tests.

## Coding conventions

- **Layering** — handlers call services; services call repository interfaces; no SQL in
  handlers, no Gin in services. See [ARCHITECTURE.md](./ARCHITECTURE.md).
- **Errors** — wrap with context; return sentinel errors from services; never leak raw
  internal errors to clients.
- **Ownership** — every user-scoped operation must verify the resource belongs to the
  authenticated user.
- **Validation** — validate text-field lengths (`internal/models/validation.go`); use
  parameterized SQL exclusively.
- **Durations** — integer minutes everywhere; convert only for display via `pkg/duration`.
- **Security** — secrets via environment variables only; never log JWTs; bcrypt for
  passwords; AES-256-GCM for stored backup credentials.

## Project structure

```
api-spec/      OpenAPI 3.1 spec (source of truth)
cmd/api/       main.go entry point + wiring
db/migrations/ ordered SQL migrations (auto-applied at startup)
docs/          developer documentation (this directory)
internal/      private app code (handlers, middleware, services, repositories, models, ...)
pkg/           reusable utilities (jwt, hash, duration, cryptoutil, email, solar)
scripts/       generate-server-types.sh, run-e2e-tests.sh, run-perf-tests.sh, run-profiling.sh, run-all-tests.sh
test/          e2e (Go) and performance (k6) suites
```

See [PACKAGES.md](./PACKAGES.md) for a package-by-package reference.

## CI/CD

GitHub Actions workflows live in `.github/workflows/`:

- **ci.yml** — unit tests, integration tests, and linting on push/PR.
- **e2e.yml** — manual end-to-end suite against the Docker stack.
- **generate-server.yml** — regenerates server types when the spec or generator changes,
  verifies the build, and commits regenerated code.
- **performance.yml** — Go benchmarks and k6 load tests (manual / labelled PRs).
- **security-audit.yml** — `govulncheck` and `gosec` on push/PR and a weekly schedule.

## Documentation

Documentation under `docs/` is part of the codebase and must stay accurate. When a change
affects behaviour, update the relevant document(s) in the **same PR**:

| If you change… | Update… |
| --- | --- |
| The HTTP contract / endpoints | `api-spec/openapi.yaml`, [API.md](./API.md), [FEATURES.md](./FEATURES.md) |
| Domain rules (flights, currency, validation, time) | [DOMAIN.md](./DOMAIN.md) |
| Entities or migrations | [DATA_MODEL.md](./DATA_MODEL.md) |
| Packages / responsibilities / wiring | [PACKAGES.md](./PACKAGES.md), [ARCHITECTURE.md](./ARCHITECTURE.md) |
| A product feature | [FEATURES.md](./FEATURES.md) |
| Auth / metrics / performance / tests tooling | the matching deep-dive doc |

This expectation is encoded for AI-assisted changes in
[`.github/copilot-instructions.md`](../.github/copilot-instructions.md#documentation-maintenance).
