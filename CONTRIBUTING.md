# Contributing to NinerLog API

Thank you for your interest in contributing to the NinerLog API! This document provides guidelines specific to the backend repository.

For general project guidelines, see the [project-level CONTRIBUTING.md](https://github.com/fjaeckel/ninerlog-project/blob/main/CONTRIBUTING.md).

## Prerequisites

- **Go 1.25+**
- **PostgreSQL 16+** (or Docker)
- **Docker & Docker Compose**
- **golangci-lint**
- **k6** (for performance tests, optional)

## Getting Started

```bash
git clone git@github.com:fjaeckel/ninerlog-api.git
cd ninerlog-api
go mod download
cp .env.example .env        # Configure your environment
docker compose up -d         # Start PostgreSQL
make migrate-up              # Run database migrations
make run                     # Start the API server
```

Verify:

```bash
curl http://localhost:3333/health
```

## Development Workflow

### API-First Development

**All API changes start with the OpenAPI spec.** This is non-negotiable.

1. Edit `api-spec/openapi.yaml`
2. Regenerate server types: `make generate` (or `bash scripts/generate-server-types.sh`)
3. Implement the handler/service logic
4. Write tests
5. Also regenerate the frontend client: `cd ../ninerlog-frontend && bash scripts/generate-api-client.sh`

### Project Structure

```
cmd/api/            → Application entrypoint
internal/
  api/              → HTTP handlers and middleware
  config/           → Configuration loading
  models/           → Domain types and validation
  repository/       → Database access layer
  service/          → Business logic layer
  testutil/         → Shared test helpers
pkg/                → Public utility packages
db/migrations/      → SQL migration files
api-spec/           → OpenAPI specification
test/e2e/           → End-to-end tests
```

### Key Libraries

| Purpose | Library |
|---------|---------|
| HTTP framework | Gin |
| Database | lib/pq (PostgreSQL) |
| Authentication | golang-jwt/jwt/v5 |
| Testing | testify (assert/mock) |
| Migrations | golang-migrate |
| OpenAPI codegen | oapi-codegen |
| Metrics | prometheus/client_golang |

## Coding Standards

### Error Handling

```go
// ✅ Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to create flight: %w", err)
}

// ✅ Use sentinel errors for known conditions
var ErrNotFound = errors.New("not found")

// ❌ Never return raw err.Error() to API clients
// Use sendError() with a static, user-friendly message
```

### Input Validation

- Validate all text field lengths using `internal/models/validation.go`
- Validate at the **service layer**, not the handler layer
- Use parameterized queries only — never concatenate user input into SQL

### Resource Ownership

Every handler accessing a user-specific resource must verify:

```go
if resource.UserID != requestingUserID {
    return ErrUnauthorized
}
```

### Table-Driven Tests

```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "hello", "HELLO", false},
        {"empty input", "", "", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
                return
            }
            assert.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

## Testing

**All code must be tested. No exceptions.**

### Running Tests

```bash
make test              # Unit tests
make test-integration  # Integration tests (starts test DB via Docker)
make test-e2e          # E2E tests (starts test DB via Docker)
make test-all          # All tests (unit + integration + e2e)
make coverage          # Generate HTML coverage report
```

Or directly:

```bash
go test ./...                         # Unit tests
bash scripts/run-e2e-tests.sh         # Full e2e suite against Docker API
bash scripts/run-all-tests.sh         # Everything
```

### Coverage Target

- **Minimum 90% code coverage**
- Unit tests: services, repositories, utilities, validators
- Integration tests: database queries, migrations
- E2E tests: full API request/response flows

### Pre-Commit Checklist

Before committing, **all** of these must pass:

```bash
make lint              # golangci-lint
make fmt               # go fmt
go test ./...          # Unit tests
bash scripts/run-e2e-tests.sh   # E2E tests
```

## Commit Guidelines

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```bash
feat(flights): add night currency validation
fix(auth): correct token expiry calculation
refactor(services): extract validation logic
test(e2e): add license deletion scenarios
docs(api): update OpenAPI spec for export endpoint
```

### Types

| Type | Use |
|------|-----|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `refactor` | Code refactoring |
| `test` | Adding or updating tests |
| `chore` | Maintenance tasks |
| `perf` | Performance improvement |

## Pull Request Process

1. Branch from `main` using `feature/`, `fix/`, or `docs/` prefix
2. Ensure all tests pass (`make test-all`)
3. Ensure linting passes (`make lint`)
4. Update the OpenAPI spec if the API surface changed
5. Open a PR with a clear title following conventional commits
6. Address review feedback

## Security

- Never log or expose JWT tokens
- Never hardcode secrets — use environment variables
- New endpoints are authenticated by default (allowlist public routes in `cmd/api/main.go`)
- All new endpoints must be documented in `api-spec/openapi.yaml`
- Run `go vet ./...` to catch common issues

## Questions?

- Create an issue with the `question` label
- Email: frederic@ninerlog.app
