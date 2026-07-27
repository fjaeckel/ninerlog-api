package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// A flight carrying a completed instructor signature is locked: UpdateFlight
// and DeleteFlight both refuse it with ErrFlightLocked. The bulk
// aircraft-rename path bypassed that lock and rewrote aircraft_reg on signed
// entries, mutating attested logbook content while the signature still read
// "completed" — and since signatures store no content hash, undetectably.
//
// This test pins the guard into the query itself.
func TestUpdateWithFlightRename_SkipsSignedFlights(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

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
	mock.ExpectExec("UPDATE aircraft").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// The flights rename MUST carry the signature_id IS NULL predicate. If the
	// guard is dropped this expectation no longer matches and the test fails.
	mock.ExpectExec(`UPDATE flights SET aircraft_reg = \$1, updated_at = NOW\(\)\s+WHERE user_id = \$2 AND aircraft_reg = \$3 AND signature_id IS NULL`).
		WithArgs(ac.Registration, ac.UserID, "D-AAAA").
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
