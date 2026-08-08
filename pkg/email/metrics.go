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

	// EmailDeliveryTotal breaks send attempts down by what the SMTP
	// conversation actually said, and for which kind of message.
	//
	// It sits alongside EmailSendTotal rather than replacing it: the coarse
	// counter above is what existing dashboards and alert rules are written
	// against, while this one is what tells a hard bounce (a dead address)
	// apart from a server error (a broken mail setup).
	//
	// Statuses: delivered, hard_bounce, soft_bounce, rejected, invalid_address,
	// suppressed, server_error, dry_run.
	EmailDeliveryTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "email_delivery_total",
			Help: "Email send attempts by message type and delivery outcome.",
		},
		[]string{"type", "status"},
	)

	// EmailSuppressedAddresses is the number of addresses currently refused
	// because they hard-bounced. A rising line means real users are losing
	// mail.
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
