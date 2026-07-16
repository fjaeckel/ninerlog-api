package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestTimeoutMiddleware_SetsDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestTimeoutMiddleware(50 * time.Millisecond))

	var hasDeadline bool
	router.GET("/test", func(c *gin.Context) {
		_, hasDeadline = c.Request.Context().Deadline()
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if !hasDeadline {
		t.Error("expected request context to carry a deadline")
	}
}

func TestRequestTimeoutMiddleware_CancelsOnTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestTimeoutMiddleware(10 * time.Millisecond))

	var ctxErr error
	router.GET("/test", func(c *gin.Context) {
		<-c.Request.Context().Done()
		ctxErr = c.Request.Context().Err()
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if ctxErr != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", ctxErr)
	}
}
