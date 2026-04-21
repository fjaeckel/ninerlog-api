package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ---- Mock repositories ----

type mockCredentialRepo struct {
	credentials map[uuid.UUID]*models.Credential
}

func newMockCredentialRepo() *mockCredentialRepo {
	return &mockCredentialRepo{credentials: make(map[uuid.UUID]*models.Credential)}
}

func (m *mockCredentialRepo) Create(_ context.Context, c *models.Credential) error {
	c.ID = uuid.New()
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	m.credentials[c.ID] = c
	return nil
}
func (m *mockCredentialRepo) GetByID(_ context.Context, id uuid.UUID) (*models.Credential, error) {
	c, ok := m.credentials[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return c, nil
}
func (m *mockCredentialRepo) GetByUserID(_ context.Context, userID uuid.UUID) ([]*models.Credential, error) {
	var out []*models.Credential
	for _, c := range m.credentials {
		if c.UserID == userID {
			out = append(out, c)
		}
	}
	return out, nil
}
func (m *mockCredentialRepo) Update(_ context.Context, c *models.Credential) error {
	m.credentials[c.ID] = c
	return nil
}
func (m *mockCredentialRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(m.credentials, id)
	return nil
}

type mockAircraftRepo struct {
	aircraft map[uuid.UUID]*models.Aircraft
}

func newMockAircraftRepo() *mockAircraftRepo {
	return &mockAircraftRepo{aircraft: make(map[uuid.UUID]*models.Aircraft)}
}

func (m *mockAircraftRepo) Create(_ context.Context, a *models.Aircraft) error {
	a.ID = uuid.New()
	a.CreatedAt = time.Now()
	a.UpdatedAt = time.Now()
	m.aircraft[a.ID] = a
	return nil
}
func (m *mockAircraftRepo) GetByID(_ context.Context, id uuid.UUID) (*models.Aircraft, error) {
	a, ok := m.aircraft[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return a, nil
}
func (m *mockAircraftRepo) GetByUserID(_ context.Context, userID uuid.UUID) ([]*models.Aircraft, error) {
	var out []*models.Aircraft
	for _, a := range m.aircraft {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return out, nil
}
func (m *mockAircraftRepo) Update(_ context.Context, a *models.Aircraft) error {
	m.aircraft[a.ID] = a
	return nil
}
func (m *mockAircraftRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(m.aircraft, id)
	return nil
}
func (m *mockAircraftRepo) CountByUserID(_ context.Context, userID uuid.UUID) (int, error) {
	count := 0
	for _, a := range m.aircraft {
		if a.UserID == userID {
			count++
		}
	}
	return count, nil
}

type mockContactRepo struct {
	contacts map[uuid.UUID]*models.Contact
}

func newMockContactRepo() *mockContactRepo {
	return &mockContactRepo{contacts: make(map[uuid.UUID]*models.Contact)}
}

func (m *mockContactRepo) Create(_ context.Context, c *models.Contact) error {
	c.ID = uuid.New()
	c.CreatedAt = time.Now()
	c.UpdatedAt = time.Now()
	m.contacts[c.ID] = c
	return nil
}
func (m *mockContactRepo) GetByID(_ context.Context, id uuid.UUID) (*models.Contact, error) {
	c, ok := m.contacts[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return c, nil
}
func (m *mockContactRepo) GetByUserID(_ context.Context, userID uuid.UUID) ([]*models.Contact, error) {
	var out []*models.Contact
	for _, c := range m.contacts {
		if c.UserID == userID {
			out = append(out, c)
		}
	}
	return out, nil
}
func (m *mockContactRepo) GetByExactName(_ context.Context, userID uuid.UUID, name string) (*models.Contact, error) {
	return nil, repository.ErrNotFound
}
func (m *mockContactRepo) Search(_ context.Context, userID uuid.UUID, query string, limit int) ([]*models.Contact, error) {
	return nil, nil
}
func (m *mockContactRepo) Update(_ context.Context, c *models.Contact) error {
	m.contacts[c.ID] = c
	return nil
}
func (m *mockContactRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(m.contacts, id)
	return nil
}

type mockUserRepo struct {
	users map[uuid.UUID]*models.User
}

func newHandlerMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[uuid.UUID]*models.User)}
}

func (m *mockUserRepo) Create(_ context.Context, u *models.User) error {
	u.ID = uuid.New()
	m.users[u.ID] = u
	return nil
}
func (m *mockUserRepo) GetByID(_ context.Context, id uuid.UUID) (*models.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}
func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (*models.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (m *mockUserRepo) Update(_ context.Context, u *models.User) error {
	m.users[u.ID] = u
	return nil
}
func (m *mockUserRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(m.users, id)
	return nil
}
func (m *mockUserRepo) IncrementFailedLoginAttempts(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (m *mockUserRepo) ResetFailedLoginAttempts(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (m *mockUserRepo) LockAccount(_ context.Context, _ uuid.UUID, _ time.Time) error {
	return nil
}

type mockRefreshTokenRepo struct {
	tokens map[string]*models.RefreshToken
}

func newHandlerMockRefreshTokenRepo() *mockRefreshTokenRepo {
	return &mockRefreshTokenRepo{tokens: make(map[string]*models.RefreshToken)}
}

func (m *mockRefreshTokenRepo) Create(_ context.Context, t *models.RefreshToken) error {
	t.ID = uuid.New()
	m.tokens[t.TokenHash] = t
	return nil
}
func (m *mockRefreshTokenRepo) GetByTokenHash(_ context.Context, h string) (*models.RefreshToken, error) {
	t, ok := m.tokens[h]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return t, nil
}
func (m *mockRefreshTokenRepo) RevokeByTokenHash(_ context.Context, h string) error {
	if t, ok := m.tokens[h]; ok {
		t.Revoked = true
	}
	return nil
}
func (m *mockRefreshTokenRepo) RevokeAllForUser(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockRefreshTokenRepo) DeleteForUser(_ context.Context, _ uuid.UUID) error    { return nil }
func (m *mockRefreshTokenRepo) DeleteExpired(_ context.Context) error                 { return nil }

type mockPasswordResetRepo struct{}

func (m *mockPasswordResetRepo) Create(_ context.Context, _ *models.PasswordResetToken) error {
	return nil
}
func (m *mockPasswordResetRepo) GetByTokenHash(_ context.Context, _ string) (*models.PasswordResetToken, error) {
	return nil, repository.ErrNotFound
}
func (m *mockPasswordResetRepo) MarkAsUsed(_ context.Context, _ string) error       { return nil }
func (m *mockPasswordResetRepo) DeleteExpired(_ context.Context) error              { return nil }
func (m *mockPasswordResetRepo) DeleteForUser(_ context.Context, _ uuid.UUID) error { return nil }

// ---- Mock flight repo ----

type mockFlightRepo struct {
	flights map[uuid.UUID]*models.Flight
}

func newMockFlightRepo() *mockFlightRepo {
	return &mockFlightRepo{flights: make(map[uuid.UUID]*models.Flight)}
}

func (m *mockFlightRepo) Create(_ context.Context, f *models.Flight) error {
	f.ID = uuid.New()
	f.CreatedAt = time.Now()
	f.UpdatedAt = time.Now()
	m.flights[f.ID] = f
	return nil
}
func (m *mockFlightRepo) GetByID(_ context.Context, id uuid.UUID) (*models.Flight, error) {
	f, ok := m.flights[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return f, nil
}
func (m *mockFlightRepo) GetByUserID(_ context.Context, userID uuid.UUID, _ *repository.FlightQueryOptions) ([]*models.Flight, error) {
	var out []*models.Flight
	for _, f := range m.flights {
		if f.UserID == userID {
			out = append(out, f)
		}
	}
	return out, nil
}
func (m *mockFlightRepo) Update(_ context.Context, f *models.Flight) error {
	if _, ok := m.flights[f.ID]; !ok {
		return repository.ErrNotFound
	}
	m.flights[f.ID] = f
	return nil
}
func (m *mockFlightRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(m.flights, id)
	return nil
}
func (m *mockFlightRepo) DeleteAllByUserID(_ context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	for id, f := range m.flights {
		if f.UserID == userID {
			delete(m.flights, id)
			count++
		}
	}
	return count, nil
}
func (m *mockFlightRepo) CountByUserID(_ context.Context, userID uuid.UUID, _ *repository.FlightQueryOptions) (int, error) {
	count := 0
	for _, f := range m.flights {
		if f.UserID == userID {
			count++
		}
	}
	return count, nil
}
func (m *mockFlightRepo) GetStatsByUserID(_ context.Context, userID uuid.UUID, _, _ *time.Time) (*models.FlightStatistics, error) {
	return &models.FlightStatistics{}, nil
}
func (m *mockFlightRepo) GetCurrencyData(_ context.Context, _ uuid.UUID, _ time.Time) (*models.CurrencyData, error) {
	return &models.CurrencyData{}, nil
}

// ---- Test setup helpers ----

func setupTestHandler() (*APIHandler, *mockUserRepo) {
	gin.SetMode(gin.TestMode)

	userRepo := newHandlerMockUserRepo()
	refreshRepo := newHandlerMockRefreshTokenRepo()
	passwordRepo := &mockPasswordResetRepo{}
	jwtMgr := jwt.NewManager("test-access", "test-refresh", 15*time.Minute, 7*24*time.Hour)

	authSvc := service.NewAuthService(userRepo, refreshRepo, passwordRepo, jwtMgr)
	credSvc := service.NewCredentialService(newMockCredentialRepo())
	aircraftSvc := service.NewAircraftService(newMockAircraftRepo())
	contactSvc := service.NewContactService(newMockContactRepo())
	flightSvc := service.NewFlightService(newMockFlightRepo())

	handler := &APIHandler{
		authService:       authSvc,
		credentialService: credSvc,
		aircraftService:   aircraftSvc,
		contactService:    contactSvc,
		flightService:     flightSvc,
		jwtManager:        jwtMgr,
		adminEmail:        "admin@test.com",
	}

	return handler, userRepo
}

func authenticatedContext(w *httptest.ResponseRecorder, userID uuid.UUID) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", userID)
	return c
}

// ---- Auth handler tests ----

func TestRegisterUser_Success(t *testing.T) {
	h, _ := setupTestHandler()

	body := `{"email":"test@example.com","password":"password1234","name":"Test User"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RegisterUser(c)

	if w.Code != http.StatusCreated {
		t.Errorf("RegisterUser() status = %d, want 201", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["accessToken"] == nil {
		t.Error("Expected accessToken in response")
	}
}

func TestRegisterUser_DuplicateEmail(t *testing.T) {
	h, _ := setupTestHandler()

	body := `{"email":"test@example.com","password":"password1234","name":"Test User"}`

	// First registration
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	c1.Request.Header.Set("Content-Type", "application/json")
	h.RegisterUser(c1)

	// Second registration
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.RegisterUser(c2)

	if w2.Code != http.StatusConflict {
		t.Errorf("Duplicate RegisterUser() status = %d, want 409", w2.Code)
	}
}

func TestRegisterUser_InvalidBody(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString("not json"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RegisterUser(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("InvalidBody RegisterUser() status = %d, want 400", w.Code)
	}
}

func TestRegisterUser_ShortPassword(t *testing.T) {
	h, _ := setupTestHandler()

	body := `{"email":"test@example.com","password":"short","name":"Test User"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RegisterUser(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ShortPassword RegisterUser() status = %d, want 400", w.Code)
	}
}

func TestLoginUser_Success(t *testing.T) {
	h, _ := setupTestHandler()

	// Register first
	regBody := `{"email":"login@example.com","password":"password1234","name":"Login User"}`
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(regBody))
	c1.Request.Header.Set("Content-Type", "application/json")
	h.RegisterUser(c1)

	// Login
	loginBody := `{"email":"login@example.com","password":"password1234"}`
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("POST", "/auth/login", bytes.NewBufferString(loginBody))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.LoginUser(c2)

	if w2.Code != http.StatusOK {
		t.Errorf("LoginUser() status = %d, want 200", w2.Code)
	}
}

func TestLoginUser_InvalidCredentials(t *testing.T) {
	h, _ := setupTestHandler()

	body := `{"email":"nobody@example.com","password":"password1234"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/login", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.LoginUser(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("InvalidCredentials LoginUser() status = %d, want 401", w.Code)
	}
}

func TestLoginUser_InvalidBody(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/login", bytes.NewBufferString("{bad"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.LoginUser(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("InvalidBody LoginUser() status = %d, want 400", w.Code)
	}
}

func TestRefreshToken_InvalidBody(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/refresh", bytes.NewBufferString("{bad"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RefreshToken(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("InvalidBody RefreshToken() status = %d, want 400", w.Code)
	}
}

func TestRefreshToken_InvalidToken(t *testing.T) {
	h, _ := setupTestHandler()

	body := `{"refreshToken":"invalid-token"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/refresh", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RefreshToken(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("InvalidToken RefreshToken() status = %d, want 401", w.Code)
	}
}

// ---- User handler tests ----

func TestGetCurrentUser_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/users/me", nil)
	// No userID in context

	h.GetCurrentUser(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Unauthorized GetCurrentUser() status = %d, want 401", w.Code)
	}
}

func TestGetCurrentUser_NotFound(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c := authenticatedContext(w, uuid.New())
	c.Request = httptest.NewRequest("GET", "/users/me", nil)

	h.GetCurrentUser(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("NotFound GetCurrentUser() status = %d, want 404", w.Code)
	}
}

func TestGetCurrentUser_Success(t *testing.T) {
	h, userRepo := setupTestHandler()

	userID := uuid.New()
	userRepo.users[userID] = &models.User{
		ID:    userID,
		Email: "pilot@test.com",
		Name:  "Test Pilot",
	}

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/users/me", nil)

	h.GetCurrentUser(c)

	if w.Code != http.StatusOK {
		t.Errorf("GetCurrentUser() status = %d, want 200", w.Code)
	}
}

func TestUpdateCurrentUser_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PATCH", "/users/me", bytes.NewBufferString(`{"name":"New"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateCurrentUser(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Unauthorized UpdateCurrentUser() status = %d, want 401", w.Code)
	}
}

func TestChangePassword_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/change-password", bytes.NewBufferString(`{"currentPassword":"old","newPassword":"new"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.ChangePassword(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Unauthorized ChangePassword() status = %d, want 401", w.Code)
	}
}

func TestDeleteCurrentUser_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/users/me", bytes.NewBufferString(`{"password":"test"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.DeleteCurrentUser(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Unauthorized DeleteCurrentUser() status = %d, want 401", w.Code)
	}
}

// ---- Credential handler tests ----

func TestListCredentials_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/credentials", nil)

	h.ListCredentials(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Unauthorized ListCredentials() status = %d, want 401", w.Code)
	}
}

func TestListCredentials_Success(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/credentials", nil)

	h.ListCredentials(c)

	if w.Code != http.StatusOK {
		t.Errorf("ListCredentials() status = %d, want 200", w.Code)
	}
}

func TestCreateCredential_InvalidBody(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("POST", "/credentials", bytes.NewBufferString("not json"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateCredential(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("InvalidBody CreateCredential() status = %d, want 400", w.Code)
	}
}

// ---- Aircraft handler tests ----

func TestListAircraft_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/aircraft", nil)

	h.ListAircraft(c, generated.ListAircraftParams{})

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Unauthorized ListAircraft() status = %d, want 401", w.Code)
	}
}

func TestListAircraft_Success(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/aircraft", nil)

	h.ListAircraft(c, generated.ListAircraftParams{})

	if w.Code != http.StatusOK {
		t.Errorf("ListAircraft() status = %d, want 200", w.Code)
	}
}

func TestCreateAircraft_InvalidBody(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("POST", "/aircraft", bytes.NewBufferString("{bad"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateAircraft(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("InvalidBody CreateAircraft() status = %d, want 400", w.Code)
	}
}

func TestCreateAircraft_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/aircraft", bytes.NewBufferString(`{"registration":"D-EFGH"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateAircraft(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Unauthorized CreateAircraft() status = %d, want 401", w.Code)
	}
}

// ---- Contact handler tests ----

func TestListContacts_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/contacts", nil)

	h.ListContacts(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Unauthorized ListContacts() status = %d, want 401", w.Code)
	}
}

func TestListContacts_Success(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/contacts", nil)

	h.ListContacts(c)

	if w.Code != http.StatusOK {
		t.Errorf("ListContacts() status = %d, want 200", w.Code)
	}
}

func TestCreateContact_InvalidBody(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("POST", "/contacts", bytes.NewBufferString("not json"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateContact(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("InvalidBody CreateContact() status = %d, want 400", w.Code)
	}
}

// ---- Airport handler tests ----

func TestGetAirport_NotFound(t *testing.T) {
	h, _ := setupTestHandler()
	// Set empty airport DB
	airports.SetTestDB(map[string]airports.AirportInfo{})
	defer airports.SetTestDB(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/airports/ZZZZ", nil)

	h.GetAirport(c, "ZZZZ")

	if w.Code != http.StatusNotFound {
		t.Errorf("GetAirport(unknown) status = %d, want 404", w.Code)
	}
}

func TestGetAirport_Found(t *testing.T) {
	h, _ := setupTestHandler()
	airports.SetTestDB(map[string]airports.AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt Airport", Latitude: 50.0333, Longitude: 8.5706},
	})
	defer airports.SetTestDB(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/airports/EDDF", nil)

	h.GetAirport(c, "EDDF")

	if w.Code != http.StatusOK {
		t.Errorf("GetAirport(EDDF) status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["icao"] != "EDDF" {
		t.Errorf("Airport ICAO = %v, want EDDF", resp["icao"])
	}
}

func TestGetAirport_CaseInsensitive(t *testing.T) {
	h, _ := setupTestHandler()
	airports.SetTestDB(map[string]airports.AirportInfo{
		"EDDF": {ICAO: "EDDF", Name: "Frankfurt Airport", Latitude: 50.0333, Longitude: 8.5706},
	})
	defer airports.SetTestDB(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/airports/eddf", nil)

	h.GetAirport(c, "eddf")

	if w.Code != http.StatusOK {
		t.Errorf("GetAirport(eddf) status = %d, want 200", w.Code)
	}
}

// ---- sendError tests ----

func TestSendError_BasicMessage(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h.sendError(c, http.StatusBadRequest, "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("sendError status = %d, want 400", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "something went wrong" {
		t.Errorf("error = %v, want 'something went wrong'", resp["error"])
	}
}

func TestSendError_WithDetails(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	h.sendError(c, http.StatusBadRequest, "Validation failed", map[string]string{
		"field":   "email",
		"message": "invalid format",
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("sendError status = %d, want 400", w.Code)
	}
}

// ---- getUserIDFromContext tests ----

func TestGetUserIDFromContext_FromMiddleware(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)

	got, err := h.getUserIDFromContext(c)
	if err != nil {
		t.Fatalf("getUserIDFromContext() error = %v", err)
	}
	if got != userID {
		t.Errorf("getUserIDFromContext() = %v, want %v", got, userID)
	}
}

func TestGetUserIDFromContext_NoAuth(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)

	_, err := h.getUserIDFromContext(c)
	if err == nil {
		t.Error("getUserIDFromContext() should error without auth")
	}
}

func TestGetUserIDFromContext_FromBearerToken(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	token, _ := h.jwtManager.GenerateAccessToken(userID)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/test", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)

	got, err := h.getUserIDFromContext(c)
	if err != nil {
		t.Fatalf("getUserIDFromContext(Bearer) error = %v", err)
	}
	if got != userID {
		t.Errorf("getUserIDFromContext() = %v, want %v", got, userID)
	}
}

// ---- Contact CRUD handler tests ----

func TestCreateContact_Success(t *testing.T) {
	h, _ := setupTestHandler()
	userID := uuid.New()

	body := `{"name":"John Doe","email":"john@example.com"}`
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("POST", "/contacts", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateContact(c)

	if w.Code != http.StatusCreated {
		t.Errorf("CreateContact() status = %d, want 201", w.Code)
	}
}

func TestGetContact_NotFound(t *testing.T) {
	h, _ := setupTestHandler()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/contacts/"+uuid.New().String(), nil)

	h.GetContact(c, uuid.New())

	if w.Code != http.StatusNotFound {
		t.Errorf("GetContact(nonexistent) status = %d, want 404", w.Code)
	}
}

func TestDeleteContact_NotFound(t *testing.T) {
	h, _ := setupTestHandler()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("DELETE", "/contacts/"+uuid.New().String(), nil)

	h.DeleteContact(c, uuid.New())

	if w.Code != http.StatusNotFound {
		t.Errorf("DeleteContact(nonexistent) status = %d, want 404", w.Code)
	}
}

func TestUpdateContact_InvalidBody(t *testing.T) {
	h, _ := setupTestHandler()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("PUT", "/contacts/"+uuid.New().String(), bytes.NewBufferString("{bad"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateContact(c, uuid.New())

	if w.Code != http.StatusBadRequest {
		t.Errorf("UpdateContact(badBody) status = %d, want 400", w.Code)
	}
}

func TestSearchContacts_EmptyQuery(t *testing.T) {
	h, _ := setupTestHandler()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/contacts/search?q=", nil)

	h.SearchContacts(c, generated.SearchContactsParams{Q: ""})

	if w.Code != http.StatusOK {
		t.Errorf("SearchContacts(empty) status = %d, want 200", w.Code)
	}
}

func TestSearchContacts_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/contacts/search?q=test", nil)

	h.SearchContacts(c, generated.SearchContactsParams{Q: "test"})

	if w.Code != http.StatusUnauthorized {
		t.Errorf("SearchContacts(unauth) status = %d, want 401", w.Code)
	}
}

func TestGetContact_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/contacts/"+uuid.New().String(), nil)

	h.GetContact(c, uuid.New())

	if w.Code != http.StatusUnauthorized {
		t.Errorf("GetContact(unauth) status = %d, want 401", w.Code)
	}
}

func TestUpdateContact_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PUT", "/contacts/"+uuid.New().String(), bytes.NewBufferString(`{"name":"x"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateContact(c, uuid.New())

	if w.Code != http.StatusUnauthorized {
		t.Errorf("UpdateContact(unauth) status = %d, want 401", w.Code)
	}
}

func TestDeleteContact_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/contacts/"+uuid.New().String(), nil)

	h.DeleteContact(c, uuid.New())

	if w.Code != http.StatusUnauthorized {
		t.Errorf("DeleteContact(unauth) status = %d, want 401", w.Code)
	}
}

func TestCreateContact_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/contacts", bytes.NewBufferString(`{"name":"x"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateContact(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("CreateContact(unauth) status = %d, want 401", w.Code)
	}
}

// ---- Credential CRUD handler tests ----

func TestGetCredential_NotFound(t *testing.T) {
	h, _ := setupTestHandler()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/credentials/"+uuid.New().String(), nil)

	h.GetCredential(c, uuid.New())

	if w.Code != http.StatusNotFound {
		t.Errorf("GetCredential(nonexistent) status = %d, want 404", w.Code)
	}
}

func TestGetCredential_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/credentials/"+uuid.New().String(), nil)

	h.GetCredential(c, uuid.New())

	if w.Code != http.StatusUnauthorized {
		t.Errorf("GetCredential(unauth) status = %d, want 401", w.Code)
	}
}

func TestDeleteCredential_NotFound(t *testing.T) {
	h, _ := setupTestHandler()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("DELETE", "/credentials/"+uuid.New().String(), nil)

	h.DeleteCredential(c, uuid.New())

	if w.Code != http.StatusNotFound {
		t.Errorf("DeleteCredential(nonexistent) status = %d, want 404", w.Code)
	}
}

func TestDeleteCredential_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/credentials/"+uuid.New().String(), nil)

	h.DeleteCredential(c, uuid.New())

	if w.Code != http.StatusUnauthorized {
		t.Errorf("DeleteCredential(unauth) status = %d, want 401", w.Code)
	}
}

func TestUpdateCredential_InvalidBody2(t *testing.T) {
	h, _ := setupTestHandler()
	userID := uuid.New()

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("PUT", "/credentials/"+uuid.New().String(), bytes.NewBufferString("{bad"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateCredential(c, uuid.New())

	if w.Code != http.StatusBadRequest {
		t.Errorf("UpdateCredential(badBody) status = %d, want 400", w.Code)
	}
}

func TestUpdateCredential_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PUT", "/credentials/"+uuid.New().String(), bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateCredential(c, uuid.New())

	if w.Code != http.StatusUnauthorized {
		t.Errorf("UpdateCredential(unauth) status = %d, want 401", w.Code)
	}
}

// ---- Notification handler tests ----

func TestGetNotificationPreferences_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/users/me/notifications", nil)

	h.GetNotificationPreferences(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("GetNotificationPreferences(unauth) status = %d, want 401", w.Code)
	}
}

func TestUpdateNotificationPreferences_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PATCH", "/users/me/notifications", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateNotificationPreferences(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("UpdateNotificationPreferences(unauth) status = %d, want 401", w.Code)
	}
}

func TestUpdateNotificationPreferences_InvalidBody(t *testing.T) {
	h, _ := setupTestHandler()
	addNotificationService(h)

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("PATCH", "/users/me/notifications", bytes.NewBufferString("{bad"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateNotificationPreferences(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("UpdateNotificationPreferences(badBody) status = %d, want 400", w.Code)
	}
}

func TestGetNotificationHistory_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/users/me/notifications/history", nil)

	h.GetNotificationHistory(c, generated.GetNotificationHistoryParams{})

	if w.Code != http.StatusUnauthorized {
		t.Errorf("GetNotificationHistory(unauth) status = %d, want 401", w.Code)
	}
}

func TestGetNotificationPreferences_Success(t *testing.T) {
	h, _ := setupTestHandler()
	addNotificationService(h)

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/users/me/notifications", nil)

	h.GetNotificationPreferences(c)

	if w.Code != http.StatusOK {
		t.Errorf("GetNotificationPreferences() status = %d, want 200", w.Code)
	}
}

func TestGetNotificationHistory_Success(t *testing.T) {
	h, _ := setupTestHandler()
	addNotificationService(h)

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/users/me/notifications/history", nil)

	h.GetNotificationHistory(c, generated.GetNotificationHistoryParams{})

	if w.Code != http.StatusOK {
		t.Errorf("GetNotificationHistory() status = %d, want 200", w.Code)
	}
}

// ---- 2FA handler tests ----

func TestSetup2FA_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/2fa/setup", nil)

	h.Setup2FA(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Setup2FA(unauth) status = %d, want 401", w.Code)
	}
}

func TestVerify2FA_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/2fa/verify", bytes.NewBufferString(`{"code":"123456"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Verify2FA(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Verify2FA(unauth) status = %d, want 401", w.Code)
	}
}

func TestDisable2FA_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/2fa/disable", bytes.NewBufferString(`{"password":"test"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Disable2FA(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Disable2FA(unauth) status = %d, want 401", w.Code)
	}
}

func TestLogin2FA_InvalidBody(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/2fa/login", bytes.NewBufferString("{bad"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Login2FA(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Login2FA(badBody) status = %d, want 400", w.Code)
	}
}

// ---- User statistics handler tests ----

func TestGetMyStatistics_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/users/me/statistics", nil)

	h.GetMyStatistics(c, generated.GetMyStatisticsParams{})

	if w.Code != http.StatusUnauthorized {
		t.Errorf("GetMyStatistics(unauth) status = %d, want 401", w.Code)
	}
}

// ---- Bulk delete handler tests ----

func TestDeleteAllFlights_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/flights/delete-all", nil)

	h.DeleteAllFlights(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("DeleteAllFlights(unauth) status = %d, want 401", w.Code)
	}
}

func TestDeleteAllUserData_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/users/me/data", nil)

	h.DeleteAllUserData(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("DeleteAllUserData(unauth) status = %d, want 401", w.Code)
	}
}

// ---- Currency handler tests ----

func TestGetAllCurrencyStatus_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/currency", nil)

	h.GetAllCurrencyStatus(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("GetAllCurrencyStatus(unauth) status = %d, want 401", w.Code)
	}
}

// ---- RecalculateFlights handler tests ----

func TestRecalculateFlights_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/flights/recalculate", nil)

	h.RecalculateFlights(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("RecalculateFlights(unauth) status = %d, want 401", w.Code)
	}
}

func TestRecalculateFlights_Success(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("POST", "/flights/recalculate", nil)

	h.RecalculateFlights(c)

	if w.Code != http.StatusOK {
		t.Errorf("RecalculateFlights() status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"] == nil {
		t.Error("Expected 'total' in response")
	}
}

// ---- UpdateCurrentUser handler tests ----

func TestUpdateCurrentUser_Success(t *testing.T) {
	h, userRepo := setupTestHandler()

	userID := uuid.New()
	userRepo.users[userID] = &models.User{
		ID:    userID,
		Email: "pilot@test.com",
		Name:  "Test Pilot",
	}

	body := `{"name":"Updated Name"}`
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("PATCH", "/users/me", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateCurrentUser(c)

	if w.Code != http.StatusOK {
		t.Errorf("UpdateCurrentUser() status = %d, want 200", w.Code)
	}
}

func TestUpdateCurrentUser_InvalidBody(t *testing.T) {
	h, userRepo := setupTestHandler()

	userID := uuid.New()
	userRepo.users[userID] = &models.User{ID: userID, Email: "test@test.com", Name: "Test"}

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("PATCH", "/users/me", bytes.NewBufferString("{bad"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateCurrentUser(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("UpdateCurrentUser(badBody) status = %d, want 400", w.Code)
	}
}

// ---- GetMyStatistics success test ----

func TestGetMyStatistics_Success(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/users/me/statistics", nil)

	h.GetMyStatistics(c, generated.GetMyStatisticsParams{})

	if w.Code != http.StatusOK {
		t.Errorf("GetMyStatistics() status = %d, want 200", w.Code)
	}
}

// ---- Flight handler tests ----

func TestCreateFlight_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/flights", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateFlight(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("CreateFlight(unauth) status = %d, want 401", w.Code)
	}
}

func TestCreateFlight_InvalidBody(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("POST", "/flights", bytes.NewBufferString("{bad"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateFlight(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("CreateFlight(badBody) status = %d, want 400", w.Code)
	}
}

func TestGetFlight_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/flights/"+uuid.New().String(), nil)

	h.GetFlight(c, uuid.New())

	if w.Code != http.StatusUnauthorized {
		t.Errorf("GetFlight(unauth) status = %d, want 401", w.Code)
	}
}

func TestGetFlight_NotFound(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/flights/"+uuid.New().String(), nil)

	h.GetFlight(c, uuid.New())

	if w.Code != http.StatusNotFound {
		t.Errorf("GetFlight(nonexistent) status = %d, want 404", w.Code)
	}
}

func TestDeleteFlight_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/flights/"+uuid.New().String(), nil)

	h.DeleteFlight(c, uuid.New())

	if w.Code != http.StatusUnauthorized {
		t.Errorf("DeleteFlight(unauth) status = %d, want 401", w.Code)
	}
}

func TestDeleteFlight_NotFound(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("DELETE", "/flights/"+uuid.New().String(), nil)

	h.DeleteFlight(c, uuid.New())

	if w.Code != http.StatusNotFound {
		t.Errorf("DeleteFlight(nonexistent) status = %d, want 404", w.Code)
	}
}

func TestUpdateFlight_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PUT", "/flights/"+uuid.New().String(), bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateFlight(c, uuid.New())

	if w.Code != http.StatusUnauthorized {
		t.Errorf("UpdateFlight(unauth) status = %d, want 401", w.Code)
	}
}

func TestUpdateFlight_InvalidBody(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("PUT", "/flights/"+uuid.New().String(), bytes.NewBufferString("{bad"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateFlight(c, uuid.New())

	if w.Code != http.StatusBadRequest {
		t.Errorf("UpdateFlight(badBody) status = %d, want 400", w.Code)
	}
}

func TestListFlights_Unauthorized(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/flights", nil)

	h.ListFlights(c, generated.ListFlightsParams{})

	if w.Code != http.StatusUnauthorized {
		t.Errorf("ListFlights(unauth) status = %d, want 401", w.Code)
	}
}

func TestListFlights_Success(t *testing.T) {
	h, _ := setupTestHandler()

	userID := uuid.New()
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("GET", "/flights", nil)

	h.ListFlights(c, generated.ListFlightsParams{})

	if w.Code != http.StatusOK {
		t.Errorf("ListFlights() status = %d, want 200", w.Code)
	}
}

// ---- UpdateNotificationPreferences success test ----

func TestUpdateNotificationPreferences_Success(t *testing.T) {
	h, _ := setupTestHandler()
	addNotificationService(h)

	userID := uuid.New()
	body := `{"emailEnabled":true,"checkHour":10}`
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("PATCH", "/users/me/notifications", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateNotificationPreferences(c)

	if w.Code != http.StatusOK {
		t.Errorf("UpdateNotificationPreferences() status = %d, want 200", w.Code)
	}
}

func TestUpdateNotificationPreferences_InvalidCheckHour(t *testing.T) {
	h, _ := setupTestHandler()
	addNotificationService(h)

	userID := uuid.New()
	body := `{"checkHour":25}`
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("PATCH", "/users/me/notifications", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdateNotificationPreferences(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("UpdateNotificationPreferences(invalidHour) status = %d, want 400", w.Code)
	}
}

// ---- Utility ----

func addNotificationService(h *APIHandler) {
	notifRepo := &mockHandlerNotifRepo{prefs: make(map[uuid.UUID]*models.NotificationPreferences)}
	h.notificationService = service.NewNotificationService(
		notifRepo, nil, nil, nil, nil, nil, nil,
	)
}

type mockHandlerNotifRepo struct {
	prefs map[uuid.UUID]*models.NotificationPreferences
	logs  []*models.NotificationLog
}

func (m *mockHandlerNotifRepo) GetPreferences(_ context.Context, userID uuid.UUID) (*models.NotificationPreferences, error) {
	p, ok := m.prefs[userID]
	if !ok {
		return &models.NotificationPreferences{UserID: userID}, nil
	}
	return p, nil
}
func (m *mockHandlerNotifRepo) UpsertPreferences(_ context.Context, p *models.NotificationPreferences) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	m.prefs[p.UserID] = p
	return nil
}
func (m *mockHandlerNotifRepo) LogNotification(_ context.Context, l *models.NotificationLog) error {
	m.logs = append(m.logs, l)
	return nil
}
func (m *mockHandlerNotifRepo) HasBeenSent(_ context.Context, _ uuid.UUID, _ string, _ uuid.UUID, _ int, _ *time.Time) (bool, error) {
	return false, nil
}
func (m *mockHandlerNotifRepo) GetAllUsersWithPreferences(_ context.Context) ([]*models.NotificationPreferences, error) {
	return nil, nil
}
func (m *mockHandlerNotifRepo) GetNotificationHistory(_ context.Context, _ uuid.UUID, _, _ int) ([]*models.NotificationLog, int, error) {
	return []*models.NotificationLog{}, 0, nil
}
