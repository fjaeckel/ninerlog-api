package repository

import (
	"context"
	"time"

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

// LicenseRepository defines the interface for license data access
type LicenseRepository interface {
	// Create creates a new license
	Create(ctx context.Context, license *models.License) error

	// GetByID retrieves a license by its ID
	GetByID(ctx context.Context, id uuid.UUID) (*models.License, error)

	// GetByUserID retrieves all licenses for a user
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.License, error)

	// GetActiveByUserID retrieves all active licenses for a user
	GetActiveByUserID(ctx context.Context, userID uuid.UUID) ([]*models.License, error)

	// Update updates a license
	Update(ctx context.Context, license *models.License) error

	// Delete deletes a license
	Delete(ctx context.Context, id uuid.UUID) error
}

// FlightRepository defines the interface for flight data access
type FlightRepository interface {
	// Create creates a new flight
	Create(ctx context.Context, flight *models.Flight) error

	// GetByID retrieves a flight by its ID
	GetByID(ctx context.Context, id uuid.UUID) (*models.Flight, error)

	// GetByUserID retrieves flights for a user with optional filters
	GetByUserID(ctx context.Context, userID uuid.UUID, opts *FlightQueryOptions) ([]*models.Flight, error)

	// GetByLicenseID retrieves flights for a license with optional filters
	GetByLicenseID(ctx context.Context, licenseID uuid.UUID, opts *FlightQueryOptions) ([]*models.Flight, error)

	// Update updates a flight
	Update(ctx context.Context, flight *models.Flight) error

	// Delete deletes a flight
	Delete(ctx context.Context, id uuid.UUID) error

	// CountByUserID counts flights for a user with optional filters
	CountByUserID(ctx context.Context, userID uuid.UUID, opts *FlightQueryOptions) (int, error)

	// GetStatsByLicenseID returns aggregated flight statistics for a license
	GetStatsByLicenseID(ctx context.Context, licenseID uuid.UUID, startDate, endDate *time.Time) (*models.FlightStatistics, error)

	// GetCurrencyData returns landing counts and flight count for a license within a date range
	GetCurrencyData(ctx context.Context, licenseID uuid.UUID, since time.Time) (*models.CurrencyData, error)
}

// FlightQueryOptions represents query parameters for filtering flights
type FlightQueryOptions struct {
	LicenseID   *uuid.UUID
	StartDate   *time.Time
	EndDate     *time.Time
	AircraftReg *string
	Page        int
	PageSize    int
	SortBy      string // "date", "totalTime", "createdAt"
	SortOrder   string // "asc", "desc"
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

// CredentialRepository defines the interface for credential data access
type CredentialRepository interface {
	Create(ctx context.Context, credential *models.Credential) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Credential, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Credential, error)
	Update(ctx context.Context, credential *models.Credential) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type AircraftRepository interface {
	Create(ctx context.Context, aircraft *models.Aircraft) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Aircraft, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Aircraft, error)
	Update(ctx context.Context, aircraft *models.Aircraft) error
	Delete(ctx context.Context, id uuid.UUID) error
	CountByUserID(ctx context.Context, userID uuid.UUID) (int, error)
}

// NotificationRepository defines the interface for notification data access
type NotificationRepository interface {
	GetPreferences(ctx context.Context, userID uuid.UUID) (*models.NotificationPreferences, error)
	UpsertPreferences(ctx context.Context, prefs *models.NotificationPreferences) error
	LogNotification(ctx context.Context, log *models.NotificationLog) error
	HasBeenSent(ctx context.Context, userID uuid.UUID, notificationType string, referenceID uuid.UUID, daysBeforeExpiry int) (bool, error)
	GetAllUsersWithPreferences(ctx context.Context) ([]*models.NotificationPreferences, error)
}
