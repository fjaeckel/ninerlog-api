package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
)

// setupAuthServiceWithRepo builds an AuthService and returns the underlying
// user repo so tests can flip account-state flags after registration.
func setupAuthServiceWithRepo() (*service.AuthService, *mockUserRepo) {
	userRepo := newMockUserRepo()
	refreshTokenRepo := newMockRefreshTokenRepo()
	passwordResetRepo := newMockPasswordResetRepo()
	emailVerifyRepo := newMockEmailVerificationRepo()
	jwtManager := jwt.NewManager("test-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	return service.NewAuthService(userRepo, refreshTokenRepo, passwordResetRepo, emailVerifyRepo, jwtManager), userRepo
}

const enumPassword = "correct-horse-battery"

func registerEnumUser(t *testing.T, svc *service.AuthService, email string) {
	t.Helper()
	if _, _, err := svc.Register(context.Background(), service.RegisterInput{
		Email:    email,
		Password: enumPassword,
		Name:     "Enum User",
	}); err != nil {
		t.Fatalf("register %s: %v", email, err)
	}
}

// A disabled account must not be distinguishable from any other account when
// the password is wrong: the pre-auth response must be the generic
// ErrInvalidCredentials, not ErrAccountDisabled.
func TestLogin_DisabledAccount_WrongPassword_IsGeneric(t *testing.T) {
	svc, repo := setupAuthServiceWithRepo()
	ctx := context.Background()
	registerEnumUser(t, svc, "disabled@example.com")

	u, _ := repo.GetByEmail(ctx, "disabled@example.com")
	u.Disabled = true
	_ = repo.Update(ctx, u)

	_, _, err := svc.Login(ctx, service.LoginInput{Email: "disabled@example.com", Password: "wrong-password"})
	if err != service.ErrInvalidCredentials {
		t.Errorf("disabled account + wrong password: got %v, want ErrInvalidCredentials (state leak)", err)
	}
}

// The legitimate owner (correct password) still learns the account is disabled.
func TestLogin_DisabledAccount_CorrectPassword_RevealsDisabled(t *testing.T) {
	svc, repo := setupAuthServiceWithRepo()
	ctx := context.Background()
	registerEnumUser(t, svc, "disabled2@example.com")

	u, _ := repo.GetByEmail(ctx, "disabled2@example.com")
	u.Disabled = true
	_ = repo.Update(ctx, u)

	_, _, err := svc.Login(ctx, service.LoginInput{Email: "disabled2@example.com", Password: enumPassword})
	if err != service.ErrAccountDisabled {
		t.Errorf("disabled account + correct password: got %v, want ErrAccountDisabled", err)
	}
}

// An unverified account must not be distinguishable when the password is wrong.
func TestLogin_UnverifiedAccount_WrongPassword_IsGeneric(t *testing.T) {
	svc, repo := setupAuthServiceWithRepo()
	ctx := context.Background()
	registerEnumUser(t, svc, "unverified@example.com")

	u, _ := repo.GetByEmail(ctx, "unverified@example.com")
	u.EmailVerified = false
	_ = repo.Update(ctx, u)

	_, _, err := svc.Login(ctx, service.LoginInput{Email: "unverified@example.com", Password: "wrong-password"})
	if err != service.ErrInvalidCredentials {
		t.Errorf("unverified account + wrong password: got %v, want ErrInvalidCredentials (state leak)", err)
	}
}

// The legitimate owner (correct password) still learns the email is unverified.
func TestLogin_UnverifiedAccount_CorrectPassword_RevealsUnverified(t *testing.T) {
	svc, repo := setupAuthServiceWithRepo()
	ctx := context.Background()
	registerEnumUser(t, svc, "unverified2@example.com")

	u, _ := repo.GetByEmail(ctx, "unverified2@example.com")
	u.EmailVerified = false
	_ = repo.Update(ctx, u)

	_, _, err := svc.Login(ctx, service.LoginInput{Email: "unverified2@example.com", Password: enumPassword})
	if err != service.ErrEmailNotVerified {
		t.Errorf("unverified account + correct password: got %v, want ErrEmailNotVerified", err)
	}
}

// The core enumeration guarantee: a wrong password against an existing account
// and a login for a non-existent account return the exact same error.
func TestLogin_UnknownVsWrongPassword_SameError(t *testing.T) {
	svc, _ := setupAuthServiceWithRepo()
	ctx := context.Background()
	registerEnumUser(t, svc, "known@example.com")

	_, _, errUnknown := svc.Login(ctx, service.LoginInput{Email: "nobody@example.com", Password: "whatever-123"})
	_, _, errWrong := svc.Login(ctx, service.LoginInput{Email: "known@example.com", Password: "wrong-password"})

	if errUnknown != service.ErrInvalidCredentials {
		t.Errorf("unknown user: got %v, want ErrInvalidCredentials", errUnknown)
	}
	if errWrong != service.ErrInvalidCredentials {
		t.Errorf("wrong password: got %v, want ErrInvalidCredentials", errWrong)
	}
	if errUnknown != errWrong {
		t.Errorf("responses differ (enumeration): unknown=%v wrong=%v", errUnknown, errWrong)
	}
}
