# HTTP API

This document describes the HTTP API surface, the OpenAPI-first workflow that defines it,
and how routes are registered, generated, and secured. For authentication mechanics
(tokens, 2FA, WebAuthn, lockout), see [AUTHENTICATION.md](./AUTHENTICATION.md).

## OpenAPI-first contract

`api-spec/openapi.yaml` is the **single source of truth** for the HTTP contract
(OpenAPI 3.1). Server-side Go types and route registration are generated from it; you do
not hand-write request/response structs or routing tables.

```
api-spec/openapi.yaml          ← edit this first
      │  scripts/generate-server-types.sh  (oapi-codegen)
      ▼
internal/api/generated/
  ├─ types.go    request/response models
  ├─ server.go   ServerInterface + RegisterHandlersWithOptions
  ├─ spec.go     embedded spec
  └─ generate.go //go:generate directive
```

- `internal/api/generated/generate.go` carries `//go:generate bash
  ../../scripts/generate-server-types.sh`, so `go generate ./...` (or `make generate`)
  regenerates the package.
- The generator first transpiles the 3.1 spec to a 3.0-compatible temporary file
  (because `oapi-codegen` does not support some 3.1 constructs such as
  `type: [string, 'null']`), then runs `oapi-codegen`.
- **Never hand-edit files in `internal/api/generated/`.** Change the spec and regenerate.

### Keeping the frontend in sync

API changes affect the separate `ninerlog-frontend` repository, which generates its own
client from the same spec. After changing `api-spec/openapi.yaml`, regenerate both:

```bash
# in ninerlog-api
make generate                # or: bash scripts/generate-server-types.sh
# in ninerlog-frontend
bash scripts/generate-api-client.sh
```

## How a handler maps to the spec

`internal/api/generated/server.go` declares a `ServerInterface` with one method per
OpenAPI operation. `internal/api/handlers.APIHandler` implements that interface — each
operation is a method on `APIHandler` (organised across files like `flight.go`,
`auth.go`, `license.go`, …). `APIHandler` aggregates every service, so handlers stay thin:
extract `userID` from context, bind/validate the request, call a service, map the result
or sentinel error to a status code.

Routes are wired in `cmd/api/main.go`:

```go
api := router.Group("/api/v1")
api.Use(middleware.AuthMiddleware(jwtManager, /* public path allow-list */))
api.Use(middleware.RateLimitByPath(authRateLimit, /* /auth paths */))
api.Use(middleware.RateLimitByPath(adminRateLimit, /* /admin paths */))
generated.RegisterHandlersWithOptions(api, apiHandler, generated.GinServerOptions{...})
handlers.RegisterReportsRoutes(api, apiHandler, db)   // custom, not in OpenAPI spec
handlers.RegisterFlightUtilRoutes(api, apiHandler)    // custom, not in OpenAPI spec
```

A small number of routes (some reports and flight utilities) are registered manually
rather than through the generated code; they are still served under `/api/v1`.

## Base path, versioning, and non-API routes

- All business endpoints are under **`/api/v1`**.
- Non-versioned operational routes registered directly on the router:
  - `GET /health` — liveness/readiness check (used by the Docker healthcheck).
  - `GET /metrics` — Prometheus metrics (when metrics are enabled). See
    [METRICS.md](./METRICS.md).

## Security model

- **JWT bearer authentication** (`bearerAuth` in the spec). Clients send the access
  token in the HTTP `Authorization` request header using the `Bearer` scheme;
  `middleware.AuthMiddleware` validates the token and stores the `userID` on the Gin
  context.
- **Public allow-list** — auth endpoints (register, login, refresh, password reset, email
  verification) and a few read-only lookups (airport search/lookup, public announcements)
  are exempt from auth via the allow-list passed to the middleware.
- **Rate limiting** — `/auth/*` and `/admin/*` get stricter per-path limits via
  `middleware.RateLimitByPath`.
- **Admin authorization** — admin endpoints additionally require the caller to be an
  admin (configured via `ADMIN_EMAIL`).
- **Trusted proxies & forwarded IPs** are configured so client IPs are read correctly
  behind a reverse proxy.
- **Security headers** are added to every response.

Errors are returned as JSON with an appropriate status code; internal error details are
never leaked to clients.

## Endpoint catalogue

The spec defines the operations below, grouped by tag. This is a high-level map — consult
`api-spec/openapi.yaml` for exact request/response schemas, parameters, and status codes.

### Authentication
Registration, email verification (+ resend), login, token refresh, change/reset password,
TOTP 2FA (setup/verify/disable/login), and WebAuthn (register/login options + verify, list
and delete credentials).

### Users
`GET/PATCH/DELETE /users/me`, notification preferences and history, baseline
(`GET/PUT/DELETE /users/me/baseline`), personal statistics, and account-data deletion.

### Licenses
CRUD on `/licenses`, per-license statistics and currency, and nested class ratings
(`/licenses/{id}/ratings`).

### Aircraft
CRUD on `/aircraft`.

### Flights
CRUD on `/flights`, plus `DELETE /flights/delete-all` and `POST /flights/recalculate`
(re-run auto-calculations respecting overrides).

### Credentials
CRUD on `/credentials` (medicals, language proficiency, clearances).

### Currency
`GET /currency` (all ratings) and `GET /licenses/{id}/currency`.

### Maps & Reports
Airport lookup/search, route and airport statistics, trends, and stats-by-class.

### Contacts
CRUD and search on `/contacts` (reusable crew/instructor records).

### Import / Export
CSV/XLSX/JSON import (upload → preview → confirm, plus direct JSON import and import
history) and export to CSV, JSON, and PDF.

### Admin
User management (list, disable/enable, unlock, reset 2FA, delete), platform stats,
audit log, config, maintenance (token cleanup, SMTP test, trigger notifications), and
announcements.

### Backups
List providers, manage destinations (CRUD), test/run a destination, and inspect run
history. See [FEATURES.md](./FEATURES.md#cloud-backups).

### Public
`GET /announcements`.

## Conventions

- JSON field names are `camelCase`.
- Resource ownership is enforced in services: a user can only read/modify their own data;
  violations return 403.
- Pagination is used on list endpoints that can grow large (e.g. flights); see the spec
  for parameter names.

> When you add or change an endpoint, update `api-spec/openapi.yaml` first, regenerate,
> implement the handler/service, add tests, and update this document and
> [FEATURES.md](./FEATURES.md) if the feature surface changed.
