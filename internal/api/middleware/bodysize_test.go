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
	router.Use(MaxBodyBytesMiddleware(10, 1<<20, nil))
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
	router.Use(MaxBodyBytesMiddleware(10, 1<<20, nil))
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
	router.Use(MaxBodyBytesMiddleware(10, 1<<20, map[string]int64{"/imports/json": 1000}))
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
	router.Use(MaxBodyBytesMiddleware(10, 1<<20, map[string]int64{"/imports/json": 100}))
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

// Multipart is no longer subject to the small JSON default limit; it gets its
// own (larger) cap instead. See TestMaxBodyBytes_MultipartIsCapped.
func TestMaxBodyBytesMiddleware_MultipartUsesOwnLimitNotJSONDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MaxBodyBytesMiddleware(10, 1<<20, nil))
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

// Multipart requests used to be exempt from any cap. router.MaxMultipartMemory
// only decides how much buffers in RAM before Go spills to disk, and the CSV
// handler's own 10 MB check runs after the body is fully consumed — so an
// oversized upload was received and written to disk before being rejected.
func TestMaxBodyBytes_MultipartIsCapped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MaxBodyBytesMiddleware(10, 64, nil))
	router.POST("/imports/upload", func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusOK)
	})

	// Declared Content-Length over the cap is refused up front.
	req := httptest.NewRequest("POST", "/imports/upload", bytes.NewReader(make([]byte, 512)))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized multipart: status = %d, want 413", w.Code)
	}

	// A body within the cap still passes through.
	req = httptest.NewRequest("POST", "/imports/upload", bytes.NewReader(make([]byte, 16)))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("small multipart: status = %d, want 200", w.Code)
	}
}

// Under-declared / chunked bodies must still be stopped by MaxBytesReader.
func TestMaxBodyBytes_MultipartUnderdeclaredIsCapped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MaxBodyBytesMiddleware(10, 64, nil))
	router.POST("/imports/upload", func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/imports/upload", bytes.NewReader(make([]byte, 512)))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
	req.ContentLength = -1 // unknown length, as with chunked encoding
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("under-declared multipart: status = %d, want 413", w.Code)
	}
}
