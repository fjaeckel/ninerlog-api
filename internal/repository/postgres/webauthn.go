package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type webauthnCredentialRepository struct {
	db *sql.DB
}

// NewWebAuthnCredentialRepository creates a new WebAuthn credential repository.
func NewWebAuthnCredentialRepository(db *sql.DB) repository.WebAuthnCredentialRepository {
	return &webauthnCredentialRepository{db: db}
}

func (r *webauthnCredentialRepository) Create(ctx context.Context, c *models.WebAuthnCredential) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	if c.Transports == nil {
		c.Transports = pq.StringArray{}
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO webauthn_credentials (
			id, user_id, credential_id, public_key, attestation_type,
			aaguid, sign_count, transports, label,
			user_present, user_verified, backup_eligible, backup_state,
			created_at, last_used_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`,
		c.ID, c.UserID, c.CredentialID, c.PublicKey, c.AttestationType,
		c.AAGUID, int64(c.SignCount), c.Transports, c.Label,
		c.UserPresent, c.UserVerified, c.BackupEligible, c.BackupState,
		c.CreatedAt, c.LastUsedAt,
	)
	return err
}

func (r *webauthnCredentialRepository) scan(row interface {
	Scan(dest ...interface{}) error
}) (*models.WebAuthnCredential, error) {
	c := &models.WebAuthnCredential{}
	var signCount int64
	if err := row.Scan(
		&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey, &c.AttestationType,
		&c.AAGUID, &signCount, &c.Transports, &c.Label,
		&c.UserPresent, &c.UserVerified, &c.BackupEligible, &c.BackupState,
		&c.CreatedAt, &c.LastUsedAt,
	); err != nil {
		return nil, err
	}
	// sign_count is stored as int64; clamp to the uint32 range.
	switch {
	case signCount < 0:
		c.SignCount = 0
	case signCount > int64(^uint32(0)):
		c.SignCount = ^uint32(0)
	default:
		c.SignCount = uint32(signCount)
	}
	return c, nil
}

// #nosec G101 -- SQL column identifiers include "public_key" but contain no credentials.
const webauthnCredColumns = `
	id, user_id, credential_id, public_key, attestation_type,
	aaguid, sign_count, transports, label,
	user_present, user_verified, backup_eligible, backup_state,
	created_at, last_used_at
`

func (r *webauthnCredentialRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.WebAuthnCredential, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+webauthnCredColumns+` FROM webauthn_credentials WHERE id = $1`, id)
	c, err := r.scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	return c, err
}

func (r *webauthnCredentialRepository) GetByCredentialID(ctx context.Context, credentialID []byte) (*models.WebAuthnCredential, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+webauthnCredColumns+` FROM webauthn_credentials WHERE credential_id = $1`, credentialID)
	c, err := r.scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	return c, err
}

func (r *webauthnCredentialRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.WebAuthnCredential, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+webauthnCredColumns+` FROM webauthn_credentials WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []*models.WebAuthnCredential
	for rows.Next() {
		c, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

func (r *webauthnCredentialRepository) UpdateSignCount(ctx context.Context, id uuid.UUID, signCount uint32, lastUsedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE webauthn_credentials SET sign_count = $1, last_used_at = $2 WHERE id = $3
	`, int64(signCount), lastUsedAt, id)
	return err
}

func (r *webauthnCredentialRepository) Delete(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM webauthn_credentials WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

type webauthnSessionRepository struct {
	db *sql.DB
}

// NewWebAuthnSessionRepository creates a new WebAuthn session repository.
func NewWebAuthnSessionRepository(db *sql.DB) repository.WebAuthnSessionRepository {
	return &webauthnSessionRepository{db: db}
}

func (r *webauthnSessionRepository) Create(ctx context.Context, s *models.WebAuthnSession) error {
	if len(s.IDHash) == 0 {
		return errors.New("webauthn session: empty id hash")
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO webauthn_sessions (id_hash, user_id, ceremony, data, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, s.IDHash, s.UserID, s.Ceremony, s.Data, s.ExpiresAt, s.CreatedAt)
	return err
}

// consumeQuery atomically deletes and returns an unexpired session.
const consumeQuery = `
	DELETE FROM webauthn_sessions
	 WHERE id_hash = $1 AND ceremony = $2 AND expires_at > NOW()
	RETURNING id_hash, user_id, ceremony, data, expires_at, created_at`

func (r *webauthnSessionRepository) Consume(ctx context.Context, idHash []byte, ceremony string) (*models.WebAuthnSession, error) {
	s := &models.WebAuthnSession{}
	err := r.db.QueryRowContext(ctx, consumeQuery, idHash, ceremony).
		Scan(&s.IDHash, &s.UserID, &s.Ceremony, &s.Data, &s.ExpiresAt, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("consume webauthn session: %w", err)
	}
	return s, nil
}

func (r *webauthnSessionRepository) DeleteOldestForUser(ctx context.Context, userID uuid.UUID, keep int) (int64, error) {
	if keep < 0 {
		keep = 0
	}
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM webauthn_sessions
		 WHERE id_hash IN (
			 SELECT id_hash
			   FROM webauthn_sessions
			  WHERE user_id = $1
			  ORDER BY created_at DESC
			 OFFSET $2
		 )
	`, userID, keep)
	if err != nil {
		return 0, fmt.Errorf("evict webauthn sessions: %w", err)
	}
	return res.RowsAffected()
}

func (r *webauthnSessionRepository) DeleteExpired(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM webauthn_sessions WHERE expires_at < NOW()`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
