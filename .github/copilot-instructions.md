# GitHub Copilot Instructions for NinerLog API

You are assisting with `ninerlog-api`, the Go backend for NinerLog — an EASA/FAA compliant
digital pilot logbook. Sibling repositories: `ninerlog-frontend` (React PWA, generates its API
client from the same OpenAPI spec), `ninerlog` (self-hosted Docker Compose deployment), and
`ninerlog-website`.

Developer documentation in [`docs/`](../docs/README.md) is part of the codebase and is the
authority on how this system works — start at [`docs/DEVELOPER_GUIDE.md`](../docs/DEVELOPER_GUIDE.md).
Claude Code users get the same guidance via `CLAUDE.md` and the skills in `.claude/skills/`.

## Tech stack

Go (toolchain pinned in `go.mod`) · Gin · PostgreSQL via `lib/pq` with **hand-written
parameterized SQL** · `golang-migrate` · `oapi-codegen` (server types from OpenAPI 3.1) ·
`golang-jwt/jwt/v5` + TOTP 2FA + WebAuthn · `testify` · `prometheus/client_golang` ·
`go-pdf/fpdf` (logbook export).

There is no ORM, no sqlc, and no pgx. Repositories write SQL by hand.

## Architecture

Strict layering: **handler → service → repository → models**.

- `cmd/api/main.go` — all composition and dependency injection: environment config, DB open +
  automatic migration, airport database load, repositories, services, the currency evaluator
  registry, optional subsystems (WebAuthn, cloud backups, pprof), the Gin router and middleware
  chain, route registration, background workers, graceful shutdown.
- `internal/api/handlers` — `APIHandler` implements the generated `ServerInterface`, one method
  per OpenAPI operation. Handlers are thin: read `userID` from the Gin context, bind and
  validate, call a service, map sentinel errors to status codes. **Handlers never touch SQL.**
- `internal/service` — all business logic, ownership checks, validation orchestration.
  **Services never import Gin** — they take `context.Context` and are reused by background jobs.
  Sub-engines: `currency/`, `flightcalc/`, `flightrules/`, `cloudbackup/`, `customfield/`.
- `internal/repository` — interfaces; `internal/repository/postgres` — PostgreSQL
  implementations returning domain models.
- `internal/models` — plain structs and validation helpers, no I/O.
- `pkg/` — dependency-light utilities: `jwt`, `hash`, `duration`, `cryptoutil`, `email`, `solar`.

See [`docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md) and
[`docs/PACKAGES.md`](../docs/PACKAGES.md).

## API-first development

`api-spec/openapi.yaml` (OpenAPI 3.1) is the **single source of truth** for the HTTP contract.

1. Edit the spec first.
2. Regenerate server types: `make generate` (`bash scripts/generate-server-types.sh`).
3. Implement the handler method and the backing service.
4. Regenerate the frontend client:
   `cd ../ninerlog-frontend && bash scripts/generate-api-client.sh`.
5. Add tests, then update the docs.

**Never hand-edit `internal/api/generated/`** — the generator wipes and rewrites it. The script
transpiles 3.1 → 3.0 with `sed` first because `oapi-codegen` rejects constructs such as
`type: [string, 'null']`; unsupported constructs need a new rule in the script, not a weakened
spec.

Endpoints are authenticated by default. Public routes must be added to the allow-list passed to
`middleware.AuthMiddleware` in `cmd/api/main.go` *and* documented in the spec.
See [`docs/API.md`](../docs/API.md).

## Domain invariants

- **Durations are integer minutes** everywhere — database, models, JSON. Decimal hours are a
  display concern; convert with `pkg/duration`. Times of day (`OffBlockTime`, `DepartureTime`, …)
  are separate: `HH:MM:SS` strings in UTC.
- **Auto-calculated flight fields have `*Override` flags.** Anything that recalculates —
  including `POST /flights/recalculate` — must respect them.
- **Currency is a registry of evaluators** keyed by regulatory authority (EASA, FAA, German UL,
  plus an expiry-only `Other` fallback). Evaluators never write SQL; they request aggregates
  through the `FlightDataProvider` interface.
- **Ownership is enforced in services**, not handlers: every user-scoped operation verifies the
  resource belongs to the authenticated user, returning 403 otherwise.
- **Errors** — wrap with context (`fmt.Errorf("...: %w", err)`), return sentinel errors from
  services (`ErrFlightNotFound`, `ErrUnauthorizedFlight`, …), and never leak raw internal error
  text to clients.
- **Validation** lives at the model/service layer, including text-length limits in
  `internal/models/validation.go`. Use parameterized SQL exclusively.

See [`docs/DOMAIN.md`](../docs/DOMAIN.md).

## Testing

**All code must be tested** (target: 90% coverage). Table-driven tests with `t.Run` and
`testify`; repository interfaces are mocked for unit tests.

| Tier | Location | Command |
| --- | --- | --- |
| Unit | beside the code | `make test` (`go test -short ./...`) |
| Repository | `internal/repository/postgres` | `make test-integration` (Docker Postgres on :5433) |
| Tagged integration | `*_integration_test.go` (`//go:build integration`) | `make test-integration-tagged` |
| End-to-end | `test/e2e/` (`//go:build e2e`) | `make test-e2e` / `make test-e2e-full` |
| Performance | `test/performance/` (k6) | `make test-perf`, `make bench` |

Build tags matter: e2e and tagged-integration files are invisible to a plain `go test`.

Run a single test:

```bash
go test -short -run TestFlightService_Create ./internal/service/
bash scripts/run-e2e-tests.sh -t TestE2E_Flights
```

Do not test third-party libraries or generated code.

### Before committing or pushing

```bash
make fmt
make lint
make test
bash scripts/run-e2e-tests.sh
```

All must be green in this repo **and** in `ninerlog-frontend` (`npx vitest run`,
`npx playwright test`) when a change spans both. Never commit with failing tests.

### Regressions

**Never work around a regression in a test.** Fix it, or document it as a GitHub issue so it can
be planned into the roadmap. Regressions must not be hidden behind test workarounds.

## Database migrations

`db/migrations/` holds `golang-migrate` pairs applied **automatically at startup**.

- `make migrate-create NAME=add_foo` scaffolds `NNNNNN_name.{up,down}.sql`.
- `make migrate-check` enforces the naming pattern, rejects duplicate version numbers (a
  duplicate crashes the API before it serves a request), and requires an up/down pair.
- Never edit a merged migration — add a new one.
- Conventions: `UUID PRIMARY KEY DEFAULT gen_random_uuid()`, `TIMESTAMPTZ` timestamps with the
  shared `update_updated_at_column()` trigger, `ON DELETE CASCADE` for user-owned rows, and
  `INTEGER` minutes for durations.

See [`docs/DATA_MODEL.md`](../docs/DATA_MODEL.md).

## Documentation maintenance

Docs must always reflect reality — treat them as a first-class deliverable and update them in
the **same pull request** as the behaviour change.

| If you change… | Update… |
| --- | --- |
| HTTP contract / endpoints | `api-spec/openapi.yaml`, [`docs/API.md`](../docs/API.md), [`docs/FEATURES.md`](../docs/FEATURES.md) |
| Domain rules (flights, currency, validation, time) | [`docs/DOMAIN.md`](../docs/DOMAIN.md) |
| Entities, schema, migrations | [`docs/DATA_MODEL.md`](../docs/DATA_MODEL.md) |
| Packages, responsibilities, wiring | [`docs/PACKAGES.md`](../docs/PACKAGES.md), [`docs/ARCHITECTURE.md`](../docs/ARCHITECTURE.md) |
| A product feature | [`docs/FEATURES.md`](../docs/FEATURES.md) |
| Auth, metrics, performance, test tooling | the matching deep-dive doc in `docs/` |

**Never leave documentation describing behaviour that no longer exists.** If you cannot fully
update a doc, note the gap explicitly in the PR. Do not hand-edit generated artefacts —
regenerate them.

## Conventions

- Conventional Commits: `feat(flights): add night currency validation`.
- Branch from `main` with a `feature/`, `fix/`, or `docs/` prefix.
- JSON field names are `camelCase`; list endpoints that can grow large are paginated.
- Secrets come from environment variables only; bcrypt for passwords, AES-256-GCM for stored
  backup credentials; never log JWTs.
- No emoji in code, docs, scripts, or tooling output.
