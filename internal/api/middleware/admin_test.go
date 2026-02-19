package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAdminMiddleware_AllowsAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminID := uuid.New()

	getUserEmail := func(id uuid.UUID) (string, error) {
		if id == adminID {
			return "admin@example.com", nil
		}
		return "user@example.com", nil
	}

	mw := AdminMiddleware(nil, "admin@example.com", getUserEmail)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/admin/audit-log", nil)
	c.Set("userID", adminID)

	mw(c)

	if w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
		t.Errorf("Expected admin to be allowed, got status %d", w.Code)
	}

	isAdmin, exists := c.Get("isAdmin")
	if !exists || isAdmin != true {
		t.Error("Expected isAdmin=true in context")
	}
}

func TestAdminMiddleware_BlocksNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	regularID := uuid.New()

	getUserEmail := func(id uuid.UUID) (string, error) {
		return "user@example.com", nil
	}

	mw := AdminMiddleware(nil, "admin@example.com", getUserEmail)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/admin/audit-log", nil)
	c.Set("userID", regularID)

	mw(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for non-admin, got %d", w.Code)
	}
}

func TestAdminMiddleware_BlocksUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	getUserEmail := func(id uuid.UUID) (string, error) {
		return "", nil
	}

	mw := AdminMiddleware(nil, "admin@example.com", getUserEmail)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/admin/audit-log", nil)
	// No userID set

	mw(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized when no userID, got %d", w.Code)
	}
}

func TestAdminMiddleware_BlocksWhenNoAdminConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()

	getUserEmail := func(id uuid.UUID) (string, error) {
		return "admin@example.com", nil
	}

	// Empty admin email = not configured
	mw := AdminMiddleware(nil, "", getUserEmail)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/admin/audit-log", nil)
	c.Set("userID", userID)

	mw(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden when admin not configured, got %d", w.Code)
	}
}
