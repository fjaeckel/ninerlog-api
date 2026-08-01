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
	// RateLimitHitsTotal counts requests that were rejected by a rate limiter.
	//
	// The "limiter" label names the bucket that did the rejecting (see the
	// names passed in cmd/api/main.go). Without it every limiter reports into
	// one undifferentiated series, and a 429 on a route covered by two
	// limiters — e.g. /flights, which carries both the coarse "general"
	// limiter and the "search" limiter — cannot be attributed to either.
	RateLimitHitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_hits_total",
			Help: "Total number of requests rejected by a rate limiter, by limiter and route.",
		},
		[]string{"limiter", "path"},
	)

	// RateLimitRequestsTotal counts every request that passed *through* a rate
	// limiter, whether it was allowed or rejected. It is the denominator for
	// RateLimitHitsTotal: on its own a rejection rate says nothing about
	// whether a limit is correctly sized, because 2 rejections/s is a
	// non-event against 200 req/s of traffic and an outage against 3 req/s.
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

	// Label by the Gin route template, not c.Request.URL.Path. The raw URL
	// path turns every /api/v1/flights/{uuid} into its own series — an
	// unbounded cardinality leak — and makes these counters impossible to
	// join against http_requests_total, which normalizes the same way.
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

// NewRateLimitMiddleware creates a Gin middleware that rate-limits requests.
// rate is the number of requests allowed per period (e.g., 10 requests per 1 minute).
// It uses Gin's c.ClientIP() to key rate limits by the real client IP (respecting
// X-Real-IP / X-Forwarded-For headers set by nginx) instead of the proxy's address.
func NewRateLimitMiddleware(name string, rate int64, period time.Duration) gin.HandlerFunc {
	return newRateLimitMiddleware(name, rate, period, func(c *gin.Context) string {
		// Use Gin's ClientIP which reads X-Real-IP / X-Forwarded-For from trusted proxies
		return c.ClientIP()
	})
}

// NewUserRateLimitMiddleware is like NewRateLimitMiddleware, but keys by the
// authenticated user's ID (set by AuthMiddleware as "userID") when present,
// falling back to client IP otherwise (e.g. the request never reached
// AuthMiddleware's authenticated branch). Per-user keying is more precise
// for logged-in traffic than per-IP: it isn't inflated by users sharing a
// NAT/office IP, and isn't defeated by one user rotating source IPs.
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

// apiGroupPrefix is the router group every versioned route is mounted under.
// Path predicates below are written relative to that group (e.g. "/imports"),
// while c.Request.URL.Path is absolute (e.g. "/api/v1/imports/upload"), so the
// prefix has to be stripped before matching.
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

// RateLimitByPathPrefix applies a rate-limit middleware only to requests
// whose path starts with one of the given prefixes. Unlike RateLimitByPath's
// suffix matching, this is needed for routes ending in an opaque token
// (e.g. "/sign/{token}"), which never share a fixed suffix.
func RateLimitByPathPrefix(rl gin.HandlerFunc, prefixes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Must match against the GROUP-RELATIVE path. Comparing the absolute
		// path ("/api/v1/imports/upload") against a group-relative prefix
		// ("/imports") is never true, which silently disabled both the
		// /imports and /sign/ limiters entirely.
		rel := groupRelativePath(c)
		for _, p := range prefixes {
			if strings.HasPrefix(rel, p) {
				rl(c)
				return
			}
		}
		c.Next()
	}
}

// RateLimitByPathWithQueryParam applies a rate-limit middleware only to
// requests whose path (relative to the router group) ends with the given
// suffix AND which carry a non-empty queryParam. This targets expensive
// query variants of an otherwise-cheap route (e.g. GET /flights only
// becomes costly once a free-text search "q" is present) without limiting
// plain, cheap requests to the same path.
func RateLimitByPathWithQueryParam(rl gin.HandlerFunc, path, queryParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasSuffix(groupRelativePath(c), path) && c.Query(queryParam) != "" {
			rl(c)
			return
		}
		c.Next()
	}
}
