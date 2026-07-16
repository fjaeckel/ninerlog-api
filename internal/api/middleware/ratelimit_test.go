package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

func TestRateLimitByPathPrefix_AppliesToOpaqueTokenSuffix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	rl := NewRateLimitMiddleware(1, time.Minute)
	router.Use(RateLimitByPathPrefix(rl, "/sign/"))
	router.GET("/sign/:token", func(c *gin.Context) {
		c.String(http.StatusOK, "sign")
	})
	router.GET("/flights", func(c *gin.Context) {
		c.String(http.StatusOK, "flights")
	})

	// Two different opaque tokens still share the prefix and thus the limit.
	req := httptest.NewRequest("GET", "/sign/token-one", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("First /sign/ request returned %d, want 200", w.Code)
	}

	req = httptest.NewRequest("GET", "/sign/completely-different-token", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Second /sign/ request (different token, same IP) returned %d, want 429", w.Code)
	}

	// /flights from the same IP is unaffected (different prefix).
	req = httptest.NewRequest("GET", "/flights", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Non-matching prefix returned %d, want 200", w.Code)
	}
}

func TestRateLimitByPathPrefix_NoMatchPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	rl := NewRateLimitMiddleware(1, time.Minute)
	router.Use(RateLimitByPathPrefix(rl, "/sign/"))
	router.GET("/other", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/other", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Request %d to non-matching prefix returned %d, want 200", i+1, w.Code)
		}
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

func TestNewUserRateLimitMiddleware_KeysByUserIDNotIP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userID := uuid.New()
	router := gin.New()
	// Simulate AuthMiddleware having already set "userID" in context.
	router.Use(func(c *gin.Context) {
		c.Set("userID", userID)
		c.Next()
	})
	router.Use(NewUserRateLimitMiddleware(1, time.Minute))
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// First request from IP1 succeeds.
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("First request returned %d, want 200", w.Code)
	}

	// Second request for the SAME user from a DIFFERENT IP is still blocked,
	// because keying is by user ID, not IP.
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.2:1"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Second request (same user, different IP) returned %d, want 429", w.Code)
	}
}

func TestNewUserRateLimitMiddleware_FallsBackToIPWhenUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	// No "userID" set — simulates a public/unauthenticated route.
	router.Use(NewUserRateLimitMiddleware(1, time.Minute))
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("First request returned %d, want 200", w.Code)
	}

	// Second request from the same IP is blocked (IP fallback).
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Second request (same IP) returned %d, want 429", w.Code)
	}

	// A different IP gets its own bucket.
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.2:1"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Request from different IP returned %d, want 200", w.Code)
	}
}

func TestRateLimitByPathWithQueryParam_OnlyLimitsWhenParamPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	rl := NewRateLimitMiddleware(1, time.Minute)
	router.Use(RateLimitByPathWithQueryParam(rl, "/flights", "q"))
	router.GET("/flights", func(c *gin.Context) {
		c.String(http.StatusOK, "flights")
	})

	// Plain listing requests (no "q") are never limited by this middleware.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/flights", nil)
		req.RemoteAddr = "10.0.0.1:1"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Plain request %d returned %d, want 200", i+1, w.Code)
		}
	}

	// First search request succeeds.
	req := httptest.NewRequest("GET", "/flights?q=EDDF", nil)
	req.RemoteAddr = "10.0.0.1:1"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("First search request returned %d, want 200", w.Code)
	}

	// Second search request from the same IP is rate-limited.
	req = httptest.NewRequest("GET", "/flights?q=EDDM", nil)
	req.RemoteAddr = "10.0.0.1:1"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Second search request returned %d, want 429", w.Code)
	}
}
