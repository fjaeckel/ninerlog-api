package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ---- minimal in-memory FlightSignatureRepository for handler-level tests ----

type mockFlightSignatureRepoH struct {
	byID    map[uuid.UUID]*models.FlightSignature
	byToken map[string]uuid.UUID
}

func newMockFlightSignatureRepoH() *mockFlightSignatureRepoH {
	return &mockFlightSignatureRepoH{
		byID:    make(map[uuid.UUID]*models.FlightSignature),
		byToken: make(map[string]uuid.UUID),
	}
}

func (m *mockFlightSignatureRepoH) Create(ctx context.Context, sig *models.FlightSignature) error {
	if sig.Status == models.SignatureStatusPending {
		for _, existing := range m.byID {
			if existing.FlightID == sig.FlightID && existing.Status == models.SignatureStatusPending {
				return repository.ErrDuplicate
			}
		}
	}
	if sig.ID == uuid.Nil {
		sig.ID = uuid.New()
	}
	sig.CreatedAt = time.Now()
	sig.UpdatedAt = time.Now()
	cp := *sig
	m.byID[sig.ID] = &cp
	if sig.TokenHash != nil {
		m.byToken[*sig.TokenHash] = sig.ID
	}
	return nil
}

func (m *mockFlightSignatureRepoH) GetByID(ctx context.Context, id uuid.UUID) (*models.FlightSignature, error) {
	sig, ok := m.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cp := *sig
	return &cp, nil
}

func (m *mockFlightSignatureRepoH) GetByTokenHash(ctx context.Context, tokenHash string) (*models.FlightSignature, error) {
	id, ok := m.byToken[tokenHash]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return m.GetByID(ctx, id)
}

func (m *mockFlightSignatureRepoH) GetPendingByFlightID(ctx context.Context, flightID uuid.UUID) (*models.FlightSignature, error) {
	for _, sig := range m.byID {
		if sig.FlightID == flightID && sig.Status == models.SignatureStatusPending {
			cp := *sig
			return &cp, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *mockFlightSignatureRepoH) ListByFlightID(ctx context.Context, flightID uuid.UUID) ([]*models.FlightSignature, error) {
	var out []*models.FlightSignature
	for _, sig := range m.byID {
		if sig.FlightID == flightID {
			cp := *sig
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *mockFlightSignatureRepoH) Update(ctx context.Context, sig *models.FlightSignature) error {
	existing, ok := m.byID[sig.ID]
	if !ok {
		return repository.ErrNotFound
	}
	if existing.TokenHash != nil {
		delete(m.byToken, *existing.TokenHash)
	}
	sig.UpdatedAt = time.Now()
	cp := *sig
	m.byID[sig.ID] = &cp
	if sig.TokenHash != nil {
		m.byToken[*sig.TokenHash] = sig.ID
	}
	return nil
}

func (m *mockFlightSignatureRepoH) ExpirePendingPastDue(ctx context.Context) (int64, error) {
	return 0, nil
}

// setupSignatureTestHandlerSharedRepo builds an APIHandler whose
// flightService and flightSignatureService share the same underlying
// FlightRepository instance (required for the signature lock to be
// observable across both services), and seeds one flight owned by userID.
func setupSignatureTestHandlerSharedRepo(t *testing.T) (h *APIHandler, userID uuid.UUID, flight *models.Flight) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	flightRepo := newMockFlightRepo()
	flightSvc := service.NewFlightService(flightRepo, nil)

	sigRepo := newMockFlightSignatureRepoH()
	userRepo := newHandlerMockUserRepo()
	userID = uuid.New()
	// mockUserRepo.Create always mints a fresh random ID (mirrors
	// production, where the ID isn't known until Create returns), so seed
	// the map directly to keep this test's userID stable and predictable.
	userRepo.users[userID] = &models.User{ID: userID, Email: "owner@example.com", Name: "Owner", PreferredLocale: "en"}
	sigSvc := service.NewFlightSignatureService(sigRepo, flightRepo, userRepo)

	jwtMgr := jwt.NewManager("test-access", "test-refresh", 15*time.Minute, 7*24*time.Hour)
	h = &APIHandler{
		authService:            service.NewAuthService(userRepo, newHandlerMockRefreshTokenRepo(), &mockPasswordResetRepo{}, &mockEmailVerificationRepo{}, jwtMgr),
		flightService:          flightSvc,
		flightSignatureService: sigSvc,
		jwtManager:             jwtMgr,
		adminEmail:             "admin@test.com",
	}

	flight = &models.Flight{
		UserID:       userID,
		Date:         time.Now(),
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    90,
	}
	if err := h.flightService.CreateFlight(context.Background(), flight); err != nil {
		t.Fatalf("seed flight: %v", err)
	}
	return h, userID, flight
}

func TestSignFlightLive_Success(t *testing.T) {
	h, userID, flight := setupSignatureTestHandlerSharedRepo(t)

	body := `{"signerName":"Jane Instructor","signatureImage":"aGVsbG8="}`
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("POST", "/flights/"+flight.ID.String()+"/signatures/live", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.SignFlightLive(c, generated.FlightId(flight.ID))

	if w.Code != http.StatusCreated {
		t.Fatalf("SignFlightLive() status = %d, want 201, body=%s", w.Code, w.Body.String())
	}
	var resp generated.FlightSignature
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Status != generated.FlightSignatureStatusCompleted {
		t.Errorf("status = %s, want completed", resp.Status)
	}
}

func TestSignFlightLive_LockedReturnsConflict(t *testing.T) {
	h, userID, flight := setupSignatureTestHandlerSharedRepo(t)

	body := `{"signerName":"Jane Instructor","signatureImage":"aGVsbG8="}`
	first := httptest.NewRecorder()
	c1 := authenticatedContext(first, userID)
	c1.Request = httptest.NewRequest("POST", "/flights/"+flight.ID.String()+"/signatures/live", bytes.NewBufferString(body))
	c1.Request.Header.Set("Content-Type", "application/json")
	h.SignFlightLive(c1, generated.FlightId(flight.ID))
	if first.Code != http.StatusCreated {
		t.Fatalf("first sign status = %d, want 201, body=%s", first.Code, first.Body.String())
	}

	second := httptest.NewRecorder()
	c2 := authenticatedContext(second, userID)
	c2.Request = httptest.NewRequest("POST", "/flights/"+flight.ID.String()+"/signatures/live", bytes.NewBufferString(body))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.SignFlightLive(c2, generated.FlightId(flight.ID))

	if second.Code != http.StatusConflict {
		t.Errorf("second sign status = %d, want 409", second.Code)
	}
}

func TestUpdateFlight_LockedReturnsConflict(t *testing.T) {
	h, userID, flight := setupSignatureTestHandlerSharedRepo(t)

	body := `{"signerName":"Jane Instructor","signatureImage":"aGVsbG8="}`
	sign := httptest.NewRecorder()
	cSign := authenticatedContext(sign, userID)
	cSign.Request = httptest.NewRequest("POST", "/flights/"+flight.ID.String()+"/signatures/live", bytes.NewBufferString(body))
	cSign.Request.Header.Set("Content-Type", "application/json")
	h.SignFlightLive(cSign, generated.FlightId(flight.ID))
	if sign.Code != http.StatusCreated {
		t.Fatalf("sign status = %d, want 201", sign.Code)
	}

	updateBody := `{"totalTime":120}`
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("PUT", "/flights/"+flight.ID.String(), bytes.NewBufferString(updateBody))
	c.Request.Header.Set("Content-Type", "application/json")
	h.UpdateFlight(c, generated.FlightId(flight.ID))

	if w.Code != http.StatusConflict {
		t.Errorf("UpdateFlight() on signed flight status = %d, want 409, body=%s", w.Code, w.Body.String())
	}
}

func TestCreateSignatureRequest_LinkOnly_NoEmailSent(t *testing.T) {
	h, userID, flight := setupSignatureTestHandlerSharedRepo(t)

	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("POST", "/flights/"+flight.ID.String()+"/signatures", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreateSignatureRequest(c, generated.FlightId(flight.ID))

	if w.Code != http.StatusCreated {
		t.Fatalf("CreateSignatureRequest() status = %d, want 201, body=%s", w.Code, w.Body.String())
	}
	var resp generated.SignatureRequestCreated
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.SignUrl == "" {
		t.Error("expected a non-empty signUrl for link-mode request")
	}
	if resp.Status != generated.SignatureRequestCreatedStatusPending {
		t.Errorf("status = %s, want pending", resp.Status)
	}
}

func TestVoidFlightSignature_UnlocksFlight(t *testing.T) {
	h, userID, flight := setupSignatureTestHandlerSharedRepo(t)

	body := `{"signerName":"Jane Instructor","signatureImage":"aGVsbG8="}`
	sign := httptest.NewRecorder()
	cSign := authenticatedContext(sign, userID)
	cSign.Request = httptest.NewRequest("POST", "/flights/"+flight.ID.String()+"/signatures/live", bytes.NewBufferString(body))
	cSign.Request.Header.Set("Content-Type", "application/json")
	h.SignFlightLive(cSign, generated.FlightId(flight.ID))
	var sig generated.FlightSignature
	_ = json.Unmarshal(sign.Body.Bytes(), &sig)

	voidBody := `{"reason":"typo, correcting"}`
	w := httptest.NewRecorder()
	c := authenticatedContext(w, userID)
	c.Request = httptest.NewRequest("POST", "/flights/"+flight.ID.String()+"/signatures/"+sig.Id.String()+"/void", bytes.NewBufferString(voidBody))
	c.Request.Header.Set("Content-Type", "application/json")
	h.VoidFlightSignature(c, generated.FlightId(flight.ID), generated.SignatureId(sig.Id))

	if w.Code != http.StatusOK {
		t.Fatalf("VoidFlightSignature() status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	// Flight should be editable again now.
	updateBody := `{"totalTime":120}`
	w2 := httptest.NewRecorder()
	c2 := authenticatedContext(w2, userID)
	c2.Request = httptest.NewRequest("PUT", "/flights/"+flight.ID.String(), bytes.NewBufferString(updateBody))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.UpdateFlight(c2, generated.FlightId(flight.ID))
	if w2.Code != http.StatusOK {
		t.Errorf("UpdateFlight() after void status = %d, want 200, body=%s", w2.Code, w2.Body.String())
	}
}

// ---- public /sign/{token} endpoints ----

func TestGetPublicSignatureInfo_UnknownTokenReturns404NotUnauthorized(t *testing.T) {
	h, _, _ := setupSignatureTestHandlerSharedRepo(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/sign/does-not-exist", nil)

	h.GetPublicSignatureInfo(c, "does-not-exist")

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("public signing endpoint must never return 401/403 (got %d) — would trigger the frontend's login redirect for an anonymous instructor", w.Code)
	}
}

func TestCompletePublicSignature_FullRoundTrip(t *testing.T) {
	h, userID, flight := setupSignatureTestHandlerSharedRepo(t)

	createW := httptest.NewRecorder()
	cCreate := authenticatedContext(createW, userID)
	cCreate.Request = httptest.NewRequest("POST", "/flights/"+flight.ID.String()+"/signatures", bytes.NewBufferString(`{}`))
	cCreate.Request.Header.Set("Content-Type", "application/json")
	h.CreateSignatureRequest(cCreate, generated.FlightId(flight.ID))
	if createW.Code != http.StatusCreated {
		t.Fatalf("CreateSignatureRequest() status = %d, want 201, body=%s", createW.Code, createW.Body.String())
	}
	var created generated.SignatureRequestCreated
	_ = json.Unmarshal(createW.Body.Bytes(), &created)

	token := extractTokenFromSignURL(t, created.SignUrl)

	infoW := httptest.NewRecorder()
	cInfo, _ := gin.CreateTestContext(infoW)
	cInfo.Request = httptest.NewRequest("GET", "/sign/"+token, nil)
	h.GetPublicSignatureInfo(cInfo, token)
	if infoW.Code != http.StatusOK {
		t.Fatalf("GetPublicSignatureInfo() status = %d, want 200, body=%s", infoW.Code, infoW.Body.String())
	}

	completeBody := `{"signerName":"Jane Instructor","signatureImage":"aGVsbG8="}`
	completeW := httptest.NewRecorder()
	cComplete, _ := gin.CreateTestContext(completeW)
	cComplete.Request = httptest.NewRequest("POST", "/sign/"+token, bytes.NewBufferString(completeBody))
	cComplete.Request.Header.Set("Content-Type", "application/json")
	h.CompletePublicSignature(cComplete, token)
	if completeW.Code != http.StatusOK {
		t.Fatalf("CompletePublicSignature() status = %d, want 200, body=%s", completeW.Code, completeW.Body.String())
	}

	// Reusing the same token must fail, not silently double-sign.
	reuseW := httptest.NewRecorder()
	cReuse, _ := gin.CreateTestContext(reuseW)
	cReuse.Request = httptest.NewRequest("POST", "/sign/"+token, bytes.NewBufferString(completeBody))
	cReuse.Request.Header.Set("Content-Type", "application/json")
	h.CompletePublicSignature(cReuse, token)
	if reuseW.Code != http.StatusNotFound {
		t.Errorf("token reuse status = %d, want 404", reuseW.Code)
	}

	// The flight should now be locked.
	getW := httptest.NewRecorder()
	cGet := authenticatedContext(getW, userID)
	cGet.Request = httptest.NewRequest("GET", "/flights/"+flight.ID.String(), nil)
	h.GetFlight(cGet, generated.FlightId(flight.ID))
	var gotFlight generated.Flight
	_ = json.Unmarshal(getW.Body.Bytes(), &gotFlight)
	if gotFlight.SignatureId == nil {
		t.Error("flight not locked after public signature completion")
	}
}

func extractTokenFromSignURL(t *testing.T, signURL string) string {
	t.Helper()
	const marker = "token="
	idx := bytes.Index([]byte(signURL), []byte(marker))
	if idx < 0 {
		t.Fatalf("signUrl %q has no token= param", signURL)
	}
	return signURL[idx+len(marker):]
}
