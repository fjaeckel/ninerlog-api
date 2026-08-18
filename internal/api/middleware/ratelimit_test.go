package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// hitsValue and requestsValue read a single labelled child of the rate-limit
// counters. The counters persist across tests in this package; every
// assertion below is on a delta or on a limiter name used by exactly one test.
func hitsValue(limiterName, path string) float64 {
	return testutil.ToFloat64(RateLimitHitsTotal.WithLabelValues(limiterName, path))
}

func requestsValue(limiterName, path string) float64 {
	return testutil.ToFloat64(RateLimitRequestsTotal.WithLabelValues(limiterName, path))
}

func TestNewRateLimitMiddleware_AllowsWithinLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(NewRateLimitMiddleware("test", 5, time.Minute))
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
	router.Use(NewRateLimitMiddleware("test", 3, time.Minute))
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
	router.Use(NewRateLimitMiddleware("test", 2, time.Minute))
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
	rl := NewRateLimitMiddleware("test", 1, time.Minute)
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
	rl := NewRateLimitMiddleware("test", 1, time.Minute)
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
	rl := NewRateLimitMiddleware("test", 1, time.Minute)
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
	rl := NewRateLimitMiddleware("test", 1, time.Minute)
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
	router.Use(NewUserRateLimitMiddleware("test", 1, time.Minute))
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

	// Second request for the SAME user from a DIFFERENT IP is still blocked.
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
	router.Use(NewUserRateLimitMiddleware("test", 1, time.Minute))
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
	rl := NewRateLimitMiddleware("test", 1, time.Minute)
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

// A concrete URL like /api/v1/flights/<uuid> must be recorded under the Gin
// route template, not the raw path.
func TestRateLimitMetrics_LabelsByRouteTemplateNotRawPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(NewRateLimitMiddleware("tmpl", 1, time.Minute))
	router.GET("/api/v1/flights/:id", func(c *gin.Context) {
		c.String(http.StatusOK, "flight")
	})

	// Two different flight IDs, same client: the second is rejected.
	for _, id := range []string{"11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"} {
		req := httptest.NewRequest("GET", "/api/v1/flights/"+id, nil)
		req.RemoteAddr = "10.0.0.9:1"
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	if got := hitsValue("tmpl", "/api/v1/flights/:id"); got != 1 {
		t.Errorf("hits for route template = %v, want 1", got)
	}
	// The raw path must not have produced its own series.
	if got := hitsValue("tmpl", "/api/v1/flights/22222222-2222-2222-2222-222222222222"); got != 0 {
		t.Errorf("hits recorded against raw URL path = %v, want 0 (cardinality leak)", got)
	}
	// Both requests were evaluated by the limiter, only one was rejected.
	if got := requestsValue("tmpl", "/api/v1/flights/:id"); got != 2 {
		t.Errorf("evaluated requests = %v, want 2", got)
	}
}

// The denominator metric must count allowed requests too.
func TestRateLimitMetrics_RequestsCountsAllowedAndRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(NewRateLimitMiddleware("denom", 2, time.Minute))
	router.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.10:1"
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	if got := requestsValue("denom", "/test"); got != 5 {
		t.Errorf("evaluated requests = %v, want 5", got)
	}
	if got := hitsValue("denom", "/test"); got != 3 {
		t.Errorf("rejections = %v, want 3 (5 requests, limit 2)", got)
	}
}

// The limiter name label separates a rejected search from a rejected plain
// listing on the same route.
func TestRateLimitMetrics_SearchIsDistinguishableFromPlainListing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RateLimitByPathWithQueryParam(
		NewRateLimitMiddleware("search_only", 1, time.Minute), "/flights", "q"))
	router.Use(NewRateLimitMiddleware("general_only", 3, time.Minute))
	router.GET("/flights", func(c *gin.Context) { c.String(http.StatusOK, "flights") })

	// Two searches: the second exhausts the search bucket.
	for _, q := range []string{"EDDF", "EDDM"} {
		req := httptest.NewRequest("GET", "/flights?q="+q, nil)
		req.RemoteAddr = "10.0.0.11:1"
		router.ServeHTTP(httptest.NewRecorder(), req)
	}
	// Four plain listings: the fourth exhausts the general bucket (the first
	// search already consumed a general slot).
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest("GET", "/flights", nil)
		req.RemoteAddr = "10.0.0.11:1"
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	if got := hitsValue("search_only", "/flights"); got != 1 {
		t.Errorf("search rejections = %v, want 1", got)
	}
	if got := hitsValue("general_only", "/flights"); got == 0 {
		t.Error("general rejections = 0, want >0; the two limiters are not separable")
	}
	// The search limiter must not see the plain listings at all.
	if got := requestsValue("search_only", "/flights"); got != 2 {
		t.Errorf("requests evaluated by the search limiter = %v, want 2 (searches only)", got)
	}
}
