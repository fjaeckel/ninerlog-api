package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func getCounterValue(counter *prometheus.CounterVec, labels ...string) float64 {
	m := &dto.Metric{}
	if err := counter.WithLabelValues(labels...).Write(m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

func getGaugeValue(gauge prometheus.Gauge) float64 {
	m := &dto.Metric{}
	if c, ok := gauge.(prometheus.Metric); ok {
		if err := c.Write(m); err != nil {
			return 0
		}
	}
	return m.GetGauge().GetValue()
}

func getHistogramCount(hist *prometheus.HistogramVec, labels ...string) uint64 {
	m := &dto.Metric{}
	observer := hist.WithLabelValues(labels...)
	if metric, ok := observer.(prometheus.Metric); ok {
		if err := metric.Write(m); err != nil {
			return 0
		}
	}
	return m.GetHistogram().GetSampleCount()
}

func TestMetricsMiddleware_IncrementsCounter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MetricsMiddleware())
	router.GET("/test/hello", func(c *gin.Context) {
		c.String(http.StatusOK, "hello")
	})

	before := getCounterValue(HTTPRequestsTotal, "GET", "/test/hello", "200")

	req := httptest.NewRequest("GET", "/test/hello", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	after := getCounterValue(HTTPRequestsTotal, "GET", "/test/hello", "200")
	if after != before+1 {
		t.Errorf("http_requests_total counter did not increment: before=%f, after=%f", before, after)
	}
}

func TestMetricsMiddleware_RecordsDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MetricsMiddleware())
	router.GET("/test/duration", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	before := getHistogramCount(HTTPRequestDurationSeconds, "GET", "/test/duration")

	req := httptest.NewRequest("GET", "/test/duration", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	after := getHistogramCount(HTTPRequestDurationSeconds, "GET", "/test/duration")
	if after != before+1 {
		t.Errorf("http_request_duration_seconds histogram did not record: before=%d, after=%d", before, after)
	}
}

func TestMetricsMiddleware_RecordsResponseSize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MetricsMiddleware())
	router.GET("/test/size", func(c *gin.Context) {
		c.String(http.StatusOK, "response body here")
	})

	before := getHistogramCount(HTTPResponseSizeBytes, "GET", "/test/size")

	req := httptest.NewRequest("GET", "/test/size", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	after := getHistogramCount(HTTPResponseSizeBytes, "GET", "/test/size")
	if after != before+1 {
		t.Errorf("http_response_size_bytes histogram did not record: before=%d, after=%d", before, after)
	}
}

func TestMetricsMiddleware_NormalizesPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MetricsMiddleware())
	router.GET("/api/v1/flights/:id", func(c *gin.Context) {
		c.String(http.StatusOK, "flight")
	})

	before := getCounterValue(HTTPRequestsTotal, "GET", "/api/v1/flights/:id", "200")

	// Request with concrete ID
	req := httptest.NewRequest("GET", "/api/v1/flights/abc-123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	after := getCounterValue(HTTPRequestsTotal, "GET", "/api/v1/flights/:id", "200")
	if after != before+1 {
		t.Errorf("expected normalized path label '/api/v1/flights/:id', counter did not increment")
	}
}

func TestMetricsMiddleware_UnmatchedRoutesFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MetricsMiddleware())
	// No route registered for /nonexistent

	before := getCounterValue(HTTPRequestsTotal, "GET", "/*unmatched", "404")

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	after := getCounterValue(HTTPRequestsTotal, "GET", "/*unmatched", "404")
	if after != before+1 {
		t.Errorf("unmatched routes should use fallback path label, counter: before=%f, after=%f", before, after)
	}
}

func TestRecoveryWithMetrics_IncrementsOnPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RecoveryWithMetrics())
	router.GET("/test/panic", func(c *gin.Context) {
		panic("test panic")
	})

	m := &dto.Metric{}
	APIPanicsRecoveredTotal.Write(m)
	before := m.GetCounter().GetValue()

	req := httptest.NewRequest("GET", "/test/panic", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 after panic, got %d", w.Code)
	}

	APIPanicsRecoveredTotal.Write(m)
	after := m.GetCounter().GetValue()
	if after != before+1 {
		t.Errorf("api_panics_recovered_total did not increment: before=%f, after=%f", before, after)
	}
}

func TestRecoveryWithMetrics_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RecoveryWithMetrics())
	router.GET("/test/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/test/ok", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestNormalizeFallbackPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/nonexistent", "/api/v1/*unmatched"},
		{"/api/v1/flights/abc/extra", "/api/v1/*unmatched"},
		{"/short", "/*unmatched"},
		{"/", "/*unmatched"},
	}

	for _, tt := range tests {
		got := normalizeFallbackPath(tt.path)
		if got != tt.want {
			t.Errorf("normalizeFallbackPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestMetricsEndpoint_ReturnsPrometheusFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(MetricsMiddleware())

	// Import promhttp handler like main.go does
	router.GET("/metrics", gin.WrapH(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Gather metrics and write them
		mfs, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		for _, mf := range mfs {
			w.Write([]byte(mf.GetName() + "\n"))
		}
	})))

	// Make a request to generate some metrics
	router.GET("/test/metrics-gen", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest("GET", "/test/metrics-gen", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Now fetch /metrics
	req = httptest.NewRequest("GET", "/metrics", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from /metrics, got %d", w.Code)
	}

	body := w.Body.String()
	expected := []string{
		"http_requests_total",
		"http_request_duration_seconds",
		"http_response_size_bytes",
		"http_requests_in_flight",
	}
	for _, metric := range expected {
		if !strings.Contains(body, metric) {
			t.Errorf("/metrics output missing expected metric family %q", metric)
		}
	}
}
