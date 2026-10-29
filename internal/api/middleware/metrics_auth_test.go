package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func metricsRouter(token string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/metrics", MetricsAuthMiddleware(token), func(c *gin.Context) {
		c.String(http.StatusOK, "go_goroutines 12")
	})
	return r
}

func getMetrics(r *gin.Engine, authHeader string) int {
	req := httptest.NewRequest("GET", "/metrics", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestMetricsAuth(t *testing.T) {
	r := metricsRouter("s3cret-scrape-token")

	tests := []struct {
		name, header string
		want         int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"empty bearer", "Bearer ", http.StatusUnauthorized},
		{"token prefix only", "Bearer s3cret", http.StatusUnauthorized},
		{"correct bearer", "Bearer s3cret-scrape-token", http.StatusOK},
		// Prometheus' bearer_token_file sends the Bearer form, but accept a
		// raw token too so simple scrape configs work.
		{"raw token", "s3cret-scrape-token", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := getMetrics(r, tc.header); got != tc.want {
				t.Errorf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

// A longer presented token must not be accepted just because it shares a prefix.
func TestMetricsAuth_RejectsPrefixExtension(t *testing.T) {
	r := metricsRouter("abc")
	if got := getMetrics(r, "Bearer abcdef"); got != http.StatusUnauthorized {
		t.Errorf("prefix-extended token accepted: %d", got)
	}
}
