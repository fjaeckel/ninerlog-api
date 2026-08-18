package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Segment matching covers both "/licenses/{id}/files" and
// "/licenses/{id}/files/{imageId}" with one predicate, with separate read and
// write budgets.

func newFileRouter(t *testing.T, attach func(*gin.RouterGroup)) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	attach(api)
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	api.GET("/licenses/:licenseId/files", ok)
	api.POST("/licenses/:licenseId/files", ok)
	api.GET("/licenses/:licenseId/files/:fileId", ok)
	api.DELETE("/licenses/:licenseId/files/:fileId", ok)
	api.GET("/licenses/:licenseId", ok)
	return r
}

func TestRateLimitByPathSegment_CoversCollectionAndItem(t *testing.T) {
	limit := int64(3)
	r := newFileRouter(t, func(api *gin.RouterGroup) {
		api.Use(RateLimitByPathSegment(NewRateLimitMiddleware("seg", limit, time.Minute), "/files"))
	})

	for _, path := range []string{
		"/api/v1/licenses/abc/files",
		"/api/v1/licenses/abc/files/def",
	} {
		got := countStatuses(t, r, "GET", path, 8)
		if got[http.StatusTooManyRequests] == 0 {
			t.Errorf("%s: limiter never fired: %v", path, got)
		}
	}
}

func TestRateLimitByPathSegment_LeavesOtherRoutesAlone(t *testing.T) {
	r := newFileRouter(t, func(api *gin.RouterGroup) {
		api.Use(RateLimitByPathSegment(NewRateLimitMiddleware("seg-off", 2, time.Minute), "/files"))
	})

	got := countStatuses(t, r, "GET", "/api/v1/licenses/abc", 8)
	if got[http.StatusTooManyRequests] != 0 {
		t.Errorf("a route without the segment was limited: %v", got)
	}
}

// Hammering reads must not consume the write budget, and vice versa.
func TestRateLimitByPathSegmentForMethods_SeparatesReadsFromWrites(t *testing.T) {
	r := newFileRouter(t, func(api *gin.RouterGroup) {
		api.Use(RateLimitByPathSegmentForMethods(
			NewRateLimitMiddleware("file-read", 20, time.Minute), []string{http.MethodGet}, "/files"))
		api.Use(RateLimitByPathSegmentForMethods(
			NewRateLimitMiddleware("file-write", 2, time.Minute), []string{http.MethodPost, http.MethodDelete}, "/files"))
	})

	// Ten reads: well inside the read budget, and they must not touch the
	// write budget even though they match the same segment.
	if got := countStatuses(t, r, "GET", "/api/v1/licenses/abc/files", 10); got[http.StatusTooManyRequests] != 0 {
		t.Errorf("reads were limited by the write bucket: %v", got)
	}

	// Writes still hit their own tight budget.
	if got := countStatuses(t, r, "POST", "/api/v1/licenses/abc/files", 6); got[http.StatusTooManyRequests] == 0 {
		t.Errorf("write limiter never fired: %v", got)
	}

	// And a read after the writes are exhausted still succeeds.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/licenses/abc/files", nil))
	if w.Code != http.StatusOK {
		t.Errorf("read blocked after writes were exhausted: %d", w.Code)
	}
}

func TestRateLimitByPathSegmentForMethods_IgnoresOtherMethods(t *testing.T) {
	r := newFileRouter(t, func(api *gin.RouterGroup) {
		api.Use(RateLimitByPathSegmentForMethods(
			NewRateLimitMiddleware("write-only", 2, time.Minute), []string{http.MethodPost}, "/files"))
	})

	if got := countStatuses(t, r, "DELETE", "/api/v1/licenses/abc/files/def", 8); got[http.StatusTooManyRequests] != 0 {
		t.Errorf("DELETE was limited by a POST-only limiter: %v", got)
	}
}
