package updatecheck

import "github.com/prometheus/client_golang/prometheus"

// Prometheus instrumentation for the release check. No series at all are
// emitted when the check is disabled.
var (
	// RunsTotal counts release check runs. Results: success, error.
	RunsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "update_check_runs_total",
			Help: "Total release check runs by result.",
		},
		[]string{"result"},
	)

	// ErrorsTotal counts failed release checks. Reasons: request, status,
	// decode, empty.
	ErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "update_check_errors_total",
			Help: "Total release check failures by reason.",
		},
		[]string{"reason"},
	)

	// DurationSeconds tracks how long one release check run takes, covering
	// every component queried.
	DurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "update_check_duration_seconds",
			Help:    "Latency of one release check run.",
			Buckets: prometheus.ExponentialBuckets(0.05, 2, 10), // 50ms … ~25s
		},
	)

	// LastSuccessTimestampSeconds is the Unix timestamp of the last run in
	// which every component's latest release was read.
	LastSuccessTimestampSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "update_check_last_success_timestamp_seconds",
			Help: "Unix timestamp of the last fully successful release check.",
		},
	)

	// UpdateAvailable reports whether a newer release exists for a component.
	// 1 = behind, 0 = current. Components: api. The frontend's running version
	// is known only to the browser, so it has no series here.
	UpdateAvailable = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "app_update_available",
			Help: "1 when a newer release is published for the component, 0 when current.",
		},
		[]string{"component"},
	)

	// CommitsBehind reports how many commits behind the tracked branch a
	// component's build is. Components: api. Absent for a build compared by
	// release version rather than by commit.
	CommitsBehind = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "app_commits_behind",
			Help: "Commits the tracked branch is ahead of the running build.",
		},
		[]string{"component"},
	)

	// LatestVersionInfo carries the newest published release per component.
	// Components: api, frontend.
	LatestVersionInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "update_check_latest_version_info",
			Help: "Newest published release per component, as a labelled constant 1.",
		},
		[]string{"component", "version"},
	)
)

func init() {
	prometheus.MustRegister(
		RunsTotal,
		ErrorsTotal,
		DurationSeconds,
		LastSuccessTimestampSeconds,
		UpdateAvailable,
		CommitsBehind,
		LatestVersionInfo,
	)
}
