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

// TestUpdateWithCrewRename_SkipsSignedFlights pins the signature_id IS NULL
// guard into the crew rename query.
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

	// The expectation matches only a rename carrying f.signature_id IS NULL.
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

// TestUpdateWithCrewRename_DuplicateNameRollsBack covers a colliding rename
// surfacing as ErrDuplicate without committing the transaction.
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
