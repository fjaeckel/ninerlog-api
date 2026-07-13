package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// ---- mocks ----

type mockFlightSignatureRepo struct {
	byID    map[uuid.UUID]*models.FlightSignature
	byToken map[string]uuid.UUID
}

func newMockFlightSignatureRepo() *mockFlightSignatureRepo {
	return &mockFlightSignatureRepo{
		byID:    make(map[uuid.UUID]*models.FlightSignature),
		byToken: make(map[string]uuid.UUID),
	}
}

func (m *mockFlightSignatureRepo) Create(ctx context.Context, sig *models.FlightSignature) error {
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

func (m *mockFlightSignatureRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.FlightSignature, error) {
	sig, ok := m.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cp := *sig
	return &cp, nil
}

func (m *mockFlightSignatureRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*models.FlightSignature, error) {
	id, ok := m.byToken[tokenHash]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return m.GetByID(ctx, id)
}

func (m *mockFlightSignatureRepo) GetPendingByFlightID(ctx context.Context, flightID uuid.UUID) (*models.FlightSignature, error) {
	for _, sig := range m.byID {
		if sig.FlightID == flightID && sig.Status == models.SignatureStatusPending {
			cp := *sig
			return &cp, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *mockFlightSignatureRepo) ListByFlightID(ctx context.Context, flightID uuid.UUID) ([]*models.FlightSignature, error) {
	var out []*models.FlightSignature
	for _, sig := range m.byID {
		if sig.FlightID == flightID {
			cp := *sig
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (m *mockFlightSignatureRepo) Update(ctx context.Context, sig *models.FlightSignature) error {
	existing, ok := m.byID[sig.ID]
	if !ok {
		return repository.ErrNotFound
	}
	// Mirror a real UNIQUE column: rotating token_hash must drop the old
	// value from the index, otherwise a stale token would still resolve.
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

func (m *mockFlightSignatureRepo) ExpirePendingPastDue(ctx context.Context) (int64, error) {
	var count int64
	now := time.Now()
	for _, sig := range m.byID {
		if sig.Status == models.SignatureStatusPending && sig.TokenExpiresAt != nil && sig.TokenExpiresAt.Before(now) {
			sig.Status = models.SignatureStatusExpired
			count++
		}
	}
	return count, nil
}

type mockUserRepoForSignature struct {
	users map[uuid.UUID]*models.User
}

func newMockUserRepoForSignature() *mockUserRepoForSignature {
	return &mockUserRepoForSignature{users: make(map[uuid.UUID]*models.User)}
}

func (m *mockUserRepoForSignature) Create(ctx context.Context, user *models.User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	m.users[user.ID] = user
	return nil
}
func (m *mockUserRepoForSignature) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}
func (m *mockUserRepoForSignature) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (m *mockUserRepoForSignature) Update(ctx context.Context, user *models.User) error {
	m.users[user.ID] = user
	return nil
}
func (m *mockUserRepoForSignature) Delete(ctx context.Context, id uuid.UUID) error {
	delete(m.users, id)
	return nil
}
func (m *mockUserRepoForSignature) IncrementFailedLoginAttempts(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockUserRepoForSignature) ResetFailedLoginAttempts(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockUserRepoForSignature) LockAccount(ctx context.Context, id uuid.UUID, until time.Time) error {
	return nil
}
func (m *mockUserRepoForSignature) MarkEmailVerified(ctx context.Context, id uuid.UUID) error {
	return nil
}

// ---- test setup helper ----

func newSignatureTestService() (*FlightSignatureService, *mockFlightSignatureRepo, *mockFlightRepo, *mockUserRepoForSignature) {
	sigRepo := newMockFlightSignatureRepo()
	flightRepo := newMockFlightRepo()
	userRepo := newMockUserRepoForSignature()
	svc := NewFlightSignatureService(sigRepo, flightRepo, userRepo)
	return svc, sigRepo, flightRepo, userRepo
}

func seedFlight(t *testing.T, flightRepo *mockFlightRepo, userID uuid.UUID) *models.Flight {
	t.Helper()
	f := &models.Flight{
		UserID:       userID,
		Date:         time.Now(),
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    90,
	}
	if err := flightRepo.Create(context.Background(), f); err != nil {
		t.Fatalf("seedFlight: %v", err)
	}
	return f
}

const testSigImage = "not-actually-png-but-nonzero"

// ---- SignLive ----

func TestSignLive_HappyPath(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)

	sig, err := svc.SignLive(context.Background(), flight.ID, userID, "Jane Instructor", nil, []byte(testSigImage), "1.2.3.4", "test-agent")
	if err != nil {
		t.Fatalf("SignLive() error = %v", err)
	}
	if sig.Status != models.SignatureStatusCompleted || sig.Method != models.SignatureMethodLive {
		t.Errorf("SignLive() = %+v, unexpected", sig)
	}

	updated, _ := flightRepo.GetByID(context.Background(), flight.ID)
	if updated.SignatureID == nil || *updated.SignatureID != sig.ID {
		t.Error("SignLive() did not lock the flight")
	}
}

func TestSignLive_AlreadyLocked(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)

	if _, err := svc.SignLive(context.Background(), flight.ID, userID, "Jane", nil, []byte(testSigImage), "", ""); err != nil {
		t.Fatalf("first SignLive() error = %v", err)
	}
	_, err := svc.SignLive(context.Background(), flight.ID, userID, "Jane Again", nil, []byte(testSigImage), "", "")
	if !errors.Is(err, ErrFlightLocked) {
		t.Errorf("second SignLive() error = %v, want ErrFlightLocked", err)
	}
}

func TestSignLive_MissingNameOrImage(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)

	if _, err := svc.SignLive(context.Background(), flight.ID, userID, "  ", nil, []byte(testSigImage), "", ""); !errors.Is(err, ErrSignerNameRequired) {
		t.Errorf("blank name: error = %v, want ErrSignerNameRequired", err)
	}
	if _, err := svc.SignLive(context.Background(), flight.ID, userID, "Jane", nil, nil, "", ""); !errors.Is(err, ErrSignatureImageRequired) {
		t.Errorf("no image: error = %v, want ErrSignatureImageRequired", err)
	}
}

func TestSignLive_ImageTooLarge(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)

	oversized := make([]byte, models.MaxSignatureImageBytes+1)
	if _, err := svc.SignLive(context.Background(), flight.ID, userID, "Jane", nil, oversized, "", ""); !errors.Is(err, ErrSignatureImageTooLarge) {
		t.Errorf("error = %v, want ErrSignatureImageTooLarge", err)
	}
}

func TestSignLive_WrongOwner(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	owner := uuid.New()
	flight := seedFlight(t, flightRepo, owner)

	_, err := svc.SignLive(context.Background(), flight.ID, uuid.New(), "Jane", nil, []byte(testSigImage), "", "")
	if !errors.Is(err, ErrUnauthorizedFlight) {
		t.Errorf("error = %v, want ErrUnauthorizedFlight", err)
	}
}

// ---- CreateRequest / Resend / Revoke ----

func TestCreateRequest_WithEmail_HappyPath(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)

	email := "instructor@example.com"
	sig, token, err := svc.CreateRequest(context.Background(), flight.ID, userID, &email, nil)
	if err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}
	if token == "" {
		t.Error("CreateRequest() did not return a raw token")
	}
	if sig.Status != models.SignatureStatusPending || sig.Method != models.SignatureMethodDeferred {
		t.Errorf("CreateRequest() = %+v, unexpected", sig)
	}
	if sig.InstructorEmail == nil || *sig.InstructorEmail != email {
		t.Error("CreateRequest() did not store instructor email")
	}
}

func TestCreateRequest_SecondPendingRequest_Conflicts(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)

	if _, _, err := svc.CreateRequest(context.Background(), flight.ID, userID, nil, nil); err != nil {
		t.Fatalf("first CreateRequest() error = %v", err)
	}
	_, _, err := svc.CreateRequest(context.Background(), flight.ID, userID, nil, nil)
	if !errors.Is(err, ErrSignaturePendingExists) {
		t.Errorf("second CreateRequest() error = %v, want ErrSignaturePendingExists", err)
	}
}

func TestCreateRequest_ExpiryClamping(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()

	tooLow := -5
	flight1 := seedFlight(t, flightRepo, userID)
	sig1, _, err := svc.CreateRequest(context.Background(), flight1.ID, userID, nil, &tooLow)
	if err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}
	wantMin := time.Now().Add(time.Duration(models.MinSignatureRequestExpiryHours) * time.Hour)
	if sig1.TokenExpiresAt.After(wantMin.Add(time.Minute)) {
		t.Errorf("expiry not clamped to minimum: got %v", sig1.TokenExpiresAt)
	}

	tooHigh := 10000
	flight2 := seedFlight(t, flightRepo, userID)
	sig2, _, err := svc.CreateRequest(context.Background(), flight2.ID, userID, nil, &tooHigh)
	if err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}
	wantMax := time.Now().Add(time.Duration(models.MaxSignatureRequestExpiryHours) * time.Hour)
	if sig2.TokenExpiresAt.After(wantMax.Add(time.Minute)) {
		t.Errorf("expiry not clamped to maximum: got %v", sig2.TokenExpiresAt)
	}
}

func TestCreateRequest_FlightLocked(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)

	if _, err := svc.SignLive(context.Background(), flight.ID, userID, "Jane", nil, []byte(testSigImage), "", ""); err != nil {
		t.Fatalf("SignLive() error = %v", err)
	}
	_, _, err := svc.CreateRequest(context.Background(), flight.ID, userID, nil, nil)
	if !errors.Is(err, ErrFlightLocked) {
		t.Errorf("CreateRequest() on locked flight error = %v, want ErrFlightLocked", err)
	}
}

func TestResend_RotatesTokenAndUpdatesEmail(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)

	sig, firstToken, err := svc.CreateRequest(context.Background(), flight.ID, userID, nil, nil)
	if err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}

	newEmail := "new-instructor@example.com"
	_, secondToken, err := svc.Resend(context.Background(), flight.ID, userID, sig.ID, &newEmail)
	if err != nil {
		t.Fatalf("Resend() error = %v", err)
	}
	if secondToken == firstToken {
		t.Error("Resend() did not rotate the token")
	}

	// Old token must no longer resolve.
	if _, _, err := svc.ResolveToken(context.Background(), firstToken); !errors.Is(err, ErrSignatureTokenInvalid) {
		t.Errorf("old token still resolves: error = %v, want ErrSignatureTokenInvalid", err)
	}
	// New token must resolve.
	if _, _, err := svc.ResolveToken(context.Background(), secondToken); err != nil {
		t.Errorf("new token does not resolve: %v", err)
	}
}

func TestResend_NonPendingRejected(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)

	sig, _, err := svc.CreateRequest(context.Background(), flight.ID, userID, nil, nil)
	if err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}
	if err := svc.Revoke(context.Background(), flight.ID, userID, sig.ID); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if _, _, err := svc.Resend(context.Background(), flight.ID, userID, sig.ID, nil); !errors.Is(err, ErrSignatureNotPending) {
		t.Errorf("Resend() on revoked request error = %v, want ErrSignatureNotPending", err)
	}
}

func TestRevoke_HappyPathAndDoubleRevokeRejected(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)

	sig, _, err := svc.CreateRequest(context.Background(), flight.ID, userID, nil, nil)
	if err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}
	if err := svc.Revoke(context.Background(), flight.ID, userID, sig.ID); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if err := svc.Revoke(context.Background(), flight.ID, userID, sig.ID); !errors.Is(err, ErrSignatureNotPending) {
		t.Errorf("double Revoke() error = %v, want ErrSignatureNotPending", err)
	}
}

// ---- Void ----

func TestVoid_UnlocksFlightAndRequiresReason(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)

	sig, err := svc.SignLive(context.Background(), flight.ID, userID, "Jane", nil, []byte(testSigImage), "", "")
	if err != nil {
		t.Fatalf("SignLive() error = %v", err)
	}

	if err := svc.Void(context.Background(), flight.ID, userID, sig.ID, ""); !errors.Is(err, ErrSignatureReasonRequired) {
		t.Errorf("Void() with blank reason = %v, want ErrSignatureReasonRequired", err)
	}

	if err := svc.Void(context.Background(), flight.ID, userID, sig.ID, "correcting an error"); err != nil {
		t.Fatalf("Void() error = %v", err)
	}

	updated, _ := flightRepo.GetByID(context.Background(), flight.ID)
	if updated.SignatureID != nil {
		t.Error("Void() did not unlock the flight")
	}

	// Re-editing is now allowed (flight unlocked); a fresh signature can be
	// captured (the "revalidation" workflow).
	if _, err := svc.SignLive(context.Background(), flight.ID, userID, "Jane Again", nil, []byte(testSigImage), "", ""); err != nil {
		t.Errorf("re-sign after void error = %v, want nil", err)
	}
}

func TestVoid_NonCompletedRejected(t *testing.T) {
	svc, _, flightRepo, _ := newSignatureTestService()
	userID := uuid.New()
	flight := seedFlight(t, flightRepo, userID)

	sig, _, err := svc.CreateRequest(context.Background(), flight.ID, userID, nil, nil)
	if err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}
	if err := svc.Void(context.Background(), flight.ID, userID, sig.ID, "reason"); !errors.Is(err, ErrSignatureNotCompleted) {
		t.Errorf("Void() on pending request error = %v, want ErrSignatureNotCompleted", err)
	}
}

// ---- Public token flow: enumeration / expiry / reuse safety ----

func TestResolveToken_UnknownAndCompletedTokens_ReturnSameError(t *testing.T) {
	svc, _, flightRepo, userRepo := newSignatureTestService()
	userID := uuid.New()
	_ = userRepo.Create(context.Background(), &models.User{ID: userID, Email: "owner@example.com", Name: "Owner"})
	flight := seedFlight(t, flightRepo, userID)

	_, _, unknownErr := svc.ResolveToken(context.Background(), "this-token-was-never-issued")

	sig, token, err := svc.CreateRequest(context.Background(), flight.ID, userID, nil, nil)
	if err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}
	if _, _, _, err := svc.CompleteFromToken(context.Background(), token, "Jane", nil, []byte(testSigImage), "", ""); err != nil {
		t.Fatalf("CompleteFromToken() error = %v", err)
	}
	_, _, usedErr := svc.ResolveToken(context.Background(), token)

	if !errors.Is(unknownErr, ErrSignatureTokenInvalid) || !errors.Is(usedErr, ErrSignatureTokenInvalid) {
		t.Fatalf("expected both to be ErrSignatureTokenInvalid, got unknown=%v used=%v", unknownErr, usedErr)
	}
	if unknownErr.Error() != usedErr.Error() {
		t.Errorf("unknown-token and used-token errors differ (enumeration risk): %q vs %q", unknownErr, usedErr)
	}
	_ = sig
}

func TestCompleteFromToken_TokenReuseFails(t *testing.T) {
	svc, _, flightRepo, userRepo := newSignatureTestService()
	userID := uuid.New()
	_ = userRepo.Create(context.Background(), &models.User{ID: userID, Email: "owner@example.com", Name: "Owner"})
	flight := seedFlight(t, flightRepo, userID)

	_, token, err := svc.CreateRequest(context.Background(), flight.ID, userID, nil, nil)
	if err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}

	if _, _, _, err := svc.CompleteFromToken(context.Background(), token, "Jane", nil, []byte(testSigImage), "", ""); err != nil {
		t.Fatalf("first CompleteFromToken() error = %v", err)
	}
	if _, _, _, err := svc.CompleteFromToken(context.Background(), token, "Jane", nil, []byte(testSigImage), "", ""); !errors.Is(err, ErrSignatureTokenInvalid) {
		t.Errorf("second CompleteFromToken() (reuse) error = %v, want ErrSignatureTokenInvalid", err)
	}
}

func TestCompleteFromToken_ExpiredTokenRejected(t *testing.T) {
	svc, sigRepo, flightRepo, userRepo := newSignatureTestService()
	userID := uuid.New()
	_ = userRepo.Create(context.Background(), &models.User{ID: userID, Email: "owner@example.com", Name: "Owner"})
	flight := seedFlight(t, flightRepo, userID)

	_, token, err := svc.CreateRequest(context.Background(), flight.ID, userID, nil, nil)
	if err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}
	// Force the stored expiry into the past.
	for _, sig := range sigRepo.byID {
		if sig.FlightID == flight.ID {
			past := time.Now().Add(-time.Hour)
			sig.TokenExpiresAt = &past
		}
	}

	if _, _, err := svc.ResolveToken(context.Background(), token); !errors.Is(err, ErrSignatureTokenExpired) {
		t.Errorf("ResolveToken() on expired token error = %v, want ErrSignatureTokenExpired", err)
	}
	if _, _, _, err := svc.CompleteFromToken(context.Background(), token, "Jane", nil, []byte(testSigImage), "", ""); !errors.Is(err, ErrSignatureTokenExpired) {
		t.Errorf("CompleteFromToken() on expired token error = %v, want ErrSignatureTokenExpired", err)
	}
}

func TestCompleteFromToken_UsesDBSourcedOwnerEmail(t *testing.T) {
	svc, _, flightRepo, userRepo := newSignatureTestService()
	userID := uuid.New()
	_ = userRepo.Create(context.Background(), &models.User{ID: userID, Email: "real-owner@example.com", Name: "Real Owner"})
	flight := seedFlight(t, flightRepo, userID)

	_, token, err := svc.CreateRequest(context.Background(), flight.ID, userID, nil, nil)
	if err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}

	_, ownerEmail, ownerName, err := svc.CompleteFromToken(context.Background(), token, "Jane", nil, []byte(testSigImage), "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("CompleteFromToken() error = %v", err)
	}
	if ownerEmail != "real-owner@example.com" || ownerName != "Real Owner" {
		t.Errorf("CompleteFromToken() returned owner (%s, %s), want DB-sourced owner", ownerEmail, ownerName)
	}

	updated, _ := flightRepo.GetByID(context.Background(), flight.ID)
	if updated.SignatureID == nil {
		t.Error("CompleteFromToken() did not lock the flight")
	}
}
