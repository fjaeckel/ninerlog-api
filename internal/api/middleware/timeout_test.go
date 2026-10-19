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

func TestRequestTimeoutWithOverrides_UsesTheBudgetOfTheMatchingPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	api.Use(RequestTimeoutWithOverrides(time.Second, map[string]time.Duration{"/users/me/data": time.Hour}))

	budget := map[string]time.Duration{}
	record := func(c *gin.Context) {
		if d, ok := c.Request.Context().Deadline(); ok {
			budget[c.Request.Method+" "+c.Request.URL.Path] = time.Until(d)
		}
		c.String(http.StatusOK, "ok")
	}
	api.GET("/users/me/data", record)
	api.POST("/users/me/data", record)
	api.GET("/users/me", record)

	for _, route := range [][2]string{
		{"GET", "/api/v1/users/me/data"},
		{"POST", "/api/v1/users/me/data"},
		{"GET", "/api/v1/users/me"},
	} {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(route[0], route[1], nil))
	}

	if got := budget["GET /api/v1/users/me/data"]; got < 59*time.Minute {
		t.Errorf("GET /users/me/data budget = %v, want about an hour", got)
	}
	if got := budget["POST /api/v1/users/me/data"]; got < 59*time.Minute {
		t.Errorf("POST /users/me/data budget = %v, want about an hour", got)
	}
	if got := budget["GET /api/v1/users/me"]; got > time.Second {
		t.Errorf("GET /users/me budget = %v, want the default second", got)
	}
}
