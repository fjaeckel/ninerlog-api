package airports

import "github.com/prometheus/client_golang/prometheus"

// Prometheus instrumentation for the in-memory airport database: one set of
// metrics for the periodic fetch/merge pipeline, one for the read path.
var (
	// FetchTotal counts download+parse attempts per upstream source.
	// Results: success, error.
	FetchTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "airport_db_fetch_total",
			Help: "Total airport source fetch attempts by source and result.",
		},
		[]string{"source", "result"},
	)

	// FetchErrorsTotal counts fetch failures per upstream source.
	// Reasons: request, status, decode, empty.
	FetchErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "airport_db_fetch_errors_total",
			Help: "Total airport source fetch failures by source and reason.",
		},
		[]string{"source", "reason"},
	)

	// FetchDurationSeconds tracks download+parse latency per source.
	FetchDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "airport_db_fetch_duration_seconds",
			Help:    "Latency of downloading and parsing one airport source.",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s … ~51s
		},
		[]string{"source"},
	)

	// FetchBytes reports the size of the last successful download per source.
	FetchBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "airport_db_fetch_bytes",
			Help: "Size in bytes of the last successful download per source.",
		},
		[]string{"source"},
	)

	// SourceRecords reports how many usable records the last successful fetch
	// yielded per source (before merging).
	SourceRecords = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "airport_db_source_records",
			Help: "Records parsed from each source during its last successful fetch.",
		},
		[]string{"source"},
	)

	// LoadDurationSeconds tracks a whole reload: both fetches (in parallel),
	// the merge, and the index build.
	LoadDurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "airport_db_load_duration_seconds",
			Help:    "Duration of a full airport database reload (fetch, merge, index).",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
		},
	)

	// MergeDurationSeconds tracks the merge and index build.
	MergeDurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "airport_db_merge_duration_seconds",
			Help:    "Duration of merging the sources and building the in-memory indexes.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms … ~4s
		},
	)

	// ReloadTotal counts reload outcomes.
	// Results: success (all sources), partial (some sources failed),
	// failed (no source usable), rejected (result too small, snapshot kept).
	ReloadTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "airport_db_reload_total",
			Help: "Total airport database reload attempts by result.",
		},
		[]string{"result"},
	)

	// Airports is the number of airports in the live snapshot.
	Airports = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "airport_db_airports",
			Help: "Number of airports in the active in-memory snapshot.",
		},
	)

	// RecordsByOrigin splits the live snapshot by where its records came from.
	// Origins: ourairports (only there), mwgg (only there), both (merged).
	RecordsByOrigin = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "airport_db_records_by_origin",
			Help: "Records in the active snapshot by originating source(s).",
		},
		[]string{"origin"},
	)

	// MergePreferred reports, for airports present in both sources, which
	// source supplied the winning base record.
	MergePreferred = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "airport_db_merge_preferred",
			Help: "For airports present in both sources, which source won the merge.",
		},
		[]string{"source"},
	)

	// DroppedRecords counts records discarded during the last merge for
	// unusable coordinates.
	DroppedRecords = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "airport_db_dropped_records",
			Help: "Records discarded during the last merge for unusable coordinates.",
		},
	)

	// LastSuccessTimestampSeconds is the Unix time of the last snapshot swap.
	LastSuccessTimestampSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "airport_db_last_success_timestamp_seconds",
			Help: "Unix timestamp of the last successful airport database load.",
		},
	)

	// LookupTotal counts read-path operations.
	// Operations: lookup, search, nearest. Results: hit, miss, unavailable.
	LookupTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "airport_lookup_total",
			Help: "Total airport lookups by operation and result.",
		},
		[]string{"operation", "result"},
	)

	// LookupDurationSeconds is observed only for the scanning operations
	// (search, nearest).
	LookupDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "airport_lookup_duration_seconds",
			Help:    "Latency of scanning airport lookups (search, nearest).",
			Buckets: prometheus.ExponentialBuckets(0.000001, 4, 10), // 1µs … ~260ms
		},
		[]string{"operation"},
	)
)

// Read-path counter children, resolved once at init.
var (
	lookupHit         = LookupTotal.WithLabelValues("lookup", "hit")
	lookupMiss        = LookupTotal.WithLabelValues("lookup", "miss")
	lookupUnavailable = LookupTotal.WithLabelValues("lookup", "unavailable")

	searchHit         = LookupTotal.WithLabelValues("search", "hit")
	searchMiss        = LookupTotal.WithLabelValues("search", "miss")
	searchUnavailable = LookupTotal.WithLabelValues("search", "unavailable")

	nearestHit         = LookupTotal.WithLabelValues("nearest", "hit")
	nearestMiss        = LookupTotal.WithLabelValues("nearest", "miss")
	nearestUnavailable = LookupTotal.WithLabelValues("nearest", "unavailable")

	searchDuration  = LookupDurationSeconds.WithLabelValues("search")
	nearestDuration = LookupDurationSeconds.WithLabelValues("nearest")
)

func init() {
	prometheus.MustRegister(
		FetchTotal,
		FetchErrorsTotal,
		FetchDurationSeconds,
		FetchBytes,
		SourceRecords,
		LoadDurationSeconds,
		MergeDurationSeconds,
		ReloadTotal,
		Airports,
		RecordsByOrigin,
		MergePreferred,
		DroppedRecords,
		LastSuccessTimestampSeconds,
		LookupTotal,
		LookupDurationSeconds,
	)

	// Snapshot age, computed from the live snapshot on scrape.
	prometheus.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "airport_db_age_seconds",
			Help: "Seconds since the active airport snapshot was loaded (0 when none).",
		},
		snapshotAgeSeconds,
	))
}
