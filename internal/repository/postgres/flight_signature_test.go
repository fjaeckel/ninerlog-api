package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

func TestFlightSignatureCreate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewFlightSignatureRepository(db)
	ctx := context.Background()

	sig := &models.FlightSignature{
		FlightID: uuid.New(),
		UserID:   uuid.New(),
		Method:   models.SignatureMethodLive,
		Status:   models.SignatureStatusCompleted,
	}

	mock.ExpectQuery("INSERT INTO flight_signatures").
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(time.Now(), time.Now()))

	if err := repo.Create(ctx, sig); err != nil {
		t.Errorf("Create() error = %v", err)
	}
	if sig.ID == uuid.Nil {
		t.Error("Create() did not assign an ID")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestFlightSignatureCreate_DuplicatePendingReturnsErrDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewFlightSignatureRepository(db)
	ctx := context.Background()

	sig := &models.FlightSignature{
		FlightID: uuid.New(),
		UserID:   uuid.New(),
		Method:   models.SignatureMethodDeferred,
		Status:   models.SignatureStatusPending,
	}

	mock.ExpectQuery("INSERT INTO flight_signatures").
		WillReturnError(&pqUniqueViolationError{constraint: "flight_signatures_one_pending_per_flight"})

	err = repo.Create(ctx, sig)
	if err != repository.ErrDuplicate {
		t.Errorf("Create() error = %v, want ErrDuplicate", err)
	}
}

// pqUniqueViolationError is a minimal stand-in for lib/pq's *pq.Error, whose
// Error() string contains the violated constraint name — exactly what
// flightSignatureRepository.Create string-matches against.
type pqUniqueViolationError struct {
	constraint string
}

func (e *pqUniqueViolationError) Error() string {
	return "pq: duplicate key value violates unique constraint \"" + e.constraint + "\""
}

func TestFlightSignatureGetByTokenHash(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewFlightSignatureRepository(db)
	ctx := context.Background()

	id := uuid.New()
	flightID := uuid.New()
	userID := uuid.New()
	now := time.Now()

	rows := sqlmock.NewRows([]string{
		"id", "flight_id", "user_id", "method", "status",
		"token_hash", "token_expires_at",
		"instructor_email", "email_sent_at", "email_send_count", "contact_id",
		"instructor_name", "instructor_credential_no", "signature_image", "signed_at",
		"signer_ip", "signer_user_agent",
		"voided_at", "voided_reason",
		"created_at", "updated_at",
	}).AddRow(
		id, flightID, userID, "deferred", "pending",
		"hashedtoken", now.Add(time.Hour),
		nil, nil, 0, nil,
		nil, nil, nil, nil,
		nil, nil,
		nil, nil,
		now, now,
	)

	mock.ExpectQuery("SELECT (.+) FROM flight_signatures WHERE token_hash").
		WithArgs("hashedtoken").
		WillReturnRows(rows)

	sig, err := repo.GetByTokenHash(ctx, "hashedtoken")
	if err != nil {
		t.Fatalf("GetByTokenHash() error = %v", err)
	}
	if sig.ID != id || sig.Status != models.SignatureStatusPending {
		t.Errorf("GetByTokenHash() = %+v, unexpected", sig)
	}
}

func TestFlightSignatureGetByTokenHash_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewFlightSignatureRepository(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT (.+) FROM flight_signatures WHERE token_hash").
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	_, err = repo.GetByTokenHash(ctx, "nonexistent")
	if err != repository.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestFlightSignatureUpdate_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewFlightSignatureRepository(db)
	ctx := context.Background()

	sig := &models.FlightSignature{ID: uuid.New(), Status: models.SignatureStatusRevoked}

	mock.ExpectQuery("UPDATE flight_signatures SET").
		WillReturnError(sql.ErrNoRows)

	err = repo.Update(ctx, sig)
	if err != repository.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestFlightSignatureExpirePendingPastDue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewFlightSignatureRepository(db)
	ctx := context.Background()

	mock.ExpectExec("UPDATE flight_signatures SET status = 'expired'").
		WillReturnResult(sqlmock.NewResult(0, 3))

	count, err := repo.ExpirePendingPastDue(ctx)
	if err != nil {
		t.Fatalf("ExpirePendingPastDue() error = %v", err)
	}
	if count != 3 {
		t.Errorf("ExpirePendingPastDue() = %d, want 3", count)
	}
}
