package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoggerMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(LoggerMiddleware())
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
	router.Use(LoggerMiddleware())
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
	router.Use(LoggerMiddleware())
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
	router.Use(LoggerMiddleware())
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
			router.Use(LoggerMiddleware())
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
