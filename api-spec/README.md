# API Specification

This directory contains the OpenAPI 3.1 specification for the NinerLog API.

## Files

- `openapi.yaml` - Complete API specification

## OpenAPI-First Design

The API specification is the source of truth for the contract between frontend and backend:

1. **Design first**: All endpoints are defined here before implementation
2. **Generate code**: Both frontend client and backend validators are generated from this spec
3. **Validate**: Requests and responses are validated against the spec
4. **Document**: Swagger UI is auto-generated for interactive documentation

## Viewing the Spec

### Local Swagger UI

```bash
# Option 1: Using Swagger UI Docker
docker run -p 8080:8080 -e SWAGGER_JSON=/spec/openapi.yaml \
  -v $(pwd):/spec swaggerapi/swagger-ui

# Open http://localhost:8080
```

### Online Editors

- [Swagger Editor](https://editor.swagger.io/) - Import `openapi.yaml`
- [Stoplight Studio](https://stoplight.io/studio) - More visual editor

## Generating Code

### Frontend API Client

```bash
cd ../ninerlog-frontend
npm run generate:api
```

This generates TypeScript types and API client in `src/api/` using the OpenAPI spec.

### Backend Validators

```bash
./scripts/generate-server-types.sh
```

This generates Go types and Gin handlers using oapi-codegen.

## Specification Structure

```yaml
openapi: 3.1.0
info:
  title: NinerLog API
  version: 1.0.0
  
paths:
  /auth/register: ...
  /auth/login: ...
  /licenses: ...
  /flights: ...
  
components:
  schemas:
    User: ...
    License: ...
    FlightLog: ...
  securitySchemes:
    bearerAuth: ...
```

## API Design Guidelines

### Authentication
- Use JWT Bearer tokens
- Include `Authorization: Bearer <token>` header
- Token expiry: 15 minutes (access), 7 days (refresh)

### Versioning
- Major version in URL: `/api/v1/`
- Breaking changes require new version
- Support previous version for 6 months minimum

### Error Responses
```json
{
  "error": "Validation failed",
  "details": [
    {
      "field": "totalTime",
      "message": "Must be greater than 0"
    }
  ]
}
```

### Pagination
```json
{
  "data": [...],
  "pagination": {
    "page": 1,
    "pageSize": 20,
    "total": 150,
    "totalPages": 8
  }
}
```

### Date/Time Format
- Dates: ISO 8601 date only (`2026-01-30`)
- Timestamps: ISO 8601 with timezone (`2026-01-30T14:30:00Z`)

## Multi-License Endpoints

### Key Concepts

1. **License-Scoped Data**: Flights, totals, and currency are always tied to a specific license
2. **User-Wide Resources**: Users can manage multiple licenses
3. **Cross-License Queries**: Get aggregated stats across all licenses

### Example Endpoints

```
GET    /api/v1/licenses                    # List user's licenses
POST   /api/v1/licenses                    # Create new license
GET    /api/v1/licenses/{id}               # Get license details
GET    /api/v1/licenses/{id}/flights       # List flights for license
GET    /api/v1/licenses/{id}/statistics    # Get totals for license
GET    /api/v1/licenses/{id}/currency      # Check currency status

POST   /api/v1/flights                     # Create flight (requires licenseId)
GET    /api/v1/flights/{id}                # Get flight details
PUT    /api/v1/flights/{id}                # Update flight
DELETE /api/v1/flights/{id}                # Delete flight
```

## EASA/FAA Compliance

### Validation Rules

The API enforces regulatory rules:

- **SPL flights**: Cannot have night time > 0
- **Night currency**: Tracked separately for FAA licenses
- **Medical requirements**: Validated before flight creation
- **License expiry**: Enforced for all operations

### Audit Trail

All mutations include:
- User ID
- Timestamp
- Operation type
- Changed fields (for updates)

## Testing the API

### Development Server

```bash
cd ../ninerlog-api
go run cmd/api/main.go
```

API available at `http://localhost:3000`

### Postman Collection

Import the OpenAPI spec into Postman for interactive testing:
1. Open Postman
2. Import → Link → Paste `openapi.yaml` file path
3. Collection auto-created with all endpoints

### cURL Examples

```bash
# Register
curl -X POST http://localhost:3000/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"pilot@test.com","password":"SecurePass123!","name":"Test Pilot"}'

# Login
curl -X POST http://localhost:3000/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"pilot@test.com","password":"SecurePass123!"}'

# Create License (requires token from login)
curl -X POST http://localhost:3000/api/v1/licenses \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"licenseType":"EASA_PPL","licenseNumber":"PPL-123","issueDate":"2020-01-15"}'

# Create Flight
curl -X POST http://localhost:3000/api/v1/flights \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"licenseId":"<license-id>","date":"2026-01-30","totalTime":2.5}'
```

## Contributing

When adding new endpoints:

1. Update `openapi.yaml` with complete specification
2. Include request/response schemas in `components/schemas`
3. Add authentication requirements
4. Document error responses
5. Regenerate frontend and backend code
6. Update this README if needed

## Resources

- [OpenAPI 3.1 Specification](https://spec.openapis.org/oas/v3.1.0)
- [OpenAPI Generator](https://openapi-generator.tech/)
- [oapi-codegen (Go)](https://github.com/deepmap/oapi-codegen)
