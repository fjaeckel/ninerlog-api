package middleware

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestTimeoutMiddleware bounds every request's context to the given
// duration.
func RequestTimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// RequestTimeoutWithOverrides is RequestTimeoutMiddleware with a different
// budget for paths ending in one of the override keys (matched relative to the
// router group, like RateLimitByPath).
func RequestTimeoutWithOverrides(timeout time.Duration, overrides map[string]time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		budget := timeout
		rel := groupRelativePath(c)
		for suffix, d := range overrides {
			if strings.HasSuffix(rel, suffix) {
				budget = d
				break
			}
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), budget)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
