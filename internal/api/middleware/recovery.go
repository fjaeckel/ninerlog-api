package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RecoveryWithMetrics returns a Gin middleware that recovers from panics,
// increments the api_panics_recovered_total counter, and returns a 500 response.
func RecoveryWithMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				APIPanicsRecoveredTotal.Inc()
				attrs := []any{slog.Any("panic", r)}
				if id, ok := c.Get(RequestIDKey); ok {
					attrs = append(attrs, slog.Any("request_id", id))
				}
				slog.Error("panic recovered", attrs...)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "Internal server error",
				})
			}
		}()
		c.Next()
	}
}
