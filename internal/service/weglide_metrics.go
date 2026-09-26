package service

import "github.com/prometheus/client_golang/prometheus"

var (
	// WeGlideSyncRunsTotal counts WeGlide sync runs, manual and scheduled,
	// by result.
	//
	// Results:
	//   ok      — every listed flight was processed; the cursor advanced.
	//   partial — stopped at the daily request budget, or with flights left.
	//   failed  — the key was rejected, WeGlide was unreachable, or a flight
	//             could not be stored.
	WeGlideSyncRunsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "weglide_sync_runs_total",
			Help: "WeGlide sync runs by result.",
		},
		[]string{"result"},
	)

	// WeGlideSyncFlightsImportedTotal counts flights created by WeGlide syncs.
	WeGlideSyncFlightsImportedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "weglide_sync_flights_imported_total",
			Help: "Flights imported from WeGlide.",
		},
	)

	// WeGlideSyncDurationSeconds is the duration of one sync run.
	WeGlideSyncDurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "weglide_sync_duration_seconds",
			Help:    "Duration of one WeGlide sync run.",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 12), // 100ms … ~3.4min
		},
	)

	// WeGlideSyncLastSuccessTimestampSeconds is the Unix time of the last
	// complete sync of any user.
	WeGlideSyncLastSuccessTimestampSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "weglide_sync_last_success_timestamp_seconds",
			Help: "Unix timestamp of the last complete WeGlide sync.",
		},
	)
)

func init() {
	prometheus.MustRegister(
		WeGlideSyncRunsTotal,
		WeGlideSyncFlightsImportedTotal,
		WeGlideSyncDurationSeconds,
		WeGlideSyncLastSuccessTimestampSeconds,
	)
}
