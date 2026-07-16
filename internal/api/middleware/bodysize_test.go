package middleware

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMaxBodyBytesMiddleware_AllowsUnderLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MaxBodyBytesMiddleware(10, nil))
	router.POST("/test", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.String(http.StatusRequestEntityTooLarge, "too large")
			return
		}
		c.String(http.StatusOK, string(body))
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString("short"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMaxBodyBytesMiddleware_RejectsOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MaxBodyBytesMiddleware(10, nil))
	router.POST("/test", func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.String(http.StatusRequestEntityTooLarge, "too large")
			return
		}
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString("this body is way longer than the limit"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMaxBodyBytesMiddleware_PathOverrideAllowsLargerBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MaxBodyBytesMiddleware(10, map[string]int64{"/imports/json": 1000}))
	router.POST("/imports/json", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.String(http.StatusRequestEntityTooLarge, "too large")
			return
		}
		c.String(http.StatusOK, string(body))
	})

	req := httptest.NewRequest("POST", "/imports/json", bytes.NewBufferString(strings.Repeat("x", 500)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for overridden path within its larger limit, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMaxBodyBytesMiddleware_PathOverrideStillEnforced(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MaxBodyBytesMiddleware(10, map[string]int64{"/imports/json": 100}))
	router.POST("/imports/json", func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.String(http.StatusRequestEntityTooLarge, "too large")
			return
		}
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("POST", "/imports/json", bytes.NewBufferString(strings.Repeat("x", 500)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 even for overridden path once its own limit is exceeded, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMaxBodyBytesMiddleware_ExemptsMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MaxBodyBytesMiddleware(10, nil))
	router.POST("/upload", func(c *gin.Context) {
		file, _, err := c.Request.FormFile("file")
		if err != nil {
			c.String(http.StatusBadRequest, "no file: %v", err)
			return
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			c.String(http.StatusRequestEntityTooLarge, "too large")
			return
		}
		c.String(http.StatusOK, "%d bytes", len(body))
	})

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "data.csv")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	content := strings.Repeat("x", 200) // well over the 10-byte default limit
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	req := httptest.NewRequest("POST", "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected multipart upload to be exempt from the default limit, got %d: %s", w.Code, w.Body.String())
	}
}
