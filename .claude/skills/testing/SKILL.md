---
name: testing
description: How to write and run tests in this repo — unit vs integration vs e2e, the build tags and env vars each tier needs, running a single test, and the pre-commit gate. Use before writing a test, running the suite, or claiming work is done.
---

# Testing

All code is expected to be tested (target: 90% coverage). Tests are table-driven with `t.Run`
and `testify` assertions; repositories are mocked at the interface level for unit tests.

## Tiers

| Tier | Where | Needs | Command |
| --- | --- | --- | --- |
| Unit | next to the code | nothing | `make test` (`go test -v -short -cover ./...`) |
| Repository | `internal/repository/postgres` | Docker Postgres on :5433 | `make test-integration` |
| Tagged integration | `*_integration_test.go` | `-tags=integration` + `TEST_DB_*` | see below |
| E2E | `test/e2e/` | `-tags=e2e` + test DB | `make test-e2e` |
| Full-stack E2E | `test/e2e/` | full Docker stack on :3333 | `make test-e2e-full` |
| Performance | `test/performance/` (k6) | k6 + Docker | `make test-perf`, `make bench` |

**Build tags matter.** Files under `test/e2e/` start with `//go:build e2e` and are invisible
without `-tags=e2e`. Likewise `internal/repository/postgres/*_integration_test.go` carry
`//go:build integration` — and neither `make test-integration` nor CI passes that tag, so those
files only run when you add it yourself:

```bash
export TEST_DB_HOST=localhost TEST_DB_PORT=5433 TEST_DB_USER=testuser \
       TEST_DB_PASSWORD=testpass TEST_DB_NAME=ninerlog_test
docker compose -f docker-compose.test.yaml up -d
go test -tags=integration ./internal/repository/postgres/...
```

## Running a single test

```bash
go test -short -run TestFlightService_Create ./internal/service/
go test -tags=e2e -run TestE2E_Flights ./test/e2e/...
bash scripts/run-e2e-tests.sh TestE2E_Flights      # full stack, single test
```

`make test-e2e-full` / `scripts/run-e2e-tests.sh` brings up `docker-compose.e2e.yaml`
(API + Postgres + MailPit for email + S3/SFTP/WebDAV for backup tests), waits for
`http://localhost:3333`, runs the suite, then tears down. `--keep` leaves the stack running.

Helpers live in `internal/testutil` (`SetupTestDB`, `TeardownTestDB`, fixtures, API client).

## What to test

Business logic and validation, error paths, authn/authz, ownership enforcement, regulatory
rules (EASA/FAA currency), edge/boundary values, and that responses match the OpenAPI spec.
Do not test third-party libraries or generated code (`internal/api/generated/`).

## Before committing

```bash
make fmt
make lint
make test
make route-check
bash scripts/run-e2e-tests.sh
```

Add `make migrate-check` when `db/migrations/` changed and `make dashboard-check` when a metric
or dashboard did.

All must be green. If you find a regression, **do not** work around it in the test — fix it, or
file a GitHub issue documenting it. Never mark work complete without a green run.

When a behaviour change makes an e2e test wrong, or a red test might be stale rather than a
regression, see `.claude/skills/e2e-sync/SKILL.md`.
