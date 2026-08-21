package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// stateRouter builds a router whose only route requires authentication.
func stateRouter(t *testing.T, state SessionState) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(AuthMiddlewareWithState(newTestJWTManager(), []string{"/auth/login"}, state))
	api.GET("/users/me", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func callAuthed(r *gin.Engine, token string) int {
	req := httptest.NewRequest("GET", "/api/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

// accessToken mints a token on a fresh session.
func accessToken(t *testing.T) (string, uuid.UUID, uuid.UUID) {
	t.Helper()
	userID, sessionID := uuid.New(), uuid.New()
	token, err := newTestJWTManager().GenerateAccessToken(userID, sessionID)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}
	return token, userID, sessionID
}

func TestAuthMiddleware_AcceptsLiveSession(t *testing.T) {
	token, _, _ := accessToken(t)
	r := stateRouter(t, func(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) {
		return false, true, nil
	})
	if got := callAuthed(r, token); got != http.StatusOK {
		t.Errorf("live session rejected: %d, want 200", got)
	}
}

func TestAuthMiddleware_RejectsRevokedSession(t *testing.T) {
	token, _, _ := accessToken(t)
	r := stateRouter(t, func(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) {
		return false, false, nil
	})
	if got := callAuthed(r, token); got != http.StatusUnauthorized {
		t.Errorf("revoked session accepted: %d, want 401", got)
	}
}

func TestAuthMiddleware_RejectsDisabledAccount(t *testing.T) {
	token, _, _ := accessToken(t)
	r := stateRouter(t, func(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) {
		return true, true, nil
	})
	if got := callAuthed(r, token); got != http.StatusUnauthorized {
		t.Errorf("disabled account accepted: %d, want 401", got)
	}
}

// A deleted account reports live=false.
func TestAuthMiddleware_RejectsDeletedAccount(t *testing.T) {
	token, _, _ := accessToken(t)
	r := stateRouter(t, func(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) {
		return false, false, nil
	})
	if got := callAuthed(r, token); got != http.StatusUnauthorized {
		t.Errorf("deleted account accepted: %d, want 401", got)
	}
}

func TestAuthMiddleware_LookupFailureIsNotALogout(t *testing.T) {
	token, _, _ := accessToken(t)
	r := stateRouter(t, func(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) {
		return false, false, errors.New("connection refused")
	})
	if got := callAuthed(r, token); got != http.StatusServiceUnavailable {
		t.Errorf("lookup failure: %d, want 503", got)
	}
}

// A token carrying no session ID is checked for the disabled flag only.
func TestAuthMiddleware_SessionlessTokenChecksDisabledOnly(t *testing.T) {
	mgr := newTestJWTManager()
	token, err := mgr.GenerateAccessToken(uuid.New(), uuid.Nil)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	live := stateRouter(t, func(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) {
		return false, false, nil
	})
	if got := callAuthed(live, token); got != http.StatusOK {
		t.Errorf("sessionless token rejected: %d, want 200", got)
	}

	disabled := stateRouter(t, func(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) {
		return true, false, nil
	})
	if got := callAuthed(disabled, token); got != http.StatusUnauthorized {
		t.Errorf("sessionless token of a disabled account accepted: %d, want 401", got)
	}
}

// The state callback receives the identity from the token, not from the request.
func TestAuthMiddleware_PassesTokenIdentityToState(t *testing.T) {
	mgr := newTestJWTManager()
	wantUser, wantSession := uuid.New(), uuid.New()
	token, err := mgr.GenerateAccessToken(wantUser, wantSession)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}

	var gotUser, gotSession uuid.UUID
	r := stateRouter(t, func(_ context.Context, u, s uuid.UUID) (bool, bool, error) {
		gotUser, gotSession = u, s
		return false, true, nil
	})
	callAuthed(r, token)

	if gotUser != wantUser || gotSession != wantSession {
		t.Errorf("state called with (%s, %s), want (%s, %s)", gotUser, gotSession, wantUser, wantSession)
	}
}

func TestAuthMiddleware_NilStateSkipsTheCheck(t *testing.T) {
	token, _, _ := accessToken(t)
	if got := callAuthed(stateRouter(t, nil), token); got != http.StatusOK {
		t.Errorf("nil state: %d, want 200", got)
	}
}
