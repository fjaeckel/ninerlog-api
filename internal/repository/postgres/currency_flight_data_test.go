package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func newCurrencyDataProvider(t *testing.T) (*currencyFlightDataProvider, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return &currencyFlightDataProvider{db: db}, mock, func() { db.Close() }
}

var progressColumns = []string{
	"flights", "total_minutes", "pic_minutes", "ifr_minutes", "instructor_minutes", "night_minutes",
	"landings", "day_landings", "night_landings", "approaches", "holds",
	"launches", "training_flights", "longest_training_flight_minutes",
}

func TestGetProgressByAircraftClass_BindsClassArray(t *testing.T) {
	p, mock, done := newCurrencyDataProvider(t)
	defer done()

	userID := uuid.New()
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("upper(trim(a.aircraft_class)) = ANY($2)")).
		WithArgs(userID, pq.Array([]string{"SEP_LAND", "TMG"}), since, false).
		WillReturnRows(sqlmock.NewRows(progressColumns).AddRow(3, 720, 600, 0, 60, 0, 12, 12, 0, 0, 0, 12, 1, 60))

	got, err := p.GetProgressByAircraftClass(context.Background(), userID, []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeTMG}, false, since)
	if err != nil {
		t.Fatalf("GetProgressByAircraftClass: %v", err)
	}
	if got.TotalMinutes != 720 || got.Landings != 12 || got.Flights != 3 ||
		got.Launches != 12 || got.TrainingFlights != 1 || got.LongestTrainingFlightMinutes != 60 {
		t.Errorf("progress = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestGetLastProficiencyCheck_BindsClassArray(t *testing.T) {
	p, mock, done := newCurrencyDataProvider(t)
	defer done()

	userID := uuid.New()
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	checkDate := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("upper(trim(a.aircraft_class)) = ANY($2) AND f.is_proficiency_check")).
		WithArgs(userID, pq.Array([]string{"SEP_LAND", "TMG"}), since).
		WillReturnRows(sqlmock.NewRows([]string{"date"}).AddRow(checkDate))

	got, err := p.GetLastProficiencyCheck(context.Background(), userID, []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeTMG}, since)
	if err != nil {
		t.Fatalf("GetLastProficiencyCheck: %v", err)
	}
	if got == nil || !got.Equal(checkDate) {
		t.Errorf("date = %v, want %v", got, checkDate)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestGetLastProficiencyCheck_IRIgnoresClass(t *testing.T) {
	p, mock, done := newCurrencyDataProvider(t)
	defer done()

	userID := uuid.New()
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("is_proficiency_check = true AND date >= $2")).
		WithArgs(userID, since).
		WillReturnRows(sqlmock.NewRows([]string{"date"}))

	got, err := p.GetLastProficiencyCheck(context.Background(), userID, []models.ClassType{models.ClassTypeIR}, since)
	if err != nil {
		t.Fatalf("GetLastProficiencyCheck: %v", err)
	}
	if got != nil {
		t.Errorf("date = %v, want nil", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}
