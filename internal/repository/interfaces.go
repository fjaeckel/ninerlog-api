package repository

import (
	"context"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/flightsearch"
	"github.com/fjaeckel/ninerlog-api/internal/models"
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

	// IncrementFailedLoginAttempts increments the failed login counter
	IncrementFailedLoginAttempts(ctx context.Context, id uuid.UUID) error

	// ResetFailedLoginAttempts resets the failed login counter to 0
	ResetFailedLoginAttempts(ctx context.Context, id uuid.UUID) error

	// LockAccount locks the account until the given time
	LockAccount(ctx context.Context, id uuid.UUID, until time.Time) error

	// MarkEmailVerified flips the email_verified flag to true.
	MarkEmailVerified(ctx context.Context, id uuid.UUID) error
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

	// Update updates a flight
	Update(ctx context.Context, flight *models.Flight) error

	// Delete deletes a flight
	Delete(ctx context.Context, id uuid.UUID) error

	// DeleteAllByUserID deletes all flights for a user, returns count deleted
	DeleteAllByUserID(ctx context.Context, userID uuid.UUID) (int64, error)

	// CountByUserID counts flights for a user with optional filters
	CountByUserID(ctx context.Context, userID uuid.UUID, opts *FlightQueryOptions) (int, error)

	// GetStatsByUserID returns aggregated flight statistics for a user
	GetStatsByUserID(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time) (*models.FlightStatistics, error)

	// GetCurrencyData returns landing counts and flight count for a user within a date range
	GetCurrencyData(ctx context.Context, userID uuid.UUID, since time.Time) (*models.CurrencyData, error)

	// SetSignatureLock sets (or, passing nil, clears) the flight's signature
	// lock pointer. Locked iff signatureID is non-nil.
	SetSignatureLock(ctx context.Context, flightID uuid.UUID, signatureID *uuid.UUID) error
}

// FlightQueryOptions represents query parameters for filtering flights
type FlightQueryOptions struct {
	StartDate     *time.Time
	EndDate       *time.Time
	AircraftReg   *string
	DepartureICAO *string
	ArrivalICAO   *string
	IsPIC         *bool
	IsDual        *bool
	Search        *string
	// Query is a parsed advanced search query (see internal/flightsearch).
	// It compiles to a SQL condition and is ANDed with the other filters.
	Query *flightsearch.Query
	Page          int
	PageSize      int
	SortBy        string // "date", "totalTime", "createdAt"
	SortOrder     string // "asc", "desc"

	// Logbook filtering: when FilterByRegistrations is true, only flights whose
	// aircraft_reg matches one of AircraftRegistrations (case-insensitive) are
	// returned. Registrations should be supplied upper-cased. An empty slice with
	// FilterByRegistrations=true matches no flights. This filter is applied at the
	// SQL level so it works correctly together with pagination and counting.
	FilterByRegistrations bool
	AircraftRegistrations []string
}

// FlightSessionRepository defines the interface for in-progress flight
// session (tap-to-log quick log) data access.
type FlightSessionRepository interface {
	// Create creates a new flight session. Returns ErrDuplicate when the
	// user already has an open session (enforced by a partial unique index).
	Create(ctx context.Context, session *models.FlightSession) error

	// GetOpenByUserID returns the user's open session, or ErrNotFound.
	GetOpenByUserID(ctx context.Context, userID uuid.UUID) (*models.FlightSession, error)

	// Update persists mutable session fields (timestamps, aircraft, route,
	// status, flight_id). Returns ErrNotFound when the session does not exist.
	Update(ctx context.Context, session *models.FlightSession) error
}

// FlightSignatureRepository defines the interface for instructor sign-off
// request/record data access. See models.FlightSignature for the field
// rationale; rows are append-only history, with flights.signature_id (via
// FlightRepository.SetSignatureLock) as the denormalized "is locked" pointer.
type FlightSignatureRepository interface {
	// Create creates a new flight signature row (live 'completed' or
	// deferred 'pending'). Returns ErrDuplicate if a pending row already
	// exists for the flight (enforced by a partial unique index).
	Create(ctx context.Context, sig *models.FlightSignature) error

	// GetByID retrieves a signature by ID.
	GetByID(ctx context.Context, id uuid.UUID) (*models.FlightSignature, error)

	// GetByTokenHash retrieves a signature by its hashed token, for the
	// public /sign/{token} flow. Returns ErrNotFound if no row has this hash
	// (regardless of status) so callers can't distinguish "never existed"
	// from "already used" via this lookup alone.
	GetByTokenHash(ctx context.Context, tokenHash string) (*models.FlightSignature, error)

	// GetPendingByFlightID returns the flight's current pending request, or
	// ErrNotFound if none.
	GetPendingByFlightID(ctx context.Context, flightID uuid.UUID) (*models.FlightSignature, error)

	// ListByFlightID returns the full signature history for a flight, newest
	// first.
	ListByFlightID(ctx context.Context, flightID uuid.UUID) ([]*models.FlightSignature, error)

	// Update persists all mutable fields of a signature row.
	Update(ctx context.Context, sig *models.FlightSignature) error

	// ExpirePendingPastDue flips any 'pending' row whose token_expires_at
	// has passed to 'expired' (soft, keeps audit trail) and returns the
	// number of rows affected. Used by the admin cleanup-tokens sweep.
	ExpirePendingPastDue(ctx context.Context) (int64, error)
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

// EmailVerificationTokenRepository defines the interface for email verification token data access.
type EmailVerificationTokenRepository interface {
	Create(ctx context.Context, token *models.EmailVerificationToken) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*models.EmailVerificationToken, error)
	MarkAsUsed(ctx context.Context, tokenHash string) error
	DeleteForUser(ctx context.Context, userID uuid.UUID) error
	DeleteExpired(ctx context.Context) error
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
	HasBeenSent(ctx context.Context, userID uuid.UUID, notificationType string, referenceID uuid.UUID, daysBeforeExpiry int, expiryReferenceDate *time.Time) (bool, error)
	GetAllUsersWithPreferences(ctx context.Context) ([]*models.NotificationPreferences, error)
	GetNotificationHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*models.NotificationLog, int, error)
}

// ContactRepository defines the interface for contact (reusable people) data access
type ContactRepository interface {
	Create(ctx context.Context, contact *models.Contact) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Contact, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Contact, error)
	GetByExactName(ctx context.Context, userID uuid.UUID, name string) (*models.Contact, error)
	Search(ctx context.Context, userID uuid.UUID, query string, limit int) ([]*models.Contact, error)
	Update(ctx context.Context, contact *models.Contact) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// FlightCrewRepository defines the interface for flight crew member data access
type FlightCrewRepository interface {
	SetCrewMembers(ctx context.Context, flightID uuid.UUID, members []models.FlightCrewMember) error
	GetByFlightID(ctx context.Context, flightID uuid.UUID) ([]models.FlightCrewMember, error)
	GetByFlightIDs(ctx context.Context, flightIDs []uuid.UUID) (map[uuid.UUID][]models.FlightCrewMember, error)
	DeleteByFlightID(ctx context.Context, flightID uuid.UUID) error
}

// WebAuthnCredentialRepository defines the interface for WebAuthn / passkey
// credential data access.
type WebAuthnCredentialRepository interface {
	Create(ctx context.Context, credential *models.WebAuthnCredential) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.WebAuthnCredential, error)
	GetByCredentialID(ctx context.Context, credentialID []byte) (*models.WebAuthnCredential, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.WebAuthnCredential, error)
	UpdateSignCount(ctx context.Context, id uuid.UUID, signCount uint32, lastUsedAt time.Time) error
	Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID) error
}

// WebAuthnSessionRepository defines the interface for transient WebAuthn
// ceremony session storage.
type WebAuthnSessionRepository interface {
	Create(ctx context.Context, session *models.WebAuthnSession) error
	Get(ctx context.Context, id uuid.UUID) (*models.WebAuthnSession, error)
	Delete(ctx context.Context, id uuid.UUID) error
	DeleteExpired(ctx context.Context) error
}

// BackupDestinationRepository persists per-user cloud backup destination
// configurations (provider + encrypted credentials + schedule + status).
type BackupDestinationRepository interface {
	Create(ctx context.Context, dest *models.BackupDestination) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.BackupDestination, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.BackupDestination, error)
	Update(ctx context.Context, dest *models.BackupDestination) error
	Delete(ctx context.Context, id uuid.UUID) error
	// ListDueForRun returns enabled, active destinations whose schedule
	// indicates they are due to run at the supplied wall-clock time.
	ListDueForRun(ctx context.Context, now time.Time) ([]*models.BackupDestination, error)
}

// BackupRunRepository persists immutable audit records for each backup run
// (one per attempt — success, skipped, or failed).
type BackupRunRepository interface {
	Create(ctx context.Context, run *models.BackupRun) error
	Update(ctx context.Context, run *models.BackupRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.BackupRun, error)
	// GetByDestinationID returns the requested page of runs newest-first
	// along with the total row count.
	GetByDestinationID(ctx context.Context, destinationID uuid.UUID, limit, offset int) ([]*models.BackupRun, int, error)
	DeleteByDestinationID(ctx context.Context, destinationID uuid.UUID) error
}

// FlightBaselineRepository defines the interface for the per-user "initial
// hours snapshot" data access.
type FlightBaselineRepository interface {
	// Get returns the baseline for the given user, or repository.ErrNotFound.
	Get(ctx context.Context, userID uuid.UUID) (*models.FlightBaseline, error)

	// Upsert inserts or updates the baseline for a user. The user_id field on
	// the model must be set; created_at / updated_at are populated by the DB.
	Upsert(ctx context.Context, baseline *models.FlightBaseline) error

	// Delete removes the baseline for the given user. Returns ErrNotFound when
	// no baseline existed.
	Delete(ctx context.Context, userID uuid.UUID) error
}
