package portability

import "github.com/prometheus/client_golang/prometheus"

// Metrics for the leave-whenever-you-want exports.
//
// These are not vanity counters. If pilots are trying to take their logbook
// elsewhere and the export is failing, that is the most urgent possible bug in
// this product — somebody is being held in by a broken door. ExportsTotal is
// labelled by result precisely so failures alert rather than hide inside a
// success rate.
var (
	// ExportsTotal counts portability exports by destination and outcome.
	// The "archive" target is the open format; the rest are vendor products.
	ExportsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "portability_exports_total",
			Help: "Total portability exports by destination target and result.",
		},
		[]string{"target", "result"},
	)

	// ExportDurationSeconds tracks how long an export takes to build. An
	// export that times out for a pilot with a long career is a failure to
	// hand over their data, so the upper buckets matter.
	ExportDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "portability_export_duration_seconds",
			Help:    "Time to gather and render one portability export, by target.",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
		},
		[]string{"target"},
	)

	// ExportFlightRows records how many flights each export carried, so an
	// operator can tell a genuinely empty account from a gathering bug that
	// silently produced a header-only file.
	ExportFlightRows = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "portability_export_flight_rows",
			Help:    "Number of flight rows written per portability export.",
			Buckets: []float64{0, 1, 10, 50, 100, 500, 1000, 5000, 10000},
		},
	)
)

// ResultSuccess and ResultError are the values of the "result" label.
const (
	ResultSuccess = "success"
	ResultError   = "error"
)

// ArchiveTargetLabel is the "target" label value used for the open archive,
// which is not a vendor product but shares the same counters.
const ArchiveTargetLabel = "archive"

func init() {
	prometheus.MustRegister(
		ExportsTotal,
		ExportDurationSeconds,
		ExportFlightRows,
	)
}
