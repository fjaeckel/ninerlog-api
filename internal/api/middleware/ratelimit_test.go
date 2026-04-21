package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestNewRateLimitMiddleware_AllowsWithinLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(NewRateLimitMiddleware(5, time.Minute))
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// First request should succeed
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("First request returned %d, want 200", w.Code)
	}
}

func TestNewRateLimitMiddleware_BlocksOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(NewRateLimitMiddleware(3, time.Minute))
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Make requests up to and beyond the limit
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("Request %d returned %d, want 200", i+1, w.Code)
		}
	}

	// 4th request should be rate-limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Over-limit request returned %d, want 429", w.Code)
	}
}

func TestNewRateLimitMiddleware_DifferentIPsGetSeparateLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(NewRateLimitMiddleware(2, time.Minute))
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Exhaust limit for IP1
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}

	// IP2 should still be allowed
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Different IP request returned %d, want 200", w.Code)
	}
}

func TestRateLimitByPath_AppliesOnlyToMatchingPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	rl := NewRateLimitMiddleware(1, time.Minute)
	router.Use(RateLimitByPath(rl, "/auth/login", "/auth/register"))
	router.POST("/auth/login", func(c *gin.Context) {
		c.String(http.StatusOK, "login")
	})
	router.GET("/flights", func(c *gin.Context) {
		c.String(http.StatusOK, "flights")
	})

	// First login request should succeed
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("First login returned %d, want 200", w.Code)
	}

	// Second login request should be rate-limited
	req = httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Over-limit login returned %d, want 429", w.Code)
	}

	// /flights request from same IP should NOT be rate-limited (different path)
	req = httptest.NewRequest("GET", "/flights", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Non-rate-limited path returned %d, want 200", w.Code)
	}
}

func TestRateLimitByPath_NoMatchPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	rl := NewRateLimitMiddleware(1, time.Minute)
	router.Use(RateLimitByPath(rl, "/auth/login"))
	router.GET("/other", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// Multiple requests to non-matching path should all succeed
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/other", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Request %d to non-matching path returned %d, want 200", i+1, w.Code)
		}
	}
}
