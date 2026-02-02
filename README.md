# PilotLog API

RESTful API backend for EASA/FAA compliant pilot logbook with multi-license tracking.

## Tech Stack

- **Go 1.24+** • **Gin** • **PostgreSQL 18** • **JWT Auth**
- **pgx + sqlc** (type-safe SQL) • **golang-migrate** • **OpenAPI/Swagger**

## Prerequisites

- Go 1.24+
- PostgreSQL 18
- Docker & Docker Compose (recommended)

## Quick Start

```bash
# Start PostgreSQL
make docker-up

# Run migrations
make migrate-up

# Start server
make run

# Run tests
make test
```

## Common Commands

```bash
make help              # View all available commands
make generate          # Generate Go types from OpenAPI spec
make test              # Run all tests with coverage
make test-e2e          # Run end-to-end tests
make fmt               # Format code
make lint              # Run linter
make migrate-create NAME=migration_name  # Create migration
```

## Project Structure

```
cmd/api/main.go          # Entry point
internal/
  ├── api/handlers/      # HTTP endpoints
  ├── service/           # Business logic
  ├── repository/        # Database access
  ├── models/            # Domain models
  └── validator/         # EASA/FAA rules
db/migrations/           # SQL migrations
pkg/jwt/                 # JWT utilities
test/                    # Tests (unit, integration, e2e)
```

## API Endpoints

### Auth
- `POST /api/v1/auth/register` • `POST /api/v1/auth/login`

### Licenses
- `GET /api/v1/licenses` • `POST /api/v1/licenses`
- `GET /api/v1/licenses/:id` • `PUT /api/v1/licenses/:id` • `DELETE /api/v1/licenses/:id`

### Flights
- `GET /api/v1/flights` • `POST /api/v1/flights`
- `GET /api/v1/flights/:id` • `PUT /api/v1/flights/:id` • `DELETE /api/v1/flights/:id`

## Environment Variables

```env
DATABASE_URL=postgresql://localhost:5432/pilotlog?sslmode=disable
JWT_SECRET=your-secret-key
PORT=3000
```

## Related Repositories

- **[pilotlog-project](../pilotlog-project)**: API spec & planning
- **[pilotlog-frontend](../pilotlog-frontend)**: Web frontend
