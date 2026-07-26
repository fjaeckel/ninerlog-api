package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Access tokens are stateless 15-minute JWTs, so before this check nothing
// could revoke one early: disabling an account or changing a password only
// deleted REFRESH tokens, which merely stops the session being extended. Both
// were confirmed against a running instance -- a disabled user's token still
// read and created flights, and an old token still worked after a password
// change.
func newStateRouter(t *testing.T, mgr *jwt.Manager, state UserSessionState) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(AuthMiddlewareWithState(mgr, []string{"/auth/login"}, state))
	api.GET("/users/me", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func doAuthed(r *gin.Engine, token string) int {
	req := httptest.NewRequest("GET", "/api/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestAuthMiddleware_RejectsDisabledUser(t *testing.T) {
	mgr := jwt.NewManager("access-secret", "refresh-secret", 15*time.Minute, time.Hour)
	uid := uuid.New()
	tok, err := mgr.GenerateAccessToken(uid)
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	enabled := newStateRouter(t, mgr, func(uuid.UUID) (bool, *time.Time, error) { return false, nil, nil })
	if got := doAuthed(enabled, tok); got != http.StatusOK {
		t.Fatalf("enabled user: %d, want 200", got)
	}

	disabled := newStateRouter(t, mgr, func(uuid.UUID) (bool, *time.Time, error) { return true, nil, nil })
	if got := doAuthed(disabled, tok); got != http.StatusForbidden {
		t.Errorf("disabled user's token still accepted: %d, want 403", got)
	}
}

func TestAuthMiddleware_RejectsTokenIssuedBeforeSessionEpoch(t *testing.T) {
	mgr := jwt.NewManager("access-secret", "refresh-secret", 15*time.Minute, time.Hour)
	tok, err := mgr.GenerateAccessToken(uuid.New())
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	// Epoch in the future: the token predates it and must be rejected.
	future := time.Now().Add(time.Hour)
	stale := newStateRouter(t, mgr, func(uuid.UUID) (bool, *time.Time, error) { return false, &future, nil })
	if got := doAuthed(stale, tok); got != http.StatusUnauthorized {
		t.Errorf("token issued before the session epoch was accepted: %d, want 401", got)
	}

	// Epoch in the past: the token was issued after it and stays valid.
	past := time.Now().Add(-time.Hour)
	fresh := newStateRouter(t, mgr, func(uuid.UUID) (bool, *time.Time, error) { return false, &past, nil })
	if got := doAuthed(fresh, tok); got != http.StatusOK {
		t.Errorf("token issued after the session epoch was rejected: %d, want 200", got)
	}
}

func TestAuthMiddleware_RejectsDeletedUser(t *testing.T) {
	mgr := jwt.NewManager("access-secret", "refresh-secret", 15*time.Minute, time.Hour)
	tok, _ := mgr.GenerateAccessToken(uuid.New())

	gone := newStateRouter(t, mgr, func(uuid.UUID) (bool, *time.Time, error) {
		return false, nil, http.ErrNoLocation // stand-in for "not found"
	})
	if got := doAuthed(gone, tok); got != http.StatusUnauthorized {
		t.Errorf("deleted user's token accepted: %d, want 401", got)
	}
}

// A nil state function preserves the previous behaviour for callers that do not
// supply one (AuthMiddleware delegates here).
func TestAuthMiddleware_NilStateSkipsChecks(t *testing.T) {
	mgr := jwt.NewManager("access-secret", "refresh-secret", 15*time.Minute, time.Hour)
	tok, _ := mgr.GenerateAccessToken(uuid.New())
	if got := doAuthed(newStateRouter(t, mgr, nil), tok); got != http.StatusOK {
		t.Errorf("nil state: %d, want 200", got)
	}
}
