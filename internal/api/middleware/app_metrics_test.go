package middleware

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestDBStatsCollector_DescribeAndCollect(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	collector := NewDBStatsCollector(db)

	// Test Describe
	descCh := make(chan *prometheus.Desc, 10)
	collector.Describe(descCh)
	close(descCh)

	descs := make([]*prometheus.Desc, 0)
	for d := range descCh {
		descs = append(descs, d)
	}

	if len(descs) != 6 {
		t.Errorf("expected 6 descriptors, got %d", len(descs))
	}

	// Test Collect
	metricCh := make(chan prometheus.Metric, 10)
	collector.Collect(metricCh)
	close(metricCh)

	metrics := make([]prometheus.Metric, 0)
	for m := range metricCh {
		metrics = append(metrics, m)
	}

	if len(metrics) != 6 {
		t.Errorf("expected 6 metrics, got %d", len(metrics))
	}

	// Verify each metric can be written
	for _, m := range metrics {
		d := &dto.Metric{}
		if err := m.Write(d); err != nil {
			t.Errorf("failed to write metric: %v", err)
		}
	}
}
