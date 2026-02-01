# GitHub Copilot Instructions for PilotLog API

You are assisting with the PilotLog API repository, the backend server for a EASA/FAA compliant pilot logbook application.

## Repository Context

This is the **backend API server** that provides:
- RESTful API endpoints for flight logging
- Multi-license user management
- EASA and FAA compliance validation
- JWT authentication and authorization
- PostgreSQL database integration

## Key Principles

1. **OpenAPI-First**: Implement exactly what the spec defines, validate against it
2. **Type-Safety**: Use Go's strong typing, leverage generated types from OpenAPI
3. **Security**: Follow OWASP best practices
4. **Data Integrity**: Enforce regulatory compliance at database level
5. **Performance**: Optimize queries, use proper indexing, leverage Go's concurrency
6. **Testing**: All code must be tested - unit, integration, and e2e tests required

## Testing Requirements

**ALL CODE MUST BE TESTED.** Testing is not optional.

### Unit Tests (Go testing + testify)
- **Required for**: All services, repositories, utilities, validators, and domain logic
- **Coverage target**: Minimum 90% code coverage
- **Test**: Business logic, validation rules, calculations, error handling
- **Mock**: Database, external services, time dependencies
- **Use**: Table-driven tests for multiple scenarios

```go
import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

// Service unit test
func TestFlightService_Create(t *testing.T) {
    tests := []struct {
        name          string
        input         FlightLogCreate
        licenseType   string
        expectError   bool
        errorContains string
    }{
        {
            name: "valid PPL flight",
            input: FlightLogCreate{
                Date:      time.Now(),
                TotalTime: 2.5,
                NightTime: 0.5,
            },
            licenseType: "EASA_PPL",
            expectError: false,
        },
        {
            name: "SPL cannot log night time",
            input: FlightLogCreate{
                Date:      time.Now(),
                TotalTime: 2.0,
                NightTime: 0.5,
            },
            licenseType:   "EASA_SPL",
            expectError:   true,
            errorContains: "SPL cannot log night",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup mocks
            mockRepo := new(MockFlightRepository)
            mockLicRepo := new(MockLicenseRepository)
            
            license := &License{Type: tt.licenseType}
            mockLicRepo.On("GetLicenseByID", mock.Anything, mock.Anything).
                Return(license, nil)
            
            if !tt.expectError {
                mockRepo.On("Create", mock.Anything, mock.Anything, tt.input).
                    Return(&FlightLog{ID: uuid.New()}, nil)
            }
            
            // Execute
            service := NewFlightService(mockRepo, mockLicRepo)
            result, err := service.Create(context.Background(), uuid.New(), tt.input)
            
            // Assert
            if tt.expectError {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errorContains)
                assert.Nil(t, result)
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, result)
            }
            
            mockRepo.AssertExpectations(t)
            mockLicRepo.AssertExpectations(t)
        })
    }
}

// Validator unit test
func TestValidateFlight(t *testing.T) {
    tests := []struct {
        name    string
        flight  FlightLogCreate
        license License
        wantErr bool
    }{
        {
            name: "night time exceeds total time",
            flight: FlightLogCreate{
                TotalTime: 2.0,
                NightTime: 2.5,
            },
            license: License{Type: "EASA_PPL"},
            wantErr: true,
        },
        {
            name: "valid flight",
            flight: FlightLogCreate{
                TotalTime: 2.5,
                NightTime: 0.5,
                PICTime:   2.5,
            },
            license: License{Type: "EASA_PPL"},
            wantErr: false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateFlight(tt.flight, tt.license)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Integration Tests
- **Required for**: HTTP handlers with database, auth middleware, complete request flows
- **Test**: Full request → handler → service → database → response
- **Use**: Test database (PostgreSQL test instance or Docker container)
- **Verify**: Data persistence, transactions, rollbacks, concurrent operations

```go
import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    
    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/suite"
)

type FlightHandlerTestSuite struct {
    suite.Suite
    router *gin.Engine
    db     *sql.DB
    token  string
}

func (suite *FlightHandlerTestSuite) SetupSuite() {
    // Setup test database
    suite.db = setupTestDB()
    runMigrations(suite.db)
    
    // Setup router with real dependencies
    suite.router = setupRouter(suite.db)
    
    // Create test user and get auth token
    suite.token = createTestUserAndGetToken(suite.db)
}

func (suite *FlightHandlerTestSuite) TearDownSuite() {
    suite.db.Close()
}

func (suite *FlightHandlerTestSuite) SetupTest() {
    // Clean database before each test
    cleanDatabase(suite.db)
}

func (suite *FlightHandlerTestSuite) TestCreateFlight() {
    // Create license first
    license := createTestLicense(suite.db, "EASA_PPL")
    
    // Prepare request
    flightData := map[string]interface{}{
        "licenseId": license.ID,
        "date":      "2026-01-30",
        "totalTime": 2.5,
        "picTime":   2.5,
    }
    body, _ := json.Marshal(flightData)
    
    // Make request
    req := httptest.NewRequest("POST", "/api/flights", bytes.NewBuffer(body))
    req.Header.Set("Authorization", "Bearer "+suite.token)
    req.Header.Set("Content-Type", "application/json")
    
    w := httptest.NewRecorder()
    suite.router.ServeHTTP(w, req)
    
    // Assert response
    assert.Equal(suite.T(), http.StatusCreated, w.Code)
    
    var response FlightLog
    err := json.Unmarshal(w.Body.Bytes(), &response)
    assert.NoError(suite.T(), err)
    assert.Equal(suite.T(), 2.5, response.TotalTime)
    assert.NotEmpty(suite.T(), response.ID)
    
    // Verify in database
    var count int
    suite.db.QueryRow("SELECT COUNT(*) FROM flight_logs WHERE id = $1", response.ID).Scan(&count)
    assert.Equal(suite.T(), 1, count)
}

func (suite *FlightHandlerTestSuite) TestCreateFlight_Unauthorized() {
    flightData := map[string]interface{}{"licenseId": "123"}
    body, _ := json.Marshal(flightData)
    
    req := httptest.NewRequest("POST", "/api/flights", bytes.NewBuffer(body))
    // No authorization header
    
    w := httptest.NewRecorder()
    suite.router.ServeHTTP(w, req)
    
    assert.Equal(suite.T(), http.StatusUnauthorized, w.Code)
}

func (suite *FlightHandlerTestSuite) TestGetFlights_Pagination() {
    license := createTestLicense(suite.db, "EASA_PPL")
    
    // Create 25 flights
    for i := 0; i < 25; i++ {
        createTestFlight(suite.db, license.ID)
    }
    
    // Get first page
    req := httptest.NewRequest("GET", "/api/flights?page=1&pageSize=10", nil)
    req.Header.Set("Authorization", "Bearer "+suite.token)
    
    w := httptest.NewRecorder()
    suite.router.ServeHTTP(w, req)
    
    assert.Equal(suite.T(), http.StatusOK, w.Code)
    
    var response PaginatedFlights
    json.Unmarshal(w.Body.Bytes(), &response)
    assert.Equal(suite.T(), 10, len(response.Data))
    assert.Equal(suite.T(), 25, response.Total)
}

func TestFlightHandlerSuite(t *testing.T) {
    suite.Run(t, new(FlightHandlerTestSuite))
}
```

### End-to-End API Tests
- **Required for**: Complete API flows (signup → login → CRUD operations → logout)
- **Test**: Against running server with real database
- **Use**: HTTP client to make actual API calls
- **Cover**: Authentication, authorization, data validation, error responses

```go
func TestE2E_CompleteFlightFlow(t *testing.T) {
    // Start test server
    server := startTestServer()
    defer server.Close()
    
    client := &http.Client{}
    baseURL := server.URL
    
    // 1. Register user
    registerData := map[string]string{
        "email":    "pilot@test.com",
        "password": "SecurePass123!",
        "name":     "Test Pilot",
    }
    resp := makeRequest(t, client, "POST", baseURL+"/api/auth/register", registerData, "")
    assert.Equal(t, http.StatusCreated, resp.StatusCode)
    
    // 2. Login
    loginData := map[string]string{
        "email":    "pilot@test.com",
        "password": "SecurePass123!",
    }
    resp = makeRequest(t, client, "POST", baseURL+"/api/auth/login", loginData, "")
    var authResponse AuthResponse
    json.NewDecoder(resp.Body).Decode(&authResponse)
    token := authResponse.AccessToken
    
    // 3. Create license
    licenseData := map[string]interface{}{
        "licenseType":       "EASA_PPL",
        "licenseNumber":     "PPL-123456",
        "issueDate":         "2020-01-15",
        "issuingAuthority": "EASA",
    }
    resp = makeRequest(t, client, "POST", baseURL+"/api/licenses", licenseData, token)
    var license License
    json.NewDecoder(resp.Body).Decode(&license)
    assert.Equal(t, http.StatusCreated, resp.StatusCode)
    
    // 4. Create flight
    flightData := map[string]interface{}{
        "licenseId": license.ID,
        "date":      time.Now().Format("2006-01-02"),
        "totalTime": 2.5,
        "picTime":   2.5,
    }
    resp = makeRequest(t, client, "POST", baseURL+"/api/flights", flightData, token)
    var flight FlightLog
    json.NewDecoder(resp.Body).Decode(&flight)
    assert.Equal(t, http.StatusCreated, resp.StatusCode)
    
    // 5. Get flights list
    resp = makeRequest(t, client, "GET", baseURL+"/api/flights", nil, token)
    var flights []FlightLog
    json.NewDecoder(resp.Body).Decode(&flights)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
    assert.Len(t, flights, 1)
    
    // 6. Get statistics
    resp = makeRequest(t, client, "GET", baseURL+"/api/licenses/"+license.ID+"/statistics", nil, token)
    var stats Statistics
    json.NewDecoder(resp.Body).Decode(&stats)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
    assert.Equal(t, 2.5, stats.TotalHours)
    
    // 7. Invalid operations
    invalidFlight := map[string]interface{}{
        "licenseId": "invalid-uuid",
        "date":      "2026-01-30",
    }
    resp = makeRequest(t, client, "POST", baseURL+"/api/flights", invalidFlight, token)
    assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
```

### When to Write Tests
1. **Before implementation** (TDD for complex business logic)
2. **Alongside implementation** (most handlers and services)
3. **Never after** - no untested code should be merged

### What to Test
- ✅ Business logic (validation, calculations, currency)
- ✅ Error handling (invalid input, missing data, database errors)
- ✅ Authentication and authorization
- ✅ Database operations (CRUD, transactions, concurrent access)
- ✅ Edge cases (boundary values, null/empty, special characters)
- ✅ Regulatory compliance (EASA/FAA rules)
- ✅ Concurrent operations (race conditions)
- ✅ API contract (matches OpenAPI spec)
- ❌ Third-party library internals (Gin, pgx, etc.)
- ❌ Generated code (OpenAPI client, sqlc output)

### Running Tests
```bash
# Unit tests only
go test -short ./...

# All tests with coverage
go test -cover ./...

# Integration tests only
go test -run Integration ./...

# E2E tests (requires test DB)
go test -tags=e2e ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Tech Stack

- **Language**: Go 1.24+
- **Framework**: Gin (HTTP web framework)
- **Database**: PostgreSQL 18 with pgx driver
- **SQL**: sqlc for type-safe SQL queries
- **Validation**: OpenAPI code generation with oapi-codegen
- **Auth**: JWT with refresh tokens
- **Testing**: Go standard testing + testify
- **Migrations**: golang-migrate
- **API Docs**: Auto-generated Swagger UI

## When Writing Code

### HTTP Handlers (Gin)
```go
import (
    "net/http"
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

type FlightHandler struct {
    service *service.FlightService
}

func (h *FlightHandler) CreateFlightLog(c *gin.Context) {
    var req FlightLogCreate
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    userID := c.GetString("userID") // From auth middleware
    flight, err := h.service.Create(c.Request.Context(), userID, req)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusCreated, flight)
}
```

### Database Queries (sqlc + pgx)
```go
// Use sqlc-generated type-safe queries
const getFlightsByUserAndLicense = `-- name: GetFlightsByUserAndLicense :many
SELECT * FROM flight_logs
WHERE user_id = $1 AND license_id = $2
AND date >= $3 AND date <= $4
ORDER BY date DESC
`

// Generated function signature:
func (q *Queries) GetFlightsByUserAndLicense(
    ctx context.Context,
    arg GetFlightsByUserAndLicenseParams,
) ([]FlightLog, error)

// Usage in repository:
flights, err := q.GetFlightsByUserAndLicense(ctx, GetFlightsByUserAndLicenseParams{
    UserID: userID,
    LicenseID: licenseID,
    DateStart: startDate,
    DateEnd: endDate,
})
```

### Service Layer Pattern
```go
// Keep business logic in services
type FlightLogService struct {
    repo   repository.FlightRepository
    licRepo repository.LicenseRepository
}

func (s *FlightLogService) Create(ctx context.Context, userID uuid.UUID, data FlightLogCreate) (*FlightLog, error) {
    // Validate license ownership
    license, err := s.licRepo.GetLicenseByID(ctx, data.LicenseID)
    if err != nil {
        return nil, err
    }
    if license.UserID != userID {
        return nil, errors.New("license does not belong to user")
    }
    
    // Validate flight data against license type
    if err := s.validateFlightForLicense(data, license); err != nil {
        return nil, err
    }
    
    // Create flight log
    return s.repo.Create(ctx, userID, data)
}

func (s *FlightLogService) validateFlightForLicense(data FlightLogCreate, license *License) error {
    // EASA SPL cannot log night flights
    if license.Type == "EASA_SPL" && data.NightTime > 0 {
        return errors.New("SPL cannot log night flights")
    }
    return nil
}
```

### Authentication Middleware
```go
import (
    "github.com/gin-gonic/gin"
    "github.com/golang-jwt/jwt/v5"
)

func AuthMiddleware(secret string) gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.AbortWithStatusJSON(401, gin.H{"error": "No authorization header"})
            return
        }
        
        tokenString := strings.TrimPrefix(authHeader, "Bearer ")
        token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
            if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, errors.New("invalid signing method")
            }
            return []byte(secret), nil
        })
        
        if err != nil || !token.Valid {
            c.AbortWithStatusJSON(401, gin.H{"error": "Invalid token"})
            return
        }
        
        claims, ok := token.Claims.(jwt.MapClaims)
        if !ok {
            c.AbortWithStatusJSON(401, gin.H{"error": "Invalid claims"})
            return
        }
        
        userID := claims["sub"].(string)
        c.Set("userID", userID)
        c.Next()
    }
}
```
  } catch (error) {
    return reply.code(401).send({ error: 'Unauthorized' });
  }
};
```

## File Organization

```
.
├── cmd/
│   └── api/
│       └── main.go          # Application entry point
├── internal/
│   ├── api/
│   │   ├── handlers/        # HTTP handlers
│   │   │   ├── auth.go
│   │   │   ├── flights.go
│   │   │   ├── licenses.go
│   │   │   └── users.go
│   │   ├── middleware/      # Auth, CORS, logging
│   │   └── router.go        # Route setup
│   ├── service/             # Business logic
│   │   ├── auth.go
│   │   ├── flight.go
│   │   ├── license.go
│   │   └── currency.go
│   ├── repository/          # Database access
│   │   ├── user.go
│   │   ├── license.go
│   │   └── flight.go
│   ├── models/              # Domain models
│   ├── config/              # Configuration
│   └── validator/           # Validation logic
│       ├── regulations.go   # EASA/FAA rules
│       └── flight.go
├── db/
│   ├── migrations/          # SQL migrations
│   ├── queries/             # SQL queries for sqlc
│   └── sqlc.yaml            # sqlc config
├── pkg/
│   ├── jwt/                 # JWT utilities
│   └── logger/              # Logging
└── api/                     # Generated OpenAPI code
```

## Database Schema Best Practices

### Multi-License Support
```sql
-- Database tables are managed via migrations in db/migrations/
-- sqlc generates Go code from queries in db/queries/

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

-- sqlc queries go in db/queries/licenses.sql
-- name: GetUserLicenses :many
SELECT * FROM user_licenses
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: GetLicenseByID :one
SELECT * FROM user_licenses
WHERE id = $1;
```

## Aviation Domain Knowledge

### Currency Calculations

Implement different currency rules per license type:

```go
type CurrencyService struct {
    flightRepo repository.FlightRepository
}

// EASA PPL: 3 takeoffs/landings in last 90 days
func (s *CurrencyService) CheckEasaPPLCurrency(ctx context.Context, userID, licenseID uuid.UUID) (bool, error) {
    ninetyDaysAgo := time.Now().AddDate(0, 0, -90)
    
    flights, err := s.flightRepo.GetFlightsByDateRange(ctx, userID, licenseID, ninetyDaysAgo, time.Now())
    if err != nil {
        return false, fmt.Errorf("failed to get flights: %w", err)
    }
    
    totalLandings := 0
    for _, flight := range flights {
        totalLandings += flight.LandingsDay + flight.LandingsNight
    }
    
    return totalLandings >= 3, nil
}

// FAA requires separate day/night currency
func (s *CurrencyService) CheckFAANightCurrency(ctx context.Context, userID, licenseID uuid.UUID) (bool, error) {
    ninetyDaysAgo := time.Now().AddDate(0, 0, -90)
    
    var nightLandings int
    err := s.flightRepo.SumNightLandings(ctx, userID, licenseID, ninetyDaysAgo, &nightLandings)
    if err != nil {
        return false, fmt.Errorf("failed to sum night landings: %w", err)
    }
    
    return nightLandings >= 3, nil
}
```

### Flight Validation Rules

```go
func ValidateFlight(flight FlightLogCreate, license License) error {
  // SPL (Sailplane) cannot log night flights
  if (license.licenseType.includes('SPL') && flight.nightTime > 0) {
    throw new ValidationError('Sailplane pilots cannot log night time');
  }
  
  // Night time should not exceed total time
  if (flight.nightTime > flight.totalTime) {
    throw new ValidationError('Night time cannot exceed total time');
  }
  
  // PIC + Dual + Solo should not exceed total time (with tolerance)
  const instructionalTime = flight.picTime + flight.dualTime + flight.soloTime;
  if (instructionalTime > flight.totalTime + 0.1) { // 0.1hr tolerance for rounding
    throw new ValidationError('Instructional time breakdown exceeds total time');
  }
  
  // IFR time requires appropriate rating (implement based on license)
  if (flight.ifrTime > 0 && !hasInstrumentRating(license)) {
    throw new ValidationError('IFR time requires instrument rating');
  }
};
```

### Time Calculations

```typescript
// Calculate total hours per license
export const calculateTotalHours = async (
  userId: string,
  licenseId: string
): Promise<HourTotals> => {
  const aggregates = await prisma.flightLog.aggregate({
    where: { userId, licenseId },
    _sum: {
      totalTime: true,
      picTime: true,
      dualTime: true,
      soloTime: true,
      nightTime: true,
      ifrTime: true,
      landingsDay: true,
      landingsNight: true,
    },
  });
  
  return {
    total: aggregates._sum.totalTime ?? 0,
    pic: aggregates._sum.picTime ?? 0,
    dual: aggregates._sum.dualTime ?? 0,
    solo: aggregates._sum.soloTime ?? 0,
    night: aggregates._sum.nightTime ?? 0,
    ifr: aggregates._sum.ifrTime ?? 0,
    landingsDay: aggregates._sum.landingsDay ?? 0,
    landingsNight: aggregates._sum.landingsNight ?? 0,
  };
};
```

## Security Best Practices

### Password Hashing
```typescript
import bcrypt from 'bcrypt';

export const hashPassword = async (password: string): Promise<string> => {
  return bcrypt.hash(password, 12); // 12 rounds
};

export const verifyPassword = async (
  password: string,
  hash: string
): Promise<boolean> => {
  return bcrypt.compare(password, hash);
};
```

### JWT Tokens
```typescript
export const generateTokens = (userId: string) => {
  const accessToken = jwt.sign(
    { sub: userId, type: 'access' },
    process.env.JWT_SECRET,
    { expiresIn: '15m' }
  );
  
  const refreshToken = jwt.sign(
    { sub: userId, type: 'refresh' },
    process.env.REFRESH_SECRET,
    { expiresIn: '7d' }
  );
  
  return { accessToken, refreshToken };
};
```

### Rate Limiting
```typescript
import rateLimit from '@fastify/rate-limit';

// Stricter limits on auth endpoints
app.register(rateLimit, {
  max: 5,
  timeWindow: '15 minutes',
  routePrefix: '/api/auth',
});
```

## Testing Standards

### Unit Tests
```typescript
describe('FlightLogService', () => {
  it('should create flight log with valid data', async () => {
    const service = new FlightLogService();
    const data = createMockFlightData();
    
    const flight = await service.create('user-id', data);
    
    expect(flight).toBeDefined();
    expect(flight.userId).toBe('user-id');
  });
  
  it('should reject SPL night flights', async () => {
    const service = new FlightLogService();
    const data = createMockFlightData({ 
      nightTime: 1.5,
      licenseType: 'EASA_SPL' 
    });
    
    await expect(service.create('user-id', data))
      .rejects
      .toThrow('SPL cannot log night flights');
  });
});
```

### Integration Tests
```typescript
describe('POST /api/flights', () => {
  it('should create flight log', async () => {
    const token = await getAuthToken();
    const response = await app.inject({
      method: 'POST',
      url: '/api/flights',
      headers: { authorization: `Bearer ${token}` },
      payload: mockFlightData,
    });
    
    expect(response.statusCode).toBe(201);
    expect(response.json()).toMatchObject({
      totalTime: mockFlightData.totalTime,
    });
  });
});
```

## Performance Optimization

### Database Indexing
```prisma
model FlightLog {
  // ... fields ...
  
  @@index([userId, date])
  @@index([licenseId, date])
  @@index([userId, licenseId, date])
}
```

### Query Optimization
```typescript
// Use select to fetch only needed fields
const flights = await prisma.flightLog.findMany({
  where: { userId },
  select: {
    id: true,
    date: true,
    totalTime: true,
    // Don't fetch remarks, etc. for list view
  },
});

// Use pagination for large datasets
const flights = await prisma.flightLog.findMany({
  where: { userId },
  skip: (page - 1) * pageSize,
  take: pageSize,
});
```

## OpenAPI Validation

```typescript
import OpenApiValidator from 'express-openapi-validator';

app.use(OpenApiValidator.middleware({
  apiSpec: '../pilotlog-project/api-spec/openapi.yaml',
  validateRequests: true,
  validateResponses: true,
  operationHandlers: path.join(__dirname, 'api/routes'),
}));
```

## Error Handling

```typescript
export class AppError extends Error {
  constructor(
    public statusCode: number,
    public message: string,
    public isOperational = true
  ) {
    super(message);
  }
}

// Global error handler
app.setErrorHandler((error, req, reply) => {
  if (error instanceof AppError) {
    return reply.code(error.statusCode).send({
      error: error.message,
    });
  }
  
  // Log unexpected errors
  logger.error(error);
  
  return reply.code(500).send({
    error: 'Internal server error',
  });
});
```

## Related Repositories

- **pilotlog-project**: OpenAPI spec and documentation
- **pilotlog-frontend**: React web application

Coordinate changes that affect:
- API endpoints (update spec first)
- Data models (migration plan needed)
- Authentication flow (both repos affected)

## Environment Variables

```env
NODE_ENV=development
PORT=3000
DATABASE_URL=postgresql://user:pass@localhost:5432/pilotlog
JWT_SECRET=your-secret-key
REFRESH_SECRET=your-refresh-secret
JWT_EXPIRES_IN=15m
REFRESH_EXPIRES_IN=7d
CORS_ORIGIN=http://localhost:5173
LOG_LEVEL=info
```

## Common Commands

- `air` - Start dev server with live reload
- `go run cmd/api/main.go` - Run without live reload
- `migrate -path db/migrations -database "$DATABASE_URL" up` - Run migrations
- `sqlc generate` - Generate Go code from SQL queries
- `go generate ./...` - Generate OpenAPI code
- `go test ./...` - Run tests
- `go test -cover ./...` - Run tests with coverage
- `golangci-lint run` - Lint code
- `go fmt ./...` - Format code
