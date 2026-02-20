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

func TestPasswordResetTokenCreate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	token := &models.PasswordResetToken{
		UserID:    uuid.New(),
		TokenHash: "hashed_reset_token",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Used:      false,
	}

	mock.ExpectExec("INSERT INTO password_reset_tokens").
		WithArgs(sqlmock.AnyArg(), token.UserID, token.TokenHash, token.ExpiresAt, token.Used, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.Create(ctx, token)
	if err != nil {
		t.Errorf("Create() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestPasswordResetTokenGetByTokenHash(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"id", "user_id", "token_hash", "expires_at", "used", "created_at"}).
		AddRow(uuid.New(), uuid.New(), "hashed_reset_token", time.Now(), false, time.Now())

	mock.ExpectQuery("SELECT (.+) FROM password_reset_tokens WHERE token_hash").
		WithArgs("hashed_reset_token").
		WillReturnRows(rows)

	token, err := repo.GetByTokenHash(ctx, "hashed_reset_token")
	if err != nil {
		t.Fatalf("GetByTokenHash() error = %v", err)
	}
	if token == nil {
		t.Error("GetByTokenHash() returned nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestPasswordResetTokenMarkAsUsed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	mock.ExpectExec("UPDATE password_reset_tokens SET used").
		WithArgs("hashed_reset_token").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.MarkAsUsed(ctx, "hashed_reset_token")
	if err != nil {
		t.Errorf("MarkAsUsed() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestPasswordResetTokenNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewPasswordResetTokenRepository(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT (.+) FROM password_reset_tokens WHERE token_hash").
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	_, err = repo.GetByTokenHash(ctx, "nonexistent")
	if err != repository.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}
