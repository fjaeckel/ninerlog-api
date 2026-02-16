package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// User represents a user in the system
type User struct {
	ID                  uuid.UUID      `json:"id"`
	Email               string         `json:"email"`
	PasswordHash        string         `json:"-"`
	Name                string         `json:"name"`
	TwoFactorEnabled    bool           `json:"twoFactorEnabled"`
	TwoFactorSecret     *string        `json:"-"` // never exposed in JSON
	RecoveryCodes       pq.StringArray `json:"-"` // never exposed in JSON
	FailedLoginAttempts int            `json:"-"`
	LockedUntil         *time.Time     `json:"-"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// RefreshToken represents a refresh token in the system
type RefreshToken struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PasswordResetToken represents a password reset token
type PasswordResetToken struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}
