package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type backupDestinationRepository struct {
	db *sql.DB
}

// NewBackupDestinationRepository returns a Postgres-backed repository for
// cloud backup destinations.
func NewBackupDestinationRepository(db *sql.DB) repository.BackupDestinationRepository {
	return &backupDestinationRepository{db: db}
}

const backupDestinationColumns = `
	id, user_id, provider, display_name, config, credential_hint,
	credentials_enc, credentials_nonce,
	schedule, schedule_hour_utc, schedule_day_of_week, schedule_day_of_month,
	retention_count, status, last_error, enabled, consecutive_failures,
	last_run_at, last_success_at, last_success_sha256,
	created_at, updated_at
`

func (r *backupDestinationRepository) Create(ctx context.Context, dest *models.BackupDestination) error {
	configJSON, err := marshalConfig(dest.Config)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO backup_destinations (
			user_id, provider, display_name, config, credential_hint,
			credentials_enc, credentials_nonce,
			schedule, schedule_hour_utc, schedule_day_of_week, schedule_day_of_month,
			retention_count, status, last_error, enabled, consecutive_failures,
			last_run_at, last_success_at, last_success_sha256
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		dest.UserID,
		dest.Provider,
		dest.DisplayName,
		configJSON,
		dest.CredentialHint,
		dest.CredentialsEnc,
		dest.CredentialsNonce,
		string(dest.Schedule),
		dest.ScheduleHourUTC,
		dest.ScheduleDayOfWeek,
		dest.ScheduleDayOfMonth,
		dest.RetentionCount,
		string(dest.Status),
		dest.LastError,
		dest.Enabled,
		dest.ConsecutiveFailures,
		dest.LastRunAt,
		dest.LastSuccessAt,
		dest.LastSuccessSHA256,
	).Scan(&dest.ID, &dest.CreatedAt, &dest.UpdatedAt)
}

func (r *backupDestinationRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.BackupDestination, error) {
	query := `SELECT ` + backupDestinationColumns + ` FROM backup_destinations WHERE id = $1`
	row := r.db.QueryRowContext(ctx, query, id)
	dest, err := scanBackupDestination(row)
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	return dest, err
}

func (r *backupDestinationRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.BackupDestination, error) {
	query := `SELECT ` + backupDestinationColumns + ` FROM backup_destinations WHERE user_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.BackupDestination
	for rows.Next() {
		d, err := scanBackupDestination(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *backupDestinationRepository) Update(ctx context.Context, dest *models.BackupDestination) error {
	configJSON, err := marshalConfig(dest.Config)
	if err != nil {
		return err
	}

	query := `
		UPDATE backup_destinations SET
			display_name = $1,
			config = $2,
			credential_hint = $3,
			credentials_enc = $4,
			credentials_nonce = $5,
			schedule = $6,
			schedule_hour_utc = $7,
			schedule_day_of_week = $8,
			schedule_day_of_month = $9,
			retention_count = $10,
			status = $11,
			last_error = $12,
			enabled = $13,
			consecutive_failures = $14,
			last_run_at = $15,
			last_success_at = $16,
			last_success_sha256 = $17,
			updated_at = $18
		WHERE id = $19
	`
	result, err := r.db.ExecContext(ctx, query,
		dest.DisplayName,
		configJSON,
		dest.CredentialHint,
		dest.CredentialsEnc,
		dest.CredentialsNonce,
		string(dest.Schedule),
		dest.ScheduleHourUTC,
		dest.ScheduleDayOfWeek,
		dest.ScheduleDayOfMonth,
		dest.RetentionCount,
		string(dest.Status),
		dest.LastError,
		dest.Enabled,
		dest.ConsecutiveFailures,
		dest.LastRunAt,
		dest.LastSuccessAt,
		dest.LastSuccessSHA256,
		time.Now(),
		dest.ID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *backupDestinationRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM backup_destinations WHERE id = $1", id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// ListDueForRun returns enabled, non-paused destinations whose schedule says
// they are due to run at or before `now`. The schedule check is intentionally
// generous: any destination whose schedule_hour_utc matches the current UTC
// hour AND whose previous successful run was on an earlier calendar day (for
// daily), week (for weekly), or month (for monthly) is returned. The service
// layer is responsible for at-most-once-per-window semantics via
// last_success_at.
func (r *backupDestinationRepository) ListDueForRun(ctx context.Context, now time.Time) ([]*models.BackupDestination, error) {
	now = now.UTC()
	query := `SELECT ` + backupDestinationColumns + `
		FROM backup_destinations
		WHERE enabled = TRUE
		  AND status = 'active'
		  AND schedule <> 'manual'
		  AND schedule_hour_utc = $1
		  AND (
		      schedule = 'daily'   AND (last_success_at IS NULL OR last_success_at < $2)
		      OR schedule = 'weekly'  AND schedule_day_of_week  = $3 AND (last_success_at IS NULL OR last_success_at < $2)
		      OR schedule = 'monthly' AND schedule_day_of_month = $4 AND (last_success_at IS NULL OR last_success_at < $2)
		  )
		ORDER BY user_id, id
	`
	startOfDayUTC := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dow := int(now.Weekday())
	dom := now.Day()

	rows, err := r.db.QueryContext(ctx, query, now.Hour(), startOfDayUTC, dow, dom)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.BackupDestination
	for rows.Next() {
		d, err := scanBackupDestination(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// rowScanner abstracts *sql.Row and *sql.Rows so we can share the column
// decoding logic.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanBackupDestination(s rowScanner) (*models.BackupDestination, error) {
	d := &models.BackupDestination{}
	var configJSON []byte
	var schedule string
	var status string
	var dow sql.NullInt16
	var dom sql.NullInt16
	var lastRunAt sql.NullTime
	var lastSuccessAt sql.NullTime
	err := s.Scan(
		&d.ID,
		&d.UserID,
		&d.Provider,
		&d.DisplayName,
		&configJSON,
		&d.CredentialHint,
		&d.CredentialsEnc,
		&d.CredentialsNonce,
		&schedule,
		&d.ScheduleHourUTC,
		&dow,
		&dom,
		&d.RetentionCount,
		&status,
		&d.LastError,
		&d.Enabled,
		&d.ConsecutiveFailures,
		&lastRunAt,
		&lastSuccessAt,
		&d.LastSuccessSHA256,
		&d.CreatedAt,
		&d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	d.Schedule = models.BackupSchedule(schedule)
	d.Status = models.BackupDestinationStatus(status)
	if dow.Valid {
		v := int(dow.Int16)
		d.ScheduleDayOfWeek = &v
	}
	if dom.Valid {
		v := int(dom.Int16)
		d.ScheduleDayOfMonth = &v
	}
	if lastRunAt.Valid {
		t := lastRunAt.Time
		d.LastRunAt = &t
	}
	if lastSuccessAt.Valid {
		t := lastSuccessAt.Time
		d.LastSuccessAt = &t
	}
	if len(configJSON) == 0 {
		d.Config = map[string]any{}
	} else {
		if err := json.Unmarshal(configJSON, &d.Config); err != nil {
			return nil, fmt.Errorf("decode backup destination config: %w", err)
		}
	}
	return d, nil
}

func marshalConfig(cfg map[string]any) ([]byte, error) {
	if cfg == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(cfg)
}
