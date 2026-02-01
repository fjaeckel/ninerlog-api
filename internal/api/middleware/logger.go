package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// RequestIDKey is the context key for request ID
	RequestIDKey = "request_id"
	// RequestIDHeader is the header key for request ID
	RequestIDHeader = "X-Request-ID"
)

// LoggerMiddleware creates a middleware for request logging
func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()

		// Generate request ID
		requestID := uuid.New().String()
		c.Set(RequestIDKey, requestID)
		c.Header(RequestIDHeader, requestID)

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)

		// Get status code
		status := c.Writer.Status()

		// Get error if any
		errorMessage := c.Errors.ByType(gin.ErrorTypePrivate).String()

		// Log request details
		// In production, use a structured logger like zerolog or zap
		if errorMessage != "" {
			c.Writer.Write([]byte("")) // Placeholder for actual logging
		}

		// Simple log for now
		_ = latency
		_ = status
	}
}
