package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// These tests assert the limiter predicates fire against the group-relative
// path of routes mounted under the /api/v1 group.

// newLimitedRouter builds a router shaped like main.go: an /api/v1 group with
// the limiter applied, and a handler that always 200s.
func newLimitedRouter(t *testing.T, attach func(*gin.RouterGroup)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	attach(api)
	api.POST("/imports/upload", func(c *gin.Context) { c.Status(http.StatusOK) })
	api.GET("/sign/:token", func(c *gin.Context) { c.Status(http.StatusOK) })
	api.GET("/flights", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func countStatuses(t *testing.T, r *gin.Engine, method, path string, n int) map[int]int {
	t.Helper()
	out := map[int]int{}
	for i := 0; i < n; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		out[w.Code]++
	}
	return out
}

func TestRateLimitByPathPrefix_FiresOnGroupedRoute(t *testing.T) {
	limit := int64(3)
	r := newLimitedRouter(t, func(api *gin.RouterGroup) {
		api.Use(RateLimitByPathPrefix(NewRateLimitMiddleware("test", limit, time.Minute), "/imports"))
	})

	got := countStatuses(t, r, "POST", "/api/v1/imports/upload", 8)
	if got[http.StatusTooManyRequests] == 0 {
		t.Fatalf("/imports limiter never fired: %v (limit %d over 8 requests)", got, limit)
	}
	if got[http.StatusOK] != int(limit) {
		t.Errorf("allowed %d requests, want %d", got[http.StatusOK], limit)
	}
}

func TestRateLimitByPathPrefix_FiresOnPublicSignRoute(t *testing.T) {
	r := newLimitedRouter(t, func(api *gin.RouterGroup) {
		api.Use(RateLimitByPathPrefix(NewRateLimitMiddleware("test", 2, time.Minute), "/sign/"))
	})

	got := countStatuses(t, r, "GET", "/api/v1/sign/sometoken", 6)
	if got[http.StatusTooManyRequests] == 0 {
		t.Fatalf("/sign/ limiter never fired: %v", got)
	}
}

func TestRateLimitByPathPrefix_LeavesOtherRoutesAlone(t *testing.T) {
	r := newLimitedRouter(t, func(api *gin.RouterGroup) {
		api.Use(RateLimitByPathPrefix(NewRateLimitMiddleware("test", 2, time.Minute), "/imports"))
	})

	got := countStatuses(t, r, "GET", "/api/v1/flights", 6)
	if got[http.StatusTooManyRequests] != 0 {
		t.Errorf("unrelated route was throttled: %v", got)
	}
}

// The suffix matcher fires on the group-relative path.
func TestRateLimitByPath_StillFiresOnGroupedRoute(t *testing.T) {
	r := newLimitedRouter(t, func(api *gin.RouterGroup) {
		api.Use(RateLimitByPath(NewRateLimitMiddleware("test", 2, time.Minute), "/imports/upload"))
	})

	got := countStatuses(t, r, "POST", "/api/v1/imports/upload", 6)
	if got[http.StatusTooManyRequests] == 0 {
		t.Fatalf("suffix matcher stopped firing: %v", got)
	}
}

func TestRateLimitByPathWithQueryParam_FiresOnlyWithParam(t *testing.T) {
	build := func() *gin.Engine {
		return newLimitedRouter(t, func(api *gin.RouterGroup) {
			api.Use(RateLimitByPathWithQueryParam(NewRateLimitMiddleware("test", 2, time.Minute), "/flights", "q"))
		})
	}

	withQ := countStatuses(t, build(), "GET", "/api/v1/flights?q=EDDF", 6)
	if withQ[http.StatusTooManyRequests] == 0 {
		t.Errorf("expensive search variant was not throttled: %v", withQ)
	}

	withoutQ := countStatuses(t, build(), "GET", "/api/v1/flights", 6)
	if withoutQ[http.StatusTooManyRequests] != 0 {
		t.Errorf("plain listing should stay unthrottled: %v", withoutQ)
	}
}
