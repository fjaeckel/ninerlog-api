---
name: api-change
description: The OpenAPI-first workflow for this repo. Use whenever adding, removing, or changing an HTTP endpoint, request/response field, status code, query parameter, or auth requirement — and before touching anything under internal/api/generated/ or api-spec/openapi.yaml.
---

# Changing the HTTP API

`api-spec/openapi.yaml` (OpenAPI 3.1) is the **single source of truth**. Go server types and
route registration are generated from it. Never hand-write request/response structs or routing
tables, and never hand-edit `internal/api/generated/`.

## Two rules with no exceptions

**Every route is in the spec.** Not "every REST endpoint", not "every route worth
documenting" — every route. The frontend and the iOS app both generate their client from this
file, so a route that is not in it cannot be called by either: the feature ships, the server
serves it, and no app can reach it. Custom currency rules lived that way for three migrations
and only the web client could use them, by hand-written code the iOS app never had.

Shape is not an excuse. A browser redirect (`/auth/oidc/authorize`), a file download, an
endpoint "only the web app uses" — all of them are operations with a method, a path, parameters
and responses, and all of them belong in the spec. Declare a redirect with its `302` and a
`Location` header; declare a download with its content type.

**Every route is under `/api/v1`.** Registering on the bare engine puts an endpoint outside
auth, rate limiting, the request timeout, idempotency and the device context, all of which are
middleware on the `/api/v1` group. Only `/health` and `/metrics` are outside it, because they
are operational surfaces for a probe and a scraper rather than client API — they are listed
with that reason in `scripts/check-routes.py`.

`make route-check` enforces both, and CI runs it. When it fails, the fix is to add the
operation to the spec — never to add the route to the allow-list.

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
8. **`make route-check`** — confirms the route is declared and correctly mounted.

## Naming schemas

Schema names share one namespace, and a duplicate key does not fail loudly: PyYAML keeps the
last definition and `oapi-codegen` rejects the file with a line number rather than a name.
`CurrencyRequirement` was already taken by the regulatory currency response when the custom
rule schemas arrived, so those became `CustomCurrencyWindow`, `CustomCurrencyFilter` and
`CustomCurrencyThreshold`. Before adding a schema, grep for the name; prefix it with its
feature when the bare word is something another feature could plausibly want.

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
api.Use(middleware.AuthMiddlewareWithState(jwtManager, /* public path allow-list */, authService.AccessTokenState))
api.Use(middleware.RateLimitByPath(authRateLimit, /* /auth paths */))
generated.RegisterHandlersWithOptions(api, apiHandler, generated.GinServerOptions{...})
```

That single generated call is the whole route table. There are no `handlers.Register*Routes`
helpers any more, and adding one back is the mistake this skill exists to prevent — a new
endpoint is a spec entry plus a method on `APIHandler`, never a manual registration.

- Endpoints are **authenticated by default**. A public endpoint must be added to the allow-list
  passed to `AuthMiddleware` *and* declared `security: []` in the spec.
- `/auth/*` and `/admin/*` get stricter rate limits; admin endpoints additionally require an
  admin caller (`ADMIN_EMAIL`).
- JSON field names are `camelCase`. Ownership violations return 403. List endpoints that can
  grow large are paginated.
- Never return raw internal errors to clients — use a static, user-friendly message.
