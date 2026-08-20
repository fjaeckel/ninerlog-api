package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	if user.PreferredLocale == "" {
		user.PreferredLocale = "en"
	}
	// Match the DB column defaults.
	user.RecencyPerModel = true
	user.FlightListColumnMode = models.FlightListColumnModeAuto
	user.FlightListColumns = pq.StringArray{}

	query := `
		INSERT INTO users (id, email, password_hash, name, email_verified, preferred_locale, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	user.ID = uuid.New()
	_, err := r.db.ExecContext(ctx, query,
		user.ID,
		user.Email,
		user.PasswordHash,
		user.Name,
		user.EmailVerified,
		user.PreferredLocale,
		user.CreatedAt,
		user.UpdatedAt,
	)

	if err != nil {
		if errMsg := err.Error(); errMsg == "pq: duplicate key value violates unique constraint \"users_email_key\"" ||
			errMsg == "pq: duplicate key value violates unique constraint \"users_email_key\" (23505)" {
			return repository.ErrDuplicateEmail
		}
		return err
	}

	return nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, name, email_verified, two_factor_enabled, two_factor_secret, recovery_codes,
		       failed_login_attempts, locked_until, disabled, last_login_at, time_display_format, date_format, decimal_separator, preferred_locale, recency_per_model, recency_per_registration, flight_list_column_mode, flight_list_columns, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	user := &models.User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Name,
		&user.EmailVerified,
		&user.TwoFactorEnabled,
		&user.TwoFactorSecret,
		&user.RecoveryCodes,
		&user.FailedLoginAttempts,
		&user.LockedUntil,
		&user.Disabled,
		&user.LastLoginAt,
		&user.TimeDisplayFormat,
		&user.DateFormat,
		&user.DecimalSeparator,
		&user.PreferredLocale,
		&user.RecencyPerModel,
		&user.RecencyPerRegistration,
		&user.FlightListColumnMode,
		&user.FlightListColumns,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}

	return user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, name, email_verified, two_factor_enabled, two_factor_secret, recovery_codes,
		       failed_login_attempts, locked_until, disabled, last_login_at, time_display_format, date_format, decimal_separator, preferred_locale, recency_per_model, recency_per_registration, flight_list_column_mode, flight_list_columns, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	user := &models.User{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Name,
		&user.EmailVerified,
		&user.TwoFactorEnabled,
		&user.TwoFactorSecret,
		&user.RecoveryCodes,
		&user.FailedLoginAttempts,
		&user.LockedUntil,
		&user.Disabled,
		&user.LastLoginAt,
		&user.TimeDisplayFormat,
		&user.DateFormat,
		&user.DecimalSeparator,
		&user.PreferredLocale,
		&user.RecencyPerModel,
		&user.RecencyPerRegistration,
		&user.FlightListColumnMode,
		&user.FlightListColumns,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, err
	}

	return user, nil
}

func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users
		SET email = $1, password_hash = $2, name = $3, two_factor_enabled = $4,
		    two_factor_secret = $5, recovery_codes = $6, disabled = $7,
		    last_login_at = $8, time_display_format = $9, date_format = $10, decimal_separator = $11, preferred_locale = $12,
		    recency_per_model = $13, recency_per_registration = $14, flight_list_column_mode = $15,
		    flight_list_columns = $16, email_verified = $17, updated_at = $18
		WHERE id = $19
	`

	// Both columns are NOT NULL; zero values are replaced with defaults.
	columnMode := user.FlightListColumnMode
	if columnMode == "" {
		columnMode = models.FlightListColumnModeAuto
	}
	columns := user.FlightListColumns
	if columns == nil {
		columns = pq.StringArray{}
	}

	result, err := r.db.ExecContext(ctx, query,
		user.Email,
		user.PasswordHash,
		user.Name,
		user.TwoFactorEnabled,
		user.TwoFactorSecret,
		user.RecoveryCodes,
		user.Disabled,
		user.LastLoginAt,
		user.TimeDisplayFormat,
		user.DateFormat,
		user.DecimalSeparator,
		user.PreferredLocale,
		user.RecencyPerModel,
		user.RecencyPerRegistration,
		columnMode,
		columns,
		user.EmailVerified,
		user.UpdatedAt,
		user.ID,
	)

	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") && strings.Contains(err.Error(), "email") {
			return repository.ErrDuplicateEmail
		}
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return repository.ErrNotFound
	}

	return nil
}

func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM users WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return repository.ErrNotFound
	}

	return nil
}

func (r *UserRepository) IncrementFailedLoginAttempts(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE users
		SET failed_login_attempts = failed_login_attempts + 1, updated_at = $1
		WHERE id = $2
	`
	_, err := r.db.ExecContext(ctx, query, time.Now(), id)
	return err
}

func (r *UserRepository) ResetFailedLoginAttempts(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE users
		SET failed_login_attempts = 0, locked_until = NULL, updated_at = $1
		WHERE id = $2
	`
	_, err := r.db.ExecContext(ctx, query, time.Now(), id)
	return err
}

// UpdateLastLogin stamps the moment a session was handed out. A missing row
// is not an error.
func (r *UserRepository) UpdateLastLogin(ctx context.Context, id uuid.UUID, at time.Time) error {
	query := `
		UPDATE users
		SET last_login_at = $1, updated_at = $1
		WHERE id = $2
	`
	_, err := r.db.ExecContext(ctx, query, at, id)
	return err
}

func (r *UserRepository) LockAccount(ctx context.Context, id uuid.UUID, until time.Time) error {
	query := `
		UPDATE users
		SET locked_until = $1, updated_at = $2
		WHERE id = $3
	`
	_, err := r.db.ExecContext(ctx, query, until, time.Now(), id)
	return err
}

// ConsumeRecoveryCode atomically removes one recovery code hash from the user's
// list, returning true only if this call was the one that removed it.
func (r *UserRepository) ConsumeRecoveryCode(ctx context.Context, id uuid.UUID, codeHash string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users
		 SET recovery_codes = array_remove(recovery_codes, $1), updated_at = NOW()
		 WHERE id = $2 AND $1 = ANY(recovery_codes)`,
		codeHash, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *UserRepository) MarkEmailVerified(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE users
		SET email_verified = TRUE, updated_at = $1
		WHERE id = $2
	`
	result, err := r.db.ExecContext(ctx, query, time.Now(), id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// unverifiedAccountGuards restricts every reaper query to unverified accounts
// that have never logged in and have no OIDC identity.
const unverifiedAccountGuards = `
	email_verified = FALSE
	AND last_login_at IS NULL
	AND NOT EXISTS (SELECT 1 FROM oidc_identities oi WHERE oi.user_id = users.id)
`

func (r *UserRepository) ListUnverifiedForReminder(ctx context.Context, createdBefore time.Time, limit int) ([]*models.User, error) {
	query := `
		SELECT id, email, name, preferred_locale, created_at
		FROM users
		WHERE ` + unverifiedAccountGuards + `
		  AND verification_reminder_sent_at IS NULL
		  AND created_at < $1
		ORDER BY created_at
		LIMIT $2
	`

	rows, err := r.db.QueryContext(ctx, query, createdBefore, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	users := []*models.User{}
	for rows.Next() {
		u := &models.User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.PreferredLocale, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *UserRepository) MarkVerificationReminderSent(ctx context.Context, id uuid.UUID, at time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET verification_reminder_sent_at = $1
		WHERE id = $2 AND verification_reminder_sent_at IS NULL
	`, at, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *UserRepository) DeleteUnverifiedRemindedBefore(ctx context.Context, remindedBefore time.Time) (int64, error) {
	// Dependent rows are removed via ON DELETE CASCADE.
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM users
		WHERE `+unverifiedAccountGuards+`
		  AND verification_reminder_sent_at IS NOT NULL
		  AND verification_reminder_sent_at < $1
	`, remindedBefore)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
