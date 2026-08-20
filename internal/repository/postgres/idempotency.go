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

type idempotencyRepository struct {
	db *sql.DB
}

// NewIdempotencyRepository creates a new idempotency record repository.
func NewIdempotencyRepository(db *sql.DB) repository.IdempotencyRepository {
	return &idempotencyRepository{db: db}
}

const idempotencyColumns = `
	user_id, idempotency_key, request_hash, state,
	response_status, response_body, response_content_type,
	created_at, completed_at, expires_at`

// Claim atomically takes the key in a single statement. The DO UPDATE branch
// fires only for a record that is no longer live — expired, or an in-progress
// claim taken before staleBefore; everything else is reported as a conflict.
func (r *idempotencyRepository) Claim(ctx context.Context, rec *models.IdempotencyRecord, staleBefore time.Time) (*models.IdempotencyRecord, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO idempotency_keys (
			user_id, idempotency_key, request_hash, state,
			created_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, idempotency_key) DO UPDATE
		SET request_hash          = EXCLUDED.request_hash,
		    state                 = EXCLUDED.state,
		    response_status       = NULL,
		    response_body         = NULL,
		    response_content_type = NULL,
		    created_at            = EXCLUDED.created_at,
		    completed_at          = NULL,
		    expires_at            = EXCLUDED.expires_at
		WHERE idempotency_keys.expires_at <= $5
		   OR (idempotency_keys.state = $4 AND idempotency_keys.created_at <= $7)
	`,
		rec.UserID, rec.Key, rec.RequestHash, models.IdempotencyStateInProgress,
		rec.CreatedAt, rec.ExpiresAt, staleBefore,
	)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected > 0 {
		rec.State = models.IdempotencyStateInProgress
		return nil, nil
	}

	existing, err := r.get(ctx, rec.UserID, rec.Key)
	if err != nil {
		// A vanished holder is reported as a conflict.
		if errors.Is(err, repository.ErrNotFound) {
			return &models.IdempotencyRecord{
				UserID:      rec.UserID,
				Key:         rec.Key,
				RequestHash: rec.RequestHash,
				State:       models.IdempotencyStateInProgress,
				CreatedAt:   rec.CreatedAt,
				ExpiresAt:   rec.ExpiresAt,
			}, nil
		}
		return nil, err
	}
	return existing, nil
}

func (r *idempotencyRepository) get(ctx context.Context, userID uuid.UUID, key string) (*models.IdempotencyRecord, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT `+idempotencyColumns+`
		FROM idempotency_keys
		WHERE user_id = $1 AND idempotency_key = $2
	`, userID, key)

	rec := &models.IdempotencyRecord{}
	var contentType sql.NullString
	err := row.Scan(
		&rec.UserID, &rec.Key, &rec.RequestHash, &rec.State,
		&rec.ResponseStatus, &rec.ResponseBody, &contentType,
		&rec.CreatedAt, &rec.CompletedAt, &rec.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rec.ResponseContentType = contentType.String
	return rec, nil
}

func (r *idempotencyRepository) Complete(ctx context.Context, rec *models.IdempotencyRecord) error {
	var contentType sql.NullString
	if rec.ResponseContentType != "" {
		contentType = sql.NullString{String: rec.ResponseContentType, Valid: true}
	}
	completedAt := rec.CompletedAt
	if completedAt == nil {
		now := time.Now().UTC()
		completedAt = &now
	}

	_, err := r.db.ExecContext(ctx, `
		UPDATE idempotency_keys
		SET state                 = $4,
		    response_status       = $5,
		    response_body         = $6,
		    response_content_type = $7,
		    completed_at          = $8
		WHERE user_id = $1 AND idempotency_key = $2 AND created_at = $3
	`,
		rec.UserID, rec.Key, rec.CreatedAt, models.IdempotencyStateCompleted,
		rec.ResponseStatus, rec.ResponseBody, contentType, completedAt,
	)
	return err
}

func (r *idempotencyRepository) Release(ctx context.Context, userID uuid.UUID, key string, claimedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM idempotency_keys
		WHERE user_id = $1 AND idempotency_key = $2 AND created_at = $3
	`, userID, key, claimedAt)
	return err
}

func (r *idempotencyRepository) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM idempotency_keys WHERE expires_at <= $1
	`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
