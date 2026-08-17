# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Go/Gin/PostgreSQL backend for NinerLog, an EASA/FAA compliant digital pilot logbook.
Sibling repos: `ninerlog-frontend` (React PWA, generates its API client from the same
OpenAPI spec), `ninerlog` (self-hosted Docker Compose), `ninerlog-website`.

Developer documentation in `docs/` is part of the codebase and must stay accurate —
start at `docs/DEVELOPER_GUIDE.md`.

> `.github/copilot-instructions.md` is stale (TypeScript/Prisma/sqlc examples that do not
> reflect this codebase). Prefer `docs/` and the skills below.

## Commands

```bash
make run                 # migrations auto-apply at startup, then serve (PORT, default 3000)
make build               # → bin/ninerlog-api
make generate            # regenerate internal/api/generated/ from api-spec/openapi.yaml
make fmt && make lint    # go fmt ./... ; golangci-lint run
make test                # unit tests (-short)
make test-integration    # repository tests against Docker Postgres on :5433
make test-e2e            # test DB + go test -tags=e2e ./test/e2e/...
make test-e2e-full       # full Docker stack e2e (scripts/run-e2e-tests.sh)
make migrate-create NAME=add_foo
make migrate-check       # duplicate versions / missing up-down pairs
```

Details on tiers, build tags, and running a single test: `.claude/skills/testing/SKILL.md`.

## Architecture

Strict layering: **handler → service → repository → models**.

- `cmd/api/main.go` — all composition/DI: config, DB + auto-migration, airport DB, repositories,
  services, currency evaluator registry, optional subsystems, router, background workers.
- `internal/api/handlers` — one method per OpenAPI operation on the aggregate `APIHandler`,
  which implements the generated `ServerInterface`. Thin: read `userID` from the Gin context,
  bind/validate, call a service, map sentinel errors to status codes. Never touches SQL.
- `internal/service` — all business logic, ownership checks, validation. Never imports Gin
  (services are reused by background jobs). Sub-engines: `currency/`, `flightcalc/`,
  `flightrules/`, `cloudbackup/`, `customfield/`.
- `internal/repository` (interfaces) + `internal/repository/postgres` (hand-written
  parameterized SQL, `lib/pq`). No sqlc/pgx despite older docs; `make sqlc-generate` is inactive.
- `pkg/` — `jwt`, `hash`, `duration`, `cryptoutil`, `email`, `solar`.

`/api/v1` sits behind `AuthMiddleware` (JWT, public-path allow-list) and `RateLimitByPath`.
Most routes come from `generated.RegisterHandlersWithOptions`; reports and flight utilities are
registered manually in `main.go` and are **not** in the OpenAPI spec.
Diagrams: `docs/ARCHITECTURE.md`. Package reference: `docs/PACKAGES.md`.

## Rules that always apply

1. **OpenAPI-first** — `api-spec/openapi.yaml` is the single source of truth; never hand-edit
   `internal/api/generated/`. See `.claude/skills/api-change/SKILL.md`.
2. **Docs in the same PR** as the behaviour change. See `.claude/skills/docs-sync/SKILL.md`.
3. **Tests must be green before committing** (`make fmt`, `make lint`, `make test`,
   `bash scripts/run-e2e-tests.sh`). Never weaken or skip a test to hide a regression — file a
   GitHub issue instead. A behaviour change updates its e2e tests in the same PR; see
   `.claude/skills/e2e-sync/SKILL.md`, which also covers telling a stale test from a regression.
4. **Domain invariants** (durations in integer minutes, `*Override` flags, currency evaluator
   registry, service-layer ownership checks, sentinel errors) — see
   `.claude/skills/aviation-domain/SKILL.md`.
5. **A feature ships with its operator surfaces.** Anything with counts, an env-var toggle,
   an optional subsystem or an admin action must show up under `/admin/*` — see
   `.claude/skills/admin-surface/SKILL.md`. Anything with a background job, external
   dependency, cache or limiter must ship its metric, `docs/METRICS.md` row, Grafana panel
   and (where a human should act) an alert rule — see
   `.claude/skills/metrics-dashboards/SKILL.md`.
6. Conventional Commits (`feat(flights): …`); branch from `main`.
7. **Comments state the what, never the why** — rationale lives in commit messages and
   `docs/`. See `.claude/skills/terse-comments/SKILL.md`.
8. **Security findings are never committed or pushed** — no audit reports, vulnerability
   write-ups, exploit fixtures, or commit/PR text describing an unfixed weakness. Write them to
   the gitignored `security-audits/` and report privately (`SECURITY.md`, or a GitHub Security
   Advisory). Fixes get pushed; findings do not. See `.claude/skills/security-audit/SKILL.md`.

## Delegation

Definitions in `.claude/agents/`. Keep design decisions and the final review in the main session;
delegate execution once the shape of the change is settled.

| Agent | Use for |
| --- | --- |
| `endpoint-implementer` | A decided API change, spec → handler → service → repo → tests → docs |
| `migration-author` | A decided schema change, migration pair + model/repository follow-up |
| `test-writer` | Backfilling tests for existing code; never edits production code |
| `docs-syncer` | Bringing `docs/` back in line with an implemented change |
| `code-locator` | "Where does X live?" — returns a `file:line` inventory, read-only |
| `gate-runner` | `fmt`/`lint`/`test`/`migrate-check` — reports failures verbatim, fixes nothing |

Give implementers the full contract (fields, status codes, ownership rules) up front — they are
told not to invent missing decisions.
