package service

import "github.com/prometheus/client_golang/prometheus"

var (
	// NotificationCheckRunsTotal counts total background notification check runs.
	NotificationCheckRunsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "notification_check_runs_total",
			Help: "Total number of background notification check runs.",
		},
	)

	// NotificationCheckDurationSeconds tracks how long each notification check takes.
	NotificationCheckDurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "notification_check_duration_seconds",
			Help:    "Duration of each background notification check run in seconds.",
			Buckets: prometheus.DefBuckets,
		},
	)

	// NotificationsSentTotal counts notifications sent by type.
	NotificationsSentTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_sent_total",
			Help: "Total number of notifications sent.",
		},
		[]string{"type"},
	)
)

func init() {
	prometheus.MustRegister(
		NotificationCheckRunsTotal,
		NotificationCheckDurationSeconds,
		NotificationsSentTotal,
	)
}
