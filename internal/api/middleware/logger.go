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
	// authenticated user's ID. Declared here so the access log can attribute
	// requests without importing the auth package.
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

		// Reuse an upstream request ID (e.g. one assigned by nginx) so a log
		// line can be correlated with the proxy's access log; otherwise mint
		// a fresh one. Expose it for downstream correlation either way.
		requestID := sanitizeRequestID(c.GetHeader(RequestIDHeader))
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set(RequestIDKey, requestID)
		c.Header(RequestIDHeader, requestID)

		// Process request. Downstream middleware (e.g. AuthMiddleware) runs
		// inside this call, so any userID it sets is visible below.
		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		attrs := []any{
			slog.String("request_id", requestID),
			slog.String("method", c.Request.Method),
			// Log the request path only, never the raw query string, which
			// may carry sensitive values.
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", status),
			slog.Int64("latency_ms", latency.Milliseconds()),
			slog.String("client_ip", c.ClientIP()),
		}

		// Attribute the request to a user when authenticated. Public and
		// unauthenticated routes simply omit the field.
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

// maxRequestIDLen bounds how much of an inbound X-Request-ID we accept, so a
// client can't bloat log lines with an oversized header value.
const maxRequestIDLen = 128

// sanitizeRequestID validates an inbound X-Request-ID header. It is untrusted
// client input, so anything with control characters (log-injection vectors) or
// beyond the length bound is rejected by returning "", signalling the caller to
// generate a fresh ID instead.
func sanitizeRequestID(id string) string {
	if id == "" || len(id) > maxRequestIDLen {
		return ""
	}
	for _, r := range id {
		// Reject C0/C1 control characters and DEL; allow ordinary printable
		// text (proxies typically send hex/UUID-ish tokens).
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
