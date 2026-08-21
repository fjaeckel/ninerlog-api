package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/google/uuid"
)

// sessionFixture holds an auth service wired to a mock refresh-token store,
// plus a registered, verified account to sign in with.
type sessionFixture struct {
	auth     *service.AuthService
	refresh  *mockRefreshTokenRepo
	email    string
	password string
	userID   uuid.UUID
}

func newSessionFixture(t *testing.T, policy service.SessionPolicy) *sessionFixture {
	t.Helper()

	userRepo := newMockUserRepo()
	refreshRepo := newMockRefreshTokenRepo()
	jwtManager := jwt.NewManager("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	auth := service.NewAuthService(
		userRepo, refreshRepo, newMockPasswordResetRepo(), newMockEmailVerificationRepo(),
		jwtManager, service.NewTwoFactorService(userRepo, jwtManager, nil), policy,
	)

	const email, password = "pilot@example.com", "Password1234!"
	user, _, err := auth.Register(context.Background(), service.RegisterInput{
		Email: email, Password: password, Name: "Test Pilot",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	return &sessionFixture{auth: auth, refresh: refreshRepo, email: email, password: password, userID: user.ID}
}

// login signs in from a named device and returns the issued pair.
func (f *sessionFixture) login(t *testing.T, userAgent string) *service.TokenPair {
	t.Helper()

	ctx := service.ContextWithDevice(context.Background(), service.DeviceInfo{
		UserAgent: userAgent, IPAddress: "203.0.113.7",
	})
	_, tokens, err := f.auth.Login(ctx, service.LoginInput{Email: f.email, Password: f.password})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	return tokens
}

func TestLogin_ConcurrentDevicesKeepTheirSessions(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	ctx := context.Background()

	phone := f.login(t, "Mozilla/5.0 (iPhone) Safari/605.1")
	laptop := f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")

	if phone.SessionID == laptop.SessionID {
		t.Fatal("Login() reused a session for a second device")
	}

	// The first device must survive the second device signing in.
	if _, err := f.auth.RefreshToken(ctx, phone.RefreshToken); err != nil {
		t.Errorf("RefreshToken() for the first device error = %v, want nil", err)
	}
	if _, err := f.auth.RefreshToken(ctx, laptop.RefreshToken); err != nil {
		t.Errorf("RefreshToken() for the second device error = %v, want nil", err)
	}
}

func TestLogin_EvictsLeastRecentlyUsedBeyondTheCap(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{MaxPerUser: 3})
	ctx := context.Background()

	first := f.login(t, "Mozilla/5.0 (iPhone) Safari/605.1")
	for i := 0; i < 3; i++ {
		// Each login is stamped in turn, leaving `first` the oldest.
		time.Sleep(time.Millisecond)
		f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")
	}

	sessions, err := f.auth.ListSessions(ctx, f.userID, uuid.Nil)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("ListSessions() returned %d sessions, want 3", len(sessions))
	}

	if _, err := f.auth.RefreshToken(ctx, first.RefreshToken); err == nil {
		t.Error("RefreshToken() for the evicted session succeeded, want an error")
	}
}

func TestRefreshToken_ConcurrentRefreshWithinGraceKeepsBothClients(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Minute})
	ctx := context.Background()

	tokens := f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")

	winner, err := f.auth.RefreshToken(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}

	// The second tab still holds the token the first tab just rotated.
	loser, err := f.auth.RefreshToken(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() inside the grace window error = %v, want nil", err)
	}

	if winner.SessionID != loser.SessionID {
		t.Error("RefreshToken() moved a concurrent refresh to a different session")
	}
	// Neither client may be evicted by the other.
	if _, err := f.auth.RefreshToken(ctx, winner.RefreshToken); err != nil {
		t.Errorf("RefreshToken() for the first client error = %v, want nil", err)
	}
	if _, err := f.auth.RefreshToken(ctx, loser.RefreshToken); err != nil {
		t.Errorf("RefreshToken() for the second client error = %v, want nil", err)
	}
}

func TestRefreshToken_ReplayAfterGraceRevokesTheSession(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Nanosecond})
	ctx := context.Background()

	tokens := f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")

	rotated, err := f.auth.RefreshToken(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}

	time.Sleep(time.Millisecond)
	if _, err := f.auth.RefreshToken(ctx, tokens.RefreshToken); err != service.ErrTokenReuseDetected {
		t.Errorf("RefreshToken() replay error = %v, want ErrTokenReuseDetected", err)
	}

	// The replay must take the live token down with it.
	if _, err := f.auth.RefreshToken(ctx, rotated.RefreshToken); err == nil {
		t.Error("RefreshToken() after a detected replay succeeded, want an error")
	}
}

func TestRefreshToken_LogoutGetsNoGrace(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour})
	ctx := context.Background()

	tokens := f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")
	if err := f.auth.Logout(ctx, tokens.RefreshToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if _, err := f.auth.RefreshToken(ctx, tokens.RefreshToken); err != service.ErrTokenRevoked {
		t.Errorf("RefreshToken() after logout error = %v, want ErrTokenRevoked", err)
	}
}

func TestRefreshToken_KeepsSessionIdentityAcrossRotation(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	ctx := context.Background()

	tokens := f.login(t, "Mozilla/5.0 (iPhone) Safari/605.1")
	rotated, err := f.auth.RefreshToken(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}

	if rotated.SessionID != tokens.SessionID {
		t.Errorf("RefreshToken() session = %v, want %v", rotated.SessionID, tokens.SessionID)
	}

	sessions, err := f.auth.ListSessions(ctx, f.userID, uuid.Nil)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("ListSessions() returned %d sessions, want 1", len(sessions))
	}
	if sessions[0].DeviceLabel != "Safari on iPhone" {
		t.Errorf("session device label = %q, want %q", sessions[0].DeviceLabel, "Safari on iPhone")
	}
}

func TestListSessions_FlagsTheCallersSession(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	ctx := context.Background()

	phone := f.login(t, "Mozilla/5.0 (iPhone) Safari/605.1")
	f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")

	sessions, err := f.auth.ListSessions(ctx, f.userID, phone.SessionID)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}

	var current int
	for _, s := range sessions {
		if s.Current {
			current++
			if s.ID != phone.SessionID {
				t.Errorf("flagged session %v, want %v", s.ID, phone.SessionID)
			}
		}
	}
	if current != 1 {
		t.Errorf("ListSessions() flagged %d sessions as current, want 1", current)
	}
}

func TestRevokeSession_EndsOnlyThatSession(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	ctx := context.Background()

	phone := f.login(t, "Mozilla/5.0 (iPhone) Safari/605.1")
	laptop := f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")

	if err := f.auth.RevokeSession(ctx, f.userID, phone.SessionID); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}

	if _, err := f.auth.RefreshToken(ctx, phone.RefreshToken); err == nil {
		t.Error("RefreshToken() for the revoked session succeeded, want an error")
	}
	if _, err := f.auth.RefreshToken(ctx, laptop.RefreshToken); err != nil {
		t.Errorf("RefreshToken() for the surviving session error = %v, want nil", err)
	}
}

func TestRevokeSession_RejectsAnotherUsersSession(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	ctx := context.Background()

	victim := f.login(t, "Mozilla/5.0 (iPhone) Safari/605.1")

	err := f.auth.RevokeSession(ctx, uuid.New(), victim.SessionID)
	if err != service.ErrSessionNotFound {
		t.Errorf("RevokeSession() for a foreign user error = %v, want ErrSessionNotFound", err)
	}

	if _, err := f.auth.RefreshToken(ctx, victim.RefreshToken); err != nil {
		t.Errorf("RefreshToken() after a rejected revocation error = %v, want nil", err)
	}
}

func TestRevokeOtherSessions_KeepsTheCaller(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	ctx := context.Background()

	phone := f.login(t, "Mozilla/5.0 (iPhone) Safari/605.1")
	laptop := f.login(t, "Mozilla/5.0 (Macintosh) Chrome/120.0")
	tablet := f.login(t, "Mozilla/5.0 (iPad) Safari/605.1")

	revoked, err := f.auth.RevokeOtherSessions(ctx, f.userID, laptop.SessionID)
	if err != nil {
		t.Fatalf("RevokeOtherSessions() error = %v", err)
	}
	if revoked != 2 {
		t.Errorf("RevokeOtherSessions() revoked %d sessions, want 2", revoked)
	}

	if _, err := f.auth.RefreshToken(ctx, laptop.RefreshToken); err != nil {
		t.Errorf("RefreshToken() for the kept session error = %v, want nil", err)
	}
	for name, token := range map[string]string{"phone": phone.RefreshToken, "tablet": tablet.RefreshToken} {
		if _, err := f.auth.RefreshToken(ctx, token); err == nil {
			t.Errorf("RefreshToken() for the revoked %s session succeeded, want an error", name)
		}
	}
}

func TestLogin_TwoFactorAccountStartsNoSession(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{})
	ctx := context.Background()

	user, err := f.auth.GetUserByID(ctx, f.userID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	user.TwoFactorEnabled = true

	_, tokens, err := f.auth.Login(ctx, service.LoginInput{Email: f.email, Password: f.password})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if tokens != nil {
		t.Error("Login() issued tokens before the second factor")
	}

	sessions, err := f.auth.ListSessions(ctx, f.userID, uuid.Nil)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("ListSessions() returned %d sessions, want 0", len(sessions))
	}
}

func TestDeviceLabel(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		want      string
	}{
		{"empty", "", "Unknown device"},
		{"safari on iphone", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0) AppleWebKit/605.1.15 Safari/604.1", "Safari on iPhone"},
		{"chrome on macos", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120.0 Safari/537.36", "Chrome on macOS"},
		{"edge on windows", "Mozilla/5.0 (Windows NT 10.0; Win64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36 Edg/120.0", "Edge on Windows"},
		{"firefox on android", "Mozilla/5.0 (Android 14; Mobile; rv:121.0) Gecko/121.0 Firefox/121.0", "Firefox on Android"},
		{"chrome on ios", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0) CriOS/120.0 Mobile Safari/604.1", "Chrome on iPhone"},
		{"native app", "NinerLog/1.4.0 (iOS 17.0)", "NinerLog app on iOS"},
		{"platform only", "Mozilla/5.0 (Windows NT 10.0)", "Windows"},
		{"unrecognised", "some-tool/1.0", "Unknown device"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.DeviceLabel(tt.userAgent); got != tt.want {
				t.Errorf("DeviceLabel(%q) = %q, want %q", tt.userAgent, got, tt.want)
			}
		})
	}
}
