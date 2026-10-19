package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/google/uuid"
)

func TestCurrencyDataProvider_AsOfBindsUntilDay(t *testing.T) {
	p, mock, done := newCurrencyDataProvider(t)
	defer done()

	userID := uuid.New()
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 3, 14, 18, 30, 0, 0, time.UTC)
	until := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta("f.date <= $3::date")).
		WithArgs(userID, since, until).
		WillReturnRows(sqlmock.NewRows(progressColumns).AddRow(1, 60, 60, 0, 0, 0, 1, 1, 0, 0, 0, 1, 0, 0))

	bounded, ok := currency.FlightDataProvider(p).(currency.AsOfProvider)
	if !ok {
		t.Fatal("provider does not implement currency.AsOfProvider")
	}
	if _, err := bounded.AsOf(asOf).GetProgressAll(context.Background(), userID, since); err != nil {
		t.Fatalf("GetProgressAll: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
	if p.until != nil {
		t.Error("AsOf must not bound the receiver")
	}
}
