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
	EmailVerified       bool           `json:"emailVerified"`
	TwoFactorEnabled    bool           `json:"twoFactorEnabled"`
	TwoFactorSecret     *string        `json:"-"` // never exposed in JSON
	RecoveryCodes       pq.StringArray `json:"-"` // never exposed in JSON
	FailedLoginAttempts int            `json:"-"`
	LockedUntil         *time.Time     `json:"-"`
	Disabled            bool           `json:"disabled"`
	LastLoginAt         *time.Time     `json:"lastLoginAt,omitempty"`
	TimeDisplayFormat   string         `json:"timeDisplayFormat"`
	DateFormat          string         `json:"dateFormat"`
	DecimalSeparator    string         `json:"decimalSeparator"`
	PreferredLocale     string         `json:"preferredLocale"`
	// Informational 90-day recency indicator preferences (FCL.060(b)-style)
	RecencyPerModel        bool `json:"recencyPerModel"`
	RecencyPerRegistration bool `json:"recencyPerRegistration"`
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

// EmailVerificationToken represents a single-use email-verification token sent
// to a user after registration (or via the resend endpoint).
type EmailVerificationToken struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}
