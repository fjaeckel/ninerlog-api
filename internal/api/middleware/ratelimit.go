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
	// RateLimitHitsTotal counts requests that were rate-limited.
	RateLimitHitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_hits_total",
			Help: "Total number of requests that were rate-limited.",
		},
		[]string{"path"},
	)
)

func init() {
	prometheus.MustRegister(RateLimitHitsTotal)
}

// newRateLimitMiddleware builds a Gin rate-limit middleware keyed by keyGetter.
// rate is the number of requests allowed per period (e.g., 10 requests per 1 minute).
func newRateLimitMiddleware(rate int64, period time.Duration, keyGetter mgin.KeyGetter) gin.HandlerFunc {
	r := limiter.Rate{
		Period: period,
		Limit:  rate,
	}

	store := memory.NewStore()
	instance := limiter.New(store, r)

	return mgin.NewMiddleware(instance,
		mgin.WithKeyGetter(keyGetter),
		mgin.WithErrorHandler(func(c *gin.Context, err error) {
			RateLimitHitsTotal.WithLabelValues(c.Request.URL.Path).Inc()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests, please try again later"})
			c.Abort()
		}),
		mgin.WithLimitReachedHandler(func(c *gin.Context) {
			RateLimitHitsTotal.WithLabelValues(c.Request.URL.Path).Inc()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests, please try again later"})
			c.Abort()
		}),
	)
}

// NewRateLimitMiddleware creates a Gin middleware that rate-limits requests.
// rate is the number of requests allowed per period (e.g., 10 requests per 1 minute).
// It uses Gin's c.ClientIP() to key rate limits by the real client IP (respecting
// X-Real-IP / X-Forwarded-For headers set by nginx) instead of the proxy's address.
func NewRateLimitMiddleware(rate int64, period time.Duration) gin.HandlerFunc {
	return newRateLimitMiddleware(rate, period, func(c *gin.Context) string {
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
func NewUserRateLimitMiddleware(rate int64, period time.Duration) gin.HandlerFunc {
	return newRateLimitMiddleware(rate, period, func(c *gin.Context) string {
		if userID, exists := c.Get("userID"); exists {
			if id, ok := userID.(uuid.UUID); ok {
				return "user:" + id.String()
			}
		}
		return "ip:" + c.ClientIP()
	})
}

// RateLimitByPath applies a rate-limit middleware only to requests whose path
// (relative to the router group) matches one of the given suffixes.
func RateLimitByPath(rl gin.HandlerFunc, paths ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// c.Request.URL.Path is the full path; check if it ends with any of the target paths
		for _, p := range paths {
			if strings.HasSuffix(c.Request.URL.Path, p) {
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
		for _, p := range prefixes {
			if strings.HasPrefix(c.Request.URL.Path, p) {
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
		if strings.HasSuffix(c.Request.URL.Path, path) && c.Query(queryParam) != "" {
			rl(c)
			return
		}
		c.Next()
	}
}
