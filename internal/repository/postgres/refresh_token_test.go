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

func TestRefreshTokenCreate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)
	ctx := context.Background()

	token := &models.RefreshToken{
		UserID:      uuid.New(),
		TokenHash:   "hashed_token",
		ExpiresAt:   time.Now().Add(7 * 24 * time.Hour),
		Revoked:     false,
		SessionID:   uuid.New(),
		DeviceLabel: "Safari on iPhone",
		UserAgent:   "Mozilla/5.0 (iPhone)",
		IPAddress:   "203.0.113.7",
	}

	mock.ExpectExec("INSERT INTO refresh_tokens").
		WithArgs(
			sqlmock.AnyArg(), token.UserID, token.TokenHash, token.ExpiresAt, token.Revoked,
			sqlmock.AnyArg(), sqlmock.AnyArg(),
			token.SessionID, token.DeviceLabel, token.UserAgent, token.IPAddress, sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.Create(ctx, token)
	if err != nil {
		t.Errorf("Create() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestRefreshTokenCreate_MintsSessionWhenAbsent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewRefreshTokenRepository(db)

	token := &models.RefreshToken{
		UserID:    uuid.New(),
		TokenHash: "hashed_token",
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}

	mock.ExpectExec("INSERT INTO refresh_tokens").
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.Create(context.Background(), token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if token.SessionID == uuid.Nil {
		t.Error("Create() left SessionID unset")
	}
	if token.LastUsedAt.IsZero() {
		t.Error("Create() left LastUsedAt unset")
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

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "token_hash", "expires_at", "revoked", "created_at", "updated_at",
		"session_id", "device_label", "user_agent", "ip_address", "last_used_at",
		"revoked_at", "rotated_at",
	}).AddRow(
		uuid.New(), uuid.New(), "hashed_token", time.Now(), false, time.Now(), time.Now(),
		uuid.New(), "Safari on iPhone", "Mozilla/5.0 (iPhone)", "203.0.113.7", time.Now(), nil, nil,
	)

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
