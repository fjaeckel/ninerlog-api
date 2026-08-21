package postgres

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type adminRepository struct {
	db *sql.DB
}

// NewAdminRepository creates the repository behind the admin console.
func NewAdminRepository(db *sql.DB) repository.AdminRepository {
	return &adminRepository{db: db}
}

// scanCount scans a single count value from a query row, logging and
// defaulting to 0 on error.
func (r *adminRepository) scanCount(row *sql.Row, dest *int) {
	if err := row.Scan(dest); err != nil {
		slog.Error("admin stats: count query failed", "error", err)
		*dest = 0
	}
}

func (r *adminRepository) GetStats(ctx context.Context, now time.Time) (*repository.AdminStats, error) {
	stats := &repository.AdminStats{
		ImportsByFormat:              map[string]int{},
		BackupDestinationsByProvider: map[string]int{},
	}

	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users"), &stats.TotalUsers)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM flights"), &stats.TotalFlights)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM aircraft"), &stats.TotalAircraft)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM contacts"), &stats.TotalContacts)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM credentials"), &stats.TotalCredentials)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM flight_imports"), &stats.TotalImports)

	// Flights this month
	monthStart := now.Format("2006-01") + "-01"
	r.scanCount(r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM flights WHERE created_at >= $1", monthStart,
	), &stats.FlightsThisMonth)

	// New users this week
	weekAgo := now.AddDate(0, 0, -7)
	r.scanCount(r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM users WHERE created_at >= $1", weekAgo,
	), &stats.NewUsersThisWeek)

	// Locked accounts (locked_until in the future)
	r.scanCount(r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM users WHERE locked_until IS NOT NULL AND locked_until > $1", now,
	), &stats.LockedAccounts)

	// Disabled accounts
	r.scanCount(r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM users WHERE disabled = true",
	), &stats.DisabledAccounts)

	// Live sessions across all users.
	r.scanCount(r.db.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT session_id) FROM refresh_tokens WHERE revoked = false AND expires_at > $1", now,
	), &stats.ActiveSessions)

	// Imports grouped by source format.
	if formatRows, err := r.db.QueryContext(ctx,
		"SELECT import_format::text, COUNT(*) FROM flight_imports GROUP BY import_format",
	); err != nil {
		slog.Error("admin stats: flight_imports by format query failed", "error", err)
	} else {
		defer formatRows.Close()
		for formatRows.Next() {
			var format string
			var count int
			if err := formatRows.Scan(&format, &count); err != nil {
				slog.Error("admin stats: flight_imports by format scan failed", "error", err)
				continue
			}
			stats.ImportsByFormat[format] = count
		}
	}

	// Cloud backup destinations: breakdown by provider.
	rows, err := r.db.QueryContext(ctx,
		"SELECT provider, COUNT(*) FROM backup_destinations GROUP BY provider")
	if err != nil {
		slog.Error("admin stats: backup_destinations query failed", "error", err)
		return stats, nil
	}
	defer rows.Close()
	for rows.Next() {
		var provider string
		var count int
		if err := rows.Scan(&provider, &count); err != nil {
			slog.Error("admin stats: backup_destinations scan failed", "error", err)
			continue
		}
		stats.BackupDestinationsByProvider[provider] = count
	}

	return stats, nil
}

func (r *adminRepository) MigrationVersion(ctx context.Context) (int, error) {
	var version int
	err := r.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE dirty = false",
	).Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

func (r *adminRepository) CountAuditLog(ctx context.Context) (int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM admin_audit_log").Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

func (r *adminRepository) ListAuditLog(ctx context.Context, limit, offset int) ([]*repository.AdminAuditLogEntry, error) {
	// LEFT JOIN keeps audit rows whose users have been deleted.
	rows, err := r.db.QueryContext(ctx, `
		SELECT al.id, al.admin_user_id, au.email, au.name,
		       al.action, al.target_user_id, tu.email, tu.name,
		       al.details, al.created_at
		FROM admin_audit_log al
		LEFT JOIN users au ON au.id = al.admin_user_id
		LEFT JOIN users tu ON tu.id = al.target_user_id
		ORDER BY al.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*repository.AdminAuditLogEntry
	for rows.Next() {
		e := &repository.AdminAuditLogEntry{}
		if err := rows.Scan(&e.ID, &e.AdminUserID, &e.AdminEmail, &e.AdminName,
			&e.Action, &e.TargetUserID, &e.TargetUserEmail, &e.TargetUserName,
			&e.Details, &e.CreatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *adminRepository) InsertAuditLog(ctx context.Context, entry *repository.AdminAuditLogEntry) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO admin_audit_log (id, admin_user_id, action, target_user_id, details, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, entry.ID, entry.AdminUserID, entry.Action, entry.TargetUserID, string(entry.Details), entry.CreatedAt)
	return err
}

func (r *adminRepository) ListUsers(ctx context.Context, search string, limit, offset int) ([]*repository.AdminUserRow, int, error) {
	countQuery := "SELECT COUNT(*) FROM users"
	dataQuery := `
		SELECT u.id, u.email, u.name, u.created_at, u.last_login_at,
		       u.email_verified, u.two_factor_enabled, u.disabled, u.failed_login_attempts,
		       u.locked_until, u.verification_reminder_sent_at,
		       EXISTS (SELECT 1 FROM email_suppressions es WHERE es.email = u.email) as email_suppressed,
		       (SELECT COUNT(*) FROM flights WHERE user_id = u.id) as flight_count,
		       (SELECT COUNT(*) FROM aircraft WHERE user_id = u.id) as aircraft_count
		FROM users u
	`
	var countArgs, dataArgs []interface{}
	if search != "" {
		searchClause := " WHERE LOWER(email) LIKE LOWER($1) OR LOWER(name) LIKE LOWER($1)"
		pattern := "%" + search + "%"
		countQuery += searchClause
		dataQuery += searchClause
		countArgs = append(countArgs, pattern)
		dataArgs = append(dataArgs, pattern)
		dataQuery += " ORDER BY u.created_at DESC LIMIT $2 OFFSET $3"
	} else {
		dataQuery += " ORDER BY u.created_at DESC LIMIT $1 OFFSET $2"
	}
	dataArgs = append(dataArgs, limit, offset)

	var total int
	r.scanCount(r.db.QueryRowContext(ctx, countQuery, countArgs...), &total)

	rows, err := r.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*repository.AdminUserRow
	for rows.Next() {
		u := &repository.AdminUserRow{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt, &u.LastLoginAt,
			&u.EmailVerified, &u.TwoFactorEnabled, &u.Disabled, &u.FailedLoginAttempts,
			&u.LockedUntil, &u.VerificationReminderSentAt, &u.EmailSuppressed,
			&u.FlightCount, &u.AircraftCount); err != nil {
			continue
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (r *adminRepository) UnlockUser(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE users SET failed_login_attempts = 0, locked_until = NULL, updated_at = $1 WHERE id = $2",
		time.Now(), userID)
	return err
}

func (r *adminRepository) DeleteExpiredRefreshTokens(ctx context.Context, now time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM refresh_tokens WHERE expires_at < $1 OR revoked = true", now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *adminRepository) DeleteExpiredPasswordResetTokens(ctx context.Context, now time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM password_reset_tokens WHERE expires_at < $1 OR used = true", now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
