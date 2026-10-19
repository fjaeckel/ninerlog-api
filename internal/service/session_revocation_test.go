package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

const testUserAgent = "Mozilla/5.0 (Macintosh) Chrome/120.0"

// rotatedSession is a signed-in session whose first refresh token has just
// been superseded by a refresh.
type rotatedSession struct {
	superseded *service.TokenPair
	current    *service.TokenPair
}

func (f *sessionFixture) rotatedSession(t *testing.T) rotatedSession {
	t.Helper()

	first := f.login(t, testUserAgent)
	current, err := f.auth.RefreshToken(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	return rotatedSession{superseded: first, current: current}
}

// assertSessionEnded checks that neither token of s renews the session and
// that the session no longer authorises access tokens.
func (f *sessionFixture) assertSessionEnded(t *testing.T, s rotatedSession, want error) {
	t.Helper()
	ctx := context.Background()

	pair, err := f.auth.RefreshToken(ctx, s.superseded.RefreshToken)
	if !errors.Is(err, want) {
		t.Errorf("RefreshToken() with the superseded token error = %v, want %v", err, want)
	}
	if pair != nil {
		t.Error("RefreshToken() with the superseded token issued a token pair")
	}

	if _, err := f.auth.RefreshToken(ctx, s.current.RefreshToken); err == nil {
		t.Error("RefreshToken() with the current token succeeded, want an error")
	}

	_, live, err := f.auth.AccessTokenState(ctx, f.userID, s.current.SessionID)
	if err != nil {
		t.Fatalf("AccessTokenState() error = %v", err)
	}
	if live {
		t.Error("AccessTokenState() reports the ended session live")
	}
}

func TestRefresh_RotatedTokenAfterLogout_Rejected(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour})
	s := f.rotatedSession(t)

	if err := f.auth.Logout(context.Background(), s.current.RefreshToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	f.assertSessionEnded(t, s, service.ErrTokenRevoked)
}

func TestRefresh_RotatedTokenAfterLogoutWithThatToken_Rejected(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour})
	s := f.rotatedSession(t)

	if err := f.auth.Logout(context.Background(), s.superseded.RefreshToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	f.assertSessionEnded(t, s, service.ErrTokenRevoked)
}

func TestRefresh_RotatedTokenAfterSessionRevoked_Rejected(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour})
	s := f.rotatedSession(t)

	if err := f.auth.RevokeSession(context.Background(), f.userID, s.current.SessionID); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}

	f.assertSessionEnded(t, s, service.ErrTokenRevoked)
}

func TestRefresh_RotatedTokenAfterOtherSessionsRevoked_Rejected(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour})
	s := f.rotatedSession(t)
	keeper := f.login(t, "Mozilla/5.0 (iPhone) Safari/605.1")

	if _, err := f.auth.RevokeOtherSessions(context.Background(), f.userID, keeper.SessionID); err != nil {
		t.Fatalf("RevokeOtherSessions() error = %v", err)
	}

	f.assertSessionEnded(t, s, service.ErrTokenRevoked)
	if _, err := f.auth.RefreshToken(context.Background(), keeper.RefreshToken); err != nil {
		t.Errorf("RefreshToken() for the kept session error = %v, want nil", err)
	}
}

func TestRefresh_RotatedTokenAfterPasswordChange_Rejected(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour})
	s := f.rotatedSession(t)

	if err := f.auth.ChangePassword(context.Background(), f.userID, f.password, "NewPassword456!"); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}

	f.assertSessionEnded(t, s, service.ErrTokenRevoked)
}

func TestRefresh_RotatedTokenAfterPasswordReset_Rejected(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour})
	ctx := context.Background()
	s := f.rotatedSession(t)

	reset, err := f.auth.RequestPasswordReset(ctx, f.email)
	if err != nil {
		t.Fatalf("RequestPasswordReset() error = %v", err)
	}
	if _, err := f.auth.ResetPassword(ctx, reset.Token, "NewPassword456!", ""); err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}

	f.assertSessionEnded(t, s, service.ErrTokenRevoked)
}

func TestRefresh_RotatedTokenAfterEviction_Rejected(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour, MaxPerUser: 1})
	s := f.rotatedSession(t)

	time.Sleep(time.Millisecond)
	f.login(t, "Mozilla/5.0 (iPhone) Safari/605.1")

	f.assertSessionEnded(t, s, service.ErrTokenRevoked)
}

func TestRefresh_RotatedTokenAfterAllSessionsDeleted_Rejected(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour})
	s := f.rotatedSession(t)

	if err := f.auth.RevokeAllSessions(context.Background(), f.userID); err != nil {
		t.Fatalf("RevokeAllSessions() error = %v", err)
	}

	f.assertSessionEnded(t, s, service.ErrInvalidToken)
}

func TestRefresh_RotatedTokenAfterAccountDeletion_Rejected(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour})
	s := f.rotatedSession(t)

	if err := f.auth.DeleteUser(context.Background(), f.userID, f.password); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}

	f.assertSessionEnded(t, s, service.ErrInvalidToken)
}

func TestRefresh_RotatedTokenAfterReplayRevokedTheSession_Rejected(t *testing.T) {
	const grace = 200 * time.Millisecond
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: grace})
	ctx := context.Background()

	first := f.login(t, testUserAgent)
	second, err := f.auth.RefreshToken(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	time.Sleep(grace + 100*time.Millisecond)
	third, err := f.auth.RefreshToken(ctx, second.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}

	if _, err := f.auth.RefreshToken(ctx, first.RefreshToken); !errors.Is(err, service.ErrTokenReuseDetected) {
		t.Fatalf("RefreshToken() replay error = %v, want ErrTokenReuseDetected", err)
	}

	f.assertSessionEnded(t, rotatedSession{superseded: second, current: third}, service.ErrTokenRevoked)
}

func TestRefresh_LogoutEndsPairsIssuedWithinGrace(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour})
	ctx := context.Background()
	s := f.rotatedSession(t)

	sibling, err := f.auth.RefreshToken(ctx, s.superseded.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() inside the grace window error = %v", err)
	}

	if err := f.auth.Logout(ctx, s.current.RefreshToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if _, err := f.auth.RefreshToken(ctx, sibling.RefreshToken); !errors.Is(err, service.ErrTokenRevoked) {
		t.Errorf("RefreshToken() with the sibling token error = %v, want ErrTokenRevoked", err)
	}
	f.assertSessionEnded(t, s, service.ErrTokenRevoked)
}

func TestRefresh_RotatedTokenWithinGrace_UnaffectedByAnotherSessionEnding(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour})
	ctx := context.Background()
	s := f.rotatedSession(t)
	other := f.login(t, "Mozilla/5.0 (iPhone) Safari/605.1")

	if err := f.auth.Logout(ctx, other.RefreshToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	pair, err := f.auth.RefreshToken(ctx, s.superseded.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken() inside the grace window error = %v, want nil", err)
	}
	if pair.SessionID != s.current.SessionID {
		t.Error("RefreshToken() inside the grace window moved to a different session")
	}
	if _, err := f.auth.RefreshToken(ctx, s.current.RefreshToken); err != nil {
		t.Errorf("RefreshToken() with the current token error = %v, want nil", err)
	}
}

func TestRefresh_RotatedTokenWithinGrace_ServedRepeatedly(t *testing.T) {
	f := newSessionFixture(t, service.SessionPolicy{ReuseGrace: time.Hour})
	ctx := context.Background()
	s := f.rotatedSession(t)

	seen := map[uuid.UUID]bool{}
	for i := 0; i < 3; i++ {
		pair, err := f.auth.RefreshToken(ctx, s.superseded.RefreshToken)
		if err != nil {
			t.Fatalf("RefreshToken() #%d inside the grace window error = %v", i+1, err)
		}
		seen[pair.SessionID] = true
	}
	if len(seen) != 1 || !seen[s.current.SessionID] {
		t.Errorf("RefreshToken() inside the grace window used sessions %v, want only %v", seen, s.current.SessionID)
	}
}
