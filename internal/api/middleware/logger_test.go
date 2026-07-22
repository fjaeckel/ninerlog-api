package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// newCapturingLogger returns a slog.Logger that writes JSON records to buf so
// tests can assert on the emitted access-log fields.
func newCapturingLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// lastLogRecord decodes the final JSON log line written to buf.
func lastLogRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var rec map[string]any
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	for dec.More() {
		rec = map[string]any{}
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("decode log line: %v", err)
		}
	}
	if rec == nil {
		t.Fatal("no log line emitted")
	}
	return rec
}

func TestLoggerMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(LoggerMiddleware(nil))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// Check request ID header is set
	requestID := w.Header().Get(RequestIDHeader)
	if requestID == "" {
		t.Error("Request ID header not set")
	}
}

func TestLoggerMiddlewareRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var capturedRequestID string

	router := gin.New()
	router.Use(LoggerMiddleware(nil))
	router.GET("/test", func(c *gin.Context) {
		// Capture request ID from context
		if id, exists := c.Get(RequestIDKey); exists {
			capturedRequestID = id.(string)
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify request ID in context matches header
	headerRequestID := w.Header().Get(RequestIDHeader)
	if capturedRequestID != headerRequestID {
		t.Errorf("Request ID mismatch: context=%s, header=%s", capturedRequestID, headerRequestID)
	}

	if capturedRequestID == "" {
		t.Error("Request ID not set in context")
	}
}

func TestLoggerMiddlewareWithError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(LoggerMiddleware(nil))
	router.GET("/test", func(c *gin.Context) {
		c.Error(gin.Error{
			Err:  http.ErrAbortHandler,
			Type: gin.ErrorTypePrivate,
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server error"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Middleware should still set request ID even with errors
	requestID := w.Header().Get(RequestIDHeader)
	if requestID == "" {
		t.Error("Request ID header not set even with error")
	}
}

func TestLoggerMiddlewareMultipleRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(LoggerMiddleware(nil))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	requestIDs := make(map[string]bool)

	// Make multiple requests
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		requestID := w.Header().Get(RequestIDHeader)
		if requestID == "" {
			t.Error("Request ID header not set")
		}

		// Check that each request ID is unique
		if requestIDs[requestID] {
			t.Errorf("Duplicate request ID: %s", requestID)
		}
		requestIDs[requestID] = true
	}

	if len(requestIDs) != 5 {
		t.Errorf("Expected 5 unique request IDs, got %d", len(requestIDs))
	}
}

func TestLoggerMiddlewareStatusCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		statusCode int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"400 Bad Request", http.StatusBadRequest},
		{"401 Unauthorized", http.StatusUnauthorized},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(LoggerMiddleware(nil))
			router.GET("/test", func(c *gin.Context) {
				c.JSON(tt.statusCode, gin.H{"status": tt.statusCode})
			})

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tt.statusCode {
				t.Errorf("Status = %d, want %d", w.Code, tt.statusCode)
			}

			// Request ID should be set regardless of status code
			requestID := w.Header().Get(RequestIDHeader)
			if requestID == "" {
				t.Error("Request ID header not set")
			}
		})
	}
}

func TestLoggerMiddlewareLogsCoreFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	router := gin.New()
	router.Use(LoggerMiddleware(newCapturingLogger(&buf)))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test?secret=shhh", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	rec := lastLogRecord(t, &buf)

	if rec["method"] != "GET" {
		t.Errorf("method = %v, want GET", rec["method"])
	}
	if rec["path"] != "/test" {
		t.Errorf("path = %v, want /test", rec["path"])
	}
	if got := rec["status"].(float64); int(got) != http.StatusOK {
		t.Errorf("status = %v, want 200", got)
	}
	if _, ok := rec["latency_ms"]; !ok {
		t.Error("latency_ms not logged")
	}
	if rec["request_id"] != w.Header().Get(RequestIDHeader) {
		t.Errorf("logged request_id %v != header %v", rec["request_id"], w.Header().Get(RequestIDHeader))
	}
	// The raw query string must never appear in the access log.
	if s, _ := rec["path"].(string); s != "/test" {
		t.Errorf("query string leaked into path: %v", s)
	}
	if _, ok := rec["error"]; ok {
		t.Errorf("unexpected error field: %v", rec["error"])
	}
}

func TestLoggerMiddlewareLogsUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	userID := uuid.New()

	var buf bytes.Buffer
	router := gin.New()
	router.Use(LoggerMiddleware(newCapturingLogger(&buf)))
	// Simulate AuthMiddleware populating the user ID for authenticated routes.
	router.Use(func(c *gin.Context) {
		c.Set(UserIDKey, userID)
		c.Next()
	})
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)

	rec := lastLogRecord(t, &buf)
	if rec["user_id"] != userID.String() {
		t.Errorf("user_id = %v, want %v", rec["user_id"], userID.String())
	}
}

func TestLoggerMiddlewareOmitsUserIDWhenAnonymous(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	router := gin.New()
	router.Use(LoggerMiddleware(newCapturingLogger(&buf)))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))

	rec := lastLogRecord(t, &buf)
	if _, ok := rec["user_id"]; ok {
		t.Errorf("user_id should be omitted for anonymous requests, got %v", rec["user_id"])
	}
}

func TestLoggerMiddlewareLogsRealClientIPBehindProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	router := gin.New()
	// Mirror main.go's proxy configuration: trust the private-range proxy and
	// derive the client IP from X-Forwarded-For / X-Real-IP.
	if err := router.SetTrustedProxies([]string{"10.0.0.0/8"}); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}
	router.ForwardedByClientIP = true
	router.RemoteIPHeaders = []string{"X-Real-IP", "X-Forwarded-For"}
	router.Use(LoggerMiddleware(newCapturingLogger(&buf)))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	// Request arrives from the trusted proxy...
	req.RemoteAddr = "10.1.2.3:54321"
	// ...carrying the real client's address in the forwarded header.
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	router.ServeHTTP(httptest.NewRecorder(), req)

	rec := lastLogRecord(t, &buf)
	if rec["client_ip"] != "203.0.113.7" {
		t.Errorf("client_ip = %v, want 203.0.113.7 (real client, not proxy)", rec["client_ip"])
	}
}

func TestLoggerMiddlewareHonorsInboundRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	router := gin.New()
	router.Use(LoggerMiddleware(newCapturingLogger(&buf)))
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	const upstreamID = "nginx-abc-123"
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(RequestIDHeader, upstreamID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if got := w.Header().Get(RequestIDHeader); got != upstreamID {
		t.Errorf("response request ID = %q, want inbound %q", got, upstreamID)
	}
	rec := lastLogRecord(t, &buf)
	if rec["request_id"] != upstreamID {
		t.Errorf("logged request_id = %v, want inbound %q", rec["request_id"], upstreamID)
	}
}

func TestLoggerMiddlewareRejectsBadInboundRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		id   string
	}{
		{"control chars", "bad\nid\r"},
		{"too long", strings.Repeat("x", maxRequestIDLen+1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			router := gin.New()
			router.Use(LoggerMiddleware(newCapturingLogger(&buf)))
			router.GET("/test", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set(RequestIDHeader, tt.id)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// A rejected header is replaced by a freshly generated UUID.
			got := w.Header().Get(RequestIDHeader)
			if got == tt.id {
				t.Errorf("bad inbound request ID %q was not rejected", tt.id)
			}
			if _, err := uuid.Parse(got); err != nil {
				t.Errorf("replacement request ID %q is not a UUID: %v", got, err)
			}
		})
	}
}

func TestLoggerMiddlewareLevelByStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		status    int
		wantLevel string
	}{
		{http.StatusOK, "INFO"},
		{http.StatusBadRequest, "WARN"},
		{http.StatusInternalServerError, "ERROR"},
	}

	for _, tt := range tests {
		var buf bytes.Buffer
		router := gin.New()
		router.Use(LoggerMiddleware(newCapturingLogger(&buf)))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(tt.status, gin.H{})
		})

		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))

		rec := lastLogRecord(t, &buf)
		if rec["level"] != tt.wantLevel {
			t.Errorf("status %d: level = %v, want %v", tt.status, rec["level"], tt.wantLevel)
		}
	}
}
