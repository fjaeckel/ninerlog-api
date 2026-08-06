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

type oidcIdentityRepository struct {
	db *sql.DB
}

// NewOIDCIdentityRepository returns a repository.OIDCIdentityRepository backed
// by Postgres.
func NewOIDCIdentityRepository(db *sql.DB) repository.OIDCIdentityRepository {
	return &oidcIdentityRepository{db: db}
}

const oidcIdentityColumns = `id, user_id, issuer, subject, COALESCE(email, ''), created_at, last_login_at`

func scanOIDCIdentity(row *sql.Row) (*models.OIDCIdentity, error) {
	i := &models.OIDCIdentity{}
	err := row.Scan(&i.ID, &i.UserID, &i.Issuer, &i.Subject, &i.Email, &i.CreatedAt, &i.LastLoginAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan oidc identity: %w", err)
	}
	return i, nil
}

func (r *oidcIdentityRepository) GetBySubject(ctx context.Context, issuer, subject string) (*models.OIDCIdentity, error) {
	return scanOIDCIdentity(r.db.QueryRowContext(ctx,
		`SELECT `+oidcIdentityColumns+` FROM oidc_identities WHERE issuer = $1 AND subject = $2`,
		issuer, subject))
}

func (r *oidcIdentityRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*models.OIDCIdentity, error) {
	return scanOIDCIdentity(r.db.QueryRowContext(ctx,
		`SELECT `+oidcIdentityColumns+` FROM oidc_identities WHERE user_id = $1`,
		userID))
}

func (r *oidcIdentityRepository) Create(ctx context.Context, identity *models.OIDCIdentity) error {
	if identity.ID == uuid.Nil {
		identity.ID = uuid.New()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO oidc_identities (id, user_id, issuer, subject, email, last_login_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		identity.ID, identity.UserID, identity.Issuer, identity.Subject,
		identity.Email, identity.LastLoginAt)
	if err != nil {
		var pqErr *pq.Error
		// 23505 = unique_violation: another request linked the same subject
		// concurrently. The caller re-reads and uses the winning row.
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return repository.ErrDuplicate
		}
		return fmt.Errorf("create oidc identity: %w", err)
	}
	return nil
}

func (r *oidcIdentityRepository) TouchLogin(ctx context.Context, id uuid.UUID, email string, at time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE oidc_identities SET last_login_at = $2, email = $3 WHERE id = $1`,
		id, at, email)
	if err != nil {
		return fmt.Errorf("touch oidc identity: %w", err)
	}
	return nil
}

func (r *oidcIdentityRepository) CreateLoginState(ctx context.Context, state *models.OIDCLoginState) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO oidc_login_states (state_hash, browser_hash, nonce, code_verifier, expires_at)
		VALUES ($1, $2, $3, $4, $5)`,
		state.StateHash, state.BrowserHash, state.Nonce, state.CodeVerifier, state.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create oidc login state: %w", err)
	}
	return nil
}

// consumeLoginStateQuery deletes and returns the row in one statement, making
// consumption exactly-once with no read-modify-write window. The
// `expires_at > NOW()` predicate is a correctness requirement, not an
// optimisation: a stopped reaper must never make a stale state replayable.
const consumeLoginStateQuery = `
	DELETE FROM oidc_login_states
	 WHERE state_hash = $1 AND expires_at > NOW()
	RETURNING state_hash, browser_hash, nonce, code_verifier, created_at, expires_at`

func (r *oidcIdentityRepository) ConsumeLoginState(ctx context.Context, stateHash []byte) (*models.OIDCLoginState, error) {
	s := &models.OIDCLoginState{}
	err := r.db.QueryRowContext(ctx, consumeLoginStateQuery, stateHash).
		Scan(&s.StateHash, &s.BrowserHash, &s.Nonce, &s.CodeVerifier, &s.CreatedAt, &s.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("consume oidc login state: %w", err)
	}
	return s, nil
}

func (r *oidcIdentityRepository) CreateHandoffCode(ctx context.Context, code *models.OIDCHandoffCode) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO oidc_handoff_codes (code_hash, user_id, expires_at)
		VALUES ($1, $2, $3)`,
		code.CodeHash, code.UserID, code.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create oidc handoff code: %w", err)
	}
	return nil
}

const consumeHandoffCodeQuery = `
	DELETE FROM oidc_handoff_codes
	 WHERE code_hash = $1 AND expires_at > NOW()
	RETURNING code_hash, user_id, created_at, expires_at`

func (r *oidcIdentityRepository) ConsumeHandoffCode(ctx context.Context, codeHash []byte) (*models.OIDCHandoffCode, error) {
	h := &models.OIDCHandoffCode{}
	err := r.db.QueryRowContext(ctx, consumeHandoffCodeQuery, codeHash).
		Scan(&h.CodeHash, &h.UserID, &h.CreatedAt, &h.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("consume oidc handoff code: %w", err)
	}
	return h, nil
}

func (r *oidcIdentityRepository) DeleteExpired(ctx context.Context) (int64, error) {
	var total int64
	states, err := r.db.ExecContext(ctx, `DELETE FROM oidc_login_states WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired oidc login states: %w", err)
	}
	if n, err := states.RowsAffected(); err == nil {
		total += n
	}
	codes, err := r.db.ExecContext(ctx, `DELETE FROM oidc_handoff_codes WHERE expires_at < NOW()`)
	if err != nil {
		return total, fmt.Errorf("delete expired oidc handoff codes: %w", err)
	}
	if n, err := codes.RowsAffected(); err == nil {
		total += n
	}
	return total, nil
}
