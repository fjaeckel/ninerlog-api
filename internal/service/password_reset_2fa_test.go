package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/hash"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
)

// A password reset must not be a way around the second factor: whoever controls
// the mailbox controls the reset link, so if the link alone stripped 2FA the
// factor would add nothing against a compromised mailbox. These tests pin the
// rule that a 2FA account must also prove the factor during the reset, and that
// the enrolment survives it.

// copyingUserRepo returns detached copies from the read methods, the way the
// Postgres repository does. The plain mockUserRepo hands out pointers into its
// own map, which hides aliasing bugs — notably a stale user struct being written
// back over a recovery code that was just consumed.
type copyingUserRepo struct {
	*mockUserRepo
}

func copyUser(u *models.User) *models.User {
	c := *u
	c.RecoveryCodes = append(c.RecoveryCodes[:0:0], u.RecoveryCodes...)
	return &c
}

func (m *copyingUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	u, err := m.mockUserRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return copyUser(u), nil
}

func (m *copyingUserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	u, err := m.mockUserRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return copyUser(u), nil
}

type resetFixture struct {
	auth      *service.AuthService
	twoFactor *service.TwoFactorService
	users     *mockUserRepo
	email     string
	password  string
}

func newResetFixture(t *testing.T) *resetFixture {
	t.Helper()

	userRepo := newMockUserRepo()
	jwtManager := jwt.NewManager("test-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	repo := &copyingUserRepo{mockUserRepo: userRepo}
	twoFactor := service.NewTwoFactorService(repo, jwtManager, nil)
	auth := service.NewAuthService(repo, newMockRefreshTokenRepo(), newMockPasswordResetRepo(),
		newMockEmailVerificationRepo(), jwtManager, twoFactor)

	f := &resetFixture{
		auth:      auth,
		twoFactor: twoFactor,
		users:     userRepo,
		email:     "pilot@example.com",
		password:  "originalpassword1234",
	}

	if _, _, err := auth.Register(context.Background(), service.RegisterInput{
		Email:    f.email,
		Password: f.password,
		Name:     "Test Pilot",
	}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	return f
}

func (f *resetFixture) user(t *testing.T) *models.User {
	t.Helper()
	u, err := f.users.GetByEmail(context.Background(), f.email)
	if err != nil {
		t.Fatalf("GetByEmail failed: %v", err)
	}
	return u
}

// enable2FA completes the TOTP enrolment and returns the secret plus the
// plaintext recovery codes.
func (f *resetFixture) enable2FA(t *testing.T) (string, []string) {
	t.Helper()
	ctx := context.Background()
	userID := f.user(t).ID

	secret, _, err := f.twoFactor.SetupTOTP(ctx, userID)
	if err != nil {
		t.Fatalf("SetupTOTP failed: %v", err)
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode failed: %v", err)
	}
	recoveryCodes, err := f.twoFactor.VerifyAndEnable(ctx, userID, code)
	if err != nil {
		t.Fatalf("VerifyAndEnable failed: %v", err)
	}
	return secret, recoveryCodes
}

func (f *resetFixture) requestToken(t *testing.T) string {
	t.Helper()
	reset, err := f.auth.RequestPasswordReset(context.Background(), f.email)
	if err != nil {
		t.Fatalf("RequestPasswordReset failed: %v", err)
	}
	if reset.Token == "" {
		t.Fatal("Expected a reset token")
	}
	return reset.Token
}

func TestResetPasswordWithoutTwoFactorCodeIsRejected(t *testing.T) {
	f := newResetFixture(t)
	f.enable2FA(t)
	token := f.requestToken(t)

	_, err := f.auth.ResetPassword(context.Background(), token, "brandnewpassword1234", "")
	if err != service.ErrTwoFactorRequired {
		t.Fatalf("Expected ErrTwoFactorRequired, got %v", err)
	}

	// The old password must still be the one that works.
	if _, _, err := f.auth.Login(context.Background(), service.LoginInput{
		Email:    f.email,
		Password: f.password,
	}); err != nil {
		t.Errorf("Original password should still be valid, got %v", err)
	}
	if !f.user(t).TwoFactorEnabled {
		t.Error("2FA must stay enabled after a rejected reset")
	}
}

func TestResetPasswordWithWrongTwoFactorCodeIsRejected(t *testing.T) {
	f := newResetFixture(t)
	secret, _ := f.enable2FA(t)
	token := f.requestToken(t)

	_, err := f.auth.ResetPassword(context.Background(), token, "brandnewpassword1234", "000000")
	if err != service.ErrInvalidTOTPCode {
		t.Fatalf("Expected ErrInvalidTOTPCode, got %v", err)
	}

	// A failed attempt does not consume the reset token, so the same link still
	// works once the user gets the code right.
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode failed: %v", err)
	}
	if _, err := f.auth.ResetPassword(context.Background(), token, "brandnewpassword1234", code); err != nil {
		t.Fatalf("Retry with a valid code should succeed, got %v", err)
	}
}

func TestResetPasswordWithValidTOTPKeepsTwoFactorEnabled(t *testing.T) {
	f := newResetFixture(t)
	secret, _ := f.enable2FA(t)
	token := f.requestToken(t)

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode failed: %v", err)
	}

	newPassword := "brandnewpassword1234"
	result, err := f.auth.ResetPassword(context.Background(), token, newPassword, code)
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}

	if !result.TwoFactorEnabled {
		t.Error("Expected result to report 2FA still enabled")
	}
	if result.Email != f.email {
		t.Errorf("Expected result email %q, got %q", f.email, result.Email)
	}
	if result.Name != "Test Pilot" {
		t.Errorf("Expected result name Test Pilot, got %q", result.Name)
	}

	u := f.user(t)
	if !u.TwoFactorEnabled {
		t.Error("2FA must survive a password reset")
	}
	if u.TwoFactorSecret == nil {
		t.Error("The TOTP secret must survive a password reset")
	}
	if hash.ComparePassword(u.PasswordHash, newPassword) != nil {
		t.Error("Expected the new password to be stored")
	}
}

func TestResetPasswordAcceptsRecoveryCodeAndConsumesIt(t *testing.T) {
	f := newResetFixture(t)
	_, recoveryCodes := f.enable2FA(t)
	if len(recoveryCodes) < 2 {
		t.Fatalf("Expected several recovery codes, got %d", len(recoveryCodes))
	}
	before := len(f.user(t).RecoveryCodes)

	token := f.requestToken(t)
	if _, err := f.auth.ResetPassword(context.Background(), token, "brandnewpassword1234", recoveryCodes[0]); err != nil {
		t.Fatalf("ResetPassword with a recovery code failed: %v", err)
	}

	after := f.user(t).RecoveryCodes
	if len(after) != before-1 {
		t.Errorf("Expected the recovery code to be consumed (%d -> %d), got %d", before, before-1, len(after))
	}
	for _, stored := range after {
		if hash.ComparePassword(stored, recoveryCodes[0]) == nil {
			t.Error("The used recovery code was written back and is usable again")
		}
	}
	if !f.user(t).TwoFactorEnabled {
		t.Error("2FA must stay enabled after a recovery-code reset")
	}
}

func TestResetPasswordWithoutValidatorFailsClosed(t *testing.T) {
	userRepo := newMockUserRepo()
	jwtManager := jwt.NewManager("test-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	repo := &copyingUserRepo{mockUserRepo: userRepo}
	// Deliberately no validator wired.
	auth := service.NewAuthService(repo, newMockRefreshTokenRepo(), newMockPasswordResetRepo(),
		newMockEmailVerificationRepo(), jwtManager, nil)

	ctx := context.Background()
	if _, _, err := auth.Register(ctx, service.RegisterInput{
		Email:    "pilot@example.com",
		Password: "originalpassword1234",
		Name:     "Test Pilot",
	}); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	stored, err := userRepo.GetByEmail(ctx, "pilot@example.com")
	if err != nil {
		t.Fatalf("GetByEmail failed: %v", err)
	}
	secret := "JBSWY3DPEHPK3PXP"
	stored.TwoFactorEnabled = true
	stored.TwoFactorSecret = &secret

	reset, err := auth.RequestPasswordReset(ctx, "pilot@example.com")
	if err != nil {
		t.Fatalf("RequestPasswordReset failed: %v", err)
	}

	_, err = auth.ResetPassword(ctx, reset.Token, "brandnewpassword1234", "123456")
	if err != service.ErrTwoFactorUnavailable {
		t.Fatalf("Expected ErrTwoFactorUnavailable, got %v", err)
	}
}

func TestResetPasswordWithoutTwoFactorNeedsNoCode(t *testing.T) {
	f := newResetFixture(t)
	token := f.requestToken(t)

	result, err := f.auth.ResetPassword(context.Background(), token, "brandnewpassword1234", "")
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}
	if result.TwoFactorEnabled {
		t.Error("Expected result to report 2FA disabled")
	}
}

func TestRequestPasswordResetReportsTwoFactorState(t *testing.T) {
	f := newResetFixture(t)
	f.enable2FA(t)

	reset, err := f.auth.RequestPasswordReset(context.Background(), f.email)
	if err != nil {
		t.Fatalf("RequestPasswordReset failed: %v", err)
	}
	if !reset.TwoFactorEnabled {
		t.Error("Expected TwoFactorEnabled so the mail can warn the user up front")
	}
	if reset.Name != "Test Pilot" {
		t.Errorf("Expected Name = Test Pilot, got %q", reset.Name)
	}
}
