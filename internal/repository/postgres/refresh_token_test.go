package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/google/uuid"
)

func TestRefreshTokenCreate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	ctx := context.Background()

	token := &models.RefreshToken{
		UserID:    uuid.New(),
		TokenHash: "hashed_token",
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		Revoked:   false,
	}

	mock.ExpectExec("INSERT INTO refresh_tokens").
		WithArgs(sqlmock.AnyArg(), token.UserID, token.TokenHash, token.ExpiresAt, token.Revoked, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.Create(ctx, token)
	if err != nil {
		t.Errorf("Create() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestRefreshTokenGetByTokenHash(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"id", "user_id", "token_hash", "expires_at", "revoked", "created_at", "updated_at"}).
		AddRow(uuid.New(), uuid.New(), "hashed_token", time.Now(), false, time.Now(), time.Now())

	mock.ExpectQuery("SELECT (.+) FROM refresh_tokens WHERE token_hash").
		WithArgs("hashed_token").
		WillReturnRows(rows)

	token, err := repo.GetByTokenHash(ctx, "hashed_token")
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

func TestRefreshTokenNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT (.+) FROM refresh_tokens WHERE token_hash").
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
