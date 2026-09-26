package weglide

import "github.com/prometheus/client_golang/prometheus"

const (
	statusOK            = "ok"
	statusUnauthorized  = "unauthorized"
	statusRateLimited   = "rate_limited"
	statusNotFound      = "not_found"
	statusUpstreamError = "upstream_error"
	statusNetworkError  = "network_error"
	statusTooLarge      = "too_large"
	statusBadResponse   = "bad_response"
)

var (
	// RequestsTotal counts HTTP requests to WeGlide (API and files host) by
	// outcome.
	//
	// Statuses:
	//   ok             — 2xx, body within the size cap.
	//   unauthorized   — 401/403: the key was rejected.
	//   rate_limited   — 429: WeGlide's per-key daily limit.
	//   not_found      — 404.
	//   upstream_error — 5xx.
	//   network_error  — connect, TLS, timeout or read failure.
	//   too_large      — body over the JSON (2 MB) or IGC (5 MB) cap.
	//   bad_response   — undecodable body, unexpected status or IGC path.
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "weglide_requests_total",
			Help: "HTTP requests to WeGlide by outcome.",
		},
		[]string{"status"},
	)

	// RequestDurationSeconds is the latency of one WeGlide HTTP request.
	RequestDurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "weglide_request_duration_seconds",
			Help:    "Latency of one HTTP request to WeGlide.",
			Buckets: prometheus.ExponentialBuckets(0.05, 2, 9), // 50ms … ~12.8s
		},
	)
)

func init() {
	prometheus.MustRegister(RequestsTotal, RequestDurationSeconds)
}
