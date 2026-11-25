package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// TestUpdateWithFlightRename_SkipsSignedFlights pins the signature_id IS NULL
// guard into the bulk aircraft rename query.
func TestUpdateWithFlightRename_SkipsSignedFlights(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewAircraftRepository(db)
	ctx := context.Background()

	ac := &models.Aircraft{
		ID:           uuid.New(),
		UserID:       uuid.New(),
		Registration: "D-ZZZZ",
		Type:         "C172",
		Make:         "Cessna",
		Model:        "172",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	mock.ExpectBegin()
	mock.ExpectExec("SELECT set_config").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE aircraft").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// The expectation matches only a rename carrying signature_id IS NULL.
	mock.ExpectExec(`UPDATE flights SET aircraft_reg = \$1, updated_at = NOW\(\)\s+WHERE user_id = \$2 AND aircraft_reg = \$3 AND signature_id IS NULL`).
		WithArgs(ac.Registration, ac.UserID, "D-AAAA", nil, nil).
		WillReturnResult(sqlmock.NewResult(0, 2))

	mock.ExpectExec("UPDATE flight_sessions").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	updated, err := repo.UpdateWithFlightRename(ctx, ac, "D-AAAA")
	if err != nil {
		t.Fatalf("UpdateWithFlightRename() error = %v", err)
	}
	if updated != 2 {
		t.Errorf("updated = %d, want 2 (only the unsigned flights)", updated)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("signature lock guard missing from the rename query: %v", err)
	}
}

func TestUpdateWithFlightRename_LimitsToValidityRange(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewAircraftRepository(db)
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)
	ac := &models.Aircraft{
		ID: uuid.New(), UserID: uuid.New(), Registration: "N123AB",
		Type: "PA16", Make: "Piper", Model: "Clipper",
		ValidFrom: &from, ValidTo: &to,
	}

	mock.ExpectBegin()
	mock.ExpectExec("SELECT set_config").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE aircraft").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`AND date >= COALESCE\(\$4::date, '-infinity'::date\)\s+AND date <= COALESCE\(\$5::date, 'infinity'::date\)`).
		WithArgs(ac.Registration, ac.UserID, "N123AC", "2020-01-01", "2023-12-31").
		WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectExec("UPDATE flight_sessions").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	updated, err := repo.UpdateWithFlightRename(context.Background(), ac, "N123AC")
	if err != nil {
		t.Fatalf("UpdateWithFlightRename() error = %v", err)
	}
	if updated != 4 {
		t.Errorf("updated = %d, want 4", updated)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("validity range missing from the rename query: %v", err)
	}
}
