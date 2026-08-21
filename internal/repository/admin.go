package repository

import (
	"context"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// AdminStats aggregates the instance-wide counters shown on the admin
// dashboard. A counter whose query fails is reported as zero.
type AdminStats struct {
	TotalUsers             int
	TotalFlights           int
	TotalSimulatorSessions int
	TotalAircraft          int
	TotalContacts          int
	TotalCredentials       int
	TotalImports           int
	FlightsThisMonth       int
	NewUsersThisWeek       int
	LockedAccounts         int
	DisabledAccounts       int
	// ActiveSessions counts live sessions across all users.
	ActiveSessions int
	// ImportsByFormat maps import_format value to import count. Never nil;
	// formats with no imports are absent.
	ImportsByFormat map[string]int
	// BackupDestinationsByProvider maps provider name to destination count.
	// Never nil.
	BackupDestinationsByProvider map[string]int
}

// AdminAuditLogEntry is one admin action, joined with the acting and target
// users' current email/name (nil when the account has since been deleted —
// audit rows outlive their users via ON DELETE SET NULL).
type AdminAuditLogEntry struct {
	ID              uuid.UUID
	AdminUserID     uuid.UUID
	AdminEmail      *string
	AdminName       *string
	Action          string
	TargetUserID    *uuid.UUID
	TargetUserEmail *string
	TargetUserName  *string
	// Details is the JSON metadata payload for the action.
	Details   []byte
	CreatedAt time.Time
}

// AdminUserRow is one row of the admin user listing: the account plus the
// per-user content counts and suppression flag the console displays.
type AdminUserRow struct {
	ID                         uuid.UUID
	Email                      string
	Name                       string
	CreatedAt                  time.Time
	LastLoginAt                *time.Time
	EmailVerified              bool
	TwoFactorEnabled           bool
	Disabled                   bool
	FailedLoginAttempts        int
	LockedUntil                *time.Time
	VerificationReminderSentAt *time.Time
	EmailSuppressed            bool
	FlightCount                int
	AircraftCount              int
}

// AdminRepository backs the admin console: instance-wide statistics, the
// audit log, the user listing, and the maintenance sweeps that need
// affected-row counts.
type AdminRepository interface {
	// GetStats returns the dashboard counters, evaluated relative to `now`
	// (month-to-date, trailing week, and lock expiry all derive from it).
	GetStats(ctx context.Context, now time.Time) (*AdminStats, error)

	// MigrationVersion returns the highest cleanly-applied schema migration
	// version, or 0 when none is recorded.
	MigrationVersion(ctx context.Context) (int, error)

	// CountAuditLog counts all audit log entries.
	CountAuditLog(ctx context.Context) (int, error)

	// ListAuditLog returns audit entries newest first.
	ListAuditLog(ctx context.Context, limit, offset int) ([]*AdminAuditLogEntry, error)

	// InsertAuditLog appends one action. Only ID, AdminUserID, Action,
	// TargetUserID, Details and CreatedAt are stored; the joined email/name
	// fields are read-side decoration.
	InsertAuditLog(ctx context.Context, entry *AdminAuditLogEntry) error

	// ListUsers returns one page of the user listing, newest first, plus the
	// total row count for the (optionally searched) set. A non-empty search
	// matches email or name case-insensitively as a substring.
	ListUsers(ctx context.Context, search string, limit, offset int) ([]*AdminUserRow, int, error)

	// UnlockUser resets the failed-login counter and clears any lockout.
	UnlockUser(ctx context.Context, userID uuid.UUID) error

	// DeleteExpiredRefreshTokens removes refresh tokens that are expired or
	// revoked, returning how many rows went.
	DeleteExpiredRefreshTokens(ctx context.Context, now time.Time) (int64, error)

	// DeleteExpiredPasswordResetTokens removes password-reset tokens that are
	// expired or already used, returning how many rows went.
	DeleteExpiredPasswordResetTokens(ctx context.Context, now time.Time) (int64, error)
}

// AnnouncementRepository persists operator-authored system announcements.
type AnnouncementRepository interface {
	// ListActive returns announcements that have not expired as of `now`,
	// newest first.
	ListActive(ctx context.Context, now time.Time) ([]*models.SystemAnnouncement, error)

	// Create stores a new announcement.
	Create(ctx context.Context, a *models.SystemAnnouncement) error

	// Delete removes an announcement. Returns ErrNotFound when no row had
	// this id.
	Delete(ctx context.Context, id uuid.UUID) error
}

// FlightImportRepository persists the per-run import history records.
type FlightImportRepository interface {
	// Create stores one import record (created_at is set by the database).
	Create(ctx context.Context, rec *models.FlightImport) error

	// CountByUserID counts a user's import records.
	CountByUserID(ctx context.Context, userID uuid.UUID) (int, error)

	// ListByUserID returns a user's import records newest first.
	ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*models.FlightImport, error)

	// GetByIDForUser returns one import record scoped to its owner, or
	// ErrNotFound.
	GetByIDForUser(ctx context.Context, id, userID uuid.UUID) (*models.FlightImport, error)

	// DeleteByUserID removes all of a user's import records.
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
}

// UserContentRepository wipes a user's logbook content (flights, crew rows,
// imports, licenses and their class ratings, aircraft, contacts, credentials,
// notification history) in one transaction, keeping the account itself.
type UserContentRepository interface {
	DeleteAllContent(ctx context.Context, userID uuid.UUID) error
}
