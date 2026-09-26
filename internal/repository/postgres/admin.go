package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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
		PilotProfileOverrides:        map[string]map[string]int{},
		LicencePrivilegesByKind:      map[string]int{},
	}

	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users"), &stats.TotalUsers)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM flights WHERE NOT is_simulator AND NOT is_passenger"), &stats.TotalFlights)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM flights WHERE is_simulator"), &stats.TotalSimulatorSessions)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM flights WHERE is_passenger"), &stats.TotalPassengerFlights)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM aircraft"), &stats.TotalAircraft)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM contacts"), &stats.TotalContacts)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM credentials"), &stats.TotalCredentials)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM flight_imports"), &stats.TotalImports)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM custom_reports"), &stats.TotalCustomReports)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE strict_logbook"), &stats.StrictLogbookUsers)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM flight_revisions"), &stats.FlightRevisions)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM flight_revisions WHERE is_correction"), &stats.FlightCorrections)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM flight_revisions WHERE kind = 'delete'"), &stats.VoidedFlights)
	r.scanCount(r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM logbook_certifications"), &stats.LogbookCertifications)
	r.scanGroupedCounts(ctx, "SELECT kind, COUNT(*) FROM licence_privileges GROUP BY kind", stats.LicencePrivilegesByKind)
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM flight_files",
	).Scan(&stats.FlightFiles, &stats.FlightFileBytes); err != nil {
		slog.Error("admin stats: flight_files query failed", "error", err)
		stats.FlightFiles, stats.FlightFileBytes = 0, 0
	}

	// Flights this month
	monthStart := now.Format("2006-01") + "-01"
	r.scanCount(r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM flights WHERE NOT is_simulator AND NOT is_passenger AND created_at >= $1", monthStart,
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
		defer func() { _ = formatRows.Close() }()
		for formatRows.Next() {
			var format string
			var count int
			if err := formatRows.Scan(&format, &count); err != nil {
				slog.Error("admin stats: flight_imports by format scan failed", "error", err)
				continue
			}
			stats.ImportsByFormat[format] = count
		}
		if err := formatRows.Err(); err != nil {
			slog.Error("admin stats: flight_imports by format iteration failed", "error", err)
		}
	}

	r.scanCount(r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pilot_profiles WHERE mode = 'everything'",
	), &stats.PilotProfilesEverythingMode)
	r.scanPilotProfileOverrides(ctx, stats.PilotProfileOverrides)

	// Cloud backup destinations: breakdown by provider.
	rows, err := r.db.QueryContext(ctx,
		"SELECT provider, COUNT(*) FROM backup_destinations GROUP BY provider")
	if err != nil {
		slog.Error("admin stats: backup_destinations query failed", "error", err)
		return stats, nil
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var provider string
		var count int
		if err := rows.Scan(&provider, &count); err != nil {
			slog.Error("admin stats: backup_destinations scan failed", "error", err)
			continue
		}
		stats.BackupDestinationsByProvider[provider] = count
	}
	if err := rows.Err(); err != nil {
		slog.Error("admin stats: backup_destinations iteration failed", "error", err)
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
	defer func() { _ = rows.Close() }()

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
	return insertAuditLog(ctx, r.db, entry)
}

func insertAuditLog(ctx context.Context, db execer, entry *repository.AdminAuditLogEntry) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO admin_audit_log (id, admin_user_id, action, target_user_id, details, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, entry.ID, entry.AdminUserID, entry.Action, entry.TargetUserID, string(entry.Details), entry.CreatedAt)
	return err
}

func (r *adminRepository) ListUsers(ctx context.Context, filter repository.AdminUserFilter, limit, offset int) ([]*repository.AdminUserRow, int, error) {
	var where []string
	var args []interface{}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		where = append(where, fmt.Sprintf("(LOWER(u.email) LIKE LOWER($%d) OR LOWER(u.name) LIKE LOWER($%d))", len(args), len(args)))
	}
	if filter.IsAdmin != nil {
		args = append(args, filter.BootstrapEmail)
		n := len(args)
		effective := fmt.Sprintf("(u.email_verified AND NOT u.disabled AND (u.is_admin OR ($%d <> '' AND LOWER(u.email) = LOWER($%d))))", n, n)
		if *filter.IsAdmin {
			where = append(where, effective)
		} else {
			where = append(where, "NOT "+effective)
		}
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = " WHERE " + strings.Join(where, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM users u" + whereClause
	dataQuery := `
		SELECT u.id, u.email, u.name, u.created_at, u.last_login_at, u.last_active_at,
		       u.email_verified, u.two_factor_enabled, u.disabled, u.is_admin, u.failed_login_attempts,
		       u.locked_until, u.verification_reminder_sent_at,
		       EXISTS (SELECT 1 FROM email_suppressions es WHERE es.email = u.email) as email_suppressed,
		       (SELECT COUNT(*) FROM flights WHERE user_id = u.id) as flight_count,
		       (SELECT COUNT(*) FROM aircraft WHERE user_id = u.id) as aircraft_count
		FROM users u` + whereClause +
		fmt.Sprintf(" ORDER BY u.created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	dataArgs := append(append([]interface{}{}, args...), limit, offset)

	var total int
	r.scanCount(r.db.QueryRowContext(ctx, countQuery, args...), &total)

	rows, err := r.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	var users []*repository.AdminUserRow
	for rows.Next() {
		u := &repository.AdminUserRow{}
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt, &u.LastLoginAt, &u.LastActiveAt,
			&u.EmailVerified, &u.TwoFactorEnabled, &u.Disabled, &u.IsAdmin, &u.FailedLoginAttempts,
			&u.LockedUntil, &u.VerificationReminderSentAt, &u.EmailSuppressed,
			&u.FlightCount, &u.AircraftCount); err != nil {
			continue
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

const activeAdminClause = `email_verified AND NOT disabled AND (is_admin OR ($1 <> '' AND LOWER(email) = LOWER($1)))`

func (r *adminRepository) CountAdmins(ctx context.Context, bootstrapEmail string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM users WHERE "+activeAdminClause, bootstrapEmail).Scan(&n)
	return n, err
}

func (r *adminRepository) SetAdmin(ctx context.Context, userID uuid.UUID) (repository.GrantResult, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, `
		UPDATE users SET is_admin = true, updated_at = $1
		WHERE id = $2 AND email_verified AND NOT disabled AND NOT is_admin
		RETURNING id`, time.Now(), userID).Scan(&id)
	if err == nil {
		return repository.GrantApplied, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var verified, disabled, isAdmin bool
	err = r.db.QueryRowContext(ctx,
		"SELECT email_verified, disabled, is_admin FROM users WHERE id = $1", userID,
	).Scan(&verified, &disabled, &isAdmin)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return repository.GrantNotFound, nil
	case err != nil:
		return 0, err
	case isAdmin:
		return repository.GrantAlreadyAdmin, nil
	case !verified:
		return repository.GrantUnverified, nil
	default:
		return repository.GrantDisabled, nil
	}
}

func (r *adminRepository) RevokeAdmin(ctx context.Context, userID uuid.UUID, bootstrapEmail string) (repository.AdminChange, error) {
	return r.guardedWrite(ctx, userID, bootstrapEmail, true, func(tx *sql.Tx, target adminRow) error {
		if !target.isAdmin {
			return nil
		}
		_, err := tx.ExecContext(ctx,
			"UPDATE users SET is_admin = false, updated_at = $1 WHERE id = $2", time.Now(), userID)
		return err
	})
}

func (r *adminRepository) DisableUser(ctx context.Context, userID uuid.UUID, bootstrapEmail string) (repository.AdminChange, error) {
	return r.guardedWrite(ctx, userID, bootstrapEmail, true, func(tx *sql.Tx, _ adminRow) error {
		_, err := tx.ExecContext(ctx,
			"UPDATE users SET disabled = true, is_admin = false, updated_at = $1 WHERE id = $2", time.Now(), userID)
		return err
	})
}

func (r *adminRepository) DeleteUser(ctx context.Context, userID uuid.UUID, bootstrapEmail string, audit func(flagCleared bool) []*repository.AdminAuditLogEntry) (repository.AdminChange, error) {
	return r.guardedWrite(ctx, userID, bootstrapEmail, true, func(tx *sql.Tx, target adminRow) error {
		for _, e := range audit(target.isAdmin) {
			if err := insertAuditLog(ctx, tx, e); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = $1", userID)
		return err
	})
}

func (r *adminRepository) ChangeEmail(ctx context.Context, userID uuid.UUID, email, bootstrapEmail string) (repository.AdminChange, error) {
	change, err := r.guardedWrite(ctx, userID, bootstrapEmail, false, func(tx *sql.Tx, _ adminRow) error {
		_, err := tx.ExecContext(ctx,
			"UPDATE users SET email = $1, email_verified = false, updated_at = $2 WHERE id = $3", email, time.Now(), userID)
		return err
	})
	if err != nil && strings.Contains(err.Error(), "unique constraint") && strings.Contains(err.Error(), "email") {
		return repository.AdminChange{}, repository.ErrDuplicateEmail
	}
	change.FlagCleared = false
	return change, err
}

// adminRow is one row locked by guardedWrite.
type adminRow struct {
	id                          uuid.UUID
	email                       string
	verified, disabled, isAdmin bool
}

func (a adminRow) active(bootstrapEmail string) bool {
	return a.verified && !a.disabled && (a.isAdmin || a.bootstrap(bootstrapEmail))
}

func (a adminRow) bootstrap(bootstrapEmail string) bool {
	return a.verified && bootstrapEmail != "" && strings.EqualFold(a.email, bootstrapEmail)
}

// guardedWrite locks the target and every admin row in id order, applies
// the guards described on AdminRepository, then runs write in the same
// transaction.
func (r *adminRepository) guardedWrite(ctx context.Context, userID uuid.UUID, bootstrapEmail string, refuseBootstrap bool, write func(tx *sql.Tx, target adminRow) error) (repository.AdminChange, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return repository.AdminChange{}, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := lockAdminRows(ctx, tx, `
		SELECT id, email, email_verified, disabled, is_admin FROM users
		WHERE id = $2 OR is_admin OR ($1 <> '' AND LOWER(email) = LOWER($1))
		ORDER BY id FOR UPDATE`, bootstrapEmail, userID)
	if err != nil {
		return repository.AdminChange{}, err
	}
	var target *adminRow
	othersActive := false
	for i := range rows {
		if rows[i].id == userID {
			target = &rows[i]
		} else if rows[i].active(bootstrapEmail) {
			othersActive = true
		}
	}
	switch {
	case target == nil:
		return repository.AdminChange{Guard: repository.AdminGuardNotFound}, nil
	case refuseBootstrap && target.bootstrap(bootstrapEmail):
		return repository.AdminChange{Guard: repository.AdminGuardBootstrap}, nil
	case target.active(bootstrapEmail) && !othersActive:
		return repository.AdminChange{Guard: repository.AdminGuardLastAdmin}, nil
	}

	if err := write(tx, *target); err != nil {
		return repository.AdminChange{}, err
	}
	if err := tx.Commit(); err != nil {
		return repository.AdminChange{}, err
	}
	return repository.AdminChange{Guard: repository.AdminGuardApplied, FlagCleared: target.isAdmin}, nil
}

func lockAdminRows(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]adminRow, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []adminRow
	for rows.Next() {
		var a adminRow
		if err := rows.Scan(&a.id, &a.email, &a.verified, &a.disabled, &a.isAdmin); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *adminRepository) PromoteVerifiedByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, `
		UPDATE users SET is_admin = true, updated_at = $1
		WHERE LOWER(email) = LOWER($2) AND email_verified AND NOT disabled AND NOT is_admin
		RETURNING id`, time.Now(), email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, nil
	}
	return id, err
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

// scanPilotProfileOverrides fills overrides with explicit intent counts per discipline.
func (r *adminRepository) scanPilotProfileOverrides(ctx context.Context, overrides map[string]map[string]int) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT d.key, d.value->>'intent', COUNT(*)
		FROM pilot_profiles p, jsonb_each(p.disciplines) d
		WHERE d.value->>'intent' IN ('on', 'off', 'goal')
		GROUP BY 1, 2`)
	if err != nil {
		slog.Error("admin stats: pilot_profiles overrides query failed", "error", err)
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var discipline, intent string
		var count int
		if err := rows.Scan(&discipline, &intent, &count); err != nil {
			slog.Error("admin stats: pilot_profiles overrides scan failed", "error", err)
			continue
		}
		if overrides[discipline] == nil {
			overrides[discipline] = map[string]int{}
		}
		overrides[discipline][intent] = count
	}
	if err := rows.Err(); err != nil {
		slog.Error("admin stats: pilot_profiles overrides rows failed", "error", err)
	}
}

// scanGroupedCounts fills dest from a (key, count) query, logging and leaving
// dest partial on error.
func (r *adminRepository) scanGroupedCounts(ctx context.Context, query string, dest map[string]int) {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		slog.Error("admin stats: grouped count query failed", "error", err)
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			slog.Error("admin stats: grouped count scan failed", "error", err)
			continue
		}
		dest[key] = count
	}
	if err := rows.Err(); err != nil {
		slog.Error("admin stats: grouped count rows failed", "error", err)
	}
}
