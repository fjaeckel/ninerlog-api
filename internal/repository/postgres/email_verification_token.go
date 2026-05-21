package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type emailVerificationTokenRepository struct {
	db *sql.DB
}

// NewEmailVerificationTokenRepository creates a new email verification token repository.
func NewEmailVerificationTokenRepository(db *sql.DB) repository.EmailVerificationTokenRepository {
	return &emailVerificationTokenRepository{db: db}
}

func (r *emailVerificationTokenRepository) Create(ctx context.Context, token *models.EmailVerificationToken) error {
	query := `
		INSERT INTO email_verification_tokens (id, user_id, token_hash, expires_at, used, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	token.ID = uuid.New()
	token.CreatedAt = time.Now()
	token.Used = false

	_, err := r.db.ExecContext(
		ctx,
		query,
		token.ID,
		token.UserID,
		token.TokenHash,
		token.ExpiresAt,
		token.Used,
		token.CreatedAt,
	)

	return err
}

func (r *emailVerificationTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*models.EmailVerificationToken, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, used, created_at
		FROM email_verification_tokens
		WHERE token_hash = $1
	`

	token := &models.EmailVerificationToken{}
	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.Used,
		&token.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (r *emailVerificationTokenRepository) MarkAsUsed(ctx context.Context, tokenHash string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE email_verification_tokens SET used = TRUE WHERE token_hash = $1`, tokenHash)
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

func (r *emailVerificationTokenRepository) DeleteForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM email_verification_tokens WHERE user_id = $1`, userID)
	return err
}

func (r *emailVerificationTokenRepository) DeleteExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM email_verification_tokens WHERE expires_at < $1`, time.Now())
	return err
}
