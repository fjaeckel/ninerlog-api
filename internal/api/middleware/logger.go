package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// RequestIDKey is the context key for request ID
	RequestIDKey = "request_id"
	// RequestIDHeader is the header key for request ID
	RequestIDHeader = "X-Request-ID"
	// UserIDKey is the context key AuthMiddleware uses to store the
	// authenticated user's ID.
	UserIDKey = "userID"
)

// LoggerMiddleware creates a middleware that emits a structured access-log
// line for every request: who (user ID, client IP), what (method, path),
// when (timestamp via the logger), and the outcome (status, latency). It also
// assigns each request a unique ID, exposed both in the Gin context and the
// X-Request-ID response header for correlation.
//
// The line is written through the provided *slog.Logger; pass nil to use
// slog.Default(). The log level scales with the response status: 5xx logs at
// Error, 4xx at Warn, everything else at Info.
func LoggerMiddleware(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		start := time.Now()

		// Reuse a valid upstream request ID, or mint a fresh one.
		requestID := sanitizeRequestID(c.GetHeader(RequestIDHeader))
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set(RequestIDKey, requestID)
		c.Header(RequestIDHeader, requestID)

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		attrs := []any{
			slog.String("request_id", requestID),
			slog.String("method", c.Request.Method),
			// Path only, never the raw query string.
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", status),
			slog.Int64("latency_ms", latency.Milliseconds()),
			slog.String("client_ip", c.ClientIP()),
		}

		// Attribute the request to a user when authenticated.
		if uid, ok := c.Get(UserIDKey); ok {
			attrs = append(attrs, slog.String("user_id", userIDString(uid)))
		}

		// Surface any private handler errors.
		if errMsg := c.Errors.ByType(gin.ErrorTypePrivate).String(); errMsg != "" {
			attrs = append(attrs, slog.String("error", errMsg))
		}

		switch {
		case status >= 500:
			logger.Error("http request", attrs...)
		case status >= 400:
			logger.Warn("http request", attrs...)
		default:
			logger.Info("http request", attrs...)
		}
	}
}

// maxRequestIDLen bounds an inbound X-Request-ID value.
const maxRequestIDLen = 128

// sanitizeRequestID validates an inbound X-Request-ID header, returning ""
// for values carrying control characters or beyond the length bound.
func sanitizeRequestID(id string) string {
	if id == "" || len(id) > maxRequestIDLen {
		return ""
	}
	for _, r := range id {
		// Reject C0/C1 control characters and DEL.
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return id
}

// userIDString renders whatever AuthMiddleware stored under UserIDKey as a
// string. It handles the common uuid.UUID and string cases explicitly and
// falls back to fmt-style formatting for anything else.
func userIDString(v any) string {
	switch id := v.(type) {
	case uuid.UUID:
		return id.String()
	case string:
		return id
	case interface{ String() string }:
		return id.String()
	default:
		return ""
	}
}
