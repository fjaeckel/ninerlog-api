package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/errcode"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Every 401 the middleware sends refuses the session, never a credential in
// the request, so each one carries session_invalid. The sentences are pinned
// too, because older clients still match them.

type envelope struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func assertSessionInvalid(t *testing.T, w *httptest.ResponseRecorder, wantMessage string) {
	t.Helper()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	var e envelope
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if e.Code != errcode.SessionInvalid {
		t.Errorf("code = %q, want %q", e.Code, errcode.SessionInvalid)
	}
	if e.Error != wantMessage {
		t.Errorf("error = %q, want %q", e.Error, wantMessage)
	}
}

func serveAuthed(r *gin.Engine, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/api/v1/users/me", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestUnauthorizedCodes_Middleware(t *testing.T) {
	live := func(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) { return false, true, nil }

	t.Run("missing token", func(t *testing.T) {
		w := serveAuthed(stateRouter(t, live), "")
		assertSessionInvalid(t, w, "Authentication required")
	})
	t.Run("invalid token", func(t *testing.T) {
		w := serveAuthed(stateRouter(t, live), "Bearer not-a-token")
		assertSessionInvalid(t, w, "Invalid or expired token")
	})
	t.Run("revoked session", func(t *testing.T) {
		token, _, _ := accessToken(t)
		r := stateRouter(t, func(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) { return false, false, nil })
		assertSessionInvalid(t, serveAuthed(r, "Bearer "+token), "Session ended, please sign in again")
	})
	t.Run("disabled account", func(t *testing.T) {
		token, _, _ := accessToken(t)
		r := stateRouter(t, func(context.Context, uuid.UUID, uuid.UUID) (bool, bool, error) { return true, true, nil })
		assertSessionInvalid(t, serveAuthed(r, "Bearer "+token), "Session ended, please sign in again")
	})
	t.Run("admin without user", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/admin/audit-log", nil)
		AdminMiddleware(nil, "admin@example.com", func(uuid.UUID) (string, error) { return "", nil })(c)
		assertSessionInvalid(t, w, "Unauthorized")
	})
}
