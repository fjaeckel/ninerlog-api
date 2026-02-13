package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/fjaeckel/pilotlog-api/internal/service"
	"github.com/fjaeckel/pilotlog-api/pkg/hash"
	"github.com/fjaeckel/pilotlog-api/pkg/jwt"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
)

// mock2FAUserRepo implements repository.UserRepository for 2FA tests
type mock2FAUserRepo struct {
	users map[uuid.UUID]*models.User
}

func newMock2FAUserRepo() *mock2FAUserRepo {
	return &mock2FAUserRepo{users: make(map[uuid.UUID]*models.User)}
}

func (m *mock2FAUserRepo) Create(ctx context.Context, user *models.User) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	m.users[user.ID] = user
	return nil
}

func (m *mock2FAUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return u, nil
}

func (m *mock2FAUserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *mock2FAUserRepo) Update(ctx context.Context, user *models.User) error {
	if _, ok := m.users[user.ID]; !ok {
		return repository.ErrNotFound
	}
	m.users[user.ID] = user
	return nil
}

func (m *mock2FAUserRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(m.users, id)
	return nil
}

func setup2FAService() (*service.TwoFactorService, *mock2FAUserRepo) {
	repo := newMock2FAUserRepo()
	jwtMgr := jwt.NewManager("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	svc := service.NewTwoFactorService(repo, jwtMgr)
	return svc, repo
}

func createTestUserFor2FA(repo *mock2FAUserRepo) *models.User {
	pw, _ := hash.HashPassword("testpassword")
	user := &models.User{
		ID:           uuid.New(),
		Email:        "test2fa@example.com",
		PasswordHash: pw,
		Name:         "2FA Test User",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	repo.users[user.ID] = user
	return user
}

func TestSetupTOTP(t *testing.T) {
	svc, repo := setup2FAService()
	user := createTestUserFor2FA(repo)
	ctx := context.Background()

	secret, url, err := svc.SetupTOTP(ctx, user.ID)
	if err != nil {
		t.Fatalf("SetupTOTP() error = %v", err)
	}
	if secret == "" {
		t.Error("Expected non-empty secret")
	}
	if url == "" {
		t.Error("Expected non-empty URL")
	}

	// Verify the secret was stored on the user
	updated, _ := repo.GetByID(ctx, user.ID)
	if updated.TwoFactorSecret == nil {
		t.Error("TwoFactorSecret should be set after setup")
	}
	if *updated.TwoFactorSecret != secret {
		t.Errorf("Stored secret = %s, want %s", *updated.TwoFactorSecret, secret)
	}
}

func TestSetupTOTP_AlreadyEnabled(t *testing.T) {
	svc, repo := setup2FAService()
	user := createTestUserFor2FA(repo)
	user.TwoFactorEnabled = true
	ctx := context.Background()

	_, _, err := svc.SetupTOTP(ctx, user.ID)
	if err != service.ErrTwoFactorAlreadyEnabled {
		t.Errorf("Expected ErrTwoFactorAlreadyEnabled, got %v", err)
	}
}

func TestSetupTOTP_UserNotFound(t *testing.T) {
	svc, _ := setup2FAService()
	ctx := context.Background()

	_, _, err := svc.SetupTOTP(ctx, uuid.New())
	if err == nil {
		t.Error("Expected error for non-existent user")
	}
}

func TestVerifyAndEnable(t *testing.T) {
	svc, repo := setup2FAService()
	user := createTestUserFor2FA(repo)
	ctx := context.Background()

	secret, _, err := svc.SetupTOTP(ctx, user.ID)
	if err != nil {
		t.Fatalf("SetupTOTP() error = %v", err)
	}

	// Generate a valid TOTP code
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("Failed to generate TOTP code: %v", err)
	}

	recoveryCodes, err := svc.VerifyAndEnable(ctx, user.ID, code)
	if err != nil {
		t.Fatalf("VerifyAndEnable() error = %v", err)
	}
	if len(recoveryCodes) != 8 {
		t.Errorf("Recovery codes count = %d, want 8", len(recoveryCodes))
	}

	// Verify 2FA is now enabled
	updated, _ := repo.GetByID(ctx, user.ID)
	if !updated.TwoFactorEnabled {
		t.Error("TwoFactorEnabled should be true after verification")
	}
	if len(updated.RecoveryCodes) != 8 {
		t.Errorf("Stored recovery codes count = %d, want 8", len(updated.RecoveryCodes))
	}
}

func TestVerifyAndEnable_InvalidCode(t *testing.T) {
	svc, repo := setup2FAService()
	user := createTestUserFor2FA(repo)
	ctx := context.Background()

	_, _, _ = svc.SetupTOTP(ctx, user.ID)

	_, err := svc.VerifyAndEnable(ctx, user.ID, "000000")
	if err != service.ErrInvalidTOTPCode {
		t.Errorf("Expected ErrInvalidTOTPCode, got %v", err)
	}
}

func TestVerifyAndEnable_AlreadyEnabled(t *testing.T) {
	svc, repo := setup2FAService()
	user := createTestUserFor2FA(repo)
	user.TwoFactorEnabled = true
	ctx := context.Background()

	_, err := svc.VerifyAndEnable(ctx, user.ID, "123456")
	if err != service.ErrTwoFactorAlreadyEnabled {
		t.Errorf("Expected ErrTwoFactorAlreadyEnabled, got %v", err)
	}
}

func TestDisable2FA(t *testing.T) {
	svc, repo := setup2FAService()
	user := createTestUserFor2FA(repo)
	ctx := context.Background()

	// Setup and enable 2FA
	secret, _, _ := svc.SetupTOTP(ctx, user.ID)
	code, _ := totp.GenerateCode(secret, time.Now())
	_, _ = svc.VerifyAndEnable(ctx, user.ID, code)

	// Disable with correct password
	err := svc.Disable(ctx, user.ID, "testpassword")
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	updated, _ := repo.GetByID(ctx, user.ID)
	if updated.TwoFactorEnabled {
		t.Error("TwoFactorEnabled should be false after disable")
	}
	if updated.TwoFactorSecret != nil {
		t.Error("TwoFactorSecret should be nil after disable")
	}
}

func TestDisable2FA_WrongPassword(t *testing.T) {
	svc, repo := setup2FAService()
	user := createTestUserFor2FA(repo)
	ctx := context.Background()

	secret, _, _ := svc.SetupTOTP(ctx, user.ID)
	code, _ := totp.GenerateCode(secret, time.Now())
	_, _ = svc.VerifyAndEnable(ctx, user.ID, code)

	err := svc.Disable(ctx, user.ID, "wrongpassword")
	if err == nil {
		t.Error("Expected error for wrong password")
	}
}

func TestDisable2FA_NotEnabled(t *testing.T) {
	svc, repo := setup2FAService()
	user := createTestUserFor2FA(repo)
	ctx := context.Background()

	err := svc.Disable(ctx, user.ID, "testpassword")
	if err != service.ErrTwoFactorNotEnabled {
		t.Errorf("Expected ErrTwoFactorNotEnabled, got %v", err)
	}
	_ = repo
}

func TestValidateTOTP(t *testing.T) {
	svc, repo := setup2FAService()
	user := createTestUserFor2FA(repo)
	ctx := context.Background()

	secret, _, _ := svc.SetupTOTP(ctx, user.ID)
	code, _ := totp.GenerateCode(secret, time.Now())
	_, _ = svc.VerifyAndEnable(ctx, user.ID, code)

	// Validate with a fresh TOTP code
	freshCode, _ := totp.GenerateCode(secret, time.Now())
	valid, err := svc.ValidateTOTP(ctx, user.ID, freshCode)
	if err != nil {
		t.Fatalf("ValidateTOTP() error = %v", err)
	}
	if !valid {
		t.Error("ValidateTOTP() = false, want true for valid code")
	}
}

func TestValidateTOTP_InvalidCode(t *testing.T) {
	svc, repo := setup2FAService()
	user := createTestUserFor2FA(repo)
	ctx := context.Background()

	secret, _, _ := svc.SetupTOTP(ctx, user.ID)
	code, _ := totp.GenerateCode(secret, time.Now())
	_, _ = svc.VerifyAndEnable(ctx, user.ID, code)

	valid, err := svc.ValidateTOTP(ctx, user.ID, "000000")
	if err != nil {
		t.Fatalf("ValidateTOTP() error = %v", err)
	}
	if valid {
		t.Error("ValidateTOTP() = true, want false for invalid code")
	}
}

func TestValidateTOTP_NotEnabled(t *testing.T) {
	svc, repo := setup2FAService()
	user := createTestUserFor2FA(repo)
	ctx := context.Background()

	_, err := svc.ValidateTOTP(ctx, user.ID, "123456")
	if err != service.ErrTwoFactorNotEnabled {
		t.Errorf("Expected ErrTwoFactorNotEnabled, got %v", err)
	}
	_ = repo
}

func TestIsEnabled(t *testing.T) {
	svc, repo := setup2FAService()
	user := createTestUserFor2FA(repo)
	ctx := context.Background()

	// Initially not enabled
	enabled, err := svc.IsEnabled(ctx, user.ID)
	if err != nil {
		t.Fatalf("IsEnabled() error = %v", err)
	}
	if enabled {
		t.Error("IsEnabled() = true, want false initially")
	}

	// Enable 2FA
	secret, _, _ := svc.SetupTOTP(ctx, user.ID)
	code, _ := totp.GenerateCode(secret, time.Now())
	_, _ = svc.VerifyAndEnable(ctx, user.ID, code)

	enabled, err = svc.IsEnabled(ctx, user.ID)
	if err != nil {
		t.Fatalf("IsEnabled() error = %v", err)
	}
	if !enabled {
		t.Error("IsEnabled() = false, want true after enabling")
	}
}

func TestIsEnabled_UserNotFound(t *testing.T) {
	svc, _ := setup2FAService()
	ctx := context.Background()

	_, err := svc.IsEnabled(ctx, uuid.New())
	if err == nil {
		t.Error("Expected error for non-existent user")
	}
}
