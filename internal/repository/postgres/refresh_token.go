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

type refreshTokenRepository struct {
	db *sql.DB
}

// NewRefreshTokenRepository creates a new refresh token repository
func NewRefreshTokenRepository(db *sql.DB) repository.RefreshTokenRepository {
	return &refreshTokenRepository{db: db}
}

func (r *refreshTokenRepository) Create(ctx context.Context, token *models.RefreshToken) error {
	query := `
		INSERT INTO refresh_tokens (
			id, user_id, token_hash, expires_at, revoked, created_at, updated_at,
			session_id, device_label, user_agent, ip_address, last_used_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	token.ID = uuid.New()
	token.CreatedAt = time.Now()
	token.UpdatedAt = time.Now()
	token.Revoked = false
	if token.SessionID == uuid.Nil {
		token.SessionID = uuid.New()
	}
	if token.LastUsedAt.IsZero() {
		token.LastUsedAt = token.CreatedAt
	}

	_, err := r.db.ExecContext(
		ctx,
		query,
		token.ID,
		token.UserID,
		token.TokenHash,
		token.ExpiresAt,
		token.Revoked,
		token.CreatedAt,
		token.UpdatedAt,
		token.SessionID,
		token.DeviceLabel,
		token.UserAgent,
		token.IPAddress,
		token.LastUsedAt,
	)

	return err
}

func (r *refreshTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, revoked, created_at, updated_at,
		       session_id, device_label, user_agent, ip_address, last_used_at,
		       revoked_at, rotated_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`

	token := &models.RefreshToken{}
	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.Revoked,
		&token.CreatedAt,
		&token.UpdatedAt,
		&token.SessionID,
		&token.DeviceLabel,
		&token.UserAgent,
		&token.IPAddress,
		&token.LastUsedAt,
		&token.RevokedAt,
		&token.RotatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return token, nil
}

func (r *refreshTokenRepository) RevokeByTokenHash(ctx context.Context, tokenHash string) error {
	query := `
		UPDATE refresh_tokens
		SET revoked = true, revoked_at = COALESCE(revoked_at, $1), updated_at = $1
		WHERE token_hash = $2
	`

	return r.execExpectingRow(ctx, query, time.Now(), tokenHash)
}

func (r *refreshTokenRepository) MarkRotated(ctx context.Context, tokenHash string) error {
	query := `
		UPDATE refresh_tokens
		SET revoked = true,
		    rotated_at = COALESCE(rotated_at, $1),
		    revoked_at = COALESCE(revoked_at, $1),
		    updated_at = $1
		WHERE token_hash = $2
	`

	return r.execExpectingRow(ctx, query, time.Now(), tokenHash)
}

// execExpectingRow runs a statement that must affect at least one row,
// returning repository.ErrNotFound when it affects none.
func (r *refreshTokenRepository) execExpectingRow(ctx context.Context, query string, args ...any) error {
	result, err := r.db.ExecContext(ctx, query, args...)
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

func (r *refreshTokenRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE refresh_tokens
		SET revoked = true, revoked_at = COALESCE(revoked_at, $1), updated_at = $1
		WHERE user_id = $2
	`

	_, err := r.db.ExecContext(ctx, query, time.Now(), userID)
	return err
}

func (r *refreshTokenRepository) DeleteForUser(ctx context.Context, userID uuid.UUID) error {
	query := `
		DELETE FROM refresh_tokens
		WHERE user_id = $1
	`

	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

func (r *refreshTokenRepository) DeleteExpired(ctx context.Context) error {
	query := `
		DELETE FROM refresh_tokens
		WHERE expires_at < $1
	`

	_, err := r.db.ExecContext(ctx, query, time.Now())
	return err
}

func (r *refreshTokenRepository) TouchSession(ctx context.Context, sessionID uuid.UUID, at time.Time) error {
	query := `
		UPDATE refresh_tokens
		SET last_used_at = $1
		WHERE session_id = $2
	`

	_, err := r.db.ExecContext(ctx, query, at, sessionID)
	return err
}

// liveSessionPredicate selects tokens that still authorise a refresh.
const liveSessionPredicate = `revoked = false AND expires_at > NOW()`

func (r *refreshTokenRepository) ListSessions(ctx context.Context, userID uuid.UUID) ([]*models.Session, error) {
	query := `
		SELECT session_id,
		       MIN(created_at)   AS started_at,
		       MAX(last_used_at) AS last_used_at,
		       MAX(expires_at)   AS expires_at,
		       (array_agg(device_label ORDER BY created_at DESC))[1] AS device_label,
		       (array_agg(ip_address   ORDER BY created_at DESC))[1] AS ip_address
		FROM refresh_tokens
		WHERE user_id = $1 AND ` + liveSessionPredicate + `
		GROUP BY session_id
		ORDER BY last_used_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := []*models.Session{}
	for rows.Next() {
		s := &models.Session{}
		if err := rows.Scan(
			&s.ID,
			&s.CreatedAt,
			&s.LastUsedAt,
			&s.ExpiresAt,
			&s.DeviceLabel,
			&s.IPAddress,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}

	return sessions, rows.Err()
}

func (r *refreshTokenRepository) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	query := `
		UPDATE refresh_tokens
		SET revoked = true, revoked_at = COALESCE(revoked_at, $1), updated_at = $1
		WHERE user_id = $2 AND session_id = $3 AND revoked = false
	`

	result, err := r.db.ExecContext(ctx, query, time.Now(), userID, sessionID)
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

func (r *refreshTokenRepository) RevokeSessionsExcept(ctx context.Context, userID, keep uuid.UUID) (int64, error) {
	query := `
		WITH revoked_sessions AS (
			UPDATE refresh_tokens
			SET revoked = true, revoked_at = COALESCE(revoked_at, $1), updated_at = $1
			WHERE user_id = $2 AND session_id <> $3 AND revoked = false
			RETURNING session_id
		)
		SELECT COUNT(DISTINCT session_id) FROM revoked_sessions
	`

	var count int64
	if err := r.db.QueryRowContext(ctx, query, time.Now(), userID, keep).Scan(&count); err != nil {
		return 0, err
	}

	return count, nil
}

func (r *refreshTokenRepository) EvictOldestSessions(ctx context.Context, userID uuid.UUID, keep int) (int64, error) {
	if keep < 0 {
		keep = 0
	}

	query := `
		WITH live AS (
			SELECT session_id, MAX(last_used_at) AS last_used_at
			FROM refresh_tokens
			WHERE user_id = $1 AND ` + liveSessionPredicate + `
			GROUP BY session_id
			ORDER BY last_used_at DESC
			OFFSET $2
		),
		evicted AS (
			UPDATE refresh_tokens t
			SET revoked = true, revoked_at = COALESCE(t.revoked_at, $3), updated_at = $3
			FROM live
			WHERE t.user_id = $1 AND t.session_id = live.session_id AND t.revoked = false
			RETURNING t.session_id
		)
		SELECT COUNT(DISTINCT session_id) FROM evicted
	`

	var count int64
	if err := r.db.QueryRowContext(ctx, query, userID, keep, time.Now()).Scan(&count); err != nil {
		return 0, err
	}

	return count, nil
}

func (r *refreshTokenRepository) CountActiveSessions(ctx context.Context) (int64, error) {
	query := `
		SELECT COUNT(DISTINCT session_id)
		FROM refresh_tokens
		WHERE ` + liveSessionPredicate

	var count int64
	if err := r.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, err
	}

	return count, nil
}

// AccessTokenState reads the account's disabled flag and the session's
// liveness in a single query. Returns ErrNotFound when no such user exists.
func (r *refreshTokenRepository) AccessTokenState(ctx context.Context, userID, sessionID uuid.UUID) (bool, bool, error) {
	query := `
		SELECT u.disabled,
		       EXISTS (
		           SELECT 1
		           FROM refresh_tokens
		           WHERE user_id = u.id
		             AND session_id = $2
		             AND ` + liveSessionPredicate + `
		       )
		FROM users u
		WHERE u.id = $1
	`

	var disabled, live bool
	err := r.db.QueryRowContext(ctx, query, userID, sessionID).Scan(&disabled, &live)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, repository.ErrNotFound
	}
	if err != nil {
		return false, false, err
	}

	return disabled, live, nil
}
