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

func TestUserCreate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)
	ctx := context.Background()

	user := &models.User{
		Email:        "test@example.com",
		PasswordHash: "hashed_password",
		Name:         "Test User",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	mock.ExpectExec("INSERT INTO users").
		WithArgs(sqlmock.AnyArg(), user.Email, user.PasswordHash, user.Name, user.CreatedAt, user.UpdatedAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = repo.Create(ctx, user)
	if err != nil {
		t.Errorf("Create() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestUserGetByEmail(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)
	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"id", "email", "password_hash", "name", "two_factor_enabled", "two_factor_secret", "recovery_codes", "created_at", "updated_at"}).
		AddRow(uuid.New(), "test@example.com", "hashed_password", "Test User", false, nil, nil, time.Now(), time.Now())

	mock.ExpectQuery("SELECT (.+) FROM users WHERE email").
		WithArgs("test@example.com").
		WillReturnRows(rows)

	user, err := repo.GetByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("GetByEmail() error = %v", err)
	}
	if user.Email != "test@example.com" {
		t.Errorf("Email = %s, want test@example.com", user.Email)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestUserGetByEmailNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)
	ctx := context.Background()

	mock.ExpectQuery("SELECT (.+) FROM users WHERE email").
		WithArgs("notfound@example.com").
		WillReturnError(sql.ErrNoRows)

	_, err = repo.GetByEmail(ctx, "notfound@example.com")
	if err != repository.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestUserUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)
	ctx := context.Background()

	user := &models.User{
		ID:               uuid.New(),
		Email:            "updated@example.com",
		PasswordHash:     "new_hashed_password",
		Name:             "Updated User",
		TwoFactorEnabled: false,
		TwoFactorSecret:  nil,
		RecoveryCodes:    nil,
		UpdatedAt:        time.Now(),
	}

	mock.ExpectExec("UPDATE users").
		WithArgs(user.Email, user.PasswordHash, user.Name, user.TwoFactorEnabled, user.TwoFactorSecret, user.RecoveryCodes, user.UpdatedAt, user.ID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.Update(ctx, user)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}

func TestUserDelete(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)
	ctx := context.Background()

	userID := uuid.New()

	mock.ExpectExec("DELETE FROM users WHERE id").
		WithArgs(userID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = repo.Delete(ctx, userID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Unfulfilled expectations: %v", err)
	}
}
