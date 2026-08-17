package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func newCustomDataProvider(t *testing.T) (*customCurrencyDataProvider, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return &customCurrencyDataProvider{db: db}, mock, func() { db.Close() }
}

func TestAggregateMetrics_FilterValuesAreBound(t *testing.T) {
	p, mock, done := newCustomDataProvider(t)
	defer done()

	userID := uuid.New()
	since := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	filters := []models.CurrencyFilter{
		{Field: "aircraft_class", Op: "eq", Value: "SEP_LAND"},
		{Field: "aircraft_type", Op: "in", Values: []string{"C172", "PA28"}},
		{Field: "has_night", Op: "is_true"},
	}

	// Args: userID, since, then filter values in order. is_true binds nothing.
	mock.ExpectQuery("FROM flights f").
		WithArgs(userID, since, "SEP_LAND", "C172", "PA28").
		WillReturnRows(sqlmock.NewRows([]string{"m0"}).AddRow(int64(1)))

	vals, err := p.AggregateMetrics(context.Background(), userID, since, []string{"night_landings"}, filters)
	if err != nil {
		t.Fatalf("AggregateMetrics: %v", err)
	}
	if len(vals) != 1 || vals[0] != 1 {
		t.Errorf("values = %v", vals)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestMetricRowsByDate_BindsSameParameters(t *testing.T) {
	p, mock, done := newCustomDataProvider(t)
	defer done()

	userID := uuid.New()
	since := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	flightDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	filters := []models.CurrencyFilter{
		{Field: "aircraft_class", Op: "eq", Value: "SEP_LAND"},
	}

	mock.ExpectQuery("ORDER BY f.date ASC").
		WithArgs(userID, since, "SEP_LAND").
		WillReturnRows(sqlmock.NewRows([]string{"date", "m0"}).AddRow(flightDate, int64(2)))

	rows, err := p.MetricRowsByDate(context.Background(), userID, since, []string{"landings"}, filters)
	if err != nil {
		t.Fatalf("MetricRowsByDate: %v", err)
	}
	if len(rows) != 1 || !rows[0].Date.Equal(flightDate) || rows[0].Values[0] != 2 {
		t.Errorf("rows = %+v", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestAggregateMetrics_UnknownMetricRejected(t *testing.T) {
	p, _, done := newCustomDataProvider(t)
	defer done()

	if _, err := p.AggregateMetrics(context.Background(), uuid.New(), time.Now(), []string{"evil'; DROP"}, nil); err == nil {
		t.Error("expected error for unknown metric")
	}
}

func TestBuildCustomFilterClause(t *testing.T) {
	t.Run("eq", func(t *testing.T) {
		args := []interface{}{"u", "since"}
		clause, err := buildCustomFilterClause(models.CurrencyFilter{Field: "aircraft_class", Op: "eq", Value: "SEP_LAND"}, &args)
		if err != nil {
			t.Fatal(err)
		}
		if clause != "a.aircraft_class = $3" {
			t.Errorf("clause = %q", clause)
		}
		if len(args) != 3 || args[2] != "SEP_LAND" {
			t.Errorf("args = %v", args)
		}
	})
	t.Run("in", func(t *testing.T) {
		args := []interface{}{"u", "since"}
		clause, err := buildCustomFilterClause(models.CurrencyFilter{Field: "aircraft_type", Op: "in", Values: []string{"A", "B"}}, &args)
		if err != nil {
			t.Fatal(err)
		}
		if clause != "f.aircraft_type IN ($3, $4)" {
			t.Errorf("clause = %q", clause)
		}
	})
	t.Run("is_true uses fixed predicate, binds nothing", func(t *testing.T) {
		args := []interface{}{"u", "since"}
		clause, err := buildCustomFilterClause(models.CurrencyFilter{Field: "has_ifr", Op: "is_true"}, &args)
		if err != nil {
			t.Fatal(err)
		}
		if clause != "f.ifr_time > 0" {
			t.Errorf("clause = %q", clause)
		}
		if len(args) != 2 {
			t.Errorf("is_true should not bind args, got %v", args)
		}
	})
	t.Run("unknown field rejected", func(t *testing.T) {
		args := []interface{}{}
		if _, err := buildCustomFilterClause(models.CurrencyFilter{Field: "evil'; DROP", Op: "eq", Value: "x"}, &args); err == nil {
			t.Error("expected error for unknown field")
		}
	})
	t.Run("unknown boolean field rejected", func(t *testing.T) {
		args := []interface{}{}
		if _, err := buildCustomFilterClause(models.CurrencyFilter{Field: "evil", Op: "is_true"}, &args); err == nil {
			t.Error("expected error for unknown boolean field")
		}
	})
}

// TestCustomMetricMapsStayConsistent guards the invariant the lapse
// computation relies on: every aggregate metric has a matching per-flight
// expression and vice versa.
func TestCustomMetricMapsStayConsistent(t *testing.T) {
	for m := range customMetricSQL {
		if _, ok := customMetricRowSQL[m]; !ok {
			t.Errorf("metric %q has an aggregate expression but no per-flight expression", m)
		}
	}
	for m := range customMetricRowSQL {
		if _, ok := customMetricSQL[m]; !ok {
			t.Errorf("metric %q has a per-flight expression but no aggregate expression", m)
		}
	}
}
