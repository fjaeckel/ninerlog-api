package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// The /images sub-collection sits in the middle of a path and has per-item
// routes under it, so neither prefix nor suffix matching covers both
// "/licenses/{id}/images" and "/licenses/{id}/images/{imageId}". And reads and
// writes there need different budgets: uploading is heavy and deliberate,
// while a list page fetches images once per card and again on every revisit.
// One shared bucket made the read path 429 long before any upload was abusive.

func newImageRouter(t *testing.T, attach func(*gin.RouterGroup)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	attach(api)
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	api.GET("/licenses/:licenseId/images", ok)
	api.POST("/licenses/:licenseId/images", ok)
	api.GET("/licenses/:licenseId/images/:imageId", ok)
	api.DELETE("/licenses/:licenseId/images/:imageId", ok)
	api.GET("/licenses/:licenseId", ok)
	return r
}

func TestRateLimitByPathSegment_CoversCollectionAndItem(t *testing.T) {
	limit := int64(3)
	r := newImageRouter(t, func(api *gin.RouterGroup) {
		api.Use(RateLimitByPathSegment(NewRateLimitMiddleware("seg", limit, time.Minute), "/images"))
	})

	for _, path := range []string{
		"/api/v1/licenses/abc/images",
		"/api/v1/licenses/abc/images/def",
	} {
		got := countStatuses(t, r, "GET", path, 8)
		if got[http.StatusTooManyRequests] == 0 {
			t.Errorf("%s: limiter never fired: %v", path, got)
		}
	}
}

func TestRateLimitByPathSegment_LeavesOtherRoutesAlone(t *testing.T) {
	r := newImageRouter(t, func(api *gin.RouterGroup) {
		api.Use(RateLimitByPathSegment(NewRateLimitMiddleware("seg-off", 2, time.Minute), "/images"))
	})

	got := countStatuses(t, r, "GET", "/api/v1/licenses/abc", 8)
	if got[http.StatusTooManyRequests] != 0 {
		t.Errorf("a route without the segment was limited: %v", got)
	}
}

// The property the split exists for: hammering reads must not consume the
// write budget, and vice versa.
func TestRateLimitByPathSegmentForMethods_SeparatesReadsFromWrites(t *testing.T) {
	r := newImageRouter(t, func(api *gin.RouterGroup) {
		api.Use(RateLimitByPathSegmentForMethods(
			NewRateLimitMiddleware("img-read", 20, time.Minute), []string{http.MethodGet}, "/images"))
		api.Use(RateLimitByPathSegmentForMethods(
			NewRateLimitMiddleware("img-write", 2, time.Minute), []string{http.MethodPost, http.MethodDelete}, "/images"))
	})

	// Ten reads: well inside the read budget, and they must not touch the
	// write budget even though they match the same segment.
	if got := countStatuses(t, r, "GET", "/api/v1/licenses/abc/images", 10); got[http.StatusTooManyRequests] != 0 {
		t.Errorf("reads were limited by the write bucket: %v", got)
	}

	// Writes still hit their own tight budget.
	if got := countStatuses(t, r, "POST", "/api/v1/licenses/abc/images", 6); got[http.StatusTooManyRequests] == 0 {
		t.Errorf("write limiter never fired: %v", got)
	}

	// And a read after the writes are exhausted still succeeds.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/licenses/abc/images", nil))
	if w.Code != http.StatusOK {
		t.Errorf("read blocked after writes were exhausted: %d", w.Code)
	}
}

func TestRateLimitByPathSegmentForMethods_IgnoresOtherMethods(t *testing.T) {
	r := newImageRouter(t, func(api *gin.RouterGroup) {
		api.Use(RateLimitByPathSegmentForMethods(
			NewRateLimitMiddleware("write-only", 2, time.Minute), []string{http.MethodPost}, "/images"))
	})

	if got := countStatuses(t, r, "DELETE", "/api/v1/licenses/abc/images/def", 8); got[http.StatusTooManyRequests] != 0 {
		t.Errorf("DELETE was limited by a POST-only limiter: %v", got)
	}
}
