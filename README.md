# PilotLog API

RESTful API backend for the EASA/FAA compliant pilot logbook system.

## Overview

A robust API server that manages pilot data, flight logs, and multi-license tracking with full EASA and FAA regulatory compliance. Built with OpenAPI-first design principles.

## Tech Stack

- **Language**: Go 1.24+
- **Framework**: Gin (HTTP web framework)
- **Database**: PostgreSQL 18
- **Database Access**: pgx (PostgreSQL driver) + sqlc (type-safe SQL)
- **Migrations**: golang-migrate
- **Authentication**: JWT with refresh tokens
- **Validation**: OpenAPI spec validation (oapi-codegen)
- **API Docs**: Swagger UI (auto-generated from OpenAPI)
- **Testing**: Go standard testing + testify

## Prerequisites

- Go 1.24+
- PostgreSQL 18
- Docker & Docker Compose (recommended)
- Access to pilotlog-project repo (for OpenAPI spec)
- Air (for live reload in development, optional)
- golangci-lint (for linting, optional)

## Quick Start

### Using Docker

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f api

# Stop services
docker-compose down
```

### Local Development

```bash
# Install dependencies
go mod download

# See all available commands
make help

# Generate Go types from OpenAPI spec
make generate

# Start PostgreSQL (if using Docker)
make docker-up

# Run database migrations
make migrate-up

# Start development server
make run
# Or with live reload:
air
```

## Code Generation

The Go server types are **auto-generated** from the OpenAPI specification.

### Automatic (CI/CD)
When the OpenAPI spec changes in `pilotlog-project`, GitHub Actions automatically:
1. Generates new Go types and interfaces
2. Creates a PR with changes
3. Runs tests to verify compatibility

### Manual
```bash
# Generate server types
make generate

# Or use script directly
./scripts/generate-server-types.sh

# Or use go generate
go generate ./...
```

**⚠️ Never edit files in `internal/api/generated/` manually!** They will be overwritten.

Generated files:
- `types.go` - OpenAPI schemas as Go structs
- `server.go` - Gin handler interfaces (implement these!)
- `spec.go` - Embedded OpenAPI spec

See [OpenAPI Generation Guide](../pilotlog-project/docs/OPENAPI_GENERATION.md) for details.

# Setup database (create database)
createdb pilotlog

# Run migrations
migrate -path db/migrations -database "postgresql://localhost:5432/pilotlog?sslmode=disable" up

# Generate sqlc models from SQL
sqlc generate

# Generate API code from OpenAPI spec
go generate ./...

# Start development server (with live reload)
air

# Or run directly
go run cmd/api/main.go

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...
```

## Project Structure

```
.
├── cmd/
│   └── api/
│       └── main.go          # Application entry point
├── internal/
│   ├── api/
│   │   ├── handlers/        # HTTP handlers
│   │   │   ├── auth.go      # Authentication endpoints
│   │   │   ├── users.go     # User management
│   │   │   ├── licenses.go  # License management
│   │   │   ├── flights.go   # Flight log endpoints
│   │   │   └── stats.go     # Statistics and reports
│   │   ├── middleware/      # HTTP middleware
│   │   │   ├── auth.go      # JWT authentication
│   │   │   ├── cors.go      # CORS handling
│   │   │   └── logger.go    # Request logging
│   │   └── router.go        # Route setup
│   ├── service/             # Business logic
│   │   ├── auth.go
│   │   ├── flight.go
│   │   ├── license.go
│   │   └── currency.go      # Currency calculations
│   ├── repository/          # Database access layer
│   │   ├── user.go
│   │   ├── license.go
│   │   └── flight.go
│   ├── models/              # Domain models
│   ├── config/              # Configuration
│   └── validator/           # Validation logic
│       ├── regulations.go   # EASA/FAA rules
│       └── flight.go        # Flight validation
├── db/
│   ├── migrations/          # SQL migrations
│   ├── queries/             # SQL queries for sqlc
│   └── sqlc.yaml            # sqlc configuration
├── pkg/
│   ├── jwt/                 # JWT utilities
│   └── logger/              # Logging utilities
└── api/                     # Generated OpenAPI code
```

## API Endpoints

### Authentication
- `POST /api/auth/register` - User registration
- `POST /api/auth/login` - User login
- `POST /api/auth/refresh` - Refresh access token
- `POST /api/auth/logout` - Logout

### User Management
- `GET /api/users/me` - Get current user
- `PATCH /api/users/me` - Update user profile

### License Management
- `GET /api/licenses` - List user licenses
- `POST /api/licenses` - Add new license
- `GET /api/licenses/:id` - Get license details
- `PATCH /api/licenses/:id` - Update license
- `DELETE /api/licenses/:id` - Remove license

### Flight Logs
- `GET /api/flights` - List flights (with filters)
- `POST /api/flights` - Create flight log entry
- `GET /api/flights/:id` - Get flight details
- `PATCH /api/flights/:id` - Update flight
- `DELETE /api/flights/:id` - Delete flight

### Statistics
- `GET /api/stats/totals` - Get total hours per license
- `GET /api/stats/currency` - Get currency status
- `GET /api/stats/reports` - Generate reports

## Database Schema

### Core Tables

```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TYPE license_type AS ENUM (
    'EASA_PPL', 'FAA_PPL', 'EASA_SPL', 'FAA_SPORT',
    'EASA_CPL', 'FAA_CPL', 'EASA_ATPL', 'FAA_ATPL',
    'EASA_IR', 'FAA_IR'
);

CREATE TABLE user_licenses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    license_type license_type NOT NULL,
    license_number VARCHAR(100) NOT NULL,
    issue_date DATE NOT NULL,
    expiry_date DATE,
    issuing_authority VARCHAR(255) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, license_number)
);

CREATE TABLE flight_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    license_id UUID NOT NULL REFERENCES user_licenses(id),
    date DATE NOT NULL,
    aircraft_registration VARCHAR(20) NOT NULL,
    aircraft_type VARCHAR(100) NOT NULL,
    departure_airport VARCHAR(10) NOT NULL,
    arrival_airport VARCHAR(10) NOT NULL,
    departure_time TIMESTAMPTZ NOT NULL,
    arrival_time TIMESTAMPTZ NOT NULL,
    total_time DECIMAL(5,2) NOT NULL,
    pic_time DECIMAL(5,2) DEFAULT 0,
    dual_time DECIMAL(5,2) DEFAULT 0,
    solo_time DECIMAL(5,2) DEFAULT 0,
    night_time DECIMAL(5,2) DEFAULT 0,
    ifr_time DECIMAL(5,2) DEFAULT 0,
    landings_day INTEGER DEFAULT 0,
    landings_night INTEGER DEFAULT 0,
    remarks TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_flight_logs_user_date ON flight_logs(user_id, date DESC);
CREATE INDEX idx_flight_logs_license_date ON flight_logs(license_id, date DESC);
CREATE INDEX idx_flight_logs_user_license_date ON flight_logs(user_id, license_id, date DESC);
```

## Multi-License Support

The API handles complex multi-license scenarios:

1. **Flight Attribution**: Each flight log references which license it counts toward
2. **Cross-License Hours**: Some hours may count for multiple licenses
3. **Currency Calculation**: Different rules per license type (EASA vs FAA)
4. **Validation**: License-specific field requirements

### Example: Currency Check

```go
// EASA PPL: 3 takeoffs/landings in last 90 days
// FAA PPL: 3 takeoffs/landings in last 90 days (day/night separate)
// EASA SPL: Different rules for sailplanes

type CurrencyService struct {
    repo repository.FlightRepository
}

func (s *CurrencyService) CheckEasaPPLCurrency(ctx context.Context, userID, licenseID uuid.UUID) (bool, error) {
    ninetyDaysAgo := time.Now().AddDate(0, 0, -90)
    
    flights, err := s.repo.GetFlightsByDateRange(ctx, userID, licenseID, ninetyDaysAgo, time.Now())
    if err != nil {
        return false, err
    }
    
    totalLandings := 0
    for _, flight := range flights {
        totalLandings += flight.LandingsDay + flight.LandingsNight
    }
    
    return totalLandings >= 3, nil
}
```

## Environment Variables

Create a `.env` file:

```env
APP_ENV=development
PORT=3000
DATABASE_HOST=localhost
DATABASE_PORT=5432
DATABASE_USER=postgres
DATABASE_PASSWORD=postgres
DATABASE_NAME=pilotlog
DATABASE_SSLMODE=disable
JWT_SECRET=your-secret-key-change-in-production
JWT_ACCESS_EXPIRY=15m
JWT_REFRESH_EXPIRY=168h
CORS_ALLOWED_ORIGINS=http://localhost:5173
LOG_LEVEL=debug
```

## OpenAPI Code Generation

The API uses oapi-codegen to generate server code from the OpenAPI spec:

```bash
# Generate server interfaces and models from OpenAPI spec
oapi-codegen -config oapi-codegen.yaml ../pilotlog-project/api-spec/openapi.yaml
```

```yaml
# oapi-codegen.yaml
package: api
generate:
  models: true
  gin-server: true
  strict-server: true
output: internal/api/generated.go
```

This generates:
- Type-safe request/response models
- Server interface to implement
- Automatic request validation
- Response marshaling

## Makefile Commands

The project includes a comprehensive Makefile for common tasks:

```bash
# View all available commands with descriptions
make help

# Code Generation
make generate          # Generate Go types from OpenAPI spec
make sqlc-generate     # Generate sqlc code from SQL queries

# Testing
make test             # Run all tests with coverage
make test-short       # Run unit tests only
make test-integration # Run integration tests
make test-e2e         # Run end-to-end tests
make coverage         # Generate HTML coverage report

# Development
make run              # Run application
make build            # Build binary to bin/pilotlog-api
make fmt              # Format code with go fmt
make lint             # Lint code with golangci-lint

# Database
make migrate-up       # Apply database migrations
make migrate-down     # Rollback last migration
make migrate-create NAME=migration_name  # Create new migration

# Docker
make docker-build     # Build Docker image
make docker-up        # Start Docker containers
make docker-down      # Stop Docker containers
make docker-logs      # View Docker logs

# Maintenance
make clean            # Clean build artifacts and test files
```

### Example Usage

```bash
# Start fresh development setup
make docker-up        # Start PostgreSQL
make migrate-up       # Run migrations
make generate         # Generate code
make test             # Verify setup
make run             # Start server

# Before committing
make fmt             # Format code
make lint            # Check for issues
make test            # Run tests
make coverage        # Verify coverage
```

## Testing

```bash
# Quick test commands
make test              # Run all tests
make test-short        # Unit tests only
make coverage          # Generate HTML report

# Detailed testing with go test
go test -v ./...                            # Verbose output
go test -tags=integration ./...             # Integration tests
go test ./internal/service/...              # Specific package
go test -bench=. ./...                      # Benchmarks
```

## Deployment

### Docker Production Build

```bash
docker build -t pilotlog-api:latest .
docker run -p 3000:3000 --env-file .env.production pilotlog-api:latest
```

### Database Migrations

```bash
# Using Makefile (recommended)
make migrate-up                          # Apply all migrations
make migrate-down                        # Rollback one migration
make migrate-create NAME=add_field_name  # Create new migration

# Using migrate CLI directly
migrate -path db/migrations -database "${DATABASE_URL}" up
migrate -path db/migrations -database "${DATABASE_URL}" down 1
migrate -path db/migrations -database "${DATABASE_URL}" version
migrate -path db/migrations -database "${DATABASE_URL}" force VERSION
```

## Monitoring

- **Health Check**: `GET /health`
- **Metrics**: Prometheus metrics at `/metrics`
- **API Docs**: Swagger UI at `/api-docs`

## Security

- JWT authentication with refresh tokens
- Password hashing with bcrypt (12 rounds)
- Rate limiting on auth endpoints
- CORS configuration
- SQL injection prevention (parameterized queries via pgx)
- Input validation via OpenAPI spec

## Common Commands

```bash
# Quick reference - prefer using Makefile commands (see above)

# Development
make run                        # Start development server
air                            # Start dev server with live reload (if installed)

# Database
make migrate-up                 # Run migrations
make migrate-create NAME=name   # Create new migration
make sqlc-generate             # Generate Go from SQL

# Code generation
make generate                  # Generate OpenAPI code (recommended)
go generate ./...              # Alternative method

# Testing
make test                      # Run all tests (recommended)
make coverage                  # Generate coverage report
go test -v ./...              # Verbose test output

# Linting and formatting
make fmt                       # Format code (recommended)
make lint                      # Run linter
go vet ./...                  # Go vet checks

# Building
make build                     # Build binary (recommended)
go build -o bin/api cmd/api/main.go  # Direct build

# Docker
make docker-up                 # Start all services
make docker-down               # Stop all services
make docker-logs               # View logs

# Dependencies
go mod tidy                    # Clean up dependencies
go mod download                # Download dependencies
go mod verify                  # Verify dependencies
```

## Related Repositories

- **[pilotlog-project](../pilotlog-project)**: Project planning and API spec
- **[pilotlog-frontend](../pilotlog-frontend)**: Web application frontend

## Contributing

See [CONTRIBUTING.md](../pilotlog-project/CONTRIBUTING.md) for guidelines.

## License

[To be determined]
