package service_test

import (
	"context"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// usable reports what AuthMiddleware would conclude about an access token.
func usable(t *testing.T, f *sessionFixture, sessionID uuid.UUID) bool {
	t.Helper()
	disabled, live, err := f.auth.AccessTokenState(context.Background(), f.userID, sessionID)
	if err != nil {
		t.Fatalf("AccessTokenState() error = %v", err)
	}
	return live && !disabled
}

func TestAccessTokenState_LiveSession(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	if !usable(t, f, f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0").SessionID) {
		t.Error("a freshly issued access token is not usable")
	}
}

func TestAccessTokenState_LogoutEndsTheSession(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	tokens := f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")

	if err := f.auth.Logout(context.Background(), tokens.RefreshToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if usable(t, f, tokens.SessionID) {
		t.Error("the access token survived a logout")
	}
}

func TestAccessTokenState_RevokeSessionIsScopedToOneDevice(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	phone := f.login(t, "Mozilla/5.0 (iPhone) Safari/605.1")
	laptop := f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")

	if err := f.auth.RevokeSession(context.Background(), f.userID, phone.SessionID); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}

	if usable(t, f, phone.SessionID) {
		t.Error("the revoked device's access token still works")
	}
	if !usable(t, f, laptop.SessionID) {
		t.Error("revoking one session took another device's access token with it")
	}
}

func TestAccessTokenState_PasswordChangeEndsEverySession(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	phone := f.login(t, "Mozilla/5.0 (iPhone) Safari/605.1")
	laptop := f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")

	if err := f.auth.ChangePassword(context.Background(), f.userID, f.password, "NewPassword1234!"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}

	for name, sessionID := range map[string]uuid.UUID{"phone": phone.SessionID, "laptop": laptop.SessionID} {
		if usable(t, f, sessionID) {
			t.Errorf("the %s access token survived a password change", name)
		}
	}
}

func TestAccessTokenState_DisabledAccount(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	tokens := f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")

	f.refresh.disabled = map[uuid.UUID]bool{f.userID: true}

	if usable(t, f, tokens.SessionID) {
		t.Error("a disabled account's access token is still usable")
	}
}

func TestAccessTokenState_SurvivesRotation(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	tokens := f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")

	rotated, err := f.auth.RefreshToken(context.Background(), tokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if rotated.SessionID != tokens.SessionID {
		t.Fatalf("rotation changed the session: %s -> %s", tokens.SessionID, rotated.SessionID)
	}

	if !usable(t, f, tokens.SessionID) {
		t.Error("the session went dead across a rotation")
	}
}

func TestAccessTokenState_UnknownUserIsNotAnError(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})

	disabled, live, err := f.auth.AccessTokenState(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("AccessTokenState() error = %v, want nil", err)
	}
	if live || disabled {
		t.Errorf("unknown user reported disabled=%v live=%v, want false/false", disabled, live)
	}
}
