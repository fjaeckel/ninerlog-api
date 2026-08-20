package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	limiter "github.com/ulule/limiter/v3"
	mgin "github.com/ulule/limiter/v3/drivers/middleware/gin"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

var (
	// RateLimitHitsTotal counts requests rejected by a rate limiter. The
	// "limiter" label names the bucket that did the rejecting (see the names
	// passed in cmd/api/main.go).
	RateLimitHitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_hits_total",
			Help: "Total number of requests rejected by a rate limiter, by limiter and route.",
		},
		[]string{"limiter", "path"},
	)

	// RateLimitRequestsTotal counts every request evaluated by a rate limiter,
	// allowed or rejected.
	RateLimitRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_requests_total",
			Help: "Total number of requests evaluated by a rate limiter (allowed and rejected), by limiter and route.",
		},
		[]string{"limiter", "path"},
	)
)

func init() {
	prometheus.MustRegister(RateLimitHitsTotal, RateLimitRequestsTotal)
}

// newRateLimitMiddleware builds a Gin rate-limit middleware keyed by keyGetter.
// rate is the number of requests allowed per period (e.g., 10 requests per 1 minute).
// name identifies this limiter in the rate-limit metrics and must be a small
// constant (it is a Prometheus label value).
func newRateLimitMiddleware(name string, rate int64, period time.Duration, keyGetter mgin.KeyGetter) gin.HandlerFunc {
	r := limiter.Rate{
		Period: period,
		Limit:  rate,
	}

	store := memory.NewStore()
	instance := limiter.New(store, r)

	// Label by the Gin route template, not c.Request.URL.Path.
	reject := func(c *gin.Context) {
		RateLimitHitsTotal.WithLabelValues(name, normalizeRoutePath(c)).Inc()
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests, please try again later"})
		c.Abort()
	}

	limited := mgin.NewMiddleware(instance,
		mgin.WithKeyGetter(keyGetter),
		mgin.WithErrorHandler(func(c *gin.Context, _ error) { reject(c) }),
		mgin.WithLimitReachedHandler(reject),
	)

	return func(c *gin.Context) {
		RateLimitRequestsTotal.WithLabelValues(name, normalizeRoutePath(c)).Inc()
		limited(c)
	}
}

// NewRateLimitMiddleware creates a Gin middleware that rate-limits requests,
// keyed by c.ClientIP(). rate is the number of requests allowed per period
// (e.g., 10 requests per 1 minute).
func NewRateLimitMiddleware(name string, rate int64, period time.Duration) gin.HandlerFunc {
	return newRateLimitMiddleware(name, rate, period, func(c *gin.Context) string {
		return c.ClientIP()
	})
}

// NewUserRateLimitMiddleware is like NewRateLimitMiddleware, but keys by the
// authenticated user's ID (set by AuthMiddleware as "userID") when present,
// falling back to client IP.
func NewUserRateLimitMiddleware(name string, rate int64, period time.Duration) gin.HandlerFunc {
	return newRateLimitMiddleware(name, rate, period, func(c *gin.Context) string {
		if userID, exists := c.Get("userID"); exists {
			if id, ok := userID.(uuid.UUID); ok {
				return "user:" + id.String()
			}
		}
		return "ip:" + c.ClientIP()
	})
}

// apiGroupPrefix is the router group every versioned route is mounted under;
// the path predicates below are written relative to it.
const apiGroupPrefix = "/api/v1"

// groupRelativePath returns the request path relative to the API router group.
// Falls back to the full path when the prefix is absent (e.g. /health).
func groupRelativePath(c *gin.Context) string {
	path := c.Request.URL.Path
	if idx := strings.Index(path, apiGroupPrefix); idx >= 0 {
		return path[idx+len(apiGroupPrefix):]
	}
	return path
}

// RateLimitByPath applies a rate-limit middleware only to requests whose path
// (relative to the router group) matches one of the given suffixes.
func RateLimitByPath(rl gin.HandlerFunc, paths ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		rel := groupRelativePath(c)
		for _, p := range paths {
			if strings.HasSuffix(rel, p) {
				rl(c)
				return
			}
		}
		c.Next()
	}
}

// RateLimitByPathPrefix applies a rate-limit middleware only to requests whose
// path (relative to the router group) starts with one of the given prefixes.
func RateLimitByPathPrefix(rl gin.HandlerFunc, prefixes ...string) gin.HandlerFunc {
	return RateLimitByPathPrefixExcept(rl, nil, prefixes...)
}

// RateLimitByPathPrefixExcept is RateLimitByPathPrefix with an exact-path
// escape hatch, for the cheap read that happens to live under an expensive
// prefix.
//
// "/imports" is budgeted for one-shot heavy work — parsing and inserting a
// logbook. "/imports/templates" only serves a static catalogue, and the import
// screen reads it on entry: sharing the expensive bucket would let opening that
// screen a dozen times exhaust the budget for the import the pilot came to do.
func RateLimitByPathPrefixExcept(rl gin.HandlerFunc, exempt []string, prefixes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		rel := groupRelativePath(c)
		for _, e := range exempt {
			if rel == e {
				c.Next()
				return
			}
		}
		for _, p := range prefixes {
			if strings.HasPrefix(rel, p) {
				rl(c)
				return
			}
		}
		c.Next()
	}
}

// RateLimitByPathSegment applies a rate-limit middleware to every request
// whose path contains one of the given segments, wherever it appears.
func RateLimitByPathSegment(rl gin.HandlerFunc, segments ...string) gin.HandlerFunc {
	return rateLimitByPathSegment(rl, nil, segments)
}

// RateLimitByPathSegmentForMethods is RateLimitByPathSegment narrowed to a set
// of HTTP methods.
func RateLimitByPathSegmentForMethods(rl gin.HandlerFunc, methods []string, segments ...string) gin.HandlerFunc {
	set := make(map[string]bool, len(methods))
	for _, m := range methods {
		set[strings.ToUpper(m)] = true
	}
	return rateLimitByPathSegment(rl, set, segments)
}

func rateLimitByPathSegment(rl gin.HandlerFunc, methods map[string]bool, segments []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if methods != nil && !methods[c.Request.Method] {
			c.Next()
			return
		}
		rel := groupRelativePath(c)
		for _, s := range segments {
			seg := "/" + strings.Trim(s, "/")
			if strings.HasSuffix(rel, seg) || strings.Contains(rel, seg+"/") {
				rl(c)
				return
			}
		}
		c.Next()
	}
}

// RateLimitByPathWithQueryParam applies a rate-limit middleware only to
// requests whose path (relative to the router group) ends with the given
// suffix AND which carry a non-empty queryParam.
func RateLimitByPathWithQueryParam(rl gin.HandlerFunc, path, queryParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasSuffix(groupRelativePath(c), path) && c.Query(queryParam) != "" {
			rl(c)
			return
		}
		c.Next()
	}
}
