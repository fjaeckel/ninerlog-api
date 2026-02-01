# Integration and E2E Testing - Quick Start

## Test Suite Overview

The project includes three levels of testing:

1. **Unit Tests** (45 tests) - Fast, mocked dependencies
2. **Integration Tests** (15 tests) - Real database operations
3. **E2E Tests** - Full API flow testing (framework ready)

## Quick Commands

```bash
# Run unit tests only (fast, no DB needed)
make test

# Run integration tests (requires Docker)
make test-integration

# Run e2e tests (requires Docker, when handlers are implemented)
make test-e2e

# Run all tests
make test-all
```

## Test Coverage

- **pkg/hash**: 85.7%
- **pkg/jwt**: 86.4%
- **internal/config**: 77.8%
- **internal/service**: 71.1%
- **internal/api/middleware**: 100%
- **internal/repository/postgres**: 58.3%

## Requirements

- **Unit tests**: Go 1.25+
- **Integration/E2E tests**: Docker & Docker Compose

## Documentation

See [docs/TESTING.md](docs/TESTING.md) for comprehensive testing guide.

See [docs/TEST_IMPLEMENTATION.md](docs/TEST_IMPLEMENTATION.md) for implementation details.

## Test Database

Integration and e2e tests use PostgreSQL in Docker:
- Port: 5433 (no conflict with dev DB)
- Auto-starts and stops with tests
- Uses tmpfs for fast performance

## CI/CD Ready

All tests are automated and run via Makefile commands. No manual setup required.
