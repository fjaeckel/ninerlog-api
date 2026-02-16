package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	limiter "github.com/ulule/limiter/v3"
	mgin "github.com/ulule/limiter/v3/drivers/middleware/gin"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

// NewRateLimitMiddleware creates a Gin middleware that rate-limits requests.
// rate is the number of requests allowed per period (e.g., 10 requests per 1 minute).
// It uses Gin's c.ClientIP() to key rate limits by the real client IP (respecting
// X-Real-IP / X-Forwarded-For headers set by nginx) instead of the proxy's address.
func NewRateLimitMiddleware(rate int64, period time.Duration) gin.HandlerFunc {
	r := limiter.Rate{
		Period: period,
		Limit:  rate,
	}

	store := memory.NewStore()
	instance := limiter.New(store, r)

	middleware := mgin.NewMiddleware(instance,
		mgin.WithKeyGetter(func(c *gin.Context) string {
			// Use Gin's ClientIP which reads X-Real-IP / X-Forwarded-For from trusted proxies
			return c.ClientIP()
		}),
		mgin.WithErrorHandler(func(c *gin.Context, err error) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests, please try again later"})
			c.Abort()
		}),
		mgin.WithLimitReachedHandler(func(c *gin.Context) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests, please try again later"})
			c.Abort()
		}),
	)

	return middleware
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
