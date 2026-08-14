package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Renaming a contact rewrites the denormalised name on the crew rows that
// reference it, so correcting a misspelling also corrects the logbook. A flight
// carrying a completed instructor signature must be excluded: its crew names
// are attested content, and signatures store no content hash, so rewriting one
// would be undetectable after the fact. This is the same guard the bulk
// aircraft rename carries (see TestUpdateWithFlightRename_SkipsSignedFlights).
//
// This test pins the guard into the query itself.
func TestUpdateWithCrewRename_SkipsSignedFlights(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewContactRepository(db)
	ctx := context.Background()

	contact := &models.Contact{
		ID:     uuid.New(),
		UserID: uuid.New(),
		Name:   "Hans Müller",
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE contacts").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// The crew rename MUST carry f.signature_id IS NULL. If the guard is
	// dropped this expectation no longer matches and the test fails.
	mock.ExpectExec(`UPDATE flight_crew_members fcm\s+SET name = \$1\s+FROM flights f\s+WHERE fcm\.flight_id = f\.id\s+AND fcm\.contact_id = \$2\s+AND f\.user_id = \$3\s+AND f\.signature_id IS NULL`).
		WithArgs(contact.Name, contact.ID, contact.UserID).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	renamed, err := repo.UpdateWithCrewRename(ctx, contact)
	if err != nil {
		t.Fatalf("UpdateWithCrewRename() error = %v", err)
	}
	if renamed != 3 {
		t.Errorf("renamed = %d, want 3 (only the unsigned flights)", renamed)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("signature lock guard missing from the rename query: %v", err)
	}
}

// A rename that collides with another of the user's contacts must surface as
// ErrDuplicate — not as a raw driver error — so the service can turn it into a
// 409 instead of a 500. The transaction must not commit.
func TestUpdateWithCrewRename_DuplicateNameRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewContactRepository(db)

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE contacts").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "idx_contacts_user_lower_name"})
	mock.ExpectRollback()

	_, err = repo.UpdateWithCrewRename(context.Background(), &models.Contact{
		ID: uuid.New(), UserID: uuid.New(), Name: "Anna Berg",
	})
	if !errors.Is(err, repository.ErrDuplicate) {
		t.Errorf("UpdateWithCrewRename() error = %v, want repository.ErrDuplicate", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
