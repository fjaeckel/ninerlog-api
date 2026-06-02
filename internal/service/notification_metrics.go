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

	// NotificationCheckErrorsTotal counts notification check runs that aborted
	// early due to an error (e.g. failing to load user preferences).
	NotificationCheckErrorsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "notification_check_errors_total",
			Help: "Total number of background notification check runs that failed.",
		},
	)

	// NotificationLastSuccessTimestampSeconds records the Unix timestamp of the
	// last successfully completed notification check. Use it to alert when the
	// background checker stops running (staleness).
	NotificationLastSuccessTimestampSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "notification_last_success_timestamp_seconds",
			Help: "Unix timestamp of the last successfully completed notification check.",
		},
	)
)

func init() {
	prometheus.MustRegister(
		NotificationCheckRunsTotal,
		NotificationCheckDurationSeconds,
		NotificationsSentTotal,
		NotificationCheckErrorsTotal,
		NotificationLastSuccessTimestampSeconds,
	)
}
