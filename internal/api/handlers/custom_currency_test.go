package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/gin-gonic/gin"
)

// TestRegisterCustomCurrencyRoutes asserts the route tree (mixing param
// segment ":id" with static siblings "preview"/"shared") registers without a
// panic, and that an unauthenticated request is rejected.
func TestRegisterCustomCurrencyRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("route registration panicked: %v", r)
		}
	}()

	h := NewCustomCurrencyHandler(&currency.CustomService{})
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterCustomCurrencyRoutes(api, h)

	// No userID in context -> handler must respond 401 before touching the service.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/custom-currency", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", w.Code)
	}
}
