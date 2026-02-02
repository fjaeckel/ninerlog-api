package repository

import (
	"context"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/google/uuid"
)

// UserRepository defines the interface for user data access
type UserRepository interface {
	// Create creates a new user
	Create(ctx context.Context, user *models.User) error

	// GetByID retrieves a user by ID
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)

	// GetByEmail retrieves a user by email
	GetByEmail(ctx context.Context, email string) (*models.User, error)

	// Update updates a user
	Update(ctx context.Context, user *models.User) error

	// Delete deletes a user
	Delete(ctx context.Context, id uuid.UUID) error
}

// RefreshTokenRepository defines the interface for refresh token data access
type RefreshTokenRepository interface {
	// Create creates a new refresh token
	Create(ctx context.Context, token *models.RefreshToken) error

	// GetByTokenHash retrieves a refresh token by its hash
	GetByTokenHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error)

	// RevokeByTokenHash revokes a refresh token
	RevokeByTokenHash(ctx context.Context, tokenHash string) error

	// RevokeAllForUser revokes all refresh tokens for a user
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error

	// DeleteForUser deletes all refresh tokens for a user
	DeleteForUser(ctx context.Context, userID uuid.UUID) error

	// DeleteExpired deletes expired refresh tokens
	DeleteExpired(ctx context.Context) error
}

// PasswordResetTokenRepository defines the interface for password reset token data access
type PasswordResetTokenRepository interface {
	// Create creates a new password reset token
	Create(ctx context.Context, token *models.PasswordResetToken) error

	// GetByTokenHash retrieves a password reset token by its hash
	GetByTokenHash(ctx context.Context, tokenHash string) (*models.PasswordResetToken, error)

	// MarkAsUsed marks a password reset token as used
	MarkAsUsed(ctx context.Context, tokenHash string) error

	// DeleteExpired deletes expired password reset tokens
	DeleteExpired(ctx context.Context) error

	// DeleteForUser deletes all password reset tokens for a user
	DeleteForUser(ctx context.Context, userID uuid.UUID) error
}
