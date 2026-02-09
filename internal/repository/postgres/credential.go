package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/google/uuid"
)

type credentialRepository struct {
	db *sql.DB
}

func NewCredentialRepository(db *sql.DB) repository.CredentialRepository {
	return &credentialRepository{db: db}
}

func (r *credentialRepository) Create(ctx context.Context, credential *models.Credential) error {
	query := `
		INSERT INTO credentials (user_id, credential_type, credential_number, issue_date, expiry_date, issuing_authority, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		credential.UserID,
		credential.CredentialType,
		credential.CredentialNumber,
		credential.IssueDate,
		credential.ExpiryDate,
		credential.IssuingAuthority,
		credential.Notes,
	).Scan(&credential.ID, &credential.CreatedAt, &credential.UpdatedAt)
}

func (r *credentialRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Credential, error) {
	query := `
		SELECT id, user_id, credential_type, credential_number, issue_date, expiry_date,
		       issuing_authority, notes, created_at, updated_at
		FROM credentials WHERE id = $1
	`
	c := &models.Credential{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.UserID, &c.CredentialType, &c.CredentialNumber,
		&c.IssueDate, &c.ExpiryDate, &c.IssuingAuthority, &c.Notes,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	return c, err
}

func (r *credentialRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Credential, error) {
	query := `
		SELECT id, user_id, credential_type, credential_number, issue_date, expiry_date,
		       issuing_authority, notes, created_at, updated_at
		FROM credentials WHERE user_id = $1
		ORDER BY expiry_date ASC NULLS LAST, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var credentials []*models.Credential
	for rows.Next() {
		c := &models.Credential{}
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.CredentialType, &c.CredentialNumber,
			&c.IssueDate, &c.ExpiryDate, &c.IssuingAuthority, &c.Notes,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		credentials = append(credentials, c)
	}
	return credentials, rows.Err()
}

func (r *credentialRepository) Update(ctx context.Context, credential *models.Credential) error {
	query := `
		UPDATE credentials
		SET credential_type = $1, credential_number = $2, issue_date = $3,
		    expiry_date = $4, issuing_authority = $5, notes = $6, updated_at = $7
		WHERE id = $8
	`
	result, err := r.db.ExecContext(ctx, query,
		credential.CredentialType, credential.CredentialNumber, credential.IssueDate,
		credential.ExpiryDate, credential.IssuingAuthority, credential.Notes,
		time.Now(), credential.ID,
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

func (r *credentialRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM credentials WHERE id = $1", id)
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
