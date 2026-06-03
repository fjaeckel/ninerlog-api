package email

import "github.com/prometheus/client_golang/prometheus"

var (
	// EmailSendTotal counts email send attempts by result.
	// Results: success, failure, dry_run, invalid_address.
	EmailSendTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "email_send_total",
			Help: "Total number of email send attempts.",
		},
		[]string{"result"},
	)

	// EmailSendDurationSeconds tracks SMTP send latency for delivered emails.
	EmailSendDurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "email_send_duration_seconds",
			Help:    "Latency of the SMTP send call in seconds (both successful and failed attempts).",
			Buckets: prometheus.DefBuckets,
		},
	)
)

func init() {
	prometheus.MustRegister(
		EmailSendTotal,
		EmailSendDurationSeconds,
	)
}
