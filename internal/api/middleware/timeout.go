package middleware

import (
	"context"
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
