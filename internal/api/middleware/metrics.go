package middleware

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// HTTPRequestsTotal counts total HTTP requests by method, path, and status.
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDurationSeconds tracks HTTP request latency.
	HTTPRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// HTTPResponseSizeBytes tracks HTTP response sizes.
	HTTPResponseSizeBytes = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_response_size_bytes",
			Help:    "HTTP response size in bytes.",
			Buckets: prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
		},
		[]string{"method", "path"},
	)

	// HTTPRequestsInFlight tracks the number of in-flight HTTP requests.
	HTTPRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed.",
		},
	)

	// APIPanicsRecoveredTotal counts panics recovered by the recovery middleware.
	APIPanicsRecoveredTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "api_panics_recovered_total",
			Help: "Total number of panics recovered.",
		},
	)
)

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDurationSeconds,
		HTTPResponseSizeBytes,
		HTTPRequestsInFlight,
		APIPanicsRecoveredTotal,
	)
}

// MetricsMiddleware returns a Gin middleware that records Prometheus metrics
// for every HTTP request: counter, duration histogram, response size histogram,
// and in-flight gauge.
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		HTTPRequestsInFlight.Inc()
		defer HTTPRequestsInFlight.Dec()

		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		path := normalizeRoutePath(c)
		method := c.Request.Method
		duration := time.Since(start).Seconds()
		size := float64(c.Writer.Size())

		HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		HTTPRequestDurationSeconds.WithLabelValues(method, path).Observe(duration)
		if size >= 0 {
			HTTPResponseSizeBytes.WithLabelValues(method, path).Observe(size)
		}
	}
}

// normalizeRoutePath returns the Gin route template (e.g. "/api/v1/flights/:id")
// instead of the actual URL path, to avoid high-cardinality label values.
func normalizeRoutePath(c *gin.Context) string {
	route := c.FullPath()
	if route == "" {
		// Fallback for unmatched routes (404s) — group by first two segments
		return normalizeFallbackPath(c.Request.URL.Path)
	}
	return route
}

// normalizeFallbackPath returns a stable label for requests that don't match
// any registered route (e.g. 404s). Keeps the first two path segments to give
// some routing context, appends "/*unmatched".
func normalizeFallbackPath(path string) string {
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 3)
	if len(parts) >= 2 {
		return "/" + parts[0] + "/" + parts[1] + "/*unmatched"
	}
	return "/*unmatched"
}
