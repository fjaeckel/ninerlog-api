package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjaeckel/pilotlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func newTestJWTManager() *jwt.Manager {
	return jwt.NewManager("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
}

func TestAuthMiddleware_PublicPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtMgr := newTestJWTManager()

	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(AuthMiddleware(jwtMgr, []string{"/auth/login"}))
	api.POST("/auth/login", func(c *gin.Context) {
		c.String(http.StatusOK, "login")
	})

	req := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Public path returned %d, want 200", w.Code)
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtMgr := newTestJWTManager()

	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(AuthMiddleware(jwtMgr, []string{"/auth/login"}))
	api.GET("/flights", func(c *gin.Context) {
		c.String(http.StatusOK, "flights")
	})

	req := httptest.NewRequest("GET", "/api/v1/flights", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Missing token returned %d, want 401", w.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtMgr := newTestJWTManager()

	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(AuthMiddleware(jwtMgr, []string{"/auth/login"}))
	api.GET("/flights", func(c *gin.Context) {
		c.String(http.StatusOK, "flights")
	})

	req := httptest.NewRequest("GET", "/api/v1/flights", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-here")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Invalid token returned %d, want 401", w.Code)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtMgr := newTestJWTManager()
	userID := uuid.New()

	token, err := jwtMgr.GenerateAccessToken(userID)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(AuthMiddleware(jwtMgr, []string{"/auth/login"}))
	api.GET("/flights", func(c *gin.Context) {
		// Verify the userID was set in context
		ctxUserID, exists := c.Get("userID")
		if !exists {
			t.Error("userID not set in context")
			c.String(http.StatusInternalServerError, "no userID")
			return
		}
		if ctxUserID.(uuid.UUID) != userID {
			t.Errorf("userID = %v, want %v", ctxUserID, userID)
		}
		c.String(http.StatusOK, "flights")
	})

	req := httptest.NewRequest("GET", "/api/v1/flights", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Valid token returned %d, want 200", w.Code)
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Create manager with 0 expiry so tokens are instantly expired
	jwtMgr := jwt.NewManager("test-access-secret", "test-refresh-secret", -1*time.Second, 7*24*time.Hour)
	userID := uuid.New()

	token, err := jwtMgr.GenerateAccessToken(userID)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Use a different manager with normal validation
	validatingMgr := jwt.NewManager("test-access-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)

	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(AuthMiddleware(validatingMgr, []string{}))
	api.GET("/flights", func(c *gin.Context) {
		c.String(http.StatusOK, "flights")
	})

	req := httptest.NewRequest("GET", "/api/v1/flights", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expired token returned %d, want 401", w.Code)
	}
}
