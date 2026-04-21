package middleware

import (
	"database/sql"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// AppInfo is a constant gauge (value 1) with version and go_version labels.
	AppInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "app_info",
			Help: "Application build information.",
		},
		[]string{"version", "go_version"},
	)

	// AppUptimeSeconds reports seconds since server start.
	AppUptimeSeconds = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "app_uptime_seconds",
			Help: "Seconds since server start.",
		},
		func() float64 { return 0 }, // overwritten by RegisterAppMetrics
	)

	// HealthCheckStatus is 1 when the service is healthy, 0 when unhealthy.
	HealthCheckStatus = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "health_check_status",
			Help: "Health check status: 1 = healthy, 0 = unhealthy.",
		},
	)
)

// RegisterAppMetrics registers app_info and app_uptime_seconds gauges.
// version is the application version string (e.g. "1.0.0" or a git SHA).
func RegisterAppMetrics(version string, startedAt time.Time) {
	prometheus.MustRegister(AppInfo)
	prometheus.MustRegister(HealthCheckStatus)
	AppInfo.WithLabelValues(version, runtime.Version()).Set(1)

	// Re-register uptime with the real start time.
	prometheus.MustRegister(prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "app_uptime_seconds",
			Help: "Seconds since server start.",
		},
		func() float64 {
			return time.Since(startedAt).Seconds()
		},
	))
}

// DBStatsCollector implements prometheus.Collector and exposes sql.DBStats
// as Prometheus gauges on every scrape.
type DBStatsCollector struct {
	db *sql.DB

	openConnections  *prometheus.Desc
	inUseConnections *prometheus.Desc
	idleConnections  *prometheus.Desc
	maxOpenConns     *prometheus.Desc
	waitCount        *prometheus.Desc
	waitDuration     *prometheus.Desc
}

// NewDBStatsCollector creates a collector that reads stats from db on every scrape.
func NewDBStatsCollector(db *sql.DB) *DBStatsCollector {
	return &DBStatsCollector{
		db: db,
		openConnections: prometheus.NewDesc(
			"db_connections_open",
			"Current number of open database connections.",
			nil, nil,
		),
		inUseConnections: prometheus.NewDesc(
			"db_connections_in_use",
			"Current number of in-use database connections.",
			nil, nil,
		),
		idleConnections: prometheus.NewDesc(
			"db_connections_idle",
			"Current number of idle database connections.",
			nil, nil,
		),
		maxOpenConns: prometheus.NewDesc(
			"db_connections_max_open",
			"Maximum number of open connections to the database.",
			nil, nil,
		),
		waitCount: prometheus.NewDesc(
			"db_wait_count_total",
			"Total number of connections waited for.",
			nil, nil,
		),
		waitDuration: prometheus.NewDesc(
			"db_wait_duration_seconds_total",
			"Total time spent waiting for database connections.",
			nil, nil,
		),
	}
}

// Describe sends the descriptors of each metric to the channel.
func (c *DBStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.openConnections
	ch <- c.inUseConnections
	ch <- c.idleConnections
	ch <- c.maxOpenConns
	ch <- c.waitCount
	ch <- c.waitDuration
}

// Collect reads the current sql.DBStats and sends each metric value.
func (c *DBStatsCollector) Collect(ch chan<- prometheus.Metric) {
	stats := c.db.Stats()
	ch <- prometheus.MustNewConstMetric(c.openConnections, prometheus.GaugeValue, float64(stats.OpenConnections))
	ch <- prometheus.MustNewConstMetric(c.inUseConnections, prometheus.GaugeValue, float64(stats.InUse))
	ch <- prometheus.MustNewConstMetric(c.idleConnections, prometheus.GaugeValue, float64(stats.Idle))
	ch <- prometheus.MustNewConstMetric(c.maxOpenConns, prometheus.GaugeValue, float64(stats.MaxOpenConnections))
	ch <- prometheus.MustNewConstMetric(c.waitCount, prometheus.CounterValue, float64(stats.WaitCount))
	ch <- prometheus.MustNewConstMetric(c.waitDuration, prometheus.CounterValue, stats.WaitDuration.Seconds())
}
