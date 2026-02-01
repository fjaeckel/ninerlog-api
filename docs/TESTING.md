# Testing Guide

This project includes comprehensive test coverage across three levels:

## Test Levels

### 1. Unit Tests
Fast, isolated tests with mocked dependencies.

```bash
make test
# or
go test -v -short ./...
```

**Coverage:**
- pkg/hash: 85.7%
- pkg/jwt: 86.4%
- internal/config: 77.8%
- internal/service: 71.1%
- internal/api/middleware: 100%

### 2. Integration Tests
Tests with real database connections, validating repository layer interactions.

```bash
make test-integration
```

**Requirements:**
- Docker and Docker Compose
- PostgreSQL test database (automatically started)

**What's tested:**
- User repository CRUD operations
- Refresh token creation and revocation
- Password reset token lifecycle
- Database constraints and triggers

### 3. End-to-End (E2E) Tests
Full API flow tests simulating real user interactions.

```bash
make test-e2e
```

**Requirements:**
- Docker and Docker Compose
- PostgreSQL test database (automatically started)

**What's tested:**
- Complete authentication flow (register, login, refresh)
- Password reset flow
- Protected endpoint access
- Token validation and authorization

## Running All Tests

To run unit, integration, and e2e tests in sequence:

```bash
make test-all
# or
./scripts/run-all-tests.sh
```

This will:
1. Start the test database
2. Run migrations
3. Execute unit tests
4. Execute integration tests
5. Execute e2e tests
6. Clean up test database

## Test Database

The test database runs in Docker on port 5433 to avoid conflicts with development databases.

### Manual Setup

Start the test database:
```bash
docker compose -f docker-compose.test.yaml up -d
```

Run migrations:
```bash
PGPASSWORD=testpass psql -h localhost -p 5433 -U testuser -d pilotlog_test -f db/migrations/test_init.sql
```

Stop the test database:
```bash
docker compose -f docker-compose.test.yaml down
```

## Environment Variables

Integration and E2E tests use these environment variables:

```bash
TEST_DB_HOST=localhost
TEST_DB_PORT=5433
TEST_DB_USER=testuser
TEST_DB_PASSWORD=testpass
TEST_DB_NAME=pilotlog_test
```

## Writing Tests

### Unit Tests
- Place in same package as code being tested
- Use `_test.go` suffix
- Mock external dependencies
- Use `testing.Short()` check to skip in integration runs

### Integration Tests
- Place in `*_test.go` files
- Use `testutil.SetupTestDB(t)` for database setup
- Check `testing.Short()` and skip if true
- Clean up with `testutil.TeardownTestDB(t, db)`

### E2E Tests
- Place in `test/e2e/` directory
- Use build tag `//go:build e2e`
- Use `testutil.APITestClient` for HTTP requests
- Test complete user flows

## CI/CD Integration

The test suite is designed to run in CI/CD pipelines:

```yaml
# Example GitHub Actions
- name: Run unit tests
  run: make test

- name: Run integration tests
  run: make test-integration

- name: Run e2e tests
  run: make test-e2e
```

## Coverage Reports

Generate HTML coverage report:

```bash
make coverage
open coverage.html
```

## Troubleshooting

**Database connection errors:**
- Ensure Docker is running
- Check if port 5433 is available
- Wait a few seconds after starting Docker container

**Migration errors:**
- Verify `db/migrations/test_init.sql` exists
- Check PostgreSQL client (`psql`) is installed
- Run migrations manually if needed

**Test timeouts:**
- Integration tests may take 10-15 seconds
- E2E tests may take 20-30 seconds
- Database startup requires 2-3 seconds

## Test Structure

```
pilotlog-api/
├── internal/
│   ├── testutil/           # Shared test utilities
│   │   ├── database.go     # DB setup/teardown
│   │   ├── fixtures.go     # Test data creators
│   │   └── api_client.go   # HTTP test client
│   ├── repository/postgres/
│   │   └── integration_test.go  # Integration tests
│   └── service/
│       └── *_test.go       # Unit tests
├── test/
│   └── e2e/
│       └── auth_test.go    # E2E tests
└── db/migrations/
    └── test_init.sql       # Test DB schema
```
