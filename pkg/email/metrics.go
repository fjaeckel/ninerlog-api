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

	// EmailDeliveryTotal counts send attempts by message type and delivery
	// outcome. Statuses: delivered, hard_bounce, soft_bounce, rejected,
	// invalid_address, suppressed, server_error, dry_run.
	EmailDeliveryTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "email_delivery_total",
			Help: "Email send attempts by message type and delivery outcome.",
		},
		[]string{"type", "status"},
	)

	// EmailSuppressedAddresses is the number of addresses currently suppressed
	// after a hard bounce.
	EmailSuppressedAddresses = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "email_suppressed_addresses",
			Help: "Number of email addresses suppressed after a permanent delivery failure.",
		},
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
		EmailDeliveryTotal,
		EmailSuppressedAddresses,
		EmailSendDurationSeconds,
	)
}
