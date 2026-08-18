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

// TestAuthMiddleware_Rejects2FAChallengeToken asserts a 2FA challenge token
// is not accepted as a Bearer access token on protected routes.
func TestAuthMiddleware_Rejects2FAChallengeToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtMgr := newTestJWTManager()

	twoFAToken, err := jwtMgr.Generate2FAToken(uuid.New())
	if err != nil {
		t.Fatalf("Failed to generate 2FA token: %v", err)
	}

	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(AuthMiddleware(jwtMgr, []string{"/auth/login"}))
	api.GET("/flights", func(c *gin.Context) {
		c.String(http.StatusOK, "flights")
	})

	req := httptest.NewRequest("GET", "/api/v1/flights", nil)
	req.Header.Set("Authorization", "Bearer "+twoFAToken)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("2FA challenge token used as access token returned %d, want 401 (2FA bypass!)", w.Code)
	}
}

// TestAuthMiddleware_PublicPath_WithPathParam asserts a publicPaths entry
// shaped as a gin route pattern ("/sign/:token") matches via c.FullPath().
func TestAuthMiddleware_PublicPath_WithPathParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtMgr := newTestJWTManager()

	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(AuthMiddleware(jwtMgr, []string{"/sign/:token"}))
	api.GET("/sign/:token", func(c *gin.Context) {
		c.String(http.StatusOK, "sign:"+c.Param("token"))
	})
	api.GET("/flights/:flightId", func(c *gin.Context) {
		c.String(http.StatusOK, "flight")
	})

	req := httptest.NewRequest("GET", "/api/v1/sign/abc123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Parameterized public path returned %d, want 200", w.Code)
	}

	// A parameterized route not in publicPaths must still require auth.
	req2 := httptest.NewRequest("GET", "/api/v1/flights/some-id", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Errorf("Non-public parameterized route returned %d, want 401", w2.Code)
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Negative expiry: tokens are already expired when minted.
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
