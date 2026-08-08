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
	// ConsumeRecoveryCode atomically removes a recovery code hash, returning
	// true only for the caller that actually removed it. Prevents the same
	// single-use code authenticating more than once under concurrency.
	ConsumeRecoveryCode(ctx context.Context, id uuid.UUID, codeHash string) (bool, error)
	MarkEmailVerified(ctx context.Context, id uuid.UUID) error

	// ListUnverifiedForReminder returns accounts that are still unverified,
	// were created before `createdBefore`, and have not yet been sent the
	// follow-up verification reminder.
	//
	// Accounts that have ever logged in and accounts linked to an OIDC identity
	// are excluded by the implementation: neither is a dead signup, and neither
	// is reachable by our verification email.
	ListUnverifiedForReminder(ctx context.Context, createdBefore time.Time, limit int) ([]*models.User, error)

	// MarkVerificationReminderSent stamps the reminder time, which is what
	// starts the deletion clock.
	MarkVerificationReminderSent(ctx context.Context, id uuid.UUID, at time.Time) error

	// DeleteUnverifiedRemindedBefore deletes still-unverified accounts whose
	// reminder was sent before the given instant, and reports how many went.
	// It applies the same never-logged-in and non-OIDC guards as the listing.
	DeleteUnverifiedRemindedBefore(ctx context.Context, remindedBefore time.Time) (int64, error)
}

// EmailDeliveryRepository stores what SMTP said about each send, and the set of
// addresses that have been given up on.
type EmailDeliveryRepository interface {
	// RecordEvent appends one attempt to the delivery log.
	RecordEvent(ctx context.Context, event *models.EmailDeliveryEvent) error

	// ListEvents returns the most recent events, newest first, optionally
	// narrowed to one recipient address.
	ListEvents(ctx context.Context, recipient string, limit int) ([]*models.EmailDeliveryEvent, error)

	// IsSuppressed reports whether the address is on the suppression list.
	IsSuppressed(ctx context.Context, email string) (bool, error)

	// Suppress adds or updates a suppression, bumping the bounce count when the
	// address is already listed.
	Suppress(ctx context.Context, email, reason string, smtpCode *int, detail string) error

	// Unsuppress removes an address from the suppression list, so mail to it is
	// attempted again. Returns ErrNotFound when the address was not listed.
	Unsuppress(ctx context.Context, email string) error

	// ListSuppressions returns suppressed addresses, most recently bounced first.
	ListSuppressions(ctx context.Context, limit int) ([]*models.EmailSuppression, error)

	// CountSuppressions reports the size of the suppression list, for metrics.
	CountSuppressions(ctx context.Context) (int, error)

	// UserIDForEmail resolves an address to an account so a delivery event can
	// be attributed. Returns ErrNotFound when no account has that address.
	UserIDForEmail(ctx context.Context, email string) (uuid.UUID, error)
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

	// GetByUserID retrieves all licenses for a user. A non-nil updatedSince
	// narrows the result to licences changed strictly after that instant.
	GetByUserID(ctx context.Context, userID uuid.UUID, updatedSince *time.Time) ([]*models.License, error)

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
	// UpdatedSince restricts the result to flights whose updated_at is
	// strictly after the given instant (delta sync). Compared at full
	// timestamp precision, unlike the day-granular date filters above.
	UpdatedSince *time.Time
	// Query is a parsed advanced search query (see internal/flightsearch).
	// It compiles to a SQL condition and is ANDed with the other filters.
	Query     *flightsearch.Query
	Page      int
	PageSize  int
	SortBy    string // "date", "totalTime", "createdAt"
	SortOrder string // "asc", "desc"

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
	// CountEmailsSentSince returns how many signature-request emails the user
	// has triggered since the given instant, for per-account rate limiting.
	CountEmailsSentSince(ctx context.Context, userID uuid.UUID, since time.Time) (int, error)
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

// CustomCurrencyRuleRepository defines data access for user-authored currency rules.
type CustomCurrencyRuleRepository interface {
	Create(ctx context.Context, rule *models.CustomCurrencyRule) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.CustomCurrencyRule, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.CustomCurrencyRule, error)
	GetByShareToken(ctx context.Context, token string) (*models.CustomCurrencyRule, error)
	Update(ctx context.Context, rule *models.CustomCurrencyRule) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// CredentialRepository defines the interface for credential data access
type CredentialRepository interface {
	Create(ctx context.Context, credential *models.Credential) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Credential, error)
	// GetByUserID retrieves all credentials for a user. A non-nil updatedSince
	// narrows the result to credentials changed strictly after that instant.
	GetByUserID(ctx context.Context, userID uuid.UUID, updatedSince *time.Time) ([]*models.Credential, error)
	Update(ctx context.Context, credential *models.Credential) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type AircraftRepository interface {
	Create(ctx context.Context, aircraft *models.Aircraft) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Aircraft, error)
	// GetByUserID retrieves all aircraft for a user. A non-nil updatedSince
	// narrows the result to aircraft changed strictly after that instant.
	GetByUserID(ctx context.Context, userID uuid.UUID, updatedSince *time.Time) ([]*models.Aircraft, error)
	Update(ctx context.Context, aircraft *models.Aircraft) error
	// UpdateWithFlightRename updates the aircraft and, in the same
	// transaction, repoints flights (and open flight sessions) logged under
	// oldRegistration to the aircraft's new registration. Returns the number
	// of flights updated.
	UpdateWithFlightRename(ctx context.Context, aircraft *models.Aircraft, oldRegistration string) (int, error)
	Delete(ctx context.Context, id uuid.UUID) error
	CountByUserID(ctx context.Context, userID uuid.UUID) (int, error)
	// GetStatsByUserID aggregates flight statistics per aircraft registration
	GetStatsByUserID(ctx context.Context, userID uuid.UUID) ([]*models.AircraftStats, error)
	// GetTypeStatsByUserID aggregates flight statistics per aircraft type
	GetTypeStatsByUserID(ctx context.Context, userID uuid.UUID) ([]*models.AircraftTypeStats, error)
	// GetRecencyRowsByUserID returns per-day landing counts for flights in
	// the preceding 90 days, newest first, for recency derivation
	GetRecencyRowsByUserID(ctx context.Context, userID uuid.UUID) ([]*models.AircraftRecencyRow, error)
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
	// GetByUserID retrieves all contacts for a user. A non-nil updatedSince
	// narrows the result to contacts changed strictly after that instant.
	GetByUserID(ctx context.Context, userID uuid.UUID, updatedSince *time.Time) ([]*models.Contact, error)
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
	// Create stores ceremony state keyed by the SHA-256 of the handle.
	Create(ctx context.Context, session *models.WebAuthnSession) error

	// Consume atomically deletes and returns the session if it exists, matches
	// the ceremony, and has not expired. Returns ErrNotFound otherwise, so that
	// expired, already-consumed, wrong-ceremony and forged handles are
	// indistinguishable to the caller.
	Consume(ctx context.Context, idHash []byte, ceremony string) (*models.WebAuthnSession, error)

	// DeleteOldestForUser keeps the newest `keep` sessions belonging to userID
	// and deletes the rest, bounding how many ceremonies a user can hold open.
	// Returns the number deleted.
	DeleteOldestForUser(ctx context.Context, userID uuid.UUID, keep int) (int64, error)

	// DeleteExpired removes expired rows. Returns the number deleted.
	DeleteExpired(ctx context.Context) (int64, error)
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

// OIDCIdentityRepository persists the mapping between external OpenID Connect
// identities and local users, plus the two kinds of short-lived, single-use
// state a login round trip needs (authorization state and browser handoff
// codes). Only used when OIDC_ISSUER is configured; the tables stay empty
// otherwise.
type OIDCIdentityRepository interface {
	// GetBySubject returns the identity for an (issuer, subject) pair, or
	// ErrNotFound. This is the only supported way to resolve an incoming
	// login to an existing account.
	GetBySubject(ctx context.Context, issuer, subject string) (*models.OIDCIdentity, error)

	// GetByUserID returns the identity linked to a local user, or ErrNotFound.
	GetByUserID(ctx context.Context, userID uuid.UUID) (*models.OIDCIdentity, error)

	// Create links an external identity to an existing user.
	Create(ctx context.Context, identity *models.OIDCIdentity) error

	// TouchLogin records a successful login and refreshes the cached email.
	TouchLogin(ctx context.Context, id uuid.UUID, email string, at time.Time) error

	// CreateLoginState stores the nonce and PKCE verifier for one pending
	// authorization request, keyed by the SHA-256 of the state parameter.
	CreateLoginState(ctx context.Context, state *models.OIDCLoginState) error

	// ConsumeLoginState atomically deletes and returns an unexpired login
	// state. Returns ErrNotFound for expired, already-consumed and forged
	// state values alike, so they stay indistinguishable to the caller.
	ConsumeLoginState(ctx context.Context, stateHash []byte) (*models.OIDCLoginState, error)

	// CreateHandoffCode stores a single-use code the SPA can exchange for a
	// token pair, keyed by the SHA-256 of the code.
	CreateHandoffCode(ctx context.Context, code *models.OIDCHandoffCode) error

	// ConsumeHandoffCode atomically deletes and returns an unexpired handoff
	// code, or ErrNotFound.
	ConsumeHandoffCode(ctx context.Context, codeHash []byte) (*models.OIDCHandoffCode, error)

	// DeleteExpired removes expired login states and handoff codes. Returns
	// the number of rows deleted across both tables.
	DeleteExpired(ctx context.Context) (int64, error)
}

// IdempotencyRepository stores the server-side replay records that make
// mutating requests safe to retry (see db/migrations/000052).
//
// Every method is scoped by user ID: one user's key can never read, claim or
// overwrite another's.
type IdempotencyRepository interface {
	// Claim atomically takes ownership of (rec.UserID, rec.Key) for the
	// current request.
	//
	// It returns (nil, nil) when the claim succeeded, meaning the caller must
	// run the request and then either Complete or Release the record. It
	// returns the live record when the key is already held — either still
	// in progress, or completed and awaiting replay — and the caller must not
	// execute the request.
	//
	// A record that has expired, or whose in-progress claim was taken before
	// staleBefore (its owner died without finalizing), is taken over rather
	// than reported as a conflict.
	Claim(ctx context.Context, rec *models.IdempotencyRecord, staleBefore time.Time) (*models.IdempotencyRecord, error)

	// Complete finalizes a claimed record with the captured response. It is a
	// no-op when the claim identified by rec.CreatedAt is no longer held, so a
	// straggler cannot overwrite a record another request has since taken over.
	Complete(ctx context.Context, rec *models.IdempotencyRecord) error

	// Release drops a claim so the key can be used again, for requests that
	// must stay retryable (server errors). Like Complete it only acts on a
	// claim still owned by claimedAt.
	Release(ctx context.Context, userID uuid.UUID, key string, claimedAt time.Time) error

	// DeleteExpired removes records that expired at or before `before`.
	// Returns the number deleted.
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}

// DeletionRepository reads the tombstones that make deletions visible to a
// delta-syncing client. Rows are written by database triggers, never from Go,
// so that a delete reaching the database by any route — repository, raw SQL, or
// ON DELETE CASCADE — is recorded. There is deliberately no Create method.
type DeletionRepository interface {
	// ListSince returns the user's deletions strictly after `since`, oldest
	// first, optionally narrowed to one entity type.
	ListSince(ctx context.Context, userID uuid.UUID, since time.Time, entity *models.DeletionEntityType, limit, offset int) ([]*models.Deletion, error)

	// CountSince counts the set ListSince pages over.
	CountSince(ctx context.Context, userID uuid.UUID, since time.Time, entity *models.DeletionEntityType) (int, error)

	// DeleteExpired sweeps tombstones older than `before`, bounding retention.
	// Returns the number deleted.
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}
