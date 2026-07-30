---
name: api-change
description: The OpenAPI-first workflow for this repo. Use whenever adding, removing, or changing an HTTP endpoint, request/response field, status code, query parameter, or auth requirement — and before touching anything under internal/api/generated/ or api-spec/openapi.yaml.
---

# Changing the HTTP API

`api-spec/openapi.yaml` (OpenAPI 3.1) is the **single source of truth**. Go server types and
route registration are generated from it. Never hand-write request/response structs or routing
tables, and never hand-edit `internal/api/generated/`.

## The loop

1. **Edit `api-spec/openapi.yaml`** first — path, schema, parameters, status codes, tag, and
   whether the operation needs `bearerAuth`.
2. **`make generate`** (= `scripts/generate-server-types.sh` + `go mod tidy`) regenerates
   `internal/api/generated/{types.go,server.go,spec.go}`.
3. **Implement the handler method** on `APIHandler` in `internal/api/handlers/` (one file per
   resource). Keep it thin: `getUserIDFromContext`, bind/validate the body, call the service,
   map sentinel errors to status codes.
4. **Implement/extend the service** in `internal/service/` — this is where validation, ownership
   checks, and business rules live.
5. **Regenerate the frontend client**:
   `cd ../ninerlog-frontend && bash scripts/generate-api-client.sh`.
6. **Tests** — unit tests for the service, e2e coverage in `test/e2e/` for the endpoint.
7. **Docs** — `docs/API.md` and `docs/FEATURES.md` in the same PR (see the `docs-sync` skill).

## Generator gotchas

- `oapi-codegen` does not support some OpenAPI 3.1 constructs, so
  `scripts/generate-server-types.sh` transpiles 3.1 → 3.0 with `sed` first
  (`type: [string, 'null']` → `type: string` + `nullable: true`, and similar for
  integer/number/enum). If a new nullable or union construct is not handled, add a sed rule
  there rather than dumbing down the spec.
- The script wipes `internal/api/generated/*.go` before regenerating, so any manual edit there
  is silently lost.
- `internal/api/generated/generate.go` carries the `//go:generate` directive, so
  `go generate ./...` works too.

## Routing and security

Routes are wired in `cmd/api/main.go`:

```go
api := router.Group("/api/v1")
api.Use(middleware.AuthMiddleware(jwtManager, /* public path allow-list */))
api.Use(middleware.RateLimitByPath(authRateLimit, /* /auth paths */))
generated.RegisterHandlersWithOptions(api, apiHandler, generated.GinServerOptions{...})
handlers.RegisterReportsRoutes(api, apiHandler, db)   // custom, not in the spec
handlers.RegisterFlightUtilRoutes(api, apiHandler)    // custom, not in the spec
```

- Endpoints are **authenticated by default**. A public endpoint must be added to the allow-list
  passed to `AuthMiddleware` *and* reflected in the spec.
- `/auth/*` and `/admin/*` get stricter rate limits; admin endpoints additionally require an
  admin caller (`ADMIN_EMAIL`).
- JSON field names are `camelCase`. Ownership violations return 403. List endpoints that can
  grow large are paginated.
- Never return raw internal errors to clients — use a static, user-friendly message.
