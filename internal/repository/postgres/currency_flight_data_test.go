package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
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

func TestGetProgressByULKind_BindsKindsAndUnspecified(t *testing.T) {
	p, mock, done := newCurrencyDataProvider(t)
	defer done()

	userID := uuid.New()
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("(a.ul_kind = ANY($2) OR ($3 AND a.ul_kind IS NULL)) AND ($4 = 0 OR a.mtom_kg >= $4) AND f.date >= $5")).
		WithArgs(userID, pq.Array([]string{"THREE_AXIS"}), true, 0, since, false).
		WillReturnRows(sqlmock.NewRows(progressColumns).AddRow(2, 180, 180, 0, 60, 0, 4, 4, 0, 0, 0, 4, 1, 60))

	got, err := p.GetProgressByULKind(context.Background(), userID, currency.ULSelector{Kinds: []models.ULKind{models.ULKindThreeAxis}, IncludeUnspecified: true}, false, since)
	if err != nil {
		t.Fatalf("GetProgressByULKind: %v", err)
	}
	if got.TotalMinutes != 180 || got.Landings != 4 {
		t.Errorf("progress = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestGetLastProficiencyCheckByULKind(t *testing.T) {
	p, mock, done := newCurrencyDataProvider(t)
	defer done()

	userID := uuid.New()
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("a.mtom_kg >= $4) AND f.is_proficiency_check = true")).
		WithArgs(userID, pq.Array([]string{"GYROPLANE"}), false, 450, since).
		WillReturnRows(sqlmock.NewRows([]string{"date"}))

	got, err := p.GetLastProficiencyCheckByULKind(context.Background(), userID, currency.ULSelector{Kinds: []models.ULKind{models.ULKindGyroplane}, MinMTOMKg: 450}, since)
	if err != nil {
		t.Fatalf("GetLastProficiencyCheckByULKind: %v", err)
	}
	if got != nil {
		t.Errorf("date = %v, want nil", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

func TestGetLandingDaysByULKind(t *testing.T) {
	p, mock, done := newCurrencyDataProvider(t)
	defer done()

	userID := uuid.New()
	since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	day := time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("a.mtom_kg >= $4) AND f.date >= $5")).
		WithArgs(userID, pq.Array([]string{"SAILPLANE"}), true, 0, since, true).
		WillReturnRows(sqlmock.NewRows([]string{"date", "day_landings", "night_landings", "takeoffs"}).AddRow(day, 3, 0, 2))

	got, err := p.GetLandingDaysByULKind(context.Background(), userID, currency.ULSelector{Kinds: []models.ULKind{models.ULKindSailplane}, IncludeUnspecified: true}, true, since)
	if err != nil {
		t.Fatalf("GetLandingDaysByULKind: %v", err)
	}
	if len(got) != 1 || got[0].DayLandings != 3 || got[0].Takeoffs != 2 || !got[0].Date.Equal(day) {
		t.Errorf("days = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}
