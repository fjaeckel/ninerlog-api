package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestRecordActivity_AuthenticatedRequestOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var recorded []uuid.UUID
	r := gin.New()
	api := r.Group("/api/v1")
	api.Use(AuthMiddleware(newTestJWTManager(), []string{"/auth/login"}))
	api.Use(RecordActivity(func(_ context.Context, id uuid.UUID) { recorded = append(recorded, id) }))
	api.GET("/users/me", func(c *gin.Context) { c.Status(http.StatusOK) })
	api.POST("/auth/login", func(c *gin.Context) { c.Status(http.StatusOK) })

	token, userID, _ := accessToken(t)
	if got := callAuthed(r, token); got != http.StatusOK {
		t.Fatalf("authed request: %d, want 200", got)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/auth/login", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("public request: %d, want 200", w.Code)
	}

	if got := callAuthed(r, "bogus"); got != http.StatusUnauthorized {
		t.Fatalf("bad token: %d, want 401", got)
	}

	if len(recorded) != 1 || recorded[0] != userID {
		t.Errorf("recorded %v, want exactly [%s]", recorded, userID)
	}
}
