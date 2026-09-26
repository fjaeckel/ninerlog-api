package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type weGlideLinkRepository struct {
	db *sql.DB
}

// NewWeGlideLinkRepository returns the PostgreSQL WeGlideLinkRepository.
func NewWeGlideLinkRepository(db *sql.DB) repository.WeGlideLinkRepository {
	return &weGlideLinkRepository{db: db}
}

func dateOnly(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

func (r *weGlideLinkRepository) Get(ctx context.Context, userID uuid.UUID) (*models.WeGlideLink, error) {
	l := &models.WeGlideLink{}
	var weglideUserID, status, syncErr sql.NullString
	var lastSync, day sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT user_id, api_key_encrypted, weglide_user_id, last_sync_at, last_sync_status,
		       last_sync_error, requests_today, requests_day, created_at, updated_at
		FROM weglide_links WHERE user_id = $1`, userID).Scan(
		&l.UserID, &l.APIKeyEncrypted, &weglideUserID, &lastSync, &status,
		&syncErr, &l.RequestsToday, &day, &l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if weglideUserID.Valid {
		l.WeGlideUserID = &weglideUserID.String
	}
	if lastSync.Valid {
		l.LastSyncAt = &lastSync.Time
	}
	if status.Valid {
		l.LastSyncStatus = &status.String
	}
	if syncErr.Valid {
		l.LastSyncError = &syncErr.String
	}
	if day.Valid {
		l.RequestsDay = &day.Time
	}
	return l, nil
}

func (r *weGlideLinkRepository) Upsert(ctx context.Context, userID uuid.UUID, apiKeyEncrypted []byte, weglideUserID string, day time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO weglide_links (user_id, api_key_encrypted, weglide_user_id, requests_today, requests_day)
		VALUES ($1, $2, $3, 1, $4::date)
		ON CONFLICT (user_id) DO UPDATE SET
			api_key_encrypted = EXCLUDED.api_key_encrypted,
			weglide_user_id   = EXCLUDED.weglide_user_id,
			last_sync_at      = CASE WHEN weglide_links.weglide_user_id IS NOT DISTINCT FROM EXCLUDED.weglide_user_id
			                         THEN weglide_links.last_sync_at END,
			last_sync_status  = CASE WHEN weglide_links.weglide_user_id IS NOT DISTINCT FROM EXCLUDED.weglide_user_id
			                         THEN weglide_links.last_sync_status END,
			last_sync_error   = CASE WHEN weglide_links.weglide_user_id IS NOT DISTINCT FROM EXCLUDED.weglide_user_id
			                         THEN weglide_links.last_sync_error END`,
		userID, apiKeyEncrypted, weglideUserID, dateOnly(day))
	return err
}

func (r *weGlideLinkRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM weglide_links WHERE user_id = $1`, userID)
	return err
}

func (r *weGlideLinkRepository) TakeRequest(ctx context.Context, userID uuid.UUID, day time.Time, limit int) (int, bool, error) {
	var used int
	err := r.db.QueryRowContext(ctx, `
		UPDATE weglide_links SET
			requests_today = CASE WHEN requests_day = $2::date THEN requests_today + 1 ELSE 1 END,
			requests_day   = $2::date
		WHERE user_id = $1
		  AND (requests_day IS DISTINCT FROM $2::date OR requests_today < $3)
		RETURNING requests_today`, userID, dateOnly(day), limit).Scan(&used)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		if err := r.db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM weglide_links WHERE user_id = $1)`, userID).Scan(&exists); err != nil {
			return 0, false, err
		}
		if !exists {
			return 0, false, repository.ErrNotFound
		}
		return limit, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return used, true, nil
}

func (r *weGlideLinkRepository) ExhaustRequests(ctx context.Context, userID uuid.UUID, day time.Time, limit int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE weglide_links SET requests_today = GREATEST($3,
			CASE WHEN requests_day = $2::date THEN requests_today ELSE 0 END), requests_day = $2::date
		WHERE user_id = $1`, userID, dateOnly(day), limit)
	return err
}

func (r *weGlideLinkRepository) RecordSync(ctx context.Context, userID uuid.UUID, status string, syncErr *string, completedAt *time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE weglide_links SET
			last_sync_status = $2,
			last_sync_error  = $3,
			last_sync_at     = COALESCE($4, last_sync_at)
		WHERE user_id = $1`, userID, status, syncErr, completedAt)
	return err
}

func (r *weGlideLinkRepository) ListDueForSync(ctx context.Context, before, day time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id FROM weglide_links
		WHERE (last_sync_at IS NULL OR last_sync_at < $1)
		  AND requests_day IS DISTINCT FROM $2::date
		ORDER BY last_sync_at NULLS FIRST, user_id
		LIMIT $3`, before, dateOnly(day), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *weGlideLinkRepository) ImportedFlightFilenames(ctx context.Context, userID uuid.UUID, prefix string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT filename FROM flight_files
		WHERE user_id = $1 AND left(filename, char_length($2)) = $2`, userID, prefix)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
